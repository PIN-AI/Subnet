package matcher

import (
	"context"
	"testing"
	"time"

	"subnet/internal/logging"
	pb "subnet/proto/subnet"
	rootpb "subnet/proto/rootlayer"
)

func TestSubmitBidBatch(t *testing.T) {
	// Create test logger
	logger := logging.NewDefaultLogger()

	// Create test server
	server := &Server{
		logger:       logger,
		bidsByIntent: make(map[string][]*pb.Bid),
		intents:      make(map[string]*IntentWithWindow),
	}

	// Create a test intent
	intentID := "test-intent-1"
	server.intents[intentID] = &IntentWithWindow{
		Intent: &rootpb.Intent{
			IntentId: intentID,
			Status:   rootpb.IntentStatus_INTENT_STATUS_PENDING,
		},
		BiddingStartTime: time.Now().Add(-1 * time.Hour).Unix(),
		BiddingEndTime:   time.Now().Add(1 * time.Hour).Unix(),
		WindowClosed:     false,
	}

	ctx := context.Background()

	// Test 1: Empty batch
	t.Run("empty_batch", func(t *testing.T) {
		req := &pb.SubmitBidBatchRequest{
			Bids: []*pb.Bid{},
		}
		_, err := server.SubmitBidBatch(ctx, req)
		if err == nil {
			t.Error("Expected error for empty batch")
		}
	})

	// Test 2: Valid batch
	t.Run("valid_batch", func(t *testing.T) {
		req := &pb.SubmitBidBatchRequest{
			Bids: []*pb.Bid{
				{
					BidId:    "bid-1",
					IntentId: intentID,
					AgentId:  "agent-1",
					Price:    100,
				},
				{
					BidId:    "bid-2",
					IntentId: intentID,
					AgentId:  "agent-2",
					Price:    200,
				},
			},
			BatchId: "batch-1",
		}

		resp, err := server.SubmitBidBatch(ctx, req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(resp.Acks) != 2 {
			t.Errorf("Expected 2 acks, got %d", len(resp.Acks))
		}

		if resp.Success != 2 {
			t.Errorf("Expected success=2, got %d", resp.Success)
		}

		if resp.Failed != 0 {
			t.Errorf("Expected failed=0, got %d", resp.Failed)
		}

		// Verify bids were stored
		server.mu.RLock()
		bids := server.bidsByIntent[intentID]
		server.mu.RUnlock()

		if len(bids) != 2 {
			t.Errorf("Expected 2 bids stored, got %d", len(bids))
		}
	})


	// Test 5: Intent not found
	t.Run("intent_not_found", func(t *testing.T) {
		req := &pb.SubmitBidBatchRequest{
			Bids: []*pb.Bid{
				{
					BidId:    "bid-9",
					IntentId: "non-existent-intent",
					AgentId:  "agent-9",
					Price:    900,
				},
			},
			BatchId: "batch-4",
		}

		resp, err := server.SubmitBidBatch(ctx, req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// All bids should fail
		if resp.Success != 0 {
			t.Errorf("Expected success=0, got %d", resp.Success)
		}

		if resp.Failed != 1 {
			t.Errorf("Expected failed=1, got %d", resp.Failed)
		}
	})
}
