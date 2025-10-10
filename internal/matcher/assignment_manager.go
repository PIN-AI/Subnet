package matcher

import (
	"fmt"
	"sync"
	"time"

	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/types"
)

// AssignmentManager manages assignment creation, distribution, and tracking
type AssignmentManager struct {
	cfg    *Config
	logger logging.Logger
	signer crypto.Signer // Signer for creating signatures
	mu     sync.RWMutex

	// Assignment tracking
	assignments       map[string]*types.Assignment // assignment_id -> assignment
	assignmentsByIntent map[string]string          // intent_id -> assignment_id
	assignmentsByAgent  map[string][]string         // agent_id -> assignment_ids

	// Runner-up tracking for failover
	runnerUps map[string][]*types.Bid // intent_id -> runner-up bids

	// Agent connections for pushing assignments
	agentConnections map[string]AgentConnection // agent_id -> connection

	// Pending assignments queue
	pendingQueue []string // assignment_ids waiting to be sent
}

// AgentConnection represents a connection to an agent
type AgentConnection interface {
	SendAssignment(assignment *types.Assignment) error
	IsConnected() bool
}

// NewAssignmentManager creates a new assignment manager
func NewAssignmentManager(cfg *Config, logger logging.Logger) (*AssignmentManager, error) {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	// Create signer if private key is provided
	var signer crypto.Signer
	if cfg.PrivateKey != "" {
		var err error
		signer, err = crypto.NewECDSASignerFromHex(cfg.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create signer: %w", err)
		}
	}

	return &AssignmentManager{
		cfg:                 cfg,
		logger:              logger,
		signer:              signer,
		assignments:         make(map[string]*types.Assignment),
		assignmentsByIntent: make(map[string]string),
		assignmentsByAgent:  make(map[string][]string),
		runnerUps:           make(map[string][]*types.Bid),
		agentConnections:    make(map[string]AgentConnection),
		pendingQueue:        make([]string, 0),
	}, nil
}

// CreateAssignment creates a new assignment from matching result
func (m *AssignmentManager) CreateAssignment(result *types.MatchingResult) *types.Assignment {
	if result == nil || result.WinningBid == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if intent already has assignment
	if existingID, exists := m.assignmentsByIntent[result.IntentID]; exists {
		m.logger.Warnf("Intent %s already has assignment %s", result.IntentID, existingID)
		return m.assignments[existingID]
	}

	// Create assignment
	assignment := &types.Assignment{
		AssignmentID: generateAssignmentID(result.IntentID, result.WinningAgentID),
		IntentID:     result.IntentID,
		AgentID:      result.WinningAgentID,
		BidID:        result.WinningBid.BidID,
		AssignedAt:   time.Now().Unix(),
		LeaseTTL:     300, // 5 minutes default
		Status:       "PENDING",
		MatcherID:    result.MatcherID,
	}

	// Sign assignment
	if m.signer != nil {
		// Create message to sign
		message := fmt.Sprintf("%s:%s:%s:%d",
			assignment.AssignmentID,
			assignment.IntentID,
			assignment.AgentID,
			assignment.AssignedAt)
		msgHash := crypto.HashMessage([]byte(message))

		// Sign the hash
		signature, err := m.signer.Sign(msgHash[:])
		if err != nil {
			m.logger.Warnf("Failed to sign assignment: %v", err)
			assignment.Signature = ""
		} else {
			assignment.Signature = fmt.Sprintf("%x", signature)
		}
	} else {
		m.logger.Warn("No signer configured, assignment will be unsigned")
		assignment.Signature = ""
	}

	// Store assignment
	m.assignments[assignment.AssignmentID] = assignment
	m.assignmentsByIntent[result.IntentID] = assignment.AssignmentID

	if _, exists := m.assignmentsByAgent[assignment.AgentID]; !exists {
		m.assignmentsByAgent[assignment.AgentID] = make([]string, 0)
	}
	m.assignmentsByAgent[assignment.AgentID] = append(m.assignmentsByAgent[assignment.AgentID], assignment.AssignmentID)

	// Store runner-ups for potential failover
	if len(result.RunnerUpBids) > 0 {
		m.runnerUps[result.IntentID] = result.RunnerUpBids
	}

	// Add to pending queue
	m.pendingQueue = append(m.pendingQueue, assignment.AssignmentID)

	m.logger.Infof("Created assignment %s for intent %s -> agent %s",
		assignment.AssignmentID, assignment.IntentID, assignment.AgentID)

	return assignment
}

