package matcher_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"subnet/internal/logging"
	"subnet/internal/matcher"
	rootpb "rootlayer/proto"
	pb "subnet/proto/subnet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

// TestBiddingWindowAndMatching tests the complete flow with bidding windows and matching
func TestBiddingWindowAndMatching(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewDefaultLogger()

	// Create in-memory listener
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	// Create matcher with test config including private key
	cfg := matcher.NewTestConfig("test-matcher", 3) // 3 seconds for testing

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

	t.Run("BiddingWindowFlow", func(t *testing.T) {
		// Add an intent to the matcher (simulating RootLayer push)
		intent := &rootpb.Intent{
			IntentId:    "test-intent-1",
			IntentType:  "compute",
			Requester:   "user-1",
			SubnetId:    "subnet-1",
		}

		err := server.AddIntent(intent)
		require.NoError(t, err)

		// Submit multiple bids from different agents
		agents := []struct {
			id    string
			price uint64
		}{
			{"agent-1", 150},
			{"agent-2", 100}, // Winner (lowest price)
			{"agent-3", 200},
			{"agent-4", 120},
		}

		for _, agent := range agents {
			bid := &pb.Bid{
				BidId:       fmt.Sprintf("bid-%s", agent.id),
				IntentId:    intent.IntentId,
				AgentId:     agent.id,
				Price:       agent.price,
				Token:       "PIN",
				SubmittedAt: time.Now().Unix(),
			}

			req := &pb.SubmitBidRequest{Bid: bid}
			resp, err := client.SubmitBid(ctx, req)
			require.NoError(t, err)
			require.NotNil(t, resp.Ack)
			assert.True(t, resp.Ack.Accepted)
		}

		// Get snapshot before window closes
		snapshotReq := &pb.GetIntentSnapshotRequest{
			IntentId:      intent.IntentId,
			IncludeClosed: false,
		}

		resp, err := client.GetIntentSnapshot(ctx, snapshotReq)
		require.NoError(t, err)
		require.NotNil(t, resp.Snapshot)
		assert.Equal(t, intent.IntentId, resp.Snapshot.IntentId)
		assert.Len(t, resp.Snapshot.Bids, 4)
		assert.False(t, resp.Snapshot.BiddingClosed)

		// Wait for bidding window to close
		t.Logf("Waiting for bidding window to close (3 seconds)...")
		time.Sleep(4 * time.Second)

		// Check that matching occurred
		result := server.GetMatchingResult(intent.IntentId)
		require.NotNil(t, result)
		assert.Equal(t, "agent-2", result.WinningAgentId) // Lowest price wins
		assert.Equal(t, "bid-agent-2", result.WinningBidId)
		assert.Contains(t, result.RunnerUpBidIds, "bid-agent-4")
		assert.Contains(t, result.RunnerUpBidIds, "bid-agent-1")

		// Check assignment was created
		assignment := server.GetAssignment(intent.IntentId)
		require.NotNil(t, assignment)
		assert.Equal(t, intent.IntentId, assignment.IntentId)
		assert.Equal(t, "agent-2", assignment.AgentId)
		assert.Equal(t, rootpb.AssignmentStatus_ASSIGNMENT_ACTIVE, assignment.Status)

		// Verify snapshot shows closed window
		resp, err = client.GetIntentSnapshot(ctx, snapshotReq)
		require.NoError(t, err)
		// Window is closed, so no snapshot unless IncludeClosed is true
		assert.Nil(t, resp.Snapshot)

		// Get with IncludeClosed
		snapshotReq.IncludeClosed = true
		resp, err = client.GetIntentSnapshot(ctx, snapshotReq)
		require.NoError(t, err)
		require.NotNil(t, resp.Snapshot)
		assert.True(t, resp.Snapshot.BiddingClosed)
	})

	t.Run("LateBidRejection", func(t *testing.T) {
		// Try to submit bid after window closed
		lateBid := &pb.Bid{
			BidId:       "late-bid",
			IntentId:    "test-intent-1",
			AgentId:     "late-agent",
			Price:       50,
			Token:       "PIN",
			SubmittedAt: time.Now().Unix(),
		}

		req := &pb.SubmitBidRequest{Bid: lateBid}
		resp, err := client.SubmitBid(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp.Ack)
		assert.False(t, resp.Ack.Accepted)
		assert.Equal(t, "Bidding window closed", resp.Ack.Reason)
	})
}

// TestIntentStreaming tests the intent streaming functionality
func TestIntentStreaming(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := logging.NewDefaultLogger()

	// Create in-memory listener
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	cfg := matcher.NewTestConfig("test-matcher", 2)

	server, err := matcher.NewServer(cfg, logger)
	require.NoError(t, err)

	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// Create gRPC server
	grpcServer := grpc.NewServer()
	server.RegisterGRPC(grpcServer)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()
	defer grpcServer.Stop()

	// Create client
	conn, err := grpc.DialContext(ctx, "bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewMatcherServiceClient(conn)

	// Start streaming
	streamReq := &pb.StreamIntentsRequest{
		SubnetId:    "test-subnet",
		IntentTypes: []string{"compute"},
	}

	stream, err := client.StreamIntents(ctx, streamReq)
	require.NoError(t, err)

	// Collect updates
	updates := make(chan *pb.MatcherIntentUpdate, 10)
	go func() {
		for {
			update, err := stream.Recv()
			if err != nil {
				return
			}
			updates <- update
		}
	}()

	// First, drain any existing intents from MockClient
	// MockClient may generate intents automatically
	drainTimeout := time.After(100 * time.Millisecond)
DrainLoop:
	for {
		select {
		case <-updates:
			// Drain any existing updates
			continue
		case <-drainTimeout:
			break DrainLoop
		}
	}

	// Add intent and observe updates
	intent := &rootpb.Intent{
		IntentId:   "stream-intent-1",
		IntentType: "compute",
		Requester:  "user-1",
		SubnetId:   "subnet-1",
	}
	err = server.AddIntent(intent)
	require.NoError(t, err)

	// Should receive NEW_INTENT update
	select {
	case update := <-updates:
		assert.Equal(t, "stream-intent-1", update.IntentId)
		assert.Equal(t, "NEW_INTENT", update.UpdateType)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for NEW_INTENT update")
	}

	// Submit a bid
	bid := &pb.Bid{
		BidId:    "stream-bid-1",
		IntentId: intent.IntentId,
		AgentId:  "stream-agent-1",
		Price:    100,
	}
	_, err = client.SubmitBid(ctx, &pb.SubmitBidRequest{Bid: bid})
	require.NoError(t, err)

	// Wait for window to close
	time.Sleep(3 * time.Second)

	// Should receive WINDOW_CLOSED and MATCHED updates
	var windowClosed, matched bool
	timeout := time.After(2 * time.Second)
	for !windowClosed || !matched {
		select {
		case update := <-updates:
			if update.UpdateType == "WINDOW_CLOSED" {
				windowClosed = true
			} else if update.UpdateType == "MATCHED" {
				matched = true
			}
		case <-timeout:
			t.Fatal("Timeout waiting for updates")
		}
	}

	assert.True(t, windowClosed)
	assert.True(t, matched)
}