package matcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"github.com/PIN-AI/intent-protocol-contract-sdk/sdk/addressbook"
	"subnet/internal/blockchain"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/rootlayer"
	rootpb "subnet/proto/rootlayer"
	pb "subnet/proto/subnet"
)

// Server implements the MatcherService with bidding windows and matching logic
type Server struct {
	pb.UnimplementedMatcherServiceServer

	// Core components
	logger          logging.Logger
	cfg             *Config
	signer          crypto.ExtendedSigner
	rootlayerClient rootlayer.Client
	verifier        *blockchain.ParticipantVerifier
	allowUnverified bool
	sdkClient       *sdk.Client         // SDK client for on-chain operations
	strategy        BidMatchingStrategy // Bid matching strategy for bid selection

	// Intent and bid management
	mu              sync.RWMutex
	intents         map[string]*IntentWithWindow  // intent_id -> intent with window
	bidsByIntent    map[string][]*pb.Bid          // intent_id -> bids
	matchingResults map[string]*pb.MatchingResult // intent_id -> result
	assignments     map[string]*rootpb.Assignment // assignment_id -> assignment

	// Assignment confirmation tracking
	pendingAssignments map[string]*PendingAssignment // assignment_id -> pending assignment
	assignmentAgents   map[string]string             // assignment_id -> agent_id

	// Stream management
	intentStreams map[string]chan *pb.MatcherIntentUpdate // stream_id -> channel
	agentTasks    map[string]chan *pb.ExecutionTask       // agent_id -> task channel
	taskStreams   map[string]chan *pb.ExecutionTask       // stream_id -> task stream

	// State
	started          bool
	windowCheckTimer *time.Timer
	nextStreamID     int
}

const chainAddressMetadataKey = "chain_address"

func extractChainAddressFromBid(bid *pb.Bid) string {
	if bid == nil {
		return ""
	}
	if bid.Metadata != nil {
		if addr, ok := bid.Metadata[chainAddressMetadataKey]; ok {
			if trimmed := strings.TrimSpace(addr); trimmed != "" {
				return trimmed
			}
		}
	}
	return strings.TrimSpace(bid.AgentId)
}

// IntentWithWindow tracks an intent with its bidding window
type IntentWithWindow struct {
	Intent           *rootpb.Intent
	BiddingStartTime int64
	BiddingEndTime   int64
	WindowClosed     bool
	Matched          bool
}

// PendingAssignment tracks assignments awaiting agent confirmation
type PendingAssignment struct {
	Assignment *rootpb.Assignment
	CreatedAt  time.Time
	Intent     *rootpb.Intent
}

// NewServer creates a new Matcher server with default lowest-price strategy
func NewServer(cfg *Config, logger logging.Logger) (*Server, error) {
	return NewServerWithStrategy(cfg, logger, NewLowestPriceBidStrategy())
}

// NewServerWithStrategy creates a new Matcher server with a custom matching strategy
// This allows advanced users to provide their own strategy implementation
func NewServerWithStrategy(cfg *Config, logger logging.Logger, strategy BidMatchingStrategy) (*Server, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}
	if strategy == nil {
		strategy = NewLowestPriceBidStrategy()
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create signer - REQUIRED for security
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("CRITICAL: Private key is required for matcher service - signatures are mandatory for security")
	}

	signer, err := crypto.LoadSignerFromPrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from private key: %w", err)
	}

	logger.Info("Matcher signer initialized", "address", signer.GetAddress())

	// Create RootLayer client
	// Use subnet ID from identity config (REQUIRED)
	if cfg.Identity == nil || cfg.Identity.SubnetID == "" {
		return nil, fmt.Errorf("CRITICAL: subnet_id must be configured in identity section")
	}

	// Use CompleteClient for full batch submission support
	var rootlayerClient rootlayer.Client
	if cfg.RootLayerEndpoint != "" {
		completeConfig := &rootlayer.CompleteClientConfig{
			Endpoint:   cfg.RootLayerEndpoint,
			SubnetID:   cfg.Identity.SubnetID,
			NodeID:     cfg.MatcherID,
			NodeType:   "matcher",
			PrivateKey: cfg.PrivateKey,
		}
		var err error
		rootlayerClient, err = rootlayer.NewCompleteClient(completeConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create RootLayer CompleteClient: %w", err)
		}
		logger.Info("Using RootLayer CompleteClient with batch support", "endpoint", cfg.RootLayerEndpoint)
	} else {
		// Use default config for mock client
		rootlayerCfg := rootlayer.DefaultConfig()
		rootlayerCfg.SubnetID = cfg.Identity.SubnetID
		rootlayerCfg.MatcherID = cfg.MatcherID
		rootlayerClient = rootlayer.NewMockClient(rootlayerCfg, logger)
		logger.Warn("No RootLayer endpoint configured, using mock client")
	}

	// Initialize SDK client if on-chain submission is enabled
	var sdkClient *sdk.Client
	if cfg.EnableChainSubmit {
		if cfg.ChainRPCURL == "" {
			return nil, fmt.Errorf("chain_rpc_url is required when enable_chain_submit is true")
		}
		if cfg.ChainNetwork == "" {
			return nil, fmt.Errorf("chain_network is required when enable_chain_submit is true")
		}
		if cfg.IntentManagerAddr == "" {
			return nil, fmt.Errorf("intent_manager_addr is required when enable_chain_submit is true")
		}

		// Create SDK config
		sdkConfig := sdk.Config{
			RPCURL:        cfg.ChainRPCURL,
			PrivateKeyHex: cfg.PrivateKey,
			Network:       cfg.ChainNetwork,
			Addresses: &addressbook.Addresses{
				IntentManager: common.HexToAddress(cfg.IntentManagerAddr),
			},
		}

		// Initialize SDK client
		var err error
		sdkClient, err = sdk.NewClient(context.Background(), sdkConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize SDK client: %w", err)
		}

		logger.Info("SDK client initialized for on-chain assignment submission",
			"rpc_url", cfg.ChainRPCURL,
			"network", cfg.ChainNetwork,
			"intent_manager", cfg.IntentManagerAddr)
	} else {
		logger.Info("On-chain assignment submission disabled")
	}

	s := &Server{
		cfg:                cfg,
		logger:             logger,
		signer:             signer,
		rootlayerClient:    rootlayerClient,
		sdkClient:          sdkClient,
		strategy:           strategy,
		intents:            make(map[string]*IntentWithWindow),
		bidsByIntent:       make(map[string][]*pb.Bid),
		matchingResults:    make(map[string]*pb.MatchingResult),
		assignments:        make(map[string]*rootpb.Assignment),
		pendingAssignments: make(map[string]*PendingAssignment),
		assignmentAgents:   make(map[string]string),
		intentStreams:      make(map[string]chan *pb.MatcherIntentUpdate),
		agentTasks:         make(map[string]chan *pb.ExecutionTask),
		taskStreams:        make(map[string]chan *pb.ExecutionTask),
	}

	return s, nil
}

