package matcher_test

import (
	"context"
	"net"
	"testing"
	"time"

	"subnet/internal/logging"
	"subnet/internal/matcher"
	pb "subnet/proto/subnet"
	rootpb "subnet/proto/rootlayer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

// TestProtoBasedFlow tests the matcher with actual proto messages
func TestProtoBasedFlow(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewDefaultLogger()

	// Create in-memory listener
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	// Create and start server with test config
	cfg := matcher.NewTestConfig("test-matcher", 5)
	server, err := matcher.NewServer(cfg, logger)
	require.NoError(t, err)

	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// Create gRPC server
	grpcServer := grpc.NewServer()
	server.RegisterGRPC(grpcServer)

	// Start gRPC server
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()
	defer grpcServer.Stop()

	// Create client connection
	conn, err := grpc.DialContext(ctx, "bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Create proto client
	client := pb.NewMatcherServiceClient(conn)

	t.Run("SubmitBid", func(t *testing.T) {
		// First add an intent to bid on
		intent := &rootpb.Intent{
			IntentId:   "intent-001",
			IntentType: "compute",
			Requester:  "user-1",
			SubnetId:   "subnet-1",
			Deadline:   time.Now().Add(10 * time.Minute).Unix(),
			Budget:     "1000",
		}
		err := server.AddIntent(intent)
		require.NoError(t, err)

		// Create a bid following proto structure
		bid := &pb.Bid{
			BidId:       "bid-001",
			IntentId:    "intent-001",
			AgentId:     "agent-001",
			Price:       100,
			Token:       "TEST",
			SubmittedAt: time.Now().Unix(),
			Nonce:       "nonce-123",
			Signature:   []byte("mock-signature"),
			Status:      pb.BidStatus_BID_STATUS_SUBMITTED,
		}

		req := &pb.SubmitBidRequest{
			Bid: bid,
		}

		// Submit bid
		resp, err := client.SubmitBid(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Ack)

		// Verify response
		assert.Equal(t, "bid-001", resp.Ack.BidId)
		assert.True(t, resp.Ack.Accepted)
		assert.Equal(t, pb.BidStatus_BID_STATUS_ACCEPTED, resp.Ack.Status)
	})

	t.Run("GetIntentSnapshot", func(t *testing.T) {
		// Get snapshot for the intent
		req := &pb.GetIntentSnapshotRequest{
			IntentId:      "intent-001",
			IncludeClosed: false,
		}

		resp, err := client.GetIntentSnapshot(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify snapshot
		if resp.Snapshot != nil {
			assert.Equal(t, "intent-001", resp.Snapshot.IntentId)
			// Note: bid might not be in snapshot yet due to timing
		}
	})

	t.Run("StreamIntents", func(t *testing.T) {
		// Start streaming intents
		req := &pb.StreamIntentsRequest{
			SubnetId:    "test-subnet",
			IntentTypes: []string{"compute", "storage"},
		}

		stream, err := client.StreamIntents(ctx, req)
		require.NoError(t, err)

		// Receive at least one update
		update, err := stream.Recv()
		require.NoError(t, err)
		require.NotNil(t, update)

		// Verify update structure
		assert.NotEmpty(t, update.IntentId)
		assert.NotEmpty(t, update.UpdateType)
		assert.Greater(t, update.Timestamp, int64(0))
	})

	t.Run("PublishMatchingResult", func(t *testing.T) {
		// Publish a matching result
		result := &pb.MatchingResult{
			MatcherId:       "matcher-001",
			IntentId:        "intent-001",
			WinningBidId:    "bid-001",
			WinningAgentId:  "agent-001",
			RunnerUpBidIds:  []string{"bid-002", "bid-003"},
			MatchedAt:       time.Now().Unix(),
			MatchingReason:  "Lowest price",
		}

		ack, err := client.PublishMatchingResult(ctx, result)
		require.NoError(t, err)
		require.NotNil(t, ack)

		// Verify acknowledgment
		assert.Equal(t, "bid-001", ack.BidId)
		assert.True(t, ack.Accepted)
		assert.Equal(t, pb.BidStatus_BID_STATUS_WINNER, ack.Status)
	})
}