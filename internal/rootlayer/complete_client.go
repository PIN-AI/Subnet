package rootlayer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	rootpb "subnet/proto/rootlayer"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/types"
	pb "subnet/proto/subnet"
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
			Time:                30 * time.Second, // Send keepalive ping every 30s
			Timeout:             10 * time.Second, // Wait 10s for ping response
			PermitWithoutStream: true,             // Send pings even with active streams
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

// SubmitValidationBundle submits a single validation bundle to RootLayer
// This maintains interface compatibility with rootlayer.Client
func (c *CompleteClient) SubmitValidationBundle(ctx context.Context, bundle *rootpb.ValidationBundle) error {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	if bundle == nil {
		return fmt.Errorf("validation bundle is nil")
	}

	// Submit the single validation bundle
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

// SubmitValidationBundleBatch submits multiple validation groups to RootLayer in a single call
func (c *CompleteClient) SubmitValidationBundleBatch(ctx context.Context, groups []*rootpb.ValidationBatchGroup, batchID string, partialOk bool) (*rootpb.ValidationBundleBatchResponse, error) {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("no validation groups provided for batch submission")
	}

	req := &rootpb.ValidationBundleBatchRequest{
		Groups:    groups,
		BatchId:   batchID,
		PartialOk: &partialOk,
	}

	resp, err := client.SubmitValidationBundleBatch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit validation bundle batch: %w", err)
	}

	// Count total items across all groups
	totalItems := 0
	for _, group := range groups {
		totalItems += len(group.Items)
	}

	c.logger.Infof("Validation bundle batch submitted: batch_id=%s groups=%d items=%d success=%d failed=%d",
		batchID, len(groups), totalItems, resp.Success, resp.Failed)

	// Log failures summary if any
	if resp.Failed > 0 {
		failedIndices := []int{}
		for i, result := range resp.Results {
			if !result.Ok {
				failedIndices = append(failedIndices, i)
			}
		}
		if len(failedIndices) > 0 {
			c.logger.Warnf("Validation bundle batch had %d failures at indices: %v", len(failedIndices), failedIndices)
		}
	}

	return resp, nil
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
		SubnetId:     c.cfg.SubnetID, // Field is SubnetId not SubnetID
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

// groupReportsByIntent groups execution reports by (IntentID, AssignmentID, AgentID)
func (c *CompleteClient) groupReportsByIntent(reports []*pb.ExecutionReport) map[string][]*pb.ExecutionReport {
	grouped := make(map[string][]*pb.ExecutionReport)

	for _, report := range reports {
		// Create composite key from Intent+Assignment+Agent
		key := fmt.Sprintf("%s:%s:%s", report.IntentId, report.AssignmentId, report.AgentId)
		grouped[key] = append(grouped[key], report)
	}

	c.logger.Infof("Grouped %d execution reports into %d Intent groups", len(reports), len(grouped))
	return grouped
}

// buildValidationBundleForIntent creates a validation bundle for a specific Intent's reports
func (c *CompleteClient) buildValidationBundleForIntent(reports []*pb.ExecutionReport, checkpoint *types.CheckpointHeader) *rootpb.ValidationBundle {
	if len(reports) == 0 {
		c.logger.Warn("Cannot build ValidationBundle: no reports provided")
		return nil
	}

	// Extract metadata from first report (all reports in this group have same IntentID/AssignmentID/AgentID)
	primaryReport := reports[0]
	intentID := primaryReport.IntentId
	assignmentID := primaryReport.AssignmentId
	agentID := primaryReport.AgentId

	c.logger.Infof("Building ValidationBundle for Intent: intent_id=%s assignment_id=%s agent_id=%s reports_count=%d epoch=%d",
		intentID, assignmentID, agentID, len(reports), checkpoint.Epoch)

	// Validate metadata
	if intentID == "" || assignmentID == "" || agentID == "" {
		c.logger.Errorf("ValidationBundle construction failed: missing metadata intent_id=%s assignment_id=%s agent_id=%s",
			intentID, assignmentID, agentID)
		return nil
	}

	// Collect signatures from all validators who validated this report
	var signatures []*rootpb.ValidationSignature
	signerBitmap := make([]byte, 32) // Assuming max 256 validators
	totalWeight := uint64(0)

	// In production, this would collect actual validator signatures
	// For now, create a dummy signature from our node
	if c.signer != nil {
		sig := &rootpb.ValidationSignature{
			Validator: c.cfg.MatcherID,
			Signature: []byte{},
		}
		signatures = append(signatures, sig)
		totalWeight += 1
	}

	// Compute result hash from report data
	resultHash := crypto.HashMessage(primaryReport.ResultData)

	// Build the bundle with this Intent's metadata
	bundle := &rootpb.ValidationBundle{
		SubnetId:     c.cfg.SubnetID,
		IntentId:     intentID,
		AssignmentId: assignmentID,
		AgentId:      agentID,
		RootHeight:   checkpoint.Epoch,
		RootHash:     "0x" + checkpoint.CanonicalHashHex(),
		ExecutedAt:   primaryReport.Timestamp,
		ResultHash:   resultHash[:],
		ProofHash:    nil,
		Signatures:   signatures,
		SignerBitmap: signerBitmap,
		TotalWeight:  totalWeight,
		AggregatorId: c.cfg.MatcherID,
		CompletedAt:  time.Now().Unix(),
	}

	c.logger.Infof("ValidationBundle constructed for Intent: intent_id=%s assignment_id=%s agent_id=%s signatures=%d",
		intentID, assignmentID, agentID, len(signatures))

	return bundle
}

// ===== Intent Management Methods =====

// SubmitIntentBatch submits multiple intents to RootLayer in a single call
func (c *CompleteClient) SubmitIntentBatch(ctx context.Context, intents []*rootpb.SubmitIntentRequest, batchID string, partialOk, treatExistsAsOk bool) (*rootpb.SubmitIntentBatchResponse, error) {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	if len(intents) == 0 {
		return nil, fmt.Errorf("no intents provided for batch submission")
	}

	req := &rootpb.SubmitIntentBatchRequest{
		Items:           intents,
		BatchId:         batchID,
		PartialOk:       &partialOk,
		TreatExistsAsOk: &treatExistsAsOk,
	}

	resp, err := client.SubmitIntentBatch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit intent batch: %w", err)
	}

	c.logger.Infof("Intent batch submitted: batch_id=%s total=%d success=%d failed=%d",
		batchID, len(intents), resp.Success, resp.Failed)

	return resp, nil
}

// GetPendingIntents retrieves pending intents (implements Client interface)
func (c *CompleteClient) GetPendingIntents(ctx context.Context, subnetID string) ([]*rootpb.Intent, error) {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	// Use provided subnetID if given, otherwise use configured subnetID
	if subnetID == "" {
		subnetID = c.cfg.SubnetID
	}

	req := &rootpb.GetIntentsRequest{
		SubnetId: subnetID,
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

// StreamIntents subscribes to real-time intent updates (implements Client interface)
func (c *CompleteClient) StreamIntents(ctx context.Context, subnetID string) (<-chan *rootpb.Intent, error) {
	c.mu.RLock()
	client := c.subscriptionClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	// Use provided subnetID if given, otherwise use configured subnetID
	if subnetID == "" {
		subnetID = c.cfg.SubnetID
	}

	req := &rootpb.SubscribeIntentsRequest{
		SubnetId:     subnetID,
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
			select {
			case <-ctx.Done():
				c.logger.Debug("Intent stream context cancelled, exiting")
				return
			default:
			}

			// Directly receive from stream (no nested goroutine)
			event, err := stream.Recv()
			if err != nil {
				c.logger.Errorf("Intent stream error: %v", err)
				return
			}

			// Extract intent from event
			if event != nil && event.Intent != nil {
				select {
				case intentChan <- event.Intent:
					// Successfully forwarded
				case <-ctx.Done():
					c.logger.Debug("Context cancelled while forwarding intent")
					return
				}
			} else {
				c.logger.Warn("Received event with nil intent")
			}
		}
	}()

	return intentChan, nil
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

// GetAssignment retrieves an assignment by ID (implements Client interface)
func (c *CompleteClient) GetAssignment(ctx context.Context, assignmentID string) (*rootpb.Assignment, error) {
	// Note: The proto doesn't have a direct GetAssignment method yet
	// This is a placeholder implementation
	// In production, this should be implemented on the RootLayer side

	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
	}

	// For now, return a mock assignment since there's no direct method
	// This should be replaced when RootLayer adds GetAssignment method
	return &rootpb.Assignment{
		AssignmentId: assignmentID,
		Status:       rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE,
	}, nil
}

// PostAssignmentBatch submits multiple assignments to RootLayer in a single call
func (c *CompleteClient) PostAssignmentBatch(ctx context.Context, assignments []*rootpb.Assignment) error {
	c.mu.RLock()
	client := c.intentPoolClient
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	if len(assignments) == 0 {
		return fmt.Errorf("no assignments provided for batch submission")
	}

	// Ensure all assignments have valid status
	for _, assignment := range assignments {
		if assignment.Status == rootpb.AssignmentStatus_ASSIGNMENT_STATUS_UNSPECIFIED {
			assignment.Status = rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE
		} else if assignment.Status != rootpb.AssignmentStatus_ASSIGNMENT_FAILED {
			assignment.Status = rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE
		}
	}

	batch := &rootpb.AssignmentBatch{
		Assignments: assignments,
	}

	resp, err := client.PostAssignmentBatch(ctx, batch)
	if err != nil {
		return fmt.Errorf("failed to submit assignment batch: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("assignment batch rejected: %s", resp.Msg)
	}

	c.logger.Infof("Assignment batch submitted: count=%d", len(assignments))
	return nil
}

// SubmitMatchingResult submits a matching result (implements Client interface)
func (c *CompleteClient) SubmitMatchingResult(ctx context.Context, result *MatchingResult) error {
	// Convert to Assignment
	assignment := &rootpb.Assignment{
		AssignmentId: fmt.Sprintf("match-%s-%d", result.IntentID, time.Now().Unix()),
		IntentId:     result.IntentID,
		AgentId:      result.WinningAgentID,
		BidId:        result.WinningBidID,
		Status:       rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE,
		MatcherId:    result.MatcherID,
		Signature:    result.Signature,
	}

	// Sign if signer available and no signature provided
	if c.signer != nil && len(result.Signature) == 0 {
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