// AttachBlockchainVerifier wires the on-chain verifier into the matcher.
func (s *Server) AttachBlockchainVerifier(verifier *blockchain.ParticipantVerifier, allowUnverified bool) {
	if s == nil {
		return
	}
	s.verifier = verifier
	s.allowUnverified = allowUnverified
}

// Start starts the matcher service with window checking
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("already started")
	}

	// Connect to RootLayer
	if err := s.rootlayerClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to RootLayer: %w", err)
	}

	// Start window checking goroutine
	go s.windowCheckLoop(ctx)

	// Start intent pulling from RootLayer
	go s.pullIntentsFromRootLayer(ctx)

	s.started = true
	s.logger.Info("Matcher service started with bidding window management and RootLayer integration")
	return nil
}

// Stop stops the matcher service
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	// Disconnect from RootLayer
	if s.rootlayerClient != nil {
		s.rootlayerClient.Close()
	}

	if s.verifier != nil {
		s.verifier.Close()
	}

	// Close all streams
	for _, ch := range s.intentStreams {
		close(ch)
	}
	s.intentStreams = make(map[string]chan *pb.MatcherIntentUpdate)

	// Close agent task channels
	for _, ch := range s.agentTasks {
		close(ch)
	}
	s.agentTasks = make(map[string]chan *pb.ExecutionTask)

	s.started = false
	s.logger.Info("Matcher service stopped")
	return nil
}

// AddIntent adds a new intent and starts its bidding window
func (s *Server) AddIntent(intent *rootpb.Intent) error {
	s.logger.Infof("[DEBUG] AddIntent called for: %s", intent.IntentId)

	if intent == nil || intent.IntentId == "" {
		s.logger.Error("[DEBUG] AddIntent: intent is nil or missing intent_id")
		return fmt.Errorf("intent is nil or missing intent_id")
	}

	now := time.Now().Unix()

	s.logger.Infof("[DEBUG] AddIntent: acquiring lock for %s", intent.IntentId)
	s.mu.Lock()
	s.logger.Infof("[DEBUG] AddIntent: lock acquired for %s", intent.IntentId)

	if _, exists := s.intents[intent.IntentId]; exists {
		s.mu.Unlock()
		s.logger.Infof("[DEBUG] AddIntent: intent %s already exists, returning error", intent.IntentId)
		return fmt.Errorf("intent %s already exists", intent.IntentId)
	}

	if expired, reason := s.intentExpired(intent, now); expired {
		s.mu.Unlock()
		s.logger.Warnf("Ignoring expired intent %s (%s, now=%d)", intent.IntentId, reason, now)
		s.logger.Info("[DEBUG] AddIntent: intent expired, returning nil")
		return nil
	}

	window := &IntentWithWindow{
		Intent:           intent,
		BiddingStartTime: now,
		BiddingEndTime:   now + int64(s.cfg.BiddingWindowSec),
		WindowClosed:     false,
		Matched:          false,
	}

	s.intents[intent.IntentId] = window
	s.bidsByIntent[intent.IntentId] = make([]*pb.Bid, 0)
	s.mu.Unlock()

	s.logger.Infof("Added intent %s with bidding window %d seconds",
		intent.IntentId, s.cfg.BiddingWindowSec)

	// Notify all streams about new intent
	s.broadcastIntentUpdate(intent.IntentId, "NEW_INTENT")

	return nil
}

// windowCheckLoop periodically checks for closed bidding windows
func (s *Server) windowCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndCloseWindows()
		}
	}
}

// checkAndCloseWindows checks for expired windows and triggers matching
func (s *Server) checkAndCloseWindows() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	for intentID, window := range s.intents {
		// Skip if already closed or matched
		if window.WindowClosed || window.Matched {
			continue
		}

		// Check if window should close
		if now >= window.BiddingEndTime {
			s.logger.Infof("Closing bidding window for intent %s", intentID)
			window.WindowClosed = true

			// Trigger matching
			s.performMatching(intentID)

			// Notify streams (already holding lock)
			s.broadcastIntentUpdateLocked(intentID, "WINDOW_CLOSED")
		}
	}
}

