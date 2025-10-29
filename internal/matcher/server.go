package matcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"

	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"github.com/PIN-AI/intent-protocol-contract-sdk/sdk/addressbook"
	rootpb "subnet/proto/rootlayer"
	"subnet/internal/blockchain"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/rootlayer"
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
	sdkClient       *sdk.Client // SDK client for on-chain operations

	// Intent and bid management
	mu              sync.RWMutex
	intents         map[string]*IntentWithWindow  // intent_id -> intent with window
	bidsByIntent    map[string][]*pb.Bid          // intent_id -> bids
	matchingResults map[string]*pb.MatchingResult // intent_id -> result
	assignments     map[string]*rootpb.Assignment // assignment_id -> assignment

	// Assignment confirmation tracking
	pendingAssignments map[string]*PendingAssignment // assignment_id -> pending assignment
	assignmentAgents   map[string]string             // assignment_id -> agent_id

	// Batch assignment submission
	assignmentBatchBuffer []*rootpb.Assignment // Buffer for batch submission to RootLayer
	assignmentBatchMu     sync.Mutex           // Separate mutex for batch buffer

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

// NewServer creates a new Matcher server with window management
func NewServer(cfg *Config, logger logging.Logger) (*Server, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = logging.NewDefaultLogger()
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
		cfg:                   cfg,
		logger:                logger,
		signer:                signer,
		rootlayerClient:       rootlayerClient,
		sdkClient:             sdkClient,
		intents:               make(map[string]*IntentWithWindow),
		bidsByIntent:          make(map[string][]*pb.Bid),
		matchingResults:       make(map[string]*pb.MatchingResult),
		assignments:           make(map[string]*rootpb.Assignment),
		pendingAssignments:    make(map[string]*PendingAssignment),
		assignmentAgents:      make(map[string]string),
		assignmentBatchBuffer: make([]*rootpb.Assignment, 0, 100), // Pre-allocate buffer for batch submission
		intentStreams:         make(map[string]chan *pb.MatcherIntentUpdate),
		agentTasks:            make(map[string]chan *pb.ExecutionTask),
		taskStreams:           make(map[string]chan *pb.ExecutionTask),
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

	// Start batch assignment submission loop
	go s.batchAssignmentSubmissionLoop(ctx)

	s.started = true
	s.logger.Info("Matcher service started with bidding window management, batch assignment submission, and RootLayer integration")
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
	s.mu.Lock()

	if _, exists := s.intents[intent.IntentId]; exists {
		s.mu.Unlock()
		return fmt.Errorf("intent %s already exists", intent.IntentId)
	}

	// Create intent with window
	now := time.Now().Unix()
	window := &IntentWithWindow{
		Intent:           intent,
		BiddingStartTime: now,
		BiddingEndTime:   now + int64(s.cfg.BiddingWindowSec),
		WindowClosed:     false,
		Matched:          false,
	}

	s.intents[intent.IntentId] = window
	s.bidsByIntent[intent.IntentId] = make([]*pb.Bid, 0)

	s.logger.Infof("Added intent %s with bidding window %d seconds",
		intent.IntentId, s.cfg.BiddingWindowSec)

	// Unlock before broadcasting to avoid deadlock
	// (broadcastIntentUpdate needs to acquire read lock)
	s.mu.Unlock()

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

			// Notify streams
			s.broadcastIntentUpdate(intentID, "WINDOW_CLOSED")
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

	s.logger.Infof("Performing matching for intent %s with %d bids", intentID, len(bids))

	// Sort bids by price (ascending)
	sort.Slice(bids, func(i, j int) bool {
		return bids[i].Price < bids[j].Price
	})

	// Select winner (lowest price)
	winner := bids[0]

	// Select runner-ups (next 2 lowest)
	runnerUpIDs := []string{}
	for i := 1; i < len(bids) && i < 3; i++ {
		runnerUpIDs = append(runnerUpIDs, bids[i].BidId)
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

	// Notify about matching
	s.broadcastIntentUpdate(intentID, "MATCHED")
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
			s.logger.Errorf("CRITICAL: Failed to submit assignment to blockchain, aborting assignment: %v", err)
			// Mark intent as failed or retry matching
			window := s.intents[intentID]
			if window != nil {
				window.Matched = false // Allow re-matching
			}
			return
		}
		s.logger.Infof("Assignment %s submitted to blockchain, tx: %s", assignmentID, chainTxHash)
	}

	// Step 2: Submit to RootLayer (async) - can be retried or synced from chain later
	go s.submitAssignmentToRootLayer(rootAssignment)

	// Step 3: Send task to agent (sync) - agent can also poll for tasks
	s.sendTaskToAgent(winningBid.AgentId, executionTask)
}

