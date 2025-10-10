package matcher

import (
	"fmt"
	"sync"
	"time"

	"subnet/internal/logging"
	"subnet/internal/types"
)

// IntentBidSnapshot represents bidding state for a single intent
type IntentBidSnapshot struct {
	IntentID         string
	Bids             []*types.Bid
	BiddingStartTime int64
	BiddingEndTime   int64
	BiddingClosed    bool
	Intent           *types.Intent // Store intent data for matching
}

// BidBook manages all bids across intents
type BidBook struct {
	mu     sync.RWMutex
	logger logging.Logger

	// Maps for efficient lookups
	bidsByIntent map[string]*IntentBidSnapshot // intent_id -> snapshot
	bidsByAgent  map[string][]string            // agent_id -> bid_ids
	allBids      map[string]*types.Bid          // bid_id -> bid

	// Tracking
	closedWindows []string // Intent IDs with closed bidding windows
}

// NewBidBook creates a new bid book
func NewBidBook(logger logging.Logger) *BidBook {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	return &BidBook{
		logger:        logger,
		bidsByIntent:  make(map[string]*IntentBidSnapshot),
		bidsByAgent:   make(map[string][]string),
		allBids:       make(map[string]*types.Bid),
		closedWindows: make([]string, 0),
	}
}

// StartBidding initializes bidding for a new intent
func (b *BidBook) StartBidding(intent *types.Intent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.bidsByIntent[intent.IntentID]; exists {
		return fmt.Errorf("bidding already started for intent %s", intent.IntentID)
	}

	snapshot := &IntentBidSnapshot{
		IntentID:         intent.IntentID,
		Bids:             make([]*types.Bid, 0),
		BiddingStartTime: intent.BiddingStartTime,
		BiddingEndTime:   intent.BiddingEndTime,
		BiddingClosed:    false,
		Intent:           intent,
	}

	b.bidsByIntent[intent.IntentID] = snapshot
	b.logger.Infof("Started bidding for intent %s, window: %d-%d",
		intent.IntentID, intent.BiddingStartTime, intent.BiddingEndTime)

	return nil
}

// AddBid adds a new bid to the book
func (b *BidBook) AddBid(bid *types.Bid) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if intent exists
	snapshot, exists := b.bidsByIntent[bid.IntentID]
	if !exists {
		return fmt.Errorf("intent %s not found", bid.IntentID)
	}

	// Check if bidding window is still open
	if snapshot.BiddingClosed {
		return fmt.Errorf("bidding window closed for intent %s", bid.IntentID)
	}

	currentTime := time.Now().Unix()
	if currentTime > snapshot.BiddingEndTime {
		snapshot.BiddingClosed = true
		b.closedWindows = append(b.closedWindows, bid.IntentID)
		return fmt.Errorf("bidding window expired for intent %s", bid.IntentID)
	}

	// Check for duplicate bid from same agent
	for _, existingBid := range snapshot.Bids {
		if existingBid.AgentID == bid.AgentID {
			// Update existing bid if price is better
			if bid.Price < existingBid.Price {
				existingBid.Price = bid.Price
				existingBid.SubmittedAt = bid.SubmittedAt
				existingBid.Signature = bid.Signature
				b.logger.Infof("Updated bid from agent %s for intent %s: new price %d",
					bid.AgentID, bid.IntentID, bid.Price)
				return nil
			}
			return fmt.Errorf("agent %s already submitted bid for intent %s", bid.AgentID, bid.IntentID)
		}
	}

	// Add new bid
	snapshot.Bids = append(snapshot.Bids, bid)
	b.allBids[bid.BidID] = bid

	// Track by agent
	if _, exists := b.bidsByAgent[bid.AgentID]; !exists {
		b.bidsByAgent[bid.AgentID] = make([]string, 0)
	}
	b.bidsByAgent[bid.AgentID] = append(b.bidsByAgent[bid.AgentID], bid.BidID)

	b.logger.Infof("Added bid %s from agent %s for intent %s: price %d",
		bid.BidID, bid.AgentID, bid.IntentID, bid.Price)

	return nil
}