// SendToAgent sends an assignment to the assigned agent
func (m *AssignmentManager) SendToAgent(assignment *types.Assignment) error {
	if assignment == nil {
		return fmt.Errorf("assignment is nil")
	}

	m.mu.RLock()
	conn, exists := m.agentConnections[assignment.AgentID]
	m.mu.RUnlock()

	if !exists || conn == nil {
		return fmt.Errorf("no connection to agent %s", assignment.AgentID)
	}

	if !conn.IsConnected() {
		return fmt.Errorf("agent %s is not connected", assignment.AgentID)
	}

	// Send assignment
	err := conn.SendAssignment(assignment)
	if err != nil {
		return fmt.Errorf("failed to send assignment to agent %s: %w", assignment.AgentID, err)
	}

	// Update status
	m.mu.Lock()
	assignment.Status = "SENT"
	m.mu.Unlock()

	m.logger.Infof("Sent assignment %s to agent %s", assignment.AssignmentID, assignment.AgentID)
	return nil
}

// ConfirmAssignment marks an assignment as accepted by the agent
func (m *AssignmentManager) ConfirmAssignment(assignmentID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	assignment, exists := m.assignments[assignmentID]
	if !exists {
		return fmt.Errorf("assignment %s not found", assignmentID)
	}

	if assignment.AgentID != agentID {
		return fmt.Errorf("assignment %s is for agent %s, not %s", assignmentID, assignment.AgentID, agentID)
	}

	assignment.Status = "ACTIVE"

	// Remove from pending queue
	m.removeFromPendingQueue(assignmentID)

	m.logger.Infof("Assignment %s confirmed by agent %s", assignmentID, agentID)
	return nil
}

// HandleRejection handles assignment rejection and activates runner-up if available
func (m *AssignmentManager) HandleRejection(assignmentID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	assignment, exists := m.assignments[assignmentID]
	if !exists {
		return fmt.Errorf("assignment %s not found", assignmentID)
	}

	assignment.Status = "REJECTED"
	m.logger.Infof("Assignment %s rejected: %s", assignmentID, reason)

	// Try to activate runner-up
	return m.activateRunnerUpInternal(assignment.IntentID)
}

// ActivateRunnerUp activates the next runner-up for an intent
func (m *AssignmentManager) ActivateRunnerUp(intentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.activateRunnerUpInternal(intentID)
}

