package matcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"subnet/internal/logging"
	"subnet/internal/rootlayer"
	"subnet/internal/types"
)

// IntentSubscription represents a subscription to intent updates
type IntentSubscription struct {
	AgentID      string
	Capabilities []string
	updates      chan *types.Intent
	closed       bool
	mu           sync.Mutex
}

// Updates returns the channel for receiving intent updates
func (s *IntentSubscription) Updates() <-chan *types.Intent {
	return s.updates
}

// Close closes the subscription
func (s *IntentSubscription) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		close(s.updates)
		s.closed = true
	}
}

// IntentPuller pulls intents from RootLayer and broadcasts to agents
type IntentPuller struct {
	client rootlayer.Client
	logger logging.Logger
	mu     sync.RWMutex

	// Subscriptions management
	subscriptions map[string]*IntentSubscription // agent_id -> subscription

	// Intent cache
	pendingIntents   map[string]*types.Intent // intent_id -> intent
	processedIntents map[string]bool          // intent_id -> processed
}

// NewIntentPuller creates a new intent puller
func NewIntentPuller(client rootlayer.Client, logger logging.Logger) *IntentPuller {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	return &IntentPuller{
		client:           client,
		logger:           logger,
		subscriptions:    make(map[string]*IntentSubscription),
		pendingIntents:   make(map[string]*types.Intent),
		processedIntents: make(map[string]bool),
	}
}

// PullPendingIntents fetches pending intents from RootLayer
func (p *IntentPuller) PullPendingIntents(ctx context.Context) ([]*types.Intent, error) {
	p.logger.Info("Pulling pending intents from RootLayer")

	// Call RootLayer to get pending intents
	var intents []*types.Intent

	// Check if we have a RootLayer client
	if p.client != nil {
		// Try to fetch intents from RootLayer
		// Use empty subnet ID for now - should be configured
		rootIntents, err := p.client.GetPendingIntents(ctx, "")
		if err != nil {
			// Log error but continue with empty list
			// This allows the system to work with mock client
			p.logger.Warnf("Failed to get pending intents from RootLayer: %v", err)
			intents = make([]*types.Intent, 0)
		} else {
			// Convert rootpb.Intent to types.Intent
			intents = make([]*types.Intent, 0, len(rootIntents))
			for _, ri := range rootIntents {
				// Parse budget string to uint64
				var budget uint64
				fmt.Sscanf(ri.Budget, "%d", &budget)

				intent := &types.Intent{
					IntentID:     ri.IntentId,
					SubnetID:     ri.SubnetId,
					Requester:    ri.Requester,
					IntentType:   ri.IntentType,
					CreatedAt:    ri.CreatedAt,
					Deadline:     ri.Deadline,
					Budget:       budget,         // Converted from string
					PaymentToken: ri.BudgetToken, // Use BudgetToken field
					Status:       ri.Status.String(),
				}
				if ri.Params != nil {
					intent.IntentData = ri.Params.IntentRaw
					// ParamsHash is at the Intent level, not IntentParams
					intent.ParamsHash = string(ri.ParamsHash)
				}
				intents = append(intents, intent)
			}
		}
	} else {
		// No RootLayer client, return empty list
		intents = make([]*types.Intent, 0)
	}

	// Filter out already processed intents
	p.mu.Lock()
	defer p.mu.Unlock()

	newIntents := make([]*types.Intent, 0)
	for _, intent := range intents {
		if !p.processedIntents[intent.IntentID] {
			newIntents = append(newIntents, intent)
			p.pendingIntents[intent.IntentID] = intent
			p.processedIntents[intent.IntentID] = true
		}
	}

	if len(newIntents) > 0 {
		p.logger.Infof("Pulled %d new intents from RootLayer", len(newIntents))
	}

	return newIntents, nil
}

// Subscribe creates a subscription for an agent to receive intent updates
func (p *IntentPuller) Subscribe(agentID string, capabilities []string) *IntentSubscription {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close existing subscription if any
	if existing, exists := p.subscriptions[agentID]; exists {
		existing.Close()
	}

	sub := &IntentSubscription{
		AgentID:      agentID,
		Capabilities: capabilities,
		updates:      make(chan *types.Intent, 100), // Buffered channel
	}

	p.subscriptions[agentID] = sub
	p.logger.Infof("Agent %s subscribed with capabilities: %v", agentID, capabilities)

	// Send any existing pending intents that match capabilities
	go p.sendPendingIntents(sub)

	return sub
}

// Unsubscribe removes an agent's subscription
func (p *IntentPuller) Unsubscribe(agentID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if sub, exists := p.subscriptions[agentID]; exists {
		sub.Close()
		delete(p.subscriptions, agentID)
		p.logger.Infof("Agent %s unsubscribed", agentID)
	}
}

// BroadcastIntent sends an intent to all subscribed agents
func (p *IntentPuller) BroadcastIntent(intent *types.Intent) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sent := 0
	skipped := 0
	for agentID, sub := range p.subscriptions {
		if p.matchesCapabilities(intent, sub.Capabilities) {
			select {
			case sub.updates <- intent:
				sent++
			default:
				skipped++
				p.logger.Warnf("Agent %s channel full, skipping intent %s", agentID, intent.IntentID)
			}
		}
	}

	if sent > 0 || skipped > 0 {
		p.logger.Debugf("Broadcast intent %s: sent=%d skipped=%d", intent.IntentID, sent, skipped)
	}
}

// sendPendingIntents sends existing pending intents to a new subscriber
func (p *IntentPuller) sendPendingIntents(sub *IntentSubscription) {
	p.mu.RLock()
	intents := make([]*types.Intent, 0, len(p.pendingIntents))
	for _, intent := range p.pendingIntents {
		if p.matchesCapabilities(intent, sub.Capabilities) {
			intents = append(intents, intent)
		}
	}
	p.mu.RUnlock()

	sent := 0
	timedOut := 0
	for _, intent := range intents {
		select {
		case sub.updates <- intent:
			sent++
		case <-time.After(1 * time.Second):
			timedOut++
			p.logger.Warnf("Timeout sending pending intents to agent %s after %d sent", sub.AgentID, sent)
			return
		}
	}

	if sent > 0 {
		p.logger.Debugf("Sent %d pending intents to agent %s", sent, sub.AgentID)
	}
}

// matchesCapabilities checks if an intent matches agent capabilities
func (p *IntentPuller) matchesCapabilities(intent *types.Intent, capabilities []string) bool {
	// For MVP, match all intents if agent has any capabilities
	// In production, this would do sophisticated capability matching
	if len(capabilities) == 0 {
		return false
	}

	// Simple type matching for MVP
	for _, cap := range capabilities {
		if cap == intent.IntentType || cap == "*" {
			return true
		}
	}

	return false
}

// CleanupProcessed removes old processed intents from memory
func (p *IntentPuller) CleanupProcessed(olderThan time.Duration) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	// In MVP, we'll keep a simple limit on processed intents
	// In production, this would use timestamps
	maxProcessed := 1000
	if len(p.processedIntents) > maxProcessed {
		// Clear half of the oldest entries
		count := 0
		for intentID := range p.processedIntents {
			delete(p.processedIntents, intentID)
			count++
			if count >= maxProcessed/2 {
				break
			}
		}
		p.logger.Infof("Cleaned up %d processed intent records", count)
		return count
	}

	return 0
}

// GetSubscriberCount returns the number of active subscribers
func (p *IntentPuller) GetSubscriberCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.subscriptions)
}
