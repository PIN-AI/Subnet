package matcher

import (
	"context"
	"fmt"
	"time"

	"subnet/internal/logging"
	pb "subnet/proto/subnet"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCClient implements a gRPC client for the Matcher service following proto exactly
type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.MatcherServiceClient
	logger logging.Logger
}

// NewGRPCClient creates a new Matcher gRPC client
func NewGRPCClient(endpoint string, logger logging.Logger) (*GRPCClient, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	// Create gRPC connection
	conn, err := grpc.Dial(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to matcher at %s: %w", endpoint, err)
	}

	client := pb.NewMatcherServiceClient(conn)

	return &GRPCClient{
		conn:   conn,
		client: client,
		logger: logger,
	}, nil
}

// Close closes the client connection
func (c *GRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SubmitBid submits a bid to the matcher
func (c *GRPCClient) SubmitBid(ctx context.Context, bid *pb.Bid) (*pb.BidSubmissionAck, error) {
	if bid == nil {
		return nil, fmt.Errorf("bid is nil")
	}

	req := &pb.SubmitBidRequest{
		Bid: bid,
	}

	resp, err := c.client.SubmitBid(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit bid: %w", err)
	}

	return resp.Ack, nil
}

// GetIntentSnapshot gets a snapshot of bids for an intent
func (c *GRPCClient) GetIntentSnapshot(ctx context.Context, intentID string, includeClosed bool) (*pb.IntentBidSnapshot, error) {
	req := &pb.GetIntentSnapshotRequest{
		IntentId:      intentID,
		IncludeClosed: includeClosed,
	}

	resp, err := c.client.GetIntentSnapshot(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get intent snapshot: %w", err)
	}

	return resp.Snapshot, nil
}

// StreamIntents subscribes to intent updates and returns a channel
func (c *GRPCClient) StreamIntents(ctx context.Context, agentID string, capabilities []string) (<-chan *pb.MatcherIntentUpdate, error) {
	req := &pb.StreamIntentsRequest{
		SubnetId:    agentID, // Using agentID as subnetID for now
		IntentTypes: capabilities,
	}

	stream, err := c.client.StreamIntents(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to start intent stream: %w", err)
	}

	// Create channel to return updates
	updatesChan := make(chan *pb.MatcherIntentUpdate, 100)

	// Start goroutine to read from stream
	go func() {
		defer close(updatesChan)
		for {
			update, err := stream.Recv()
			if err != nil {
				c.logger.Errorf("Error receiving intent update: %v", err)
				return
			}
			select {
			case updatesChan <- update:
			case <-ctx.Done():
				return
			}
		}
	}()

	return updatesChan, nil
}

// StreamBids subscribes to bid events
func (c *GRPCClient) StreamBids(ctx context.Context, intentID, agentID string) (pb.MatcherService_StreamBidsClient, error) {
	req := &pb.StreamBidsRequest{
		IntentId: intentID,
		AgentId:  agentID,
	}

	stream, err := c.client.StreamBids(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to start bid stream: %w", err)
	}

	return stream, nil
}

// PublishMatchingResult publishes a matching result to observers
func (c *GRPCClient) PublishMatchingResult(ctx context.Context, result *pb.MatchingResult) (*pb.BidSubmissionAck, error) {
	if result == nil {
		return nil, fmt.Errorf("result is nil")
	}

	return c.client.PublishMatchingResult(ctx, result)
}

// StreamTasks subscribes to execution tasks for a specific agent
func (c *GRPCClient) StreamTasks(ctx context.Context, agentID string) (<-chan *pb.ExecutionTask, error) {
	req := &pb.StreamTasksRequest{
		AgentId: agentID,
	}

	stream, err := c.client.StreamTasks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to start task stream: %w", err)
	}

	// Create channel to return tasks
	tasksChan := make(chan *pb.ExecutionTask, 10)

	// Start goroutine to read from stream
	go func() {
		defer close(tasksChan)
		for {
			task, err := stream.Recv()
			if err != nil {
				c.logger.Errorf("Error receiving task: %v", err)
				return
			}
			select {
			case tasksChan <- task:
			case <-ctx.Done():
				return
			}
		}
	}()

	return tasksChan, nil
}

// RespondToTask sends agent's response to a task assignment
func (c *GRPCClient) RespondToTask(ctx context.Context, response *pb.TaskResponse) error {
	if response == nil {
		return fmt.Errorf("response is nil")
	}

	req := &pb.RespondToTaskRequest{
		Response: response,
	}

	resp, err := c.client.RespondToTask(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to respond to task: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("task response failed: %s", resp.Message)
	}

	return nil
}