package rootlayer

import (
	"context"
	"fmt"
	"sync"
	"time"

	rootpb "rootlayer/proto"
	"subnet/internal/logging"
)

// MockClient implements a mock RootLayer client for testing
type MockClient struct {
	*BaseClient

	mu              sync.RWMutex
	intents         []*rootpb.Intent
	assignments     map[string]*rootpb.Assignment
	intentStream    chan *rootpb.Intent
	stopStream      chan struct{}
	intentGenerator *IntentGenerator
}

// NewMockClient creates a new mock RootLayer client
func NewMockClient(cfg *Config, logger logging.Logger) *MockClient {
	return &MockClient{
		BaseClient:      NewBaseClient(cfg, logger),
		intents:         make([]*rootpb.Intent, 0),
		assignments:     make(map[string]*rootpb.Assignment),
		intentStream:    make(chan *rootpb.Intent, 100),
		stopStream:      make(chan struct{}),
		intentGenerator: NewIntentGenerator(),
	}
}

// Connect establishes connection (mock)
func (c *MockClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	c.connected = true
	c.logger.Info("Mock RootLayer client connected")

	// Start generating mock intents
	go c.generateIntents()

	return nil
}

// Close closes the connection
func (c *MockClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	close(c.stopStream)
	close(c.intentStream)
	c.connected = false
	c.logger.Info("Mock RootLayer client disconnected")

	return nil
}

// GetPendingIntents returns pending intents for a subnet
func (c *MockClient) GetPendingIntents(ctx context.Context, subnetID string) ([]*rootpb.Intent, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	// Return some mock intents
	pendingIntents := make([]*rootpb.Intent, 0)
	for _, intent := range c.intents {
		if intent.SubnetId == subnetID && intent.Status == rootpb.IntentStatus_INTENT_STATUS_PENDING {
			pendingIntents = append(pendingIntents, intent)
		}
	}

	c.logger.Infof("Returning %d pending intents for subnet %s", len(pendingIntents), subnetID)
	return pendingIntents, nil
}

// StreamIntents streams intents for a subnet
func (c *MockClient) StreamIntents(ctx context.Context, subnetID string) (<-chan *rootpb.Intent, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	c.logger.Infof("Starting intent stream for subnet %s", subnetID)

	// Return the channel that receives generated intents
	return c.intentStream, nil
}

// UpdateIntentStatus updates the status of an intent
func (c *MockClient) UpdateIntentStatus(ctx context.Context, intentID string, status rootpb.IntentStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Find and update the intent
	for _, intent := range c.intents {
		if intent.IntentId == intentID {
			intent.Status = status
			c.logger.Infof("Updated intent %s status to %v", intentID, status)
			return nil
		}
	}

	return fmt.Errorf("intent %s not found", intentID)
}

// SubmitAssignment submits an assignment to RootLayer
func (c *MockClient) SubmitAssignment(ctx context.Context, assignment *rootpb.Assignment) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	c.assignments[assignment.AssignmentId] = assignment
	c.logger.Infof("Submitted assignment %s for intent %s to agent %s",
		assignment.AssignmentId, assignment.IntentId, assignment.AgentId)

	// Update intent status
	for _, intent := range c.intents {
		if intent.IntentId == assignment.IntentId {
			intent.Status = rootpb.IntentStatus_INTENT_STATUS_PROCESSING
			break
		}
	}

	return nil
}

// GetAssignment retrieves an assignment
func (c *MockClient) GetAssignment(ctx context.Context, assignmentID string) (*rootpb.Assignment, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	assignment, exists := c.assignments[assignmentID]
	if !exists {
		return nil, fmt.Errorf("assignment %s not found", assignmentID)
	}

	return assignment, nil
}

// SubmitMatchingResult submits matching result
func (c *MockClient) SubmitMatchingResult(ctx context.Context, result *MatchingResult) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	c.logger.Infof("Submitted matching result for intent %s: winner=%s, price=%d",
		result.IntentID, result.WinningAgentID, result.Price)

	return nil
}

// SubmitValidationBundle submits validation bundle
func (c *MockClient) SubmitValidationBundle(ctx context.Context, bundle *rootpb.ValidationBundle) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	c.logger.Infof("Submitted validation bundle with %d signatures", len(bundle.Signatures))
	return nil
}

// generateIntents generates mock intents periodically
func (c *MockClient) generateIntents() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopStream:
			return
		case <-ticker.C:
			intent := c.intentGenerator.GenerateIntent(c.cfg.SubnetID)

			c.mu.Lock()
			c.intents = append(c.intents, intent)
			c.mu.Unlock()

			// Send to stream
			select {
			case c.intentStream <- intent:
				c.logger.Infof("Generated new intent: %s", intent.IntentId)
			default:
				c.logger.Warn("Intent stream channel full, dropping intent")
			}
		}
	}
}

// IntentGenerator generates mock intents
type IntentGenerator struct {
	counter int
	mu      sync.Mutex
}

// NewIntentGenerator creates a new intent generator
func NewIntentGenerator() *IntentGenerator {
	return &IntentGenerator{}
}

// GenerateIntent generates a mock intent
func (g *IntentGenerator) GenerateIntent(subnetID string) *rootpb.Intent {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.counter++

	intentTypes := []string{"compute", "storage", "network", "ai-inference"}
	intentType := intentTypes[g.counter%len(intentTypes)]

	return &rootpb.Intent{
		IntentId:     fmt.Sprintf("intent-%06d", g.counter),
		SubnetId:     subnetID,  // Changed from SubnetID to SubnetId
		Requester:    fmt.Sprintf("user-%03d", (g.counter%10)+1),
		IntentType:   intentType,
		SettleChain:  "ethereum",
		BudgetToken:  "USDC",
		Budget:       fmt.Sprintf("%d", 100+(g.counter%500)),  // Changed to string type
		Deadline:     time.Now().Add(30 * time.Minute).Unix(),
		Status:       rootpb.IntentStatus_INTENT_STATUS_PENDING,
		CreatedAt:    time.Now().Unix(),
		Params: &rootpb.IntentParams{
			IntentRaw: []byte(fmt.Sprintf(`{"task": "process-%d", "type": "%s"}`, g.counter, intentType)),
			Metadata:  []byte(fmt.Sprintf(`{"priority": %d}`, (g.counter%3)+1)),
		},
	}
}