// GetSnapshot returns the bidding snapshot for an intent
func (b *BidBook) GetSnapshot(intentID string) *IntentBidSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()

	snapshot, exists := b.bidsByIntent[intentID]
	if !exists {
		return nil
	}

	// Check if window should be closed
	currentTime := time.Now().Unix()
	if !snapshot.BiddingClosed && currentTime > snapshot.BiddingEndTime {
		// Need to upgrade to write lock
		b.mu.RUnlock()
		b.mu.Lock()
		snapshot.BiddingClosed = true
		b.closedWindows = append(b.closedWindows, intentID)
		b.mu.Unlock()
		b.mu.RLock()
	}

	// Return a copy to avoid concurrent modification
	result := &IntentBidSnapshot{
		IntentID:         snapshot.IntentID,
		BiddingStartTime: snapshot.BiddingStartTime,
		BiddingEndTime:   snapshot.BiddingEndTime,
		BiddingClosed:    snapshot.BiddingClosed,
		Intent:           snapshot.Intent,
		Bids:             make([]*types.Bid, len(snapshot.Bids)),
	}
	copy(result.Bids, snapshot.Bids)

	return result
}

// GetClosedWindows returns and clears the list of recently closed bidding windows
func (b *BidBook) GetClosedWindows() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Also check for expired windows
	currentTime := time.Now().Unix()
	for intentID, snapshot := range b.bidsByIntent {
		if !snapshot.BiddingClosed && currentTime > snapshot.BiddingEndTime {
			snapshot.BiddingClosed = true
			b.closedWindows = append(b.closedWindows, intentID)
		}
	}

	if len(b.closedWindows) == 0 {
		return nil
	}

	// Return copy and clear
	result := make([]string, len(b.closedWindows))
	copy(result, b.closedWindows)
	b.closedWindows = b.closedWindows[:0]

	return result
}

// GetBid returns a specific bid by ID
func (b *BidBook) GetBid(bidID string) *types.Bid {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.allBids[bidID]
}

// GetAgentBids returns all bid IDs for a specific agent
func (b *BidBook) GetAgentBids(agentID string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bids, exists := b.bidsByAgent[agentID]
	if !exists {
		return nil
	}

	result := make([]string, len(bids))
	copy(result, bids)
	return result
}

// RemoveIntent removes an intent and all its bids from the book
func (b *BidBook) RemoveIntent(intentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	snapshot, exists := b.bidsByIntent[intentID]
	if !exists {
		return
	}

	// Remove all bids
	for _, bid := range snapshot.Bids {
		delete(b.allBids, bid.BidID)

		// Remove from agent tracking
		if agentBids, exists := b.bidsByAgent[bid.AgentID]; exists {
			newBids := make([]string, 0, len(agentBids)-1)
			for _, bidID := range agentBids {
				if bidID != bid.BidID {
					newBids = append(newBids, bidID)
				}
			}
			if len(newBids) > 0 {
				b.bidsByAgent[bid.AgentID] = newBids
			} else {
				delete(b.bidsByAgent, bid.AgentID)
			}
		}
	}

	// Remove intent snapshot
	delete(b.bidsByIntent, intentID)

	b.logger.Infof("Removed intent %s and %d associated bids", intentID, len(snapshot.Bids))
}

// ActiveIntentCount returns the number of active intents
func (b *BidBook) ActiveIntentCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, snapshot := range b.bidsByIntent {
		if !snapshot.BiddingClosed {
			count++
		}
	}
	return count
}

// TotalBidCount returns the total number of bids
func (b *BidBook) TotalBidCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.allBids)
}

// CleanupExpired removes intents that have been closed for more than the specified duration
func (b *BidBook) CleanupExpired(olderThan time.Duration) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	currentTime := time.Now().Unix()
	cutoffTime := currentTime - int64(olderThan.Seconds())
	removed := 0

	for intentID, snapshot := range b.bidsByIntent {
		if snapshot.BiddingClosed && snapshot.BiddingEndTime < cutoffTime {
			// Remove all associated bids
			for _, bid := range snapshot.Bids {
				delete(b.allBids, bid.BidID)
			}
			delete(b.bidsByIntent, intentID)
			removed++
		}
	}

	if removed > 0 {
		b.logger.Infof("Cleaned up %d expired intents older than %v", removed, olderThan)
	}

	return removed
}