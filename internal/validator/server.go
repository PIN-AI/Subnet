package validator

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"subnet/internal/blockchain"
	"subnet/internal/logging"
	pb "subnet/proto/subnet"
)

// Server implements the ValidatorService gRPC server
type Server struct {
	pb.UnimplementedValidatorServiceServer

	node            *Node
	logger          logging.Logger
	server          *grpc.Server
	verifier        *blockchain.ParticipantVerifier
	allowUnverified bool

	// Rate limiting
	reportRateLimit map[string]time.Time
	rateLimitMu     sync.RWMutex
}

// NewServer creates a new validator gRPC server
func NewServer(node *Node, logger logging.Logger) *Server {
	return &Server{
		node:            node,
		logger:          logger,
		reportRateLimit: make(map[string]time.Time),
	}
}

// AttachBlockchainVerifier wires the on-chain verifier into the validator service.
func (s *Server) AttachBlockchainVerifier(verifier *blockchain.ParticipantVerifier, allowUnverified bool) {
	if s == nil {
		return
	}
	s.verifier = verifier
	s.allowUnverified = allowUnverified
}

// Start starts the gRPC server
func (s *Server) Start(port int) error {
	return s.StartWithAuth(port, nil)
}

// StartWithAuth starts the gRPC server with optional authentication
func (s *Server) StartWithAuth(port int, authInterceptor *AuthInterceptor) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// Build server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(10 * 1024 * 1024), // 10MB
		grpc.MaxSendMsgSize(10 * 1024 * 1024),
	}

	// Add authentication interceptor if provided
	if authInterceptor != nil {
		opts = append(opts, grpc.UnaryInterceptor(authInterceptor.UnaryServerInterceptor()))
		s.logger.Info("Authentication enabled for gRPC server")
	} else {
		s.logger.Warn("Starting gRPC server WITHOUT authentication - not for production!")
	}

	s.server = grpc.NewServer(opts...)

	pb.RegisterValidatorServiceServer(s.server, s)

	go func() {
		if err := s.server.Serve(lis); err != nil {
			s.logger.Error("gRPC server failed", "error", err)
		}
	}()

	s.logger.Info("Validator gRPC server started", "port", port)
	return nil
}

// Stop stops the gRPC server
func (s *Server) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
	if s.verifier != nil {
		s.verifier.Close()
	}
}

// SubmitExecutionReport handles execution report submission from agents
func (s *Server) SubmitExecutionReport(ctx context.Context, report *pb.ExecutionReport) (*pb.Receipt, error) {
	if report == nil {
		return nil, status.Error(codes.InvalidArgument, "execution report is required")
	}
	// Validate report
	if err := s.validateExecutionReport(report); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid report: %v", err)
	}

	if s.verifier != nil {
		addr := extractChainAddressFromReport(report)
		verified, err := s.verifier.VerifyAgent(ctx, addr)
		if err != nil {
			s.logger.Warnf("On-chain agent verification failed for %s: %v", report.AgentId, err)
			if !s.allowUnverified {
				return nil, status.Error(codes.PermissionDenied, "agent verification failed")
			}
		} else if !verified {
			s.logger.Warnf("Agent %s not active on-chain (addr=%s)", report.AgentId, addr)
			if !s.allowUnverified {
				return nil, status.Error(codes.PermissionDenied, "agent not registered on-chain")
			}
		}
	}

	// Check rate limiting
	if !s.checkRateLimit(report.AgentId) {
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Process report
	receipt, err := s.node.ProcessExecutionReport(report)
	if err != nil {
		s.logger.Error("Failed to process execution report",
			"agent", report.AgentId,
			"assignment", report.AssignmentId,
			"error", err)
		return nil, status.Errorf(codes.Internal, "failed to process report: %v", err)
	}

	s.logger.Info("Processed execution report",
		"agent", report.AgentId,
		"assignment", report.AssignmentId,
		"receipt", receipt.ReportId)

	return receipt, nil
}

// GetCheckpoint retrieves checkpoint by epoch
func (s *Server) GetCheckpoint(ctx context.Context, req *pb.GetCheckpointRequest) (*pb.CheckpointHeader, error) {
	epoch := req.GetEpoch()
	checkpoint, err := s.node.GetCheckpoint(epoch)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "checkpoint not found: %v", err)
	}
	return checkpoint, nil
}

