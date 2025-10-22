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
	"subnet/internal/logging"
)

// GRPCClient implements the Client interface using gRPC
type GRPCClient struct {
	*BaseClient
	conn   *grpc.ClientConn
	client rootpb.IntentPoolServiceClient
	mu     sync.RWMutex
}

// NewGRPCClient creates a new gRPC client for RootLayer
func NewGRPCClient(cfg *Config, logger logging.Logger) *GRPCClient {
	return &GRPCClient{
		BaseClient: NewBaseClient(cfg, logger),
	}
}

// Connect establishes connection to RootLayer
func (c *GRPCClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// Configure gRPC connection options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()), // TODO: Add TLS
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                60 * time.Second,  // 增加到60秒，降低ping频率
			Timeout:             20 * time.Second,  // 增加超时时间
			PermitWithoutStream: false,            // 没有stream时不发ping
		}),
	}

	// Establish connection
	conn, err := grpc.Dial(c.cfg.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to RootLayer: %w", err)
	}

	c.conn = conn
	c.client = rootpb.NewIntentPoolServiceClient(conn)
	c.connected = true

	c.logger.Infof("Connected to RootLayer at %s", c.cfg.Endpoint)
	return nil
}

// Close closes the connection to RootLayer
func (c *GRPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.client = nil
		c.connected = false
		return err
	}
	return nil
}

// GetPendingIntents retrieves pending intents from RootLayer
func (c *GRPCClient) GetPendingIntents(ctx context.Context, subnetID string) ([]*rootpb.Intent, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
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

// StreamIntents streams intents from RootLayer
func (c *GRPCClient) StreamIntents(ctx context.Context, subnetID string) (<-chan *rootpb.Intent, error) {
	// Note: This method should use SubscriptionService for streaming
	// For now, we'll use GetIntents with pagination as a fallback
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to RootLayer")
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

	// Return intents as a channel
	intentChan := make(chan *rootpb.Intent, len(resp.Intents))
	go func() {
		defer close(intentChan)
		for _, intent := range resp.Intents {
			select {
			case intentChan <- intent:
			case <-ctx.Done():
				return
			}
		}
	}()

	return intentChan, nil
}

// SubmitAssignment submits an assignment to RootLayer
func (c *GRPCClient) SubmitAssignment(ctx context.Context, assignment *rootpb.Assignment) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	ack, err := client.PostAssignment(ctx, assignment)
	if err != nil {
		return fmt.Errorf("failed to submit assignment: %w", err)
	}

	if !ack.Ok {
		return fmt.Errorf("assignment rejected: %s", ack.Msg)
	}

	c.logger.Infof("Assignment %s submitted (tx: %s)", assignment.AssignmentId, ack.TxHash)
	return nil
}

// GetAssignment retrieves an assignment
func (c *GRPCClient) GetAssignment(ctx context.Context, assignmentID string) (*rootpb.Assignment, error) {
	// Note: The proto doesn't have a direct GetAssignment method
	// This is a placeholder implementation
	// In production, this should be implemented on the RootLayer side

	c.mu.RLock()
	client := c.client
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

// SubmitMatchingResult submits a matching result
func (c *GRPCClient) SubmitMatchingResult(ctx context.Context, result *MatchingResult) error {
	// Convert to Assignment for submission
	assignment := &rootpb.Assignment{
		AssignmentId: fmt.Sprintf("match-%s-%d", result.IntentID, result.Timestamp),
		IntentId:     result.IntentID,
		AgentId:      result.WinningAgentID,
		BidId:        result.WinningBidID,
		Status:       rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE,
		MatcherId:    result.MatcherID,
		Signature:    result.Signature,
	}

	return c.SubmitAssignment(ctx, assignment)
}

// SubmitValidationBundle submits a validation bundle from validators
func (c *GRPCClient) SubmitValidationBundle(ctx context.Context, bundle *rootpb.ValidationBundle) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to RootLayer")
	}

	ack, err := client.SubmitValidationBundle(ctx, bundle)
	if err != nil {
		return fmt.Errorf("failed to submit validation bundle: %w", err)
	}

	if !ack.Ok {
		return fmt.Errorf("validation bundle rejected: %s", ack.Msg)
	}

	c.logger.Infof("Validation bundle submitted (receipt: %s)",
		ack.ReceiptId)
	return nil
}