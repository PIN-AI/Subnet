package registry

import (
	"fmt"
	"sort"
	"sync"
	"time"

	pb "subnet/proto/subnet"
)

type Registry struct {
	agents     map[string]*AgentInfo
	validators map[string]*ValidatorInfo
	matchers   map[string]*MatcherInfo
	mu         sync.RWMutex
}

type AgentInfo struct {
	ID           string
	Capabilities []string
	Endpoint     string
	LastSeen     time.Time
	Status       pb.AgentStatus
}

type ValidatorInfo struct {
	ID       string
	Endpoint string
	LastSeen time.Time
	Status   pb.AgentStatus
}

type MatcherInfo struct {
	ID       string
	Endpoint string
	LastSeen time.Time
	Status   pb.AgentStatus
}

func New() *Registry {
	return &Registry{
		agents:     make(map[string]*AgentInfo),
		validators: make(map[string]*ValidatorInfo),
		matchers:   make(map[string]*MatcherInfo),
	}
}

func (r *Registry) RegisterAgent(info *AgentInfo) error {
	if info == nil {
		return fmt.Errorf("agent info cannot be nil")
	}
	if info.ID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info.LastSeen = time.Now()
	info.Status = pb.AgentStatus_AGENT_STATUS_ACTIVE
	r.agents[info.ID] = info

	return nil
}

func (r *Registry) RegisterValidator(info *ValidatorInfo) error {
	if info == nil {
		return fmt.Errorf("validator info cannot be nil")
	}
	if info.ID == "" {
		return fmt.Errorf("validator ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info.LastSeen = time.Now()
	info.Status = pb.AgentStatus_AGENT_STATUS_ACTIVE
	r.validators[info.ID] = info

	return nil
}

func (r *Registry) RegisterMatcher(info *MatcherInfo) error {
	if info == nil {
		return fmt.Errorf("matcher info cannot be nil")
	}
	if info.ID == "" {
		return fmt.Errorf("matcher ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info.LastSeen = time.Now()
	info.Status = pb.AgentStatus_AGENT_STATUS_ACTIVE
	r.matchers[info.ID] = info

	return nil
}

func (r *Registry) DiscoverAgents(capabilities []string) ([]*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*AgentInfo
	for _, agent := range r.agents {
		if agent.Status != pb.AgentStatus_AGENT_STATUS_ACTIVE {
			continue
		}
		if agent.MatchesCapabilities(capabilities) {
			// Create a copy to avoid data races
			agentCopy := &AgentInfo{
				ID:           agent.ID,
				Capabilities: append([]string{}, agent.Capabilities...), // Deep copy slice
				Endpoint:     agent.Endpoint,
				LastSeen:     agent.LastSeen,
				Status:       agent.Status,
			}
			matched = append(matched, agentCopy)
		}
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("no agents found with required capabilities")
	}

	// Sort by ID for deterministic results
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].ID < matched[j].ID
	})

	return matched, nil
}

func (r *Registry) GetAgent(id string) (*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[id]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", id)
	}

	// Return a copy to avoid data races
	agentCopy := &AgentInfo{
		ID:           agent.ID,
		Capabilities: append([]string{}, agent.Capabilities...), // Deep copy slice
		Endpoint:     agent.Endpoint,
		LastSeen:     agent.LastSeen,
		Status:       agent.Status,
	}

	return agentCopy, nil
}

func (r *Registry) GetValidator(id string) (*ValidatorInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	validator, exists := r.validators[id]
	if !exists {
		return nil, fmt.Errorf("validator %s not found", id)
	}

	// Return a copy to avoid data races
	validatorCopy := &ValidatorInfo{
		ID:       validator.ID,
		Endpoint: validator.Endpoint,
		LastSeen: validator.LastSeen,
		Status:   validator.Status,
	}

	return validatorCopy, nil
}

