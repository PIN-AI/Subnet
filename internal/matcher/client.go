package matcher

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrBidInvalid indicates the payload is missing required fields.
var ErrBidInvalid = errors.New("matcher: bid is invalid")

// Bid captures an agent's willingness to execute an intent.
type Bid struct {
	BidID           string
	IntentID        string
	AgentID         string
	Price           uint64
	Token           string
	CapabilityScore uint32
	Metadata        map[string]string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	Signature       []byte
}

// BiddingResult summarizes the matcher outcome for an intent.
type BiddingResult struct {
	IntentID   string
	SubnetID   string
	MatcherID  string
	WinningBid *Bid
	RunnerUps  []*Bid
	SnapshotID string
	MatchedAt  time.Time
	Reason     string
}

// Client exposes the minimal matcher operations the agent runtime needs.
type Client interface {
	SubmitBid(ctx context.Context, bid *Bid) error
}

// MockClient is a lightweight matcher client used in unit tests.
type MockClient struct {
	mu     sync.Mutex
	bids   []*Bid
	Err    error
	OnCall func(*Bid)
}

// SubmitBid records the payload and optionally invokes a callback.
func (m *MockClient) SubmitBid(_ context.Context, bid *Bid) error {
	if m == nil {
		return ErrBidInvalid
	}
	if bid == nil || bid.IntentID == "" || bid.AgentID == "" {
		return ErrBidInvalid
	}
	snapshot := *bid
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	m.mu.Lock()
	m.bids = append(m.bids, &snapshot)
	cb := m.OnCall
	err := m.Err
	m.mu.Unlock()
	if cb != nil {
		cb(&snapshot)
	}
	return err
}

// SubmittedBids returns a copy of the bids collected so far.
func (m *MockClient) SubmittedBids() []*Bid {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Bid, len(m.bids))
	copy(out, m.bids)
	return out
}