// performMatching executes the matching algorithm for an intent
func (s *Server) performMatching(intentID string) {
	bids := s.bidsByIntent[intentID]
	window := s.intents[intentID]

	if len(bids) == 0 {
		s.logger.Warnf("No bids for intent %s", intentID)
		window.Matched = true
		return
	}

	s.logger.Info("Performing matching for intent",
		"intent_id", intentID,
		"bids_count", len(bids),
		"strategy", s.strategy.Name())

	// Log bid details for debugging
	for i, bid := range bids {
		agentAddr := "unknown"
		if metadata, ok := bid.Metadata["chain_address"]; ok {
			agentAddr = metadata
		}
		s.logger.Debug("Bid received",
			"bid_num", i+1,
			"agent_id", bid.AgentId,
			"price", bid.Price,
			"chain_address", agentAddr)
	}

	// Use strategy to select winner and runner-ups
	winner, runnerUps, err := s.strategy.SelectWinner(window.Intent, bids)
	if err != nil {
		s.logger.Error("Matching strategy failed",
			"intent_id", intentID,
			"error", err.Error())
		window.Matched = true
		return
	}

	// Log winner details
	winnerChainAddr := "unknown"
	if metadata, ok := winner.Metadata["chain_address"]; ok {
		winnerChainAddr = metadata
	}
	s.logger.Info("Winner selected for matching",
		"agent_id", winner.AgentId,
		"price", winner.Price,
		"chain_address", winnerChainAddr,
		"bid_id", winner.BidId)

	// Extract runner-up IDs
	runnerUpIDs := make([]string, 0, len(runnerUps))
	for _, bid := range runnerUps {
		runnerUpIDs = append(runnerUpIDs, bid.BidId)
	}

	// Sign the matching result
	matcherSignature, err := s.signMatchingResult(intentID, winner.BidId, winner.AgentId)
	if err != nil {
		s.logger.Errorf("Failed to sign matching result: %v", err)
		// Continue without signature for now, but log critical error
		matcherSignature = []byte{}
	}

	// Create matching result
	result := &pb.MatchingResult{
		MatcherId:        s.cfg.MatcherID,
		IntentId:         intentID,
		WinningBidId:     winner.BidId,
		WinningAgentId:   winner.AgentId,
		RunnerUpBidIds:   runnerUpIDs,
		MatchedAt:        time.Now().Unix(),
		MatchingReason:   "Lowest price",
		ResultHash:       s.computeResultHash(winner.BidId, winner.AgentId),
		MatcherSignature: matcherSignature,
	}

	s.matchingResults[intentID] = result
	window.Matched = true

	// Create assignment
	s.createAssignment(intentID, winner)

	s.logger.Infof("Intent %s matched to agent %s (bid: %s, price: %d)",
		intentID, winner.AgentId, winner.BidId, winner.Price)

	// Notify about matching (already holding lock)
	s.broadcastIntentUpdateLocked(intentID, "MATCHED")
}

// createAssignment creates an assignment from the winning bid
func (s *Server) createAssignment(intentID string, winningBid *pb.Bid) {
	// Generate assignment ID as 32-byte hex (hash of intent + timestamp + bid)
	assignmentData := fmt.Sprintf("%s:%s:%d", intentID, winningBid.BidId, time.Now().UnixNano())
	assignmentHash := sha256.Sum256([]byte(assignmentData))
	assignmentID := "0x" + hex.EncodeToString(assignmentHash[:])

	// Get the intent for task data
	intentWindow := s.intents[intentID]
	if intentWindow == nil {
		s.logger.Errorf("Intent %s not found when creating assignment", intentID)
		return
	}

	// CRITICAL: Extract blockchain address from bid metadata
	// The agent field in Assignment must be an Ethereum address, not an arbitrary string
	agentChainAddress := extractChainAddressFromBid(winningBid)
	if agentChainAddress == "" || !strings.HasPrefix(agentChainAddress, "0x") {
		s.logger.Errorf("CRITICAL: Invalid chain address for agent %s: %s", winningBid.AgentId, agentChainAddress)
		return
	}

	// Sign the assignment (uses blockchain address for EIP-191 signature)
	assignmentSignature, err := s.signAssignment(assignmentID, intentID, winningBid.BidId, agentChainAddress)
	if err != nil {
		s.logger.Errorf("Failed to sign assignment: %v", err)
		// Continue without signature for now, but log critical error
		assignmentSignature = []byte{}
	}

	// Create RootLayer assignment (lightweight, for record-keeping)
	// IMPORTANT: RootLayer requires AgentId and MatcherId to be in Ethereum address format
	rootAssignment := &rootpb.Assignment{
		AssignmentId: assignmentID,
		IntentId:     intentID,
		AgentId:      agentChainAddress, // Use Ethereum address format (required by RootLayer)
		BidId:        winningBid.BidId,
		Status:       rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE,
		MatcherId:    s.signer.GetAddress(), // Use matcher's blockchain address (required by RootLayer)
		Signature:    assignmentSignature,
	}

	// Create ExecutionTask (with full task data for agent)
	// Safely extract intent data to avoid panic
	var intentData []byte
	if intentWindow.Intent.Params != nil {
		intentData = intentWindow.Intent.Params.IntentRaw
	}

	executionTask := &pb.ExecutionTask{
		TaskId:     assignmentID, // Use same ID for tracking
		IntentId:   intentID,
		AgentId:    winningBid.AgentId,
		BidId:      winningBid.BidId,
		CreatedAt:  time.Now().Unix(),
		Deadline:   intentWindow.Intent.Deadline,
		IntentData: intentData, // Safely accessed
		IntentType: intentWindow.Intent.IntentType,
	}

	// Store the pending assignment (waiting for agent confirmation)
	s.pendingAssignments[assignmentID] = &PendingAssignment{
		Assignment: rootAssignment,
		CreatedAt:  time.Now(),
		Intent:     intentWindow.Intent,
	}
	s.assignmentAgents[assignmentID] = winningBid.AgentId

	s.logger.Infof("Created task %s for agent %s", assignmentID, winningBid.AgentId)

	// IMPORTANT: Assignment submission strategy for atomicity
	// Priority: Blockchain > RootLayer > Agent Notification
	// Reason: Blockchain is the source of truth (immutable, prevents fraud)

	// Step 1: Submit to blockchain FIRST (synchronous) if enabled
	// This is the most critical step - if it fails, we abort the entire assignment
	if s.cfg.EnableChainSubmit && s.sdkClient != nil {
		chainTxHash, err := s.submitAssignmentToChainSync(assignmentID, intentID, winningBid.BidId, agentChainAddress, assignmentSignature)
		if err != nil {
			s.logger.Error("CRITICAL: Assignment blockchain submission failed, aborting assignment",
				"assignment_id", assignmentID,
				"intent_id", intentID,
				"agent_id", winningBid.AgentId,
				"error", err.Error())

			s.handleAssignmentFailureLocked(intentID, assignmentID, "blockchain_submission")
			return
		}
		s.logger.Info("Assignment submitted to blockchain successfully",
			"assignment_id", assignmentID,
			"tx_hash", chainTxHash)
	}

	// Step 2: Submit to RootLayer (required for validators to query assignment)
	// This is CRITICAL - validators need to query assignment from RootLayer when submitting validation results
	if s.rootlayerClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.rootlayerClient.SubmitAssignment(ctx, rootAssignment); err != nil {
			s.logger.Error("CRITICAL: Assignment RootLayer submission failed, aborting assignment",
				"assignment_id", assignmentID,
				"intent_id", intentID,
				"agent_id", winningBid.AgentId,
				"error", err.Error(),
				"reason", "validators cannot query assignment without RootLayer data")

			s.handleAssignmentFailureLocked(intentID, assignmentID, "rootlayer_submission", err)
			return
		}
		s.logger.Info("Assignment submitted to RootLayer successfully",
			"assignment_id", assignmentID)
	} else {
		s.logger.Warn("CRITICAL: RootLayer client not available, aborting assignment",
			"assignment_id", assignmentID,
			"intent_id", intentID,
			"reason", "validators cannot query assignment without RootLayer")

		s.handleAssignmentFailureLocked(intentID, assignmentID, "missing_rootlayer_client")
		return
	}

	// Step 3: Send task to agent (sync) - agent can also poll for tasks
	s.sendTaskToAgent(winningBid.AgentId, executionTask)
}

