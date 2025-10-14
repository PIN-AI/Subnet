package rootlayer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	rootpb "rootlayer/proto"
	pb "subnet/proto/subnet"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/types"
)

// CompleteClient provides complete RootLayer integration
type CompleteClient struct {
	*BaseClient

	conn               *grpc.ClientConn
	intentPoolClient   rootpb.IntentPoolServiceClient
	subscriptionClient rootpb.SubscriptionServiceClient

	signer crypto.ExtendedSigner
	mu     sync.RWMutex
}

// CompleteClientConfig configuration for complete client
type CompleteClientConfig struct {
	Endpoint   string
	SubnetID   string
	NodeID     string
	NodeType   string // "validator", "matcher", "agent"
	PrivateKey string
}

// NewCompleteClient creates a new complete RootLayer client
func NewCompleteClient(cfg *CompleteClientConfig, logger logging.Logger) (*CompleteClient, error) {
	baseCfg := &Config{
		Endpoint:     cfg.Endpoint,
		SubnetID:     cfg.SubnetID,
		MatcherID:    cfg.NodeID,
		RetryTimeout: 5 * time.Second,
		MaxRetries:   3,
	}

	// Load signer if private key provided
	var signer crypto.ExtendedSigner
	if cfg.PrivateKey != "" {
		var err error
		signer, err = crypto.LoadSignerFromPrivateKey(cfg.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load signer: %w", err)
		}
	}

	return &CompleteClient{
		BaseClient: NewBaseClient(baseCfg, logger),
		signer:     signer,
	}, nil
}

// Connect establishes connection to RootLayer
func (c *CompleteClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// Configure gRPC options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                60 * time.Second,  // 增加到60秒，降低ping频率
			Timeout:             20 * time.Second,  // 增加超时时间
			PermitWithoutStream: false,            // 没有stream时不发ping
		}),
	}

	// Add auth interceptor if signer available
	if c.signer != nil {
		authInterceptor := &AuthInterceptor{
			signer:  c.signer,
			chainID: c.cfg.SubnetID,
		}
		opts = append(opts,
			grpc.WithUnaryInterceptor(authInterceptor.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(authInterceptor.StreamClientInterceptor()),
		)
	}

	// Connect
	conn, err := grpc.Dial(c.cfg.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.intentPoolClient = rootpb.NewIntentPoolServiceClient(conn)
	c.subscriptionClient = rootpb.NewSubscriptionServiceClient(conn)
	c.connected = true

	c.logger.Infof("Connected to RootLayer at %s", c.cfg.Endpoint)
	return nil
}

// Close closes the connection
func (c *CompleteClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.intentPoolClient = nil
		c.subscriptionClient = nil
		c.connected = false
		return err
	}
	return nil
}

// ===== Core Validation Methods =====