// ProposeHeader handles checkpoint header proposal
func (s *Server) ProposeHeader(ctx context.Context, header *pb.CheckpointHeader) (*pb.Ack, error) {
	// Only leader can propose
	if !s.node.IsLeader(header.Epoch) {
		return nil, status.Error(codes.PermissionDenied, "not the leader for this epoch")
	}

	if err := s.node.HandleProposal(header); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to handle proposal: %v", err)
	}

	return &pb.Ack{
		Ok:  true,
		Msg: "Proposal accepted",
	}, nil
}

// SubmitSignature handles signature submission for checkpoint
func (s *Server) SubmitSignature(ctx context.Context, submission *pb.SignatureSubmission) (*pb.Ack, error) {
	if err := s.node.AddSignature(submission.Signature); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to add signature: %v", err)
	}

	return &pb.Ack{
		Ok:  true,
		Msg: "Signature accepted",
	}, nil
}

// GetSignatures retrieves signatures for a checkpoint
func (s *Server) GetSignatures(req *pb.GetCheckpointRequest, stream pb.ValidatorService_GetSignaturesServer) error {
	signatures := s.node.GetSignatures(req.GetEpoch())

	for _, sig := range signatures {
		if err := stream.Send(sig); err != nil {
			return err
		}
	}

	return nil
}

// GetValidatorSet returns the current validator set
func (s *Server) GetValidatorSet(ctx context.Context, req *pb.GetCheckpointRequest) (*pb.ValidatorSet, error) {
	// Convert internal ValidatorSet to proto
	vset := s.node.GetValidatorSet()

	pbValidators := make([]*pb.Validator, len(vset.Validators))
	for i, v := range vset.Validators {
		pbValidators[i] = &pb.Validator{
			Id:     v.ID,
			Pubkey: v.PubKey,
			Weight: v.Weight,
		}
	}

	return &pb.ValidatorSet{
		Validators:     pbValidators,
		MinValidators:  int32(vset.MinValidators),
		ThresholdNum:   int32(vset.ThresholdNum),
		ThresholdDenom: int32(vset.ThresholdDenom),
	}, nil
}

// GetDoubleSignEvidences returns double sign evidences
func (s *Server) GetDoubleSignEvidences(req *pb.DoubleSignQuery, stream pb.ValidatorService_GetDoubleSignEvidencesServer) error {
	evidences := s.node.GetDoubleSignEvidences(req.ValidatorId, req.Epoch, 0)

	for _, evidence := range evidences {
		if err := stream.Send(evidence); err != nil {
			return err
		}
	}

	return nil
}

// GetValidationPolicy returns the validation policy
func (s *Server) GetValidationPolicy(ctx context.Context, req *pb.GetValidationPolicyRequest) (*pb.ValidationPolicy, error) {
	policy := s.node.GetValidationPolicy()
	if policy == nil {
		return nil, status.Error(codes.NotFound, "validation policy not found")
	}
	return policy, nil
}

// GetVerificationRecords returns verification records
func (s *Server) GetVerificationRecords(req *pb.GetVerificationRecordsRequest, stream pb.ValidatorService_GetVerificationRecordsServer) error {
	records := s.node.GetVerificationRecords(req.IntentId, 0, 0)

	for _, record := range records {
		if err := stream.Send(record); err != nil {
			return err
		}
	}

	return nil
}

// GetValidatorMetrics returns validator metrics
func (s *Server) GetValidatorMetrics(ctx context.Context, req *pb.GetValidatorMetricsRequest) (*pb.ValidatorMetrics, error) {
	metrics := s.node.GetMetrics()
	return metrics, nil
}

// validateExecutionReport validates an execution report
func (s *Server) validateExecutionReport(report *pb.ExecutionReport) error {
	if report.AssignmentId == "" {
		return fmt.Errorf("assignment ID is required")
	}
	if report.AgentId == "" {
		return fmt.Errorf("agent ID is required")
	}
	if report.Timestamp == 0 {
		return fmt.Errorf("execution timestamp is required")
	}
	return nil
}

// checkRateLimit checks if agent has exceeded rate limit
func (s *Server) checkRateLimit(agentID string) bool {
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	lastTime, exists := s.reportRateLimit[agentID]
	now := time.Now()

	// Allow one report per second per agent
	if exists && now.Sub(lastTime) < time.Second {
		return false
	}

	s.reportRateLimit[agentID] = now
	return true
}

func extractChainAddressFromReport(report *pb.ExecutionReport) string {
	if report == nil {
		return ""
	}
	return strings.TrimSpace(report.AgentId)
}