// activateRunnerUpInternal activates runner-up (must be called with lock held)
func (m *AssignmentManager) activateRunnerUpInternal(intentID string) error {
	runnerUps, exists := m.runnerUps[intentID]
	if !exists || len(runnerUps) == 0 {
		m.logger.Warnf("No runner-ups available for intent %s", intentID)
		return fmt.Errorf("no runner-ups available")
	}

	// Get next runner-up
	nextBid := runnerUps[0]
	m.runnerUps[intentID] = runnerUps[1:]

	// Create new assignment
	assignment := &types.Assignment{
		AssignmentID: generateAssignmentID(intentID, nextBid.AgentID),
		IntentID:     intentID,
		AgentID:      nextBid.AgentID,
		BidID:        nextBid.BidID,
		AssignedAt:   time.Now().Unix(),
		LeaseTTL:     300,
		Status:       "PENDING",
		MatcherID:    m.cfg.MatcherID,
	}

	// Sign assignment if signer is available
	if m.signer != nil {
		message := fmt.Sprintf("%s:%s:%s:%d",
			assignment.AssignmentID,
			assignment.IntentID,
			assignment.AgentID,
			assignment.AssignedAt)
		msgHash := crypto.HashMessage([]byte(message))

		signature, err := m.signer.Sign(msgHash[:])
		if err != nil {
			m.logger.Warnf("Failed to sign runner-up assignment: %v", err)
			assignment.Signature = ""
		} else {
			assignment.Signature = fmt.Sprintf("%x", signature)
		}
	} else {
		m.logger.Warn("No signer configured for runner-up assignment")
		assignment.Signature = ""
	}

	// Store assignment
	m.assignments[assignment.AssignmentID] = assignment
	m.assignmentsByIntent[intentID] = assignment.AssignmentID

	if _, exists := m.assignmentsByAgent[assignment.AgentID]; !exists {
		m.assignmentsByAgent[assignment.AgentID] = make([]string, 0)
	}
	m.assignmentsByAgent[assignment.AgentID] = append(m.assignmentsByAgent[assignment.AgentID], assignment.AssignmentID)

	// Add to pending queue
	m.pendingQueue = append(m.pendingQueue, assignment.AssignmentID)

	m.logger.Infof("Activated runner-up: assignment %s for intent %s -> agent %s",
		assignment.AssignmentID, intentID, assignment.AgentID)

	return nil
}

// GetPendingAssignments returns assignments that need to be sent to agents
func (m *AssignmentManager) GetPendingAssignments() []*types.Assignment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.pendingQueue) == 0 {
		return nil
	}

	assignments := make([]*types.Assignment, 0, len(m.pendingQueue))
	for _, id := range m.pendingQueue {
		if assignment, exists := m.assignments[id]; exists {
			if assignment.Status == "PENDING" {
				assignments = append(assignments, assignment)
			}
		}
	}

	return assignments
}

// GetAssignment returns an assignment by ID
func (m *AssignmentManager) GetAssignment(assignmentID string) *types.Assignment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.assignments[assignmentID]
}

// PendingCount returns the number of pending assignments
func (m *AssignmentManager) PendingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, assignment := range m.assignments {
		if assignment.Status == "PENDING" || assignment.Status == "SENT" {
			count++
		}
	}
	return count
}

// RegisterAgentConnection registers a connection to an agent
func (m *AssignmentManager) RegisterAgentConnection(agentID string, conn AgentConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.agentConnections[agentID] = conn
	m.logger.Infof("Registered connection for agent %s", agentID)
}

// UnregisterAgentConnection removes an agent connection
func (m *AssignmentManager) UnregisterAgentConnection(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.agentConnections, agentID)
	m.logger.Infof("Unregistered connection for agent %s", agentID)
}

// removeFromPendingQueue removes an assignment from pending queue
func (m *AssignmentManager) removeFromPendingQueue(assignmentID string) {
	newQueue := make([]string, 0, len(m.pendingQueue))
	for _, id := range m.pendingQueue {
		if id != assignmentID {
			newQueue = append(newQueue, id)
		}
	}
	m.pendingQueue = newQueue
}

// CleanupExpired removes expired assignments
func (m *AssignmentManager) CleanupExpired(olderThan time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentTime := time.Now().Unix()
	cutoffTime := currentTime - int64(olderThan.Seconds())
	removed := 0

	for id, assignment := range m.assignments {
		if assignment.AssignedAt < cutoffTime &&
		   (assignment.Status == "REJECTED" || assignment.Status == "EXPIRED") {
			delete(m.assignments, id)
			delete(m.assignmentsByIntent, assignment.IntentID)
			removed++
		}
	}

	if removed > 0 {
		m.logger.Infof("Cleaned up %d expired assignments", removed)
	}

	return removed
}

// generateAssignmentID generates a unique assignment ID
func generateAssignmentID(intentID, agentID string) string {
	return fmt.Sprintf("assign_%s_%s_%d", intentID, agentID, time.Now().UnixNano())
}