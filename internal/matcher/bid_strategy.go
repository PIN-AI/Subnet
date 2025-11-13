package matcher

import (
	"fmt"
	"sort"

	rootpb "subnet/proto/rootlayer"
	pb "subnet/proto/subnet"
)

// BidMatchingStrategy defines the interface for bid matching algorithms used in server.go
// This is separate from the legacy MatchingStrategy interface in matching_engine.go
// This strategy works with protobuf types (pb.Bid, rootpb.Intent)
type BidMatchingStrategy interface {
	// SelectWinner selects the winning bid and runner-ups from all available bids
	// Parameters:
	//   - intent: the intent being matched
	//   - bids: all submitted bids for this intent
	// Returns:
	//   - winner: the selected winning bid
	//   - runnerUps: list of runner-up bids (up to 2)
	//   - error: error if selection fails
	SelectWinner(intent *rootpb.Intent, bids []*pb.Bid) (*pb.Bid, []*pb.Bid, error)

	// Name returns the strategy name for logging and identification
	Name() string
}

// LowestPriceBidStrategy implements a simple lowest-price matching algorithm
// This is the default strategy that selects the bid with the lowest price
type LowestPriceBidStrategy struct{}

// NewLowestPriceBidStrategy creates a new lowest price strategy
func NewLowestPriceBidStrategy() *LowestPriceBidStrategy {
	return &LowestPriceBidStrategy{}
}

// Name returns the strategy name
func (s *LowestPriceBidStrategy) Name() string {
	return "lowest_price"
}

// SelectWinner selects the bid with the lowest price as winner
// Runner-ups are the next 2 lowest prices
func (s *LowestPriceBidStrategy) SelectWinner(intent *rootpb.Intent, bids []*pb.Bid) (*pb.Bid, []*pb.Bid, error) {
	if len(bids) == 0 {
		return nil, nil, fmt.Errorf("no bids available for selection")
	}

	// Create a copy to avoid modifying the original slice
	sortedBids := make([]*pb.Bid, len(bids))
	copy(sortedBids, bids)

	// Sort bids by price (ascending - lowest first)
	sort.Slice(sortedBids, func(i, j int) bool {
		return sortedBids[i].Price < sortedBids[j].Price
	})

	// Select winner (lowest price)
	winner := sortedBids[0]

	// Select runner-ups (next 2 lowest prices)
	runnerUps := []*pb.Bid{}
	for i := 1; i < len(sortedBids) && i < 3; i++ {
		runnerUps = append(runnerUps, sortedBids[i])
	}

	return winner, runnerUps, nil
}