// broadcastIntentUpdate sends update to all listening streams
// broadcastIntentUpdate sends update to all listening streams (acquires lock)
func (s *Server) broadcastIntentUpdate(intentID, updateType string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.broadcastIntentUpdateLocked(intentID, updateType)
}

// broadcastIntentUpdateLocked sends update to all listening streams (caller must hold lock)
func (s *Server) broadcastIntentUpdateLocked(intentID, updateType string) {
	update := &pb.MatcherIntentUpdate{
		IntentId:   intentID,
		UpdateType: updateType,
		Timestamp:  time.Now().Unix(),
	}

	s.logger.Debugf("Broadcasting intent update: intent_id=%s update_type=%s streams=%d",
		intentID, updateType, len(s.intentStreams))

	for _, ch := range s.intentStreams {
		select {
		case ch <- update:
			// Successfully sent
		default:
			// Channel full, skip without logging per-stream
		}
	}
}

// computeResultHash computes a hash of the matching result
func (s *Server) computeResultHash(bidID, agentID string) []byte {
	data := fmt.Sprintf("%s:%s:%d", bidID, agentID, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hash[:]
}

// ============ RPC Implementations ============

// SubmitBid implements MatcherServiceServer
func (s *Server) SubmitBid(ctx context.Context, req *pb.SubmitBidRequest) (*pb.SubmitBidResponse, error) {
	if req == nil || req.Bid == nil {
		return nil, fmt.Errorf("invalid request: bid is required")
	}

	s.logger.Infof("Received bid: bid_id=%s agent_id=%s intent_id=%s",
		req.Bid.BidId, req.Bid.AgentId, req.Bid.IntentId)

	// Delegate to processSingleBid to avoid code duplication
	ack := s.processSingleBid(ctx, req.Bid)

	return &pb.SubmitBidResponse{
		Ack: ack,
	}, nil
}

// SubmitBidBatch implements MatcherServiceServer for batch bid submission
func (s *Server) SubmitBidBatch(ctx context.Context, req *pb.SubmitBidBatchRequest) (*pb.SubmitBidBatchResponse, error) {
	if req == nil || len(req.Bids) == 0 {
		return nil, fmt.Errorf("invalid request: bids are required")
	}

	s.logger.Infof("Received batch bid submission: batch_id=%s count=%d", req.BatchId, len(req.Bids))

	partialOk := req.PartialOk != nil && *req.PartialOk
	acks := make([]*pb.BidSubmissionAck, 0, len(req.Bids))
	successCount := int32(0)
	failedCount := int32(0)

	for _, bid := range req.Bids {
		// Process each bid individually
		ack := s.processSingleBid(ctx, bid)
		acks = append(acks, ack)

		if ack.Accepted {
			successCount++
		} else {
			failedCount++
			// If partial success not allowed, stop on first failure
			if !partialOk {
				// Fill remaining with rejection
				for i := len(acks); i < len(req.Bids); i++ {
					acks = append(acks, &pb.BidSubmissionAck{
						BidId:    req.Bids[i].BidId,
						Accepted: false,
						Reason:   "Batch processing stopped due to previous failure",
						Status:   pb.BidStatus_BID_STATUS_REJECTED,
					})
					failedCount++
				}
				break
			}
		}
	}

	msg := fmt.Sprintf("Processed %d bids: %d succeeded, %d failed", len(req.Bids), successCount, failedCount)
	s.logger.Infof("Batch bid processing complete: %s", msg)

	return &pb.SubmitBidBatchResponse{
		Acks:    acks,
		Success: successCount,
		Failed:  failedCount,
		Msg:     msg,
	}, nil
}

// processSingleBid processes a single bid and returns acknowledgment
func (s *Server) processSingleBid(ctx context.Context, bid *pb.Bid) *pb.BidSubmissionAck {
	if bid == nil {
		return &pb.BidSubmissionAck{
			BidId:    "",
			Accepted: false,
			Reason:   "Bid is nil",
			Status:   pb.BidStatus_BID_STATUS_REJECTED,
		}
	}

	// Verify agent if verifier is configured
	if s.verifier != nil {
		chainAddr := extractChainAddressFromBid(bid)
		verified, err := s.verifier.VerifyAgent(ctx, chainAddr)
		if err != nil {
			s.logger.Warnf("On-chain agent verification failed for %s: %v", bid.AgentId, err)
			if !s.allowUnverified {
				return &pb.BidSubmissionAck{
					BidId:    bid.BidId,
					Accepted: false,
					Reason:   "agent verification failed",
					Status:   pb.BidStatus_BID_STATUS_REJECTED,
				}
			}
		} else if !verified {
			s.logger.Warnf("Agent %s not active on-chain (addr=%s)", bid.AgentId, chainAddr)
			if !s.allowUnverified {
				return &pb.BidSubmissionAck{
					BidId:    bid.BidId,
					Accepted: false,
					Reason:   "agent not registered on-chain",
					Status:   pb.BidStatus_BID_STATUS_REJECTED,
				}
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if intent exists
	window, exists := s.intents[bid.IntentId]
	if !exists {
		return &pb.BidSubmissionAck{
			BidId:    bid.BidId,
			Accepted: false,
			Reason:   "Intent not found",
			Status:   pb.BidStatus_BID_STATUS_REJECTED,
		}
	}

	// Check if window is still open
	if window.WindowClosed {
		return &pb.BidSubmissionAck{
			BidId:    bid.BidId,
			Accepted: false,
			Reason:   "Bidding window closed",
			Status:   pb.BidStatus_BID_STATUS_REJECTED,
		}
	}

	// Add bid
	s.bidsByIntent[bid.IntentId] = append(s.bidsByIntent[bid.IntentId], bid)

	return &pb.BidSubmissionAck{
		BidId:      bid.BidId,
		Accepted:   true,
		Status:     pb.BidStatus_BID_STATUS_ACCEPTED,
		RecordedAt: time.Now().Unix(),
	}
}

// GetIntentSnapshot implements MatcherServiceServer
func (s *Server) GetIntentSnapshot(ctx context.Context, req *pb.GetIntentSnapshotRequest) (*pb.GetIntentSnapshotResponse, error) {
	if req == nil || req.IntentId == "" {
		return nil, fmt.Errorf("intent_id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	window, exists := s.intents[req.IntentId]
	if !exists {
		return &pb.GetIntentSnapshotResponse{}, nil
	}

	bids := s.bidsByIntent[req.IntentId]

	// Filter out closed windows if requested
	if !req.IncludeClosed && window.WindowClosed {
		return &pb.GetIntentSnapshotResponse{}, nil
	}

	return &pb.GetIntentSnapshotResponse{
		Snapshot: &pb.IntentBidSnapshot{
			IntentId:         req.IntentId,
			Bids:             bids,
			BiddingStartTime: window.BiddingStartTime,
			BiddingEndTime:   window.BiddingEndTime,
			BiddingClosed:    window.WindowClosed,
		},
	}, nil
}

// StreamIntents implements MatcherServiceServer
func (s *Server) StreamIntents(req *pb.StreamIntentsRequest, stream pb.MatcherService_StreamIntentsServer) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	s.mu.Lock()
	streamID := fmt.Sprintf("stream-%d", s.nextStreamID)
	s.nextStreamID++

	// Create channel for this stream
	ch := make(chan *pb.MatcherIntentUpdate, 100)
	s.intentStreams[streamID] = ch
	s.mu.Unlock()

	// Clean up on exit
	defer func() {
		s.mu.Lock()
		delete(s.intentStreams, streamID)
		s.mu.Unlock()
		close(ch)
	}()

	s.logger.Infof("Started intent stream: stream_id=%s subnet_id=%s", streamID, req.SubnetId)

	// Send existing intents first
	s.mu.RLock()
	for intentID, window := range s.intents {
		if !window.WindowClosed {
			update := &pb.MatcherIntentUpdate{
				IntentId:   intentID,
				UpdateType: "EXISTING_INTENT",
				Timestamp:  time.Now().Unix(),
			}
			select {
			case ch <- update:
			default:
			}
		}
	}
	s.mu.RUnlock()

	// Stream updates
	for {
		select {
		case update := <-ch:
			if err := stream.Send(update); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// StreamBids implements MatcherServiceServer (placeholder)
func (s *Server) StreamBids(req *pb.StreamBidsRequest, stream pb.MatcherService_StreamBidsServer) error {
	// Placeholder implementation
	<-stream.Context().Done()
	return stream.Context().Err()
}

// PublishMatchingResult implements MatcherServiceServer
func (s *Server) PublishMatchingResult(ctx context.Context, req *pb.MatchingResult) (*pb.BidSubmissionAck, error) {
	// This would publish to RootLayer in production
	s.logger.Infof("Publishing matching result for intent %s", req.IntentId)

	return &pb.BidSubmissionAck{
		BidId:      req.WinningBidId,
		Accepted:   true,
		Status:     pb.BidStatus_BID_STATUS_WINNER,
		RecordedAt: time.Now().Unix(),
	}, nil
}

// RespondToTask handles agent's response to a task assignment
func (s *Server) RespondToTask(ctx context.Context, req *pb.RespondToTaskRequest) (*pb.RespondToTaskResponse, error) {
	if req == nil || req.Response == nil {
		return nil, fmt.Errorf("invalid request")
	}

	resp := req.Response
	s.logger.Infof("Received task response from agent %s for task %s: accepted=%v",
		resp.AgentId, resp.TaskId, resp.Accepted)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the pending assignment
	pending, exists := s.pendingAssignments[resp.TaskId]
	if !exists {
		return &pb.RespondToTaskResponse{
			Success: false,
			Message: "Task not found or already confirmed",
		}, nil
	}

	if resp.Accepted {
		// Agent accepted the task
		s.logger.Infof("Agent %s accepted task %s", resp.AgentId, resp.TaskId)

		// Move from pending to confirmed
		s.assignments[resp.TaskId] = pending.Assignment
		delete(s.pendingAssignments, resp.TaskId)

		return &pb.RespondToTaskResponse{
			Success: true,
			Message: "Task accepted",
		}, nil
	} else {
		// Agent rejected the task
		s.logger.Warnf("Agent %s rejected task %s: %s", resp.AgentId, resp.TaskId, resp.Reason)

		// Remove from pending
		pendingAssignment, exists := s.pendingAssignments[resp.TaskId]
		delete(s.pendingAssignments, resp.TaskId)

		// Try to assign to runner-up
		if exists && pendingAssignment.Intent != nil {
			go s.tryAssignToRunnerUp(pendingAssignment.Intent, resp.AgentId, resp.Reason)
		}

		return &pb.RespondToTaskResponse{
			Success: true,
			Message: fmt.Sprintf("Task rejection recorded, attempting reassignment: %s", resp.Reason),
		}, nil
	}
}

// StreamTasks streams execution tasks to a specific agent
func (s *Server) StreamTasks(req *pb.StreamTasksRequest, stream pb.MatcherService_StreamTasksServer) error {
	if req == nil || req.AgentId == "" {
		return fmt.Errorf("agent_id is required")
	}

	s.mu.Lock()
	streamID := fmt.Sprintf("task-stream-%d", s.nextStreamID)
	s.nextStreamID++

	// Create channel for this stream
	ch := make(chan *pb.ExecutionTask, 100)
	s.taskStreams[streamID] = ch

	// Also register agent for direct task delivery
	if _, exists := s.agentTasks[req.AgentId]; !exists {
		s.agentTasks[req.AgentId] = ch
	}
	s.mu.Unlock()

	s.logger.Infof("Started task stream %s for agent %s", streamID, req.AgentId)

	// Clean up on exit
	defer func() {
		s.mu.Lock()
		delete(s.taskStreams, streamID)
		// Only delete agent channel if it's the same one we created
		if s.agentTasks[req.AgentId] == ch {
			delete(s.agentTasks, req.AgentId)
		}
		s.mu.Unlock()
		close(ch)
	}()

	// Stream tasks to agent
	for {
		select {
		case task, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(task); err != nil {
				s.logger.Errorf("Error sending task to agent %s: %v", req.AgentId, err)
				return err
			}
			s.logger.Infof("Streamed task %s to agent %s", task.TaskId, req.AgentId)

		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// GetMatchingResult returns the matching result for an intent (helper method)
func (s *Server) GetMatchingResult(intentID string) *pb.MatchingResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.matchingResults[intentID]
}

// GetAssignment returns an assignment by intent ID (helper method)
func (s *Server) GetAssignment(intentID string) *rootpb.Assignment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// First check confirmed assignments
	for _, assignment := range s.assignments {
		if assignment.IntentId == intentID {
			return assignment
		}
	}

	// Also check pending assignments (for newly created ones)
	for _, pending := range s.pendingAssignments {
		if pending.Assignment != nil && pending.Assignment.IntentId == intentID {
			return pending.Assignment
		}
	}

	return nil
}

// sendTaskToAgent sends execution task to the winning agent
func (s *Server) sendTaskToAgent(agentID string, task *pb.ExecutionTask) {
	if ch, exists := s.agentTasks[agentID]; exists {
		select {
		case ch <- task:
			s.logger.Infof("Sent task %s to agent %s", task.TaskId, agentID)
		default:
			s.logger.Warnf("Agent %s task channel full, dropping task", agentID)
		}
	} else {
		s.logger.Warnf("No task channel for agent %s", agentID)
	}
}

// RegisterAgentForTasks registers an agent to receive tasks
func (s *Server) RegisterAgentForTasks(agentID string) <-chan *pb.ExecutionTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create channel for agent if doesn't exist
	if _, exists := s.agentTasks[agentID]; !exists {
		s.agentTasks[agentID] = make(chan *pb.ExecutionTask, 10)
		s.logger.Infof("Registered agent %s for tasks", agentID)
	}

	return s.agentTasks[agentID]
}

// UnregisterAgentForTasks unregisters an agent from receiving tasks
func (s *Server) UnregisterAgentForTasks(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ch, exists := s.agentTasks[agentID]; exists {
		close(ch)
		delete(s.agentTasks, agentID)
		s.logger.Infof("Unregistered agent %s from tasks", agentID)
	}
}

// pullIntentsFromRootLayer pulls intents from RootLayer with auto-reconnect
func (s *Server) pullIntentsFromRootLayer(ctx context.Context) {
	// Use subnet ID from configuration (REQUIRED)
	if s.cfg.Identity == nil || s.cfg.Identity.SubnetID == "" {
		s.logger.Errorf("CRITICAL: subnet_id not configured, cannot stream intents")
		return
	}
	subnetID := s.cfg.Identity.SubnetID

	// Retry configuration
	retryDelay := 5 * time.Second
	maxRetryDelay := 60 * time.Second
	pollingInterval := 15 * time.Second
	streamRestoreInterval := time.Minute
	pollingRequestTimeout := 10 * time.Second
	streamingMode := true

	s.logger.Info("Started pulling intents from RootLayer")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Try streaming first
		if streamingMode {
			intentStream, err := s.rootlayerClient.StreamIntents(ctx, subnetID)
			if err != nil {
				s.logger.Warnf("Failed to connect intent stream: %v, retrying in %v", err, retryDelay)
				time.Sleep(retryDelay)
				retryDelay = min(retryDelay*2, maxRetryDelay)
				continue
			}

			s.logger.Info("Connected to RootLayer intent stream")
			retryDelay = 5 * time.Second // Reset on success

			// Process streaming intents
			streamBroken := false
			s.logger.Info("[DEBUG] Starting intent stream processing loop")
			for !streamBroken {
				select {
				case <-ctx.Done():
					s.logger.Info("[DEBUG] Context cancelled in stream processing")
					return
				case intent, ok := <-intentStream:
					if !ok {
						s.logger.Warn("Intent stream closed, switching to polling mode...")
						streamingMode = false
						streamBroken = true
						break
					}

					s.logger.Infof("[DEBUG] Received intent from stream channel: %s", intent.IntentId)
					// Add the intent to our system
					if err := s.AddIntent(intent); err != nil {
						s.logger.Errorf("Failed to add intent from stream: %v", err)
					} else {
						s.logger.Info("[DEBUG] Intent AddIntent call succeeded (no error)")
					}
				}
			}
			s.logger.Info("[DEBUG] Exited intent stream processing loop")
		}

		// Polling fallback mode
		if !streamingMode {
			s.logger.Infof("Using polling mode (interval: %v)", pollingInterval)
			pollTicker := time.NewTicker(pollingInterval)
			restoreTicker := time.NewTicker(streamRestoreInterval)

		pollingLoop:
			for {
				select {
				case <-ctx.Done():
					pollTicker.Stop()
					restoreTicker.Stop()
					return
				case <-pollTicker.C:
					// Bound RootLayer polling so we don't freeze this loop forever
					pollCtx, pollCancel := context.WithTimeout(ctx, pollingRequestTimeout)
					intents, err := s.rootlayerClient.GetPendingIntents(pollCtx, subnetID)
					pollCancel()

					if err != nil {
						s.logger.Warnf("Failed to poll intents: %v", err)
						continue
					}

					// Add polled intents
					for _, intent := range intents {
						if err := s.AddIntent(intent); err != nil {
							s.logger.Errorf("Failed to add intent from polling: %v", err)
						}
					}

				case <-restoreTicker.C:
					// Periodically try to restore streaming regardless of poll tick alignment
					s.logger.Info("Attempting to restore streaming mode...")
					streamingMode = true
					pollTicker.Stop()
					restoreTicker.Stop()
					break pollingLoop
				}
			}
		}
	}
}

// submitAssignmentToChainSync submits assignment to blockchain synchronously (waits for tx)
// Returns transaction hash on success, error on failure
func (s *Server) submitAssignmentToChainSync(assignmentID, intentID, bidID, agentID string, signature []byte) (string, error) {
	// Check if on-chain submission is enabled
	if s.sdkClient == nil {
		return "", fmt.Errorf("SDK client not initialized")
	}

	// Parse IDs to [32]byte format
	assignmentIDBytes, err := hexStringTo32Bytes(assignmentID)
	if err != nil {
		return "", fmt.Errorf("failed to parse assignment ID: %w", err)
	}

	intentIDBytes, err := hexStringTo32Bytes(intentID)
	if err != nil {
		return "", fmt.Errorf("failed to parse intent ID: %w", err)
	}

	bidIDBytes, err := hexStringTo32Bytes(bidID)
	if err != nil {
		return "", fmt.Errorf("failed to parse bid ID: %w", err)
	}

	// Create assignment data
	assignmentData := sdk.AssignmentData{
		AssignmentID: assignmentIDBytes,
		IntentID:     intentIDBytes,
		BidID:        bidIDBytes,
		Agent:        common.HexToAddress(agentID),
		Status:       sdk.AssignmentStatusActive,
		Matcher:      common.HexToAddress(s.signer.GetAddress()),
	}

	// Create signed assignment
	signedAssignment := sdk.SignedAssignment{
		Data:      assignmentData,
		Signature: signature,
	}

	// Submit to blockchain with extended timeout (blockchain operations take time)
	timeout := 30 * time.Second // Longer timeout for blockchain confirmation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.logger.Info("Submitting assignment to blockchain",
		"assignment_id", assignmentID,
		"intent_id", intentID,
		"agent", agentID)

	tx, err := s.sdkClient.Assignment.AssignIntentsBySignatures(ctx, []sdk.SignedAssignment{signedAssignment})
	if err != nil {
		// Log blockchain submission failure with structured fields
		s.logger.Error("Blockchain assignment submission failed",
			"assignment_id", assignmentID,
			"intent_id", intentID,
			"bid_id", bidID,
			"agent_address", agentID,
			"matcher_address", s.signer.GetAddress(),
			"signature_len", len(signature),
			"error", err.Error())

		// Add diagnostic hint for common errors
		if strings.Contains(err.Error(), "execution reverted") {
			s.logger.Warn("Transaction reverted - check agent registration and stake status",
				"agent_address", agentID,
				"hint", "verify with: cast call <INTENT_MANAGER> 'getAgent(address)' "+agentID)
		}

		return "", fmt.Errorf("blockchain submission failed: %w", err)
	}

	txHash := tx.Hash().Hex()
	s.logger.Infof("Assignment transaction sent, hash: %s (waiting for confirmation...)", txHash)

	// Note: We don't wait for mining confirmation here to avoid blocking too long
	// The transaction is in the mempool, which is sufficient for our purposes
	// If you need to wait for confirmation, add: bind.WaitMined(ctx, s.sdkClient.Backend, tx)

	return txHash, nil
}

// handleAssignmentFailureLocked cleans up state after assignment submission failure.
// MUST be called while holding s.mu (createAssignment already does this).
func (s *Server) handleAssignmentFailureLocked(intentID, assignmentID, stage string, errDetails ...error) {
	now := time.Now().Unix()
	window := s.intents[intentID]

	if len(errDetails) > 0 {
		if status.Code(errDetails[0]) == codes.InvalidArgument {
			// InvalidArgument typically indicates RootLayer rejected the payload (e.g., expired intent)
			s.logger.Warn("RootLayer returned InvalidArgument during assignment submission",
				"intent_id", intentID,
				"assignment_id", assignmentID,
				"stage", stage,
				"error", errDetails[0].Error())
		}
	}

	if window != nil {
		if expired, reason := s.intentExpired(window.Intent, now); expired {
			s.logger.Warn("Dropping expired intent after assignment failure",
				"intent_id", intentID,
				"stage", stage,
				"reason", reason)
			s.dropIntentLocked(intentID)
		} else {
			s.logger.Warn("Re-opening bidding window after assignment failure",
				"intent_id", intentID,
				"stage", stage)
			s.resetIntentWindowLocked(intentID)
		}
	}

	s.cleanupPendingAssignmentLocked(assignmentID)
}

// intentExpired determines if the intent has exceeded either deadline or intent_expiration.
func (s *Server) intentExpired(intent *rootpb.Intent, now int64) (bool, string) {
	if intent == nil {
		return false, ""
	}

	if intent.IntentExpiration > 0 && now >= intent.IntentExpiration {
		return true, fmt.Sprintf("intent_expiration=%d (UTC %s)", intent.IntentExpiration, time.Unix(intent.IntentExpiration, 0).UTC().Format(time.RFC3339))
	}

	if intent.Deadline > 0 && now >= intent.Deadline {
		return true, fmt.Sprintf("deadline=%d (UTC %s)", intent.Deadline, time.Unix(intent.Deadline, 0).UTC().Format(time.RFC3339))
	}

	return false, ""
}

// resetIntentWindowLocked re-opens the bidding window for a retry. Caller must hold s.mu.
func (s *Server) resetIntentWindowLocked(intentID string) {
	window, exists := s.intents[intentID]
	if !exists {
		return
	}

	now := time.Now().Unix()
	window.WindowClosed = false
	window.Matched = false
	window.BiddingStartTime = now
	window.BiddingEndTime = now + int64(s.cfg.BiddingWindowSec)
	delete(s.matchingResults, intentID)
}

// dropIntentLocked removes all matcher state for an intent. Caller must hold s.mu.
func (s *Server) dropIntentLocked(intentID string) {
	delete(s.intents, intentID)
	delete(s.bidsByIntent, intentID)
	delete(s.matchingResults, intentID)
}

// cleanupPendingAssignmentLocked removes pending assignment bookkeeping for aborted flows.
func (s *Server) cleanupPendingAssignmentLocked(assignmentID string) {
	delete(s.pendingAssignments, assignmentID)
	delete(s.assignmentAgents, assignmentID)
}

// submitAssignmentToChain submits assignment to blockchain asynchronously (legacy, for background retry)
func (s *Server) submitAssignmentToChain(assignmentID, intentID, bidID, agentID string, signature []byte) {
	txHash, err := s.submitAssignmentToChainSync(assignmentID, intentID, bidID, agentID, signature)
	if err != nil {
		s.logger.Errorf("Failed to submit assignment to blockchain: %v", err)
		return
	}
	s.logger.Infof("Successfully submitted assignment %s to blockchain, tx: %s", assignmentID, txHash)
}

// tryAssignToRunnerUp attempts to assign the intent to the next best agent
func (s *Server) tryAssignToRunnerUp(intent *rootpb.Intent, excludeAgentID string, reason string) {
	s.mu.RLock()
	// Get all bids for this intent
	bids := s.bidsByIntent[intent.IntentId]
	s.mu.RUnlock()

	if len(bids) == 0 {
		s.logger.Errorf("No bids available for reassignment of intent %s", intent.IntentId)
		return
	}

	// Find the next best bid (excluding the agent that rejected)
	var nextBest *pb.Bid
	for _, bid := range bids {
		if bid.AgentId == excludeAgentID {
			continue
		}
		if bid.Status != pb.BidStatus_BID_STATUS_SUBMITTED {
			continue
		}
		if nextBest == nil || bid.Price < nextBest.Price {
			nextBest = bid
		}
	}

	if nextBest == nil {
		s.logger.Errorf("No runner-up available for intent %s after %s rejected (%s)",
			intent.IntentId, excludeAgentID, reason)
		// Note: Intent status updates are handled by RootLayer based on validation bundles
		// UpdateIntentStatus method is not available in the gRPC service
		if s.rootlayerClient != nil {
			s.logger.Warn("No runner-up available for intent, marking as failed",
				"intent_id", intent.IntentId)
		}
		return
	}

	// Create new assignment for runner-up
	s.logger.Infof("Reassigning intent %s to runner-up agent %s", intent.IntentId, nextBest.AgentId)

	// Mark as winning bid and create assignment
	s.mu.Lock()
	nextBest.Status = pb.BidStatus_BID_STATUS_WINNER
	intentWindow := s.intents[intent.IntentId]
	s.mu.Unlock()

	if intentWindow != nil {
		// Create assignment using existing flow
		s.createAssignment(intent.IntentId, nextBest)
	}
}

// Helper method to get RootLayer submit timeout from config
func (s *Server) getRootLayerSubmitTimeout() time.Duration {
	if s.cfg != nil && s.cfg.Timeouts != nil {
		return s.cfg.Timeouts.RootLayerSubmitTimeout
	}
	return 5 * time.Second // Default fallback
}

// signMatchingResult creates a signature for matching result
func (s *Server) signMatchingResult(intentID, bidID, agentID string) ([]byte, error) {
	if s.signer == nil {
		return nil, fmt.Errorf("CRITICAL: No signer configured for matching result - signatures required for security")
	}

	// Create message to sign
	message := fmt.Sprintf("%s:%s:%s:%s:%d",
		s.cfg.MatcherID,
		intentID,
		bidID,
		agentID,
		time.Now().Unix())
	msgHash := crypto.HashMessage([]byte(message))

	// Sign the hash
	signature, err := s.signer.Sign(msgHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign matching result: %w", err)
	}

	return signature, nil
}

// signAssignment creates an EIP-191 signature for assignment using SDK
func (s *Server) signAssignment(assignmentID, intentID, bidID, agentID string) ([]byte, error) {
	// If SDK client is available, use EIP-191 standard signature
	if s.sdkClient != nil {
		// Parse assignment ID (assuming hex format like "assign-0x...")
		// Extract the hex part and convert to [32]byte
		assignmentIDBytes, err := hexStringTo32Bytes(assignmentID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse assignment ID: %w", err)
		}

		intentIDBytes, err := hexStringTo32Bytes(intentID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse intent ID: %w", err)
		}

		bidIDBytes, err := hexStringTo32Bytes(bidID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bid ID: %w", err)
		}

		// Create assignment data
		assignmentData := sdk.AssignmentData{
			AssignmentID: assignmentIDBytes,
			IntentID:     intentIDBytes,
			BidID:        bidIDBytes,
			Agent:        common.HexToAddress(agentID),
			Status:       sdk.AssignmentStatusActive,
			Matcher:      common.HexToAddress(s.signer.GetAddress()),
		}

		// Use SDK to sign assignment (EIP-191 standard)
		signature, err := s.sdkClient.Assignment.SignAssignment(assignmentData)
		if err != nil {
			return nil, fmt.Errorf("failed to sign assignment with SDK: %w", err)
		}

		return signature, nil
	}

	// Fallback to simple signature if SDK not available
	if s.signer == nil {
		return nil, fmt.Errorf("CRITICAL: No signer configured for assignment - signatures required for security")
	}

	// Create message to sign (legacy method)
	message := fmt.Sprintf("%s:%s:%s:%s:%s:%d",
		s.cfg.MatcherID,
		assignmentID,
		intentID,
		bidID,
		agentID,
		time.Now().Unix())
	msgHash := crypto.HashMessage([]byte(message))

	// Sign the hash
	signature, err := s.signer.Sign(msgHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign assignment: %w", err)
	}

	return signature, nil
}

// hexStringTo32Bytes converts a hex string (with or without 0x prefix) to [32]byte
func hexStringTo32Bytes(s string) ([32]byte, error) {
	var result [32]byte

	// Remove 0x prefix if present
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}

	// If string is shorter than 64 chars, pad with zeros on the left
	if len(s) < 64 {
		s = strings.Repeat("0", 64-len(s)) + s
	}

	// Decode hex
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return result, fmt.Errorf("invalid hex string: %w", err)
	}

	// Take last 32 bytes if longer
	if len(bytes) > 32 {
		bytes = bytes[len(bytes)-32:]
	}

	copy(result[:], bytes)
	return result, nil
}

// RegisterGRPC registers the service with a gRPC server
func (s *Server) RegisterGRPC(grpcServer *grpc.Server) {
	pb.RegisterMatcherServiceServer(grpcServer, s)
}
