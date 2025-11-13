package matcher

import (
	"testing"

	pb "subnet/proto/subnet"
)

// TestLowestPriceBidStrategy tests the lowest price matching strategy
func TestLowestPriceBidStrategy(t *testing.T) {
	strategy := NewLowestPriceBidStrategy()

	tests := []struct {
		name           string
		bids           []*pb.Bid
		expectWinnerID string
		expectRunnerUp int
		expectError    bool
	}{
		{
			name: "single bid",
			bids: []*pb.Bid{
				{BidId: "bid1", AgentId: "agent1", Price: 100},
			},
			expectWinnerID: "bid1",
			expectRunnerUp: 0,
			expectError:    false,
		},
		{
			name: "three bids - lowest wins",
			bids: []*pb.Bid{
				{BidId: "bid1", AgentId: "agent1", Price: 100},
				{BidId: "bid2", AgentId: "agent2", Price: 50}, // Winner
				{BidId: "bid3", AgentId: "agent3", Price: 75},
			},
			expectWinnerID: "bid2",
			expectRunnerUp: 2,
			expectError:    false,
		},
		{
			name: "five bids - correct runner-ups",
			bids: []*pb.Bid{
				{BidId: "bid1", AgentId: "agent1", Price: 100},
				{BidId: "bid2", AgentId: "agent2", Price: 50}, // Winner
				{BidId: "bid3", AgentId: "agent3", Price: 75}, // Runner-up 1
				{BidId: "bid4", AgentId: "agent4", Price: 80}, // Runner-up 2
				{BidId: "bid5", AgentId: "agent5", Price: 90},
			},
			expectWinnerID: "bid2",
			expectRunnerUp: 2, // Only 2 runner-ups max
			expectError:    false,
		},
		{
			name: "two bids - one runner-up",
			bids: []*pb.Bid{
				{BidId: "bid1", AgentId: "agent1", Price: 100},
				{BidId: "bid2", AgentId: "agent2", Price: 50}, // Winner
			},
			expectWinnerID: "bid2",
			expectRunnerUp: 1,
			expectError:    false,
		},
		{
			name:           "no bids - error",
			bids:           []*pb.Bid{},
			expectWinnerID: "",
			expectRunnerUp: 0,
			expectError:    true,
		},
		{
			name: "equal prices - first in slice wins",
			bids: []*pb.Bid{
				{BidId: "bid1", AgentId: "agent1", Price: 50},
				{BidId: "bid2", AgentId: "agent2", Price: 50},
				{BidId: "bid3", AgentId: "agent3", Price: 50},
			},
			expectWinnerID: "bid1", // First one wins in stable sort
			expectRunnerUp: 2,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			winner, runnerUps, err := strategy.SelectWinner(nil, tt.bids)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check winner
			if winner.BidId != tt.expectWinnerID {
				t.Errorf("expected winner %s, got %s", tt.expectWinnerID, winner.BidId)
			}

			// Check runner-ups count
			if len(runnerUps) != tt.expectRunnerUp {
				t.Errorf("expected %d runner-ups, got %d", tt.expectRunnerUp, len(runnerUps))
			}

			// Verify winner has lowest price
			for _, bid := range tt.bids {
				if bid.BidId != winner.BidId && bid.Price < winner.Price {
					t.Errorf("winner price %d is not lowest, found bid %s with price %d",
						winner.Price, bid.BidId, bid.Price)
				}
			}

			// Verify runner-ups are sorted by price
			for i := 1; i < len(runnerUps); i++ {
				if runnerUps[i-1].Price > runnerUps[i].Price {
					t.Errorf("runner-ups not sorted: position %d has price %d, position %d has price %d",
						i-1, runnerUps[i-1].Price, i, runnerUps[i].Price)
				}
			}
		})
	}
}

// TestLowestPriceBidStrategyName tests the strategy name
func TestLowestPriceBidStrategyName(t *testing.T) {
	strategy := NewLowestPriceBidStrategy()
	if strategy.Name() != "lowest_price" {
		t.Errorf("expected strategy name 'lowest_price', got '%s'", strategy.Name())
	}
}

// TestLowestPriceBidStrategyDoesNotModifyInput tests that the strategy doesn't modify the input slice
func TestLowestPriceBidStrategyDoesNotModifyInput(t *testing.T) {
	strategy := NewLowestPriceBidStrategy()

	bids := []*pb.Bid{
		{BidId: "bid1", AgentId: "agent1", Price: 100},
		{BidId: "bid2", AgentId: "agent2", Price: 50},
		{BidId: "bid3", AgentId: "agent3", Price: 75},
	}

	// Keep original order for comparison
	originalOrder := []string{"bid1", "bid2", "bid3"}

	_, _, err := strategy.SelectWinner(nil, bids)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify original slice is not modified
	for i, bid := range bids {
		if bid.BidId != originalOrder[i] {
			t.Errorf("input slice was modified: position %d expected %s, got %s",
				i, originalOrder[i], bid.BidId)
		}
	}
}

// BenchmarkLowestPriceBidStrategy benchmarks the strategy performance
func BenchmarkLowestPriceBidStrategy(b *testing.B) {
	strategy := NewLowestPriceBidStrategy()

	// Create 100 bids
	bids := make([]*pb.Bid, 100)
	for i := 0; i < 100; i++ {
		bids[i] = &pb.Bid{
			BidId:   string(rune(i)),
			AgentId: string(rune(i)),
			Price:   uint64(100 - i), // Descending prices
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = strategy.SelectWinner(nil, bids)
	}
}
