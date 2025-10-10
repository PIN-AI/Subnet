package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "subnet/proto/subnet"
)

type RegistryService struct {
	pb.UnimplementedRegistryServiceServer
	registry   *Registry
	grpcServer *grpc.Server
	grpcAddr   string
	httpServer *http.Server
	httpAddr   string
}

func NewService(grpcAddr, httpAddr string) *RegistryService {
	if grpcAddr == "" {
		grpcAddr = ":8091"
	}
	if httpAddr == "" {
		httpAddr = ":8092"
	}

	return &RegistryService{
		registry: New(),
		grpcAddr: grpcAddr,
		httpAddr: httpAddr,
	}
}

func (s *RegistryService) Start() error {
	if s.grpcAddr != "" {
		listener, err := net.Listen("tcp", s.grpcAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", s.grpcAddr, err)
		}

		s.grpcServer = grpc.NewServer()
		pb.RegisterRegistryServiceServer(s.grpcServer, s)

		go func() {
			log.Printf("Registry gRPC service starting on %s", s.grpcAddr)
			if err := s.grpcServer.Serve(listener); err != nil {
				log.Printf("Registry gRPC service error: %v", err)
			}
		}()
	}

	if s.httpAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/agents", s.handleAgents)
		mux.HandleFunc("/agents/", s.handleAgentByID)
		mux.HandleFunc("/validators", s.handleValidators)
		mux.HandleFunc("/validators/", s.handleValidatorByID)

		s.httpServer = &http.Server{
			Addr:    s.httpAddr,
			Handler: mux,
		}

		go func() {
			log.Printf("Registry HTTP service starting on %s", s.httpAddr)
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Registry HTTP service error: %v", err)
			}
		}()
	}

	go s.periodicCleanup()

	return nil
}

func (s *RegistryService) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
			log.Printf("Registry HTTP shutdown error: %v", err)
		}
	}
}

func (s *RegistryService) RegisterAgent(ctx context.Context,
	req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {

	if req.Agent == nil {
		return nil, status.Error(codes.InvalidArgument, "agent info required")
	}

	if req.Agent.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "agent ID required")
	}

	if len(req.Agent.Capabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "agent capabilities required")
	}

	info := &AgentInfo{
		ID:           req.Agent.Id,
		Capabilities: req.Agent.Capabilities,
		Endpoint:     req.Agent.Endpoint,
		LastSeen:     time.Now(),
		Status:       pb.AgentStatus_AGENT_STATUS_ACTIVE,
	}

	if err := s.registry.RegisterAgent(info); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to register agent: %v", err)
	}

	log.Printf("Registered agent: %s with capabilities: %v", info.ID, info.Capabilities)

	return &pb.RegisterAgentResponse{
		Success: true,
		Message: fmt.Sprintf("Agent %s registered successfully", req.Agent.Id),
	}, nil
}

func (s *RegistryService) UnregisterAgent(ctx context.Context,
	req *pb.UnregisterAgentRequest) (*pb.UnregisterAgentResponse, error) {

	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent ID required")
	}

	if err := s.registry.RemoveAgent(req.AgentId); err != nil {
		return nil, status.Errorf(codes.NotFound, "agent not found: %v", err)
	}

	log.Printf("Unregistered agent: %s", req.AgentId)

	return &pb.UnregisterAgentResponse{
		Success: true,
		Message: fmt.Sprintf("Agent %s unregistered successfully", req.AgentId),
	}, nil
}

func (s *RegistryService) DiscoverAgents(ctx context.Context,
	req *pb.DiscoverAgentsRequest) (*pb.DiscoverAgentsResponse, error) {

	agents, err := s.registry.DiscoverAgents(req.Capabilities)
	if err != nil {
		return &pb.DiscoverAgentsResponse{
			Agents: []*pb.Agent{},
		}, nil
	}

	protoAgents := make([]*pb.Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.IsHealthy() {
			protoAgents = append(protoAgents, &pb.Agent{
				Id:           agent.ID,
				Capabilities: agent.Capabilities,
				Endpoint:     agent.Endpoint,
				Status:       agent.Status,
				LastSeen:     agent.LastSeen.Unix(),
			})
		}
	}

	return &pb.DiscoverAgentsResponse{
		Agents: protoAgents,
	}, nil
}

func (s *RegistryService) Heartbeat(ctx context.Context,
	req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {

	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent ID required")
	}

	if err := s.registry.UpdateHeartbeat(req.AgentId); err != nil {
		return nil, status.Errorf(codes.NotFound, "agent not found: %v", err)
	}

	return &pb.HeartbeatResponse{
		Success: true,
		Message: "Heartbeat received",
	}, nil
}

func (s *RegistryService) GetAgent(ctx context.Context,
	req *pb.GetAgentRequest) (*pb.GetAgentResponse, error) {

	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent ID required")
	}

	agent, err := s.registry.GetAgent(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "agent not found: %v", err)
	}

	return &pb.GetAgentResponse{
		Agent: &pb.Agent{
			Id:           agent.ID,
			Capabilities: agent.Capabilities,
			Endpoint:     agent.Endpoint,
			Status:       agent.Status,
			LastSeen:     agent.LastSeen.Unix(),
		},
	}, nil
}

func (s *RegistryService) ListAgents(ctx context.Context,
	req *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {

	agents := s.registry.ListAgents()

	protoAgents := make([]*pb.Agent, 0, len(agents))
	for _, agent := range agents {
		protoAgents = append(protoAgents, &pb.Agent{
			Id:           agent.ID,
			Capabilities: agent.Capabilities,
			Endpoint:     agent.Endpoint,
			Status:       agent.Status,
			LastSeen:     agent.LastSeen.Unix(),
		})
	}

	return &pb.ListAgentsResponse{
		Agents: protoAgents,
		Total:  int32(len(protoAgents)),
	}, nil
}

func (s *RegistryService) periodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		removed := s.registry.CleanupStaleAgents(10 * time.Minute)
		if removed > 0 {
			log.Printf("Cleaned up %d stale agents", removed)
		}
	}
}

func (s *RegistryService) GetRegistry() *Registry {
	return s.registry
}

func (s *RegistryService) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
			Endpoint     string   `json:"endpoint"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}

		if req.ID == "" || len(req.Capabilities) == 0 {
			writeError(w, http.StatusBadRequest, "id and capabilities are required")
			return
		}

		agent := &AgentInfo{
			ID:           req.ID,
			Capabilities: req.Capabilities,
			Endpoint:     req.Endpoint,
			LastSeen:     time.Now(),
			Status:       pb.AgentStatus_AGENT_STATUS_ACTIVE,
		}

		if err := s.registry.RegisterAgent(agent); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"message": "registered"})

	case http.MethodGet:
		agents := s.registry.ListAgents()
		resp := make([]map[string]interface{}, 0, len(agents))
		for _, agent := range agents {
			resp = append(resp, map[string]interface{}{
				"id":           agent.ID,
				"capabilities": agent.Capabilities,
				"endpoint":     agent.Endpoint,
				"status":       agent.Status.String(),
				"last_seen":    agent.LastSeen.Unix(),
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"agents": resp})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *RegistryService) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	penance := strings.TrimPrefix(r.URL.Path, "/agents/")
	if penance == r.URL.Path {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	parts := strings.SplitN(penance, "/", 2)
	agentID := parts[0]
	var subPath string
	if len(parts) > 1 {
		subPath = parts[1]
	}

	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent id required")
		return
	}

	switch {
	case subPath == "" && r.Method == http.MethodDelete:
		if err := s.registry.RemoveAgent(agentID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "unregistered"})
	case subPath == "" && r.Method == http.MethodGet:
		agent, err := s.registry.GetAgent(agentID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":           agent.ID,
			"capabilities": agent.Capabilities,
			"endpoint":     agent.Endpoint,
			"status":       agent.Status.String(),
			"last_seen":    agent.LastSeen.Unix(),
		})
	case subPath == "heartbeat" && r.Method == http.MethodPost:
		if err := s.registry.UpdateHeartbeat(agentID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "heartbeat"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *RegistryService) handleValidators(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			ID       string `json:"id"`
			Endpoint string `json:"endpoint"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}

		if req.ID == "" || req.Endpoint == "" {
			writeError(w, http.StatusBadRequest, "id and endpoint are required")
			return
		}

		info := &ValidatorInfo{
			ID:       req.ID,
			Endpoint: req.Endpoint,
			LastSeen: time.Now(),
			Status:   pb.AgentStatus_AGENT_STATUS_ACTIVE,
		}

		if err := s.registry.RegisterValidator(info); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"message": "registered"})

	case http.MethodGet:
		validators := s.registry.ListValidators()
		resp := make([]map[string]interface{}, 0, len(validators))
		for _, v := range validators {
			resp = append(resp, map[string]interface{}{
				"id":        v.ID,
				"endpoint":  v.Endpoint,
				"status":    v.Status.String(),
				"last_seen": v.LastSeen.Unix(),
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"validators": resp})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *RegistryService) handleValidatorByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/validators/")
	if path == r.URL.Path {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	validatorID := parts[0]
	var subPath string
	if len(parts) > 1 {
		subPath = parts[1]
	}

	if validatorID == "" {
		writeError(w, http.StatusBadRequest, "validator id required")
		return
	}

	switch {
	case subPath == "" && r.Method == http.MethodDelete:
		if err := s.registry.RemoveValidator(validatorID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "unregistered"})

	case subPath == "" && r.Method == http.MethodGet:
		validator, err := s.registry.GetValidator(validatorID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":        validator.ID,
			"endpoint":  validator.Endpoint,
			"status":    validator.Status.String(),
			"last_seen": validator.LastSeen.Unix(),
		})

	case subPath == "heartbeat" && r.Method == http.MethodPost:
		if err := s.registry.UpdateValidatorHeartbeat(validatorID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "heartbeat"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