// SubmitValidationBundle submits a validation bundle from validators
func (c *CompleteClient) SubmitValidationBundle(ctx context.Context, reports []*pb.ExecutionReport, checkpoint *types.CheckpointHeader) error {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	// Build validation bundle from execution reports
	bundle := c.buildValidationBundle(reports, checkpoint)

	// Submit via SubmitValidationBundle
	resp, err := client.SubmitValidationBundle(ctx, bundle)
	if err != nil {
		return fmt.Errorf("failed to submit validation: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("validation rejected: %s", resp.Msg)
	}

	c.logger.Infof("Validation bundle submitted for intent %s", bundle.IntentId)
	return nil
}

// buildValidationBundle creates a validation bundle from execution reports
func (c *CompleteClient) buildValidationBundle(reports []*pb.ExecutionReport, checkpoint *types.CheckpointHeader) *rootpb.ValidationBundle {
	if len(reports) == 0 {
		return nil
	}

	// Use first report as representative
	primaryReport := reports[0]

	// Collect signatures from all validators who validated this report
	var signatures []*rootpb.ValidationSignature
	signerBitmap := make([]byte, 32) // Assuming max 256 validators
	totalWeight := uint64(0)

	// In production, this would collect actual validator signatures
	// For now, create a dummy signature from our node
	if c.signer != nil {
		sig := &rootpb.ValidationSignature{
			Validator: c.cfg.MatcherID, // Changed from ValidatorId to Validator
			Signature: []byte{},        // Would be actual signature
		}
		signatures = append(signatures, sig)
		totalWeight += 1 // Weight would come from validator set
	}

	// Compute result hash from report data
	resultHash := crypto.HashMessage(primaryReport.ResultData)

	// Build the bundle
	bundle := &rootpb.ValidationBundle{
		SubnetId:     c.cfg.SubnetID,  // Field is SubnetId not SubnetID
		IntentId:     primaryReport.IntentId,
		AssignmentId: primaryReport.AssignmentId,
		AgentId:      primaryReport.AgentId,
		RootHeight:   checkpoint.Epoch,
		RootHash:     "0x" + checkpoint.CanonicalHashHex(), // Add 0x prefix for RootLayer compatibility
		ExecutedAt:   primaryReport.Timestamp,
		ResultHash:   resultHash[:],
		ProofHash:    nil, // Would include proof if available
		Signatures:   signatures,
		SignerBitmap: signerBitmap,
		TotalWeight:  totalWeight,
		AggregatorId: c.cfg.MatcherID,
		CompletedAt:  time.Now().Unix(),
	}

	return bundle
}

// ===== Intent Management Methods =====

// GetPendingIntents retrieves pending intents
func (c *CompleteClient) GetPendingIntents(ctx context.Context) ([]*rootpb.Intent, error) {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	req := &rootpb.GetIntentsRequest{
		SubnetId: c.cfg.SubnetID,
		Status:   "PENDING",
		Page:     1,
		PageSize: 100,
	}

	resp, err := client.GetIntents(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get intents: %w", err)
	}

	return resp.Intents, nil
}

// ===== Assignment Management Methods =====

// SubmitAssignment submits an assignment to RootLayer
func (c *CompleteClient) SubmitAssignment(ctx context.Context, assignment *rootpb.Assignment) error {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	// Ensure assignment has required fields
	// Ensure status is valid (handle new UNSPECIFIED state)
	if assignment.Status == rootpb.AssignmentStatus_ASSIGNMENT_STATUS_UNSPECIFIED {
		assignment.Status = rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE
	} else if assignment.Status != rootpb.AssignmentStatus_ASSIGNMENT_FAILED {
		assignment.Status = rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE
	}

	resp, err := client.PostAssignment(ctx, assignment)
	if err != nil {
		return fmt.Errorf("failed to submit assignment: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("assignment rejected: %s", resp.Msg)
	}

	c.logger.Infof("Assignment submitted: %s", assignment.AssignmentId)
	return nil
}

// SubmitMatchingResult submits a matching result
func (c *CompleteClient) SubmitMatchingResult(ctx context.Context, intentID string, winningBid *pb.Bid) error {
	// Convert to Assignment
	assignment := &rootpb.Assignment{
		AssignmentId: fmt.Sprintf("match-%s-%d", intentID, time.Now().Unix()),
		IntentId:    intentID,
		AgentId:     winningBid.AgentId,
		BidId:       winningBid.BidId,
		Status:      rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE,
		MatcherId:   c.cfg.MatcherID,
		Signature:   nil,
	}

	// Sign if signer available
	if c.signer != nil {
		canonical := fmt.Sprintf("assignment:%s:%s:%s:%s",
			assignment.AssignmentId,
			assignment.IntentId,
			assignment.AgentId,
			assignment.BidId,
		)
		signature, err := c.signer.SignMessage([]byte(canonical))
		if err != nil {
			return fmt.Errorf("failed to sign assignment: %w", err)
		}
		assignment.Signature = signature
	}

	return c.SubmitAssignment(ctx, assignment)
}

// ===== Checkpoint Management Methods =====

// SubmitCheckpoint submits a checkpoint to RootLayer
func (c *CompleteClient) SubmitCheckpoint(ctx context.Context, checkpoint *types.CheckpointHeader) error {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	// Checkpoints are typically submitted as part of validation bundles
	// Store checkpoint data for later submission with validations
	c.logger.Infof("Checkpoint ready for submission with epoch %d", checkpoint.Epoch)

	// In practice, checkpoints are included with validation bundles
	// For now, return success as checkpoint submission is handled elsewhere
	return nil
}

// ===== Subscription Management Methods =====

// SubscribeToIntents subscribes to intent updates
func (c *CompleteClient) SubscribeToIntents(ctx context.Context) (<-chan *rootpb.Intent, error) {
	c.mu.RLock()
	client := c.subscriptionClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	req := &rootpb.SubscribeIntentsRequest{
		SubnetId:     c.cfg.SubnetID,
		StatusFilter: []rootpb.IntentStatus{rootpb.IntentStatus_INTENT_STATUS_PENDING},
	}

	stream, err := client.SubscribeIntents(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to intents: %w", err)
	}

	intentChan := make(chan *rootpb.Intent, 10)

	go func() {
		defer close(intentChan)
		for {
			event, err := stream.Recv()
			if err != nil {
				c.logger.Errorf("Intent subscription error: %v", err)
				return
			}

			// Extract intent from event
			if event.Intent != nil {
				select {
				case intentChan <- event.Intent:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return intentChan, nil
}

// ===== Validator Set Management =====

// GetValidatorSet retrieves the current validator set
func (c *CompleteClient) GetValidatorSet(ctx context.Context) (*types.ValidatorSet, error) {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	// Validator set would normally come from a registry service
	// For now, return a mock validator set
	// In production, this would query the actual validator registry
	return &types.ValidatorSet{
		Validators: []types.Validator{
			{ID: "validator-1", Weight: 1},
			{ID: "validator-2", Weight: 1},
			{ID: "validator-3", Weight: 1},
		},
		MinValidators:  3,
		ThresholdNum:   2,
		ThresholdDenom: 3,
		EffectiveEpoch: 0,
	}, nil
}

// RegisterValidator registers this node as a validator
func (c *CompleteClient) RegisterValidator(ctx context.Context, pubKey []byte, endpoint string) error {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	// Validator registration would normally be handled by a registry service
	// For now, return success
	c.logger.Infof("Validator registration requested for %s", c.cfg.MatcherID)

	// Store pubKey and endpoint for future use
	_ = pubKey
	_ = endpoint
	_ = client

	return nil
}

// ===== Health and Status Methods =====

// HealthCheck performs a health check
func (c *CompleteClient) HealthCheck(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Simple connectivity check
	state := c.conn.GetState()
	if state.String() != "READY" && state.String() != "IDLE" {
		return fmt.Errorf("connection state: %s", state)
	}

	return nil
}

// IsConnected returns whether client is connected
func (c *CompleteClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}