package matcher

import (
	"fmt"
	"sort"

	"subnet/internal/logging"
	"subnet/internal/types"
)

// MatchingStrategy defines the interface for matching algorithms
type MatchingStrategy interface {
	Match(snapshot *IntentBidSnapshot) *types.MatchingResult
}

// MatchingEngine handles the matching of bids to intents
type MatchingEngine struct {
	cfg      *Config
	logger   logging.Logger
	strategy MatchingStrategy
}

// NewMatchingEngine creates a new matching engine
func NewMatchingEngine(cfg *Config, logger logging.Logger) (*MatchingEngine, error) {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	engine := &MatchingEngine{
		cfg:    cfg,
		logger: logger,
	}

	// Select strategy based on configuration
	// For MVP, use weighted scoring strategy
	matcherID := cfg.MatcherID
	if matcherID == "" && cfg.Identity != nil {
		matcherID = cfg.Identity.MatcherID
	}
	if matcherID == "" {
		// MatcherID is critical for identifying the source of matching decisions
		// Using a default would cause confusion and potential security issues
		return nil, fmt.Errorf("matcher_id must be configured - cannot use default ID")
	}

	engine.strategy = &WeightedScoringStrategy{
		priceWeight:      0.5,
		reputationWeight: 0.3,
		capacityWeight:   0.2,
		matcherID:        matcherID,
		logger:           logger,
	}

	return engine, nil
}

// Match runs the matching algorithm on a bid snapshot
func (e *MatchingEngine) Match(snapshot *IntentBidSnapshot) *types.MatchingResult {
	if snapshot == nil || len(snapshot.Bids) == 0 {
		e.logger.Warnf("No bids to match for intent %s", snapshot.IntentID)
		return nil
	}

	result := e.strategy.Match(snapshot)
	if result != nil {
		e.logger.Infof("Matched intent %s to agent %s with bid %s",
			snapshot.IntentID, result.WinningAgentID, result.WinningBid.BidID)
	}

	return result
}

// WeightedScoringStrategy implements a weighted scoring matching algorithm
type WeightedScoringStrategy struct {
	priceWeight      float64
	reputationWeight float64
	capacityWeight   float64
	matcherID        string
	logger           logging.Logger
}

// Match implements the weighted scoring algorithm
func (s *WeightedScoringStrategy) Match(snapshot *IntentBidSnapshot) *types.MatchingResult {
	if len(snapshot.Bids) == 0 {
		return nil
	}

	// Sort bids by score
	scoredBids := s.scoreBids(snapshot.Bids, snapshot.Intent)
	sort.Slice(scoredBids, func(i, j int) bool {
		return scoredBids[i].score > scoredBids[j].score
	})

	// Select winner and runner-ups
	winner := scoredBids[0]
	runnerUps := make([]*types.Bid, 0)

	// Keep up to 2 runner-ups for failover
	for i := 1; i < len(scoredBids) && i < 3; i++ {
		runnerUps = append(runnerUps, scoredBids[i].bid)
	}

	result := &types.MatchingResult{
		IntentID:       snapshot.IntentID,
		WinningBid:     winner.bid,
		WinningAgentID: winner.bid.AgentID,
		RunnerUpBids:   runnerUps,
		MatchedAt:      snapshot.BiddingEndTime,
		MatcherID:      s.matcherID,
	}

	s.logger.Infof("Matched intent %s: winner=%s (score=%.2f), %d runner-ups",
		snapshot.IntentID, winner.bid.AgentID, winner.score, len(runnerUps))

	return result
}

// scoredBid holds a bid with its calculated score
type scoredBid struct {
	bid   *types.Bid
	score float64
}

// scoreBids calculates scores for all bids
func (s *WeightedScoringStrategy) scoreBids(bids []*types.Bid, intent *types.Intent) []scoredBid {
	scored := make([]scoredBid, 0, len(bids))

	// Find min and max prices for normalization
	minPrice, maxPrice := s.findPriceRange(bids)

	for _, bid := range bids {
		score := s.calculateScore(bid, intent, minPrice, maxPrice)
		scored = append(scored, scoredBid{
			bid:   bid,
			score: score,
		})
	}

	return scored
}

// calculateScore calculates the weighted score for a bid
func (s *WeightedScoringStrategy) calculateScore(bid *types.Bid, intent *types.Intent, minPrice, maxPrice uint64) float64 {
	// Price score (lower is better, normalized)
	priceScore := 1.0
	if maxPrice > minPrice {
		priceScore = 1.0 - float64(bid.Price-minPrice)/float64(maxPrice-minPrice)
	}

	// Reputation score (placeholder for MVP)
	reputationScore := 0.5 // Default middle score

	// Capacity score (placeholder for MVP)
	capacityScore := 0.5 // Default middle score

	// Calculate weighted score
	totalScore := priceScore*s.priceWeight +
		reputationScore*s.reputationWeight +
		capacityScore*s.capacityWeight

	return totalScore
}

// findPriceRange finds the minimum and maximum prices in bids
func (s *WeightedScoringStrategy) findPriceRange(bids []*types.Bid) (uint64, uint64) {
	if len(bids) == 0 {
		return 0, 0
	}

	minPrice := bids[0].Price
	maxPrice := bids[0].Price

	for _, bid := range bids[1:] {
		if bid.Price < minPrice {
			minPrice = bid.Price
		}
		if bid.Price > maxPrice {
			maxPrice = bid.Price
		}
	}

	return minPrice, maxPrice
}