// broadcastIntentUpdate sends update to all listening streams
func (s *Server) broadcastIntentUpdate(intentID, updateType string) {
	s.logger.Infof("[MATCHER DEBUG] broadcastIntentUpdate called for intent %s, type %s", intentID, updateType)

	update := &pb.MatcherIntentUpdate{
		IntentId:   intentID,
		UpdateType: updateType,
		Timestamp:  time.Now().Unix(),
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	s.logger.Infof("[MATCHER DEBUG] Broadcasting to %d streams", len(s.intentStreams))

	for streamID, ch := range s.intentStreams {
		s.logger.Infof("[MATCHER DEBUG] Attempting to send to stream %s", streamID)
		select {
		case ch <- update:
			s.logger.Infof("[MATCHER DEBUG] ✓ Sent update to stream %s", streamID)
		default:
			s.logger.Warnf("[MATCHER DEBUG] Stream %s channel full, skipping update", streamID)
		}
	}

	s.logger.Infof("[MATCHER DEBUG] Broadcast complete")
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

	bid := req.Bid
	s.logger.Infof("Received bid %s from agent %s for intent %s",
		bid.BidId, bid.AgentId, bid.IntentId)

	if s.verifier != nil {
		chainAddr := extractChainAddressFromBid(bid)
		verified, err := s.verifier.VerifyAgent(ctx, chainAddr)
		if err != nil {
			s.logger.Warnf("On-chain agent verification failed for %s: %v", bid.AgentId, err)
			if !s.allowUnverified {
				return &pb.SubmitBidResponse{
					Ack: &pb.BidSubmissionAck{
						BidId:    bid.BidId,
						Accepted: false,
						Reason:   "agent verification failed",
						Status:   pb.BidStatus_BID_STATUS_REJECTED,
					},
				}, nil
			}
		} else if !verified {
			s.logger.Warnf("Agent %s not active on-chain (addr=%s)", bid.AgentId, chainAddr)
			if !s.allowUnverified {
				return &pb.SubmitBidResponse{
					Ack: &pb.BidSubmissionAck{
						BidId:    bid.BidId,
						Accepted: false,
						Reason:   "agent not registered on-chain",
						Status:   pb.BidStatus_BID_STATUS_REJECTED,
					},
				}, nil
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if intent exists
	window, exists := s.intents[bid.IntentId]
	if !exists {
		return &pb.SubmitBidResponse{
			Ack: &pb.BidSubmissionAck{
				BidId:    bid.BidId,
				Accepted: false,
				Reason:   "Intent not found",
				Status:   pb.BidStatus_BID_STATUS_REJECTED,
			},
		}, nil
	}

	// Check if window is still open
	if window.WindowClosed {
		return &pb.SubmitBidResponse{
			Ack: &pb.BidSubmissionAck{
				BidId:    bid.BidId,
				Accepted: false,
				Reason:   "Bidding window closed",
				Status:   pb.BidStatus_BID_STATUS_REJECTED,
			},
		}, nil
	}

	// Add bid
	s.bidsByIntent[bid.IntentId] = append(s.bidsByIntent[bid.IntentId], bid)

	return &pb.SubmitBidResponse{
		Ack: &pb.BidSubmissionAck{
			BidId:      bid.BidId,
			Accepted:   true,
			Status:     pb.BidStatus_BID_STATUS_ACCEPTED,
			RecordedAt: time.Now().Unix(),
		},
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
	s.logger.Infof("[MATCHER DEBUG] StreamIntents called! Request: %+v", req)

	if req == nil {
		s.logger.Errorf("[MATCHER DEBUG] Request is nil, returning error")
		return fmt.Errorf("request is nil")
	}

	s.logger.Infof("[MATCHER DEBUG] Request valid, acquiring lock...")
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

	s.logger.Infof("Started intent stream %s for subnet %s", streamID, req.SubnetId)

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

		// Submit to RootLayer
		go s.submitAssignmentToRootLayer(pending.Assignment)

		return &pb.RespondToTaskResponse{
			Success: true,
			Message: "Task accepted and assignment submitted to RootLayer",
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
			for !streamBroken {
				select {
				case <-ctx.Done():
					return
				case intent, ok := <-intentStream:
					if !ok {
						s.logger.Warn("Intent stream closed, switching to polling mode...")
						streamingMode = false
						streamBroken = true
						break
					}

					// Add the intent to our system
					if err := s.AddIntent(intent); err != nil {
						s.logger.Errorf("Failed to add intent from stream: %v", err)
					}
				}
			}
		}

		// Polling fallback mode
		if !streamingMode {
			s.logger.Infof("Using polling mode (interval: %v)", pollingInterval)
			ticker := time.NewTicker(pollingInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Try to get pending intents via polling
					intents, err := s.rootlayerClient.GetPendingIntents(ctx, subnetID)
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

					// Periodically try to restore streaming
					if time.Now().Unix()%60 == 0 { // Every ~60 seconds
						s.logger.Info("Attempting to restore streaming mode...")
						streamingMode = true
						ticker.Stop()
						goto retry
					}
				}
			}
		retry:
		}
	}
}

// submitAssignmentToRootLayer adds assignment to batch buffer for submission to RootLayer
func (s *Server) submitAssignmentToRootLayer(assignment *rootpb.Assignment) {
	s.assignmentBatchMu.Lock()
	defer s.assignmentBatchMu.Unlock()

	// Add assignment to batch buffer
	s.assignmentBatchBuffer = append(s.assignmentBatchBuffer, assignment)
	s.logger.Debugf("Added assignment %s to batch buffer (current size: %d)",
		assignment.AssignmentId, len(s.assignmentBatchBuffer))
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

	s.logger.Infof("Submitting assignment %s to blockchain...", assignmentID)
	tx, err := s.sdkClient.Assignment.AssignIntentsBySignatures(ctx, []sdk.SignedAssignment{signedAssignment})
	if err != nil {
		return "", fmt.Errorf("blockchain submission failed: %w", err)
	}

	txHash := tx.Hash().Hex()
	s.logger.Infof("Assignment transaction sent, hash: %s (waiting for confirmation...)", txHash)

	// Note: We don't wait for mining confirmation here to avoid blocking too long
	// The transaction is in the mempool, which is sufficient for our purposes
	// If you need to wait for confirmation, add: bind.WaitMined(ctx, s.sdkClient.Backend, tx)

	return txHash, nil
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

// batchAssignmentSubmissionLoop periodically flushes assignment batch buffer to RootLayer
func (s *Server) batchAssignmentSubmissionLoop(ctx context.Context) {
	// Batch submission interval - submit every 5 seconds or when buffer reaches 10 assignments
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	const maxBatchSize = 10

	for {
		select {
		case <-ctx.Done():
			// Final flush before exit
			s.flushAssignmentBatch()
			return
		case <-ticker.C:
			// Periodic flush
			s.flushAssignmentBatch()
		default:
			// Check if buffer is full and needs immediate flush
			s.assignmentBatchMu.Lock()
			bufferSize := len(s.assignmentBatchBuffer)
			s.assignmentBatchMu.Unlock()

			if bufferSize >= maxBatchSize {
				s.flushAssignmentBatch()
			}
			time.Sleep(100 * time.Millisecond) // Small sleep to avoid busy loop
		}
	}
}

// flushAssignmentBatch submits all buffered assignments to RootLayer in batch
func (s *Server) flushAssignmentBatch() {
	s.assignmentBatchMu.Lock()
	if len(s.assignmentBatchBuffer) == 0 {
		s.assignmentBatchMu.Unlock()
		return
	}

	// Take all assignments from buffer and clear it
	assignments := make([]*rootpb.Assignment, len(s.assignmentBatchBuffer))
	copy(assignments, s.assignmentBatchBuffer)
	s.assignmentBatchBuffer = s.assignmentBatchBuffer[:0] // Clear buffer
	s.assignmentBatchMu.Unlock()

	// Check if RootLayer client supports batch submission
	type batchAssignmentSubmitter interface {
		PostAssignmentBatch(ctx context.Context, assignments []*rootpb.Assignment) error
	}

	batchClient, supportsBatch := s.rootlayerClient.(batchAssignmentSubmitter)

	if supportsBatch {
		// Use batch submission API
		s.logger.Infof("Submitting %d assignments in batch to RootLayer", len(assignments))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := batchClient.PostAssignmentBatch(ctx, assignments)
		if err != nil {
			s.logger.Warnf("Assignment batch submission failed, falling back to individual submission error=%v", err)
			s.submitIndividualAssignments(assignments)
			return
		}

		s.logger.Infof("Assignment batch submission completed: %d assignments", len(assignments))
	} else {
		// Fallback to individual submission
		s.logger.Warn("RootLayer client doesn't support batch assignment submission, using individual submission")
		s.submitIndividualAssignments(assignments)
	}
}

// submitIndividualAssignments submits assignments one by one with retry for intent confirmation (fallback method)
func (s *Server) submitIndividualAssignments(assignments []*rootpb.Assignment) {
	for _, assignment := range assignments {
		go s.submitAssignmentWithRetry(assignment)
	}
}

// submitAssignmentWithRetry submits an assignment with retry logic for intent confirmation
func (s *Server) submitAssignmentWithRetry(assignment *rootpb.Assignment) {
	const maxRetries = 3
	const baseDelay = 15 * time.Second // Wait 15s between retries for blockchain confirmation

	for attempt := 0; attempt < maxRetries; attempt++ {
		timeout := s.getRootLayerSubmitTimeout()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		err := s.rootlayerClient.SubmitAssignment(ctx, assignment)
		cancel()

		if err == nil {
			s.logger.Infof("Successfully submitted assignment %s to RootLayer (attempt %d)",
				assignment.AssignmentId, attempt+1)
			return
		}

		// Check if error is due to intent not confirmed on blockchain
		errMsg := err.Error()
		isIntentNotConfirmed := strings.Contains(errMsg, "intent pending not confirmed") ||
			strings.Contains(errMsg, "intent not confirmed") ||
			strings.Contains(errMsg, "Invalid intent status")

		if !isIntentNotConfirmed {
			// Other error, don't retry
			s.logger.Errorf("Failed to submit assignment %s: %v (non-retryable error)",
				assignment.AssignmentId, err)
			return
		}

		// Intent not confirmed yet, wait and retry
		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(attempt+1) // Exponential backoff: 15s, 30s, 45s
			s.logger.Warnf("Assignment %s submission failed (intent not confirmed yet), retrying in %v (attempt %d/%d)",
				assignment.AssignmentId, delay, attempt+1, maxRetries)
			time.Sleep(delay)
		} else {
			// Final attempt failed
			s.logger.Errorf("Failed to submit assignment %s after %d attempts: intent still not confirmed on blockchain",
				assignment.AssignmentId, maxRetries)
		}
	}
}

// RegisterGRPC registers the service with a gRPC server
func (s *Server) RegisterGRPC(grpcServer *grpc.Server) {
	pb.RegisterMatcherServiceServer(grpcServer, s)
}