func (r *Registry) GetMatcher(id string) (*MatcherInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matcher, exists := r.matchers[id]
	if !exists {
		return nil, fmt.Errorf("matcher %s not found", id)
	}

	// Return a copy to avoid data races
	matcherCopy := &MatcherInfo{
		ID:       matcher.ID,
		Endpoint: matcher.Endpoint,
		LastSeen: matcher.LastSeen,
		Status:   matcher.Status,
	}

	return matcherCopy, nil
}

func (r *Registry) ListAgents() []*AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(r.agents))
	for _, agent := range r.agents {
		// Create a copy to avoid data races
		agentCopy := &AgentInfo{
			ID:           agent.ID,
			Capabilities: append([]string{}, agent.Capabilities...), // Deep copy slice
			Endpoint:     agent.Endpoint,
			LastSeen:     agent.LastSeen,
			Status:       agent.Status,
		}
		agents = append(agents, agentCopy)
	}

	// Sort by ID for deterministic ordering (critical for consensus!)
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].ID < agents[j].ID
	})

	return agents
}

func (r *Registry) ListValidators() []*ValidatorInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	validators := make([]*ValidatorInfo, 0, len(r.validators))
	for _, validator := range r.validators {
		// Create a copy to avoid data races
		validatorCopy := &ValidatorInfo{
			ID:       validator.ID,
			Endpoint: validator.Endpoint,
			LastSeen: validator.LastSeen,
			Status:   validator.Status,
		}
		validators = append(validators, validatorCopy)
	}

	// Sort by ID for deterministic ordering
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].ID < validators[j].ID
	})

	return validators
}

func (r *Registry) ListMatchers() []*MatcherInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matchers := make([]*MatcherInfo, 0, len(r.matchers))
	for _, matcher := range r.matchers {
		// Create a copy to avoid data races
		matcherCopy := &MatcherInfo{
			ID:       matcher.ID,
			Endpoint: matcher.Endpoint,
			LastSeen: matcher.LastSeen,
			Status:   matcher.Status,
		}
		matchers = append(matchers, matcherCopy)
	}

	// Sort by ID for deterministic ordering
	sort.Slice(matchers, func(i, j int) bool {
		return matchers[i].ID < matchers[j].ID
	})

	return matchers
}

func (r *Registry) UpdateHeartbeat(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.LastSeen = time.Now()
	agent.Status = pb.AgentStatus_AGENT_STATUS_ACTIVE

	return nil
}

func (r *Registry) MarkInactive(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.Status = pb.AgentStatus_AGENT_STATUS_INACTIVE

	return nil
}

func (r *Registry) RemoveAgent(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agentID]; !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	delete(r.agents, agentID)
	return nil
}

func (r *Registry) UpdateValidatorHeartbeat(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	validator, exists := r.validators[id]
	if !exists {
		return fmt.Errorf("validator %s not found", id)
	}

	validator.LastSeen = time.Now()
	validator.Status = pb.AgentStatus_AGENT_STATUS_ACTIVE
	return nil
}

func (r *Registry) RemoveValidator(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.validators[id]; !exists {
		return fmt.Errorf("validator %s not found", id)
	}

	delete(r.validators, id)
	return nil
}

func (r *Registry) CleanupStaleAgents(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, agent := range r.agents {
		if agent.LastSeen.Before(cutoff) {
			delete(r.agents, id)
			removed++
		}
	}

	return removed
}

func (a *AgentInfo) MatchesCapabilities(required []string) bool {
	if len(required) == 0 {
		return true
	}

	capMap := make(map[string]bool)
	for _, cap := range a.Capabilities {
		capMap[cap] = true
	}

	for _, req := range required {
		if !capMap[req] {
			return false
		}
	}

	return true
}

func (a *AgentInfo) IsHealthy() bool {
	return a.Status == pb.AgentStatus_AGENT_STATUS_ACTIVE &&
		time.Since(a.LastSeen) < 5*time.Minute
}
