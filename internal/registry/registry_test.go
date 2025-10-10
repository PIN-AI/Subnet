package registry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	pb "subnet/proto/subnet"
)

// TestListAgentsDeterministic verifies that ListAgents returns agents in deterministic order
func TestListAgentsDeterministic(t *testing.T) {
	reg := New()

	// Register agents in different order
	agents := []string{"agent-c", "agent-a", "agent-b", "agent-z", "agent-m"}
	for _, id := range agents {
		err := reg.RegisterAgent(&AgentInfo{
			ID:           id,
			Capabilities: []string{"compute"},
			Endpoint:     "localhost:8080",
		})
		if err != nil {
			t.Fatalf("Failed to register agent %s: %v", id, err)
		}
	}

	// Call ListAgents multiple times
	var results [][]string
	for i := 0; i < 10; i++ {
		list := reg.ListAgents()
		var ids []string
		for _, agent := range list {
			ids = append(ids, agent.ID)
		}
		results = append(results, ids)
	}

	// Verify all results are identical (deterministic)
	expected := results[0]
	for i := 1; i < len(results); i++ {
		if len(results[i]) != len(expected) {
			t.Errorf("Result %d has different length: got %d, want %d",
				i, len(results[i]), len(expected))
		}
		for j := 0; j < len(expected); j++ {
			if results[i][j] != expected[j] {
				t.Errorf("Result %d differs at position %d: got %s, want %s",
					i, j, results[i][j], expected[j])
			}
		}
	}

	// Verify sorted order
	for i := 1; i < len(expected); i++ {
		if expected[i-1] >= expected[i] {
			t.Errorf("Agents not sorted: %s >= %s at position %d",
				expected[i-1], expected[i], i)
		}
	}
}

// TestDataRaceSafety verifies that returned objects are copies, not references
func TestDataRaceSafety(t *testing.T) {
	reg := New()

	// Register an agent
	agentID := "test-agent"
	err := reg.RegisterAgent(&AgentInfo{
		ID:           agentID,
		Capabilities: []string{"compute", "storage"},
		Endpoint:     "localhost:8080",
	})
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Get the agent
	agent1, err := reg.GetAgent(agentID)
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	// Simulate concurrent access
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Modify the returned agent
	go func() {
		defer wg.Done()
		statuses := []pb.AgentStatus{
			pb.AgentStatus_AGENT_STATUS_ACTIVE,
			pb.AgentStatus_AGENT_STATUS_INACTIVE,
			pb.AgentStatus_AGENT_STATUS_UNHEALTHY,
		}
		for i := 0; i < 100; i++ {
			agent1.Status = statuses[i%len(statuses)]
			agent1.LastSeen = time.Now()
			time.Sleep(time.Microsecond)
		}
	}()

	// Goroutine 2: Update the registry's internal agent
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			reg.UpdateHeartbeat(agentID)
			time.Sleep(time.Microsecond)
		}
	}()

	wg.Wait()

	// Get the agent again
	agent2, err := reg.GetAgent(agentID)
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	// The modifications to agent1 should NOT affect agent2
	// since they are copies, not references
	if agent1.ID != agent2.ID {
		t.Errorf("Agent IDs should be the same")
	}
}

// TestCapabilitiesDeepCopy verifies that capability slices are deep copied
func TestCapabilitiesDeepCopy(t *testing.T) {
	reg := New()

	originalCaps := []string{"compute", "storage"}
	err := reg.RegisterAgent(&AgentInfo{
		ID:           "test-agent",
		Capabilities: originalCaps,
		Endpoint:     "localhost:8080",
	})
	if err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// Get the agent
	agent, err := reg.GetAgent("test-agent")
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	// Modify the returned capabilities
	agent.Capabilities[0] = "modified"

	// Get the agent again
	agent2, err := reg.GetAgent("test-agent")
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	// The original capabilities should be unchanged
	if agent2.Capabilities[0] != "compute" {
		t.Errorf("Capabilities were not deep copied: got %s, want compute",
			agent2.Capabilities[0])
	}
}

func TestRegisterValidatorHeartbeatAndList(t *testing.T) {
	reg := New()

	validators := []struct {
		id       string
		endpoint string
	}{
		{"validator-b", "127.0.0.1:9096"},
		{"validator-a", "127.0.0.1:9095"},
	}

	for _, v := range validators {
		if err := reg.RegisterValidator(&ValidatorInfo{ID: v.id, Endpoint: v.endpoint}); err != nil {
			t.Fatalf("register validator %s failed: %v", v.id, err)
		}
	}

	list := reg.ListValidators()
	if len(list) != 2 {
		t.Fatalf("expected 2 validators, got %d", len(list))
	}
	if list[0].ID != "validator-a" || list[1].ID != "validator-b" {
		t.Fatalf("validators not sorted by ID: %v", []string{list[0].ID, list[1].ID})
	}

	list[0].Endpoint = "mutated"
	listAgain := reg.ListValidators()
	if listAgain[0].Endpoint == "mutated" {
		t.Fatalf("ListValidators returned shared slice")
	}

	before, err := reg.GetValidator("validator-a")
	if err != nil {
		t.Fatalf("get validator failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := reg.UpdateValidatorHeartbeat("validator-a"); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	after, err := reg.GetValidator("validator-a")
	if err != nil {
		t.Fatalf("get validator after heartbeat failed: %v", err)
	}

	if !after.LastSeen.After(before.LastSeen) {
		t.Fatalf("heartbeat did not update last seen time")
	}
	if after.Status != pb.AgentStatus_AGENT_STATUS_ACTIVE {
		t.Fatalf("expected status active, got %s", after.Status)
	}
}

func TestRegistryServiceHandleValidators(t *testing.T) {
	svc := NewService("", "")

	registerPayload := map[string]string{
		"id":       "validator-1",
		"endpoint": "127.0.0.1:9095",
	}
	body, err := json.Marshal(registerPayload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/validators", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	svc.handleValidators(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/validators", nil)
	getResp := httptest.NewRecorder()
	svc.handleValidators(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getResp.Code)
	}

	var listResp struct {
		Validators []struct {
			ID       string `json:"id"`
			Endpoint string `json:"endpoint"`
			Status   string `json:"status"`
			LastSeen int64  `json:"last_seen"`
		}
	}
	if err := json.Unmarshal(getResp.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response failed: %v", err)
	}

	if len(listResp.Validators) != 1 {
		t.Fatalf("expected 1 validator, got %d", len(listResp.Validators))
	}
	validator := listResp.Validators[0]
	if validator.ID != "validator-1" {
		t.Fatalf("unexpected validator id: %s", validator.ID)
	}
	if validator.Status != pb.AgentStatus_AGENT_STATUS_ACTIVE.String() {
		t.Fatalf("unexpected status: %s", validator.Status)
	}
	if validator.LastSeen == 0 {
		t.Fatalf("expected last_seen to be set")
	}

	hbReq := httptest.NewRequest(http.MethodPost, "/validators/validator-1/heartbeat", nil)
	hbResp := httptest.NewRecorder()
	svc.handleValidatorByID(hbResp, hbReq)
	if hbResp.Code != http.StatusOK {
		t.Fatalf("heartbeat handler expected 200, got %d", hbResp.Code)
	}

	getByIDReq := httptest.NewRequest(http.MethodGet, "/validators/validator-1", nil)
	getByIDResp := httptest.NewRecorder()
	svc.handleValidatorByID(getByIDResp, getByIDReq)
	if getByIDResp.Code != http.StatusOK {
		t.Fatalf("validator get handler expected 200, got %d", getByIDResp.Code)
	}

	var single map[string]interface{}
	if err := json.Unmarshal(getByIDResp.Body.Bytes(), &single); err != nil {
		t.Fatalf("decode validator response failed: %v", err)
	}
	if single["endpoint"] != "127.0.0.1:9095" {
		t.Fatalf("unexpected endpoint: %v", single["endpoint"])
	}
}

// TestDiscoverAgentsSorted verifies that DiscoverAgents returns sorted results
func TestDiscoverAgentsSorted(t *testing.T) {
	reg := New()

	// Register agents with matching capabilities
	agents := []string{"zebra", "alpha", "beta", "gamma"}
	for _, id := range agents {
		err := reg.RegisterAgent(&AgentInfo{
			ID:           id,
			Capabilities: []string{"compute"},
			Endpoint:     "localhost:8080",
		})
		if err != nil {
			t.Fatalf("Failed to register agent %s: %v", id, err)
		}
	}

	// Discover agents
	discovered, err := reg.DiscoverAgents([]string{"compute"})
	if err != nil {
		t.Fatalf("Failed to discover agents: %v", err)
	}

	// Verify sorted order
	for i := 1; i < len(discovered); i++ {
		if discovered[i-1].ID >= discovered[i].ID {
			t.Errorf("Agents not sorted: %s >= %s at position %d",
				discovered[i-1].ID, discovered[i].ID, i)
		}
	}

	// Expected order
	expectedOrder := []string{"alpha", "beta", "gamma", "zebra"}
	if len(discovered) != len(expectedOrder) {
		t.Fatalf("Wrong number of agents: got %d, want %d",
			len(discovered), len(expectedOrder))
	}

	for i, agent := range discovered {
		if agent.ID != expectedOrder[i] {
			t.Errorf("Wrong order at position %d: got %s, want %s",
				i, agent.ID, expectedOrder[i])
		}
	}
}
