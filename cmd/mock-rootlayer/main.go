package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	rootpb "rootlayer/proto"
	"subnet/internal/logging"
	"subnet/internal/rootlayer"
)

// MockRootLayerServer provides a mock RootLayer HTTP API
type MockRootLayerServer struct {
	logger          logging.Logger
	intentGenerator *rootlayer.IntentGenerator
	mu              sync.RWMutex
	intents         []*rootpb.Intent
	assignments     map[string]*rootpb.Assignment
}

// NewMockRootLayerServer creates a new mock server
func NewMockRootLayerServer(logger logging.Logger) *MockRootLayerServer {
	server := &MockRootLayerServer{
		logger:          logger,
		intentGenerator: rootlayer.NewIntentGenerator(),
		intents:         make([]*rootpb.Intent, 0),
		assignments:     make(map[string]*rootpb.Assignment),
	}

	// Generate some initial intents
	for i := 0; i < 5; i++ {
		intent := server.intentGenerator.GenerateIntent("subnet-1")
		server.intents = append(server.intents, intent)
	}

	// Start periodic intent generation
	go server.generateIntentsPeriodically()

	return server
}

// generateIntentsPeriodically generates new intents every 15 seconds
func (s *MockRootLayerServer) generateIntentsPeriodically() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		intent := s.intentGenerator.GenerateIntent("subnet-1")
		s.intents = append(s.intents, intent)
		s.logger.Infof("Generated new intent: %s", intent.IntentId)
		s.mu.Unlock()
	}
}

// HandleGetIntents handles GET /api/v1/intents
func (s *MockRootLayerServer) HandleGetIntents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subnetID := r.URL.Query().Get("subnet_id")
	if subnetID == "" {
		subnetID = "subnet-1"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter intents by subnet and status
	pendingIntents := make([]*rootpb.Intent, 0)
	for _, intent := range s.intents {
		if intent.SubnetId == subnetID && intent.Status == rootpb.IntentStatus_INTENT_STATUS_PENDING {
			pendingIntents = append(pendingIntents, intent)
		}
	}

	response := map[string]interface{}{
		"intents": pendingIntents,
		"count":   len(pendingIntents),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	s.logger.Infof("Served %d pending intents for subnet %s", len(pendingIntents), subnetID)
}

// HandleSubmitAssignment handles POST /api/v1/assignments
func (s *MockRootLayerServer) HandleSubmitAssignment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var assignment rootpb.Assignment
	if err := json.NewDecoder(r.Body).Decode(&assignment); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Store assignment
	s.assignments[assignment.AssignmentId] = &assignment

	// Update intent status
	for _, intent := range s.intents {
		if intent.IntentId == assignment.IntentId {
			intent.Status = rootpb.IntentStatus_INTENT_STATUS_PROCESSING
			break
		}
	}

	response := map[string]interface{}{
		"success":       true,
		"assignment_id": assignment.AssignmentId,
		"message":       fmt.Sprintf("Assignment accepted for intent %s", assignment.IntentId),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	s.logger.Infof("Received assignment %s for intent %s (agent: %s)",
		assignment.AssignmentId, assignment.IntentId, assignment.AgentId)
}

// HandleGetAssignments handles GET /api/v1/assignments
func (s *MockRootLayerServer) HandleGetAssignments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	assignmentList := make([]*rootpb.Assignment, 0, len(s.assignments))
	for _, assignment := range s.assignments {
		assignmentList = append(assignmentList, assignment)
	}

	response := map[string]interface{}{
		"assignments": assignmentList,
		"count":       len(assignmentList),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleWebSocket handles WebSocket connections for real-time intent streaming
func (s *MockRootLayerServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// For simplicity, we'll use Server-Sent Events instead of WebSocket
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send existing intents first
	s.mu.RLock()
	for _, intent := range s.intents {
		if intent.Status == rootpb.IntentStatus_INTENT_STATUS_PENDING {
			data, _ := json.Marshal(intent)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
	s.mu.RUnlock()

	// Keep connection alive and send new intents
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"time\": \"%s\"}\n\n", time.Now().Format(time.RFC3339))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// HandleStatus handles GET /status
func (s *MockRootLayerServer) HandleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"status":           "healthy",
		"total_intents":    len(s.intents),
		"pending_intents":  s.countPendingIntents(),
		"total_assignments": len(s.assignments),
		"uptime":           time.Since(startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *MockRootLayerServer) countPendingIntents() int {
	count := 0
	for _, intent := range s.intents {
		if intent.Status == rootpb.IntentStatus_INTENT_STATUS_PENDING {
			count++
		}
	}
	return count
}

var startTime = time.Now()

func main() {
	var (
		httpAddr = flag.String("http", ":9090", "HTTP server address")
	)
	flag.Parse()

	// Setup logger
	logger := logging.NewDefaultLogger()
	logger.Info("Starting Mock RootLayer Server")

	// Create server
	server := NewMockRootLayerServer(logger)

	// Setup routes
	http.HandleFunc("/api/v1/intents", server.HandleGetIntents)
	http.HandleFunc("/api/v1/assignments", server.HandleSubmitAssignment)
	http.HandleFunc("/api/v1/assignments/list", server.HandleGetAssignments)
	http.HandleFunc("/api/v1/stream", server.HandleWebSocket)
	http.HandleFunc("/status", server.HandleStatus)

	// Root handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"service": "Mock RootLayer",
			"version": "1.0.0",
			"endpoints": "GET /api/v1/intents, POST /api/v1/assignments, GET /api/v1/stream, GET /status",
		})
	})

	logger.Infof("Mock RootLayer server listening on %s", *httpAddr)
	logger.Info("")
	logger.Info("Available endpoints:")
	logger.Info("  GET  /api/v1/intents?subnet_id=subnet-1  - Get pending intents")
	logger.Info("  POST /api/v1/assignments                 - Submit assignment")
	logger.Info("  GET  /api/v1/assignments/list            - List all assignments")
	logger.Info("  GET  /api/v1/stream                      - Stream intents (SSE)")
	logger.Info("  GET  /status                             - Server status")
	logger.Info("")

	if err := http.ListenAndServe(*httpAddr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}