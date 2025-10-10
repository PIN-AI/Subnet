package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	pb "subnet/proto/subnet"
	rootpb "rootlayer/proto"
)

// MessageSigner handles signing of protocol messages
type MessageSigner struct {
	signer  ExtendedSigner
	chainID string
}

// NewMessageSigner creates a new message signer
func NewMessageSigner(signer ExtendedSigner, chainID string) *MessageSigner {
	return &MessageSigner{
		signer:  signer,
		chainID: chainID,
	}
}

// SignBid signs a bid message
func (ms *MessageSigner) SignBid(bid *pb.Bid) error {
	if bid == nil {
		return fmt.Errorf("bid is nil")
	}

	// Create canonical bid message
	canonicalBid := ms.createCanonicalBid(bid)

	// Sign the canonical message
	signature, err := ms.signer.SignMessage(canonicalBid)
	if err != nil {
		return fmt.Errorf("failed to sign bid: %w", err)
	}

	bid.Signature = signature
	return nil
}

// SignExecutionTask signs an execution task
func (ms *MessageSigner) SignExecutionTask(task *pb.ExecutionTask) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	// Create canonical task message
	canonicalTask := ms.createCanonicalTask(task)

	// Sign the canonical message
	signature, err := ms.signer.SignMessage(canonicalTask)
	if err != nil {
		return nil, fmt.Errorf("failed to sign execution task: %w", err)
	}

	// Return signature separately since ExecutionTask doesn't have a Signature field
	return signature, nil
}

// SignTaskResponse signs a task response
func (ms *MessageSigner) SignTaskResponse(response *pb.TaskResponse) ([]byte, error) {
	if response == nil {
		return nil, fmt.Errorf("response is nil")
	}

	// Create canonical response message
	canonicalResponse := ms.createCanonicalTaskResponse(response)

	// Sign the canonical message
	signature, err := ms.signer.SignMessage(canonicalResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to sign task response: %w", err)
	}

	// Return signature separately since TaskResponse doesn't have a Signature field
	return signature, nil
}

// SignAssignment signs an assignment
func (ms *MessageSigner) SignAssignment(assignment *rootpb.Assignment) error {
	if assignment == nil {
		return fmt.Errorf("assignment is nil")
	}

	// Create canonical assignment message
	canonicalAssignment := ms.createCanonicalAssignment(assignment)

	// Sign the canonical message
	signature, err := ms.signer.SignMessage(canonicalAssignment)
	if err != nil {
		return fmt.Errorf("failed to sign assignment: %w", err)
	}

	assignment.Signature = signature
	return nil
}

// SignMatchingResult signs a matching result
func (ms *MessageSigner) SignMatchingResult(result *pb.MatchingResult) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("result is nil")
	}

	// Create canonical result message
	canonicalResult := ms.createCanonicalMatchingResult(result)

	// Sign the canonical message
	signature, err := ms.signer.SignMessage(canonicalResult)
	if err != nil {
		return nil, fmt.Errorf("failed to sign matching result: %w", err)
	}

	// MatchingResult already has MatcherSignature field
	result.MatcherSignature = signature
	return signature, nil
}

// SignExecutionReport signs an execution report
func (ms *MessageSigner) SignExecutionReport(report *pb.ExecutionReport) error {
	if report == nil {
		return fmt.Errorf("report is nil")
	}

	// Create canonical report message
	canonicalReport := ms.createCanonicalExecutionReport(report)

	// Sign the canonical message
	signature, err := ms.signer.SignMessage(canonicalReport)
	if err != nil {
		return fmt.Errorf("failed to sign execution report: %w", err)
	}

	report.Signature = signature
	return nil
}

// Canonical message creation functions

func (ms *MessageSigner) createCanonicalBid(bid *pb.Bid) []byte {
	// Create deterministic canonical form
	canonical := struct {
		ChainID     string `json:"chain_id"`
		BidID       string `json:"bid_id"`
		IntentID    string `json:"intent_id"`
		AgentID     string `json:"agent_id"`
		Price       uint64 `json:"price"`
		Token       string `json:"token"`
		SubmittedAt int64  `json:"submitted_at"`
		Nonce       string `json:"nonce"`
	}{
		ChainID:     ms.chainID,
		BidID:       bid.BidId,
		IntentID:    bid.IntentId,
		AgentID:     bid.AgentId,
		Price:       bid.Price,
		Token:       bid.Token,
		SubmittedAt: bid.SubmittedAt,
		Nonce:       bid.Nonce,
	}

	data, _ := json.Marshal(canonical)
	return data
}

func (ms *MessageSigner) createCanonicalTask(task *pb.ExecutionTask) []byte {
	canonical := struct {
		ChainID    string `json:"chain_id"`
		TaskID     string `json:"task_id"`
		IntentID   string `json:"intent_id"`
		AgentID    string `json:"agent_id"`
		BidID      string `json:"bid_id"`
		CreatedAt  int64  `json:"created_at"`
		Deadline   int64  `json:"deadline"`
		IntentType string `json:"intent_type"`
	}{
		ChainID:    ms.chainID,
		TaskID:     task.TaskId,
		IntentID:   task.IntentId,
		AgentID:    task.AgentId,
		BidID:      task.BidId,
		CreatedAt:  task.CreatedAt,
		Deadline:   task.Deadline,
		IntentType: task.IntentType,
	}

	data, _ := json.Marshal(canonical)
	return data
}

func (ms *MessageSigner) createCanonicalTaskResponse(response *pb.TaskResponse) []byte {
	canonical := struct {
		ChainID  string `json:"chain_id"`
		TaskID   string `json:"task_id"`
		AgentID  string `json:"agent_id"`
		Accepted bool   `json:"accepted"`
		Reason   string `json:"reason"`
	}{
		ChainID:  ms.chainID,
		TaskID:   response.TaskId,
		AgentID:  response.AgentId,
		Accepted: response.Accepted,
		Reason:   response.Reason,
	}

	data, _ := json.Marshal(canonical)
	return data
}

func (ms *MessageSigner) createCanonicalAssignment(assignment *rootpb.Assignment) []byte {
	canonical := struct {
		ChainID      string `json:"chain_id"`
		AssignmentID string `json:"assignment_id"`
		IntentID     string `json:"intent_id"`
		AgentID      string `json:"agent_id"`
		BidID        string `json:"bid_id"`
		MatcherID    string `json:"matcher_id"`
		Status       string `json:"status"`
	}{
		ChainID:      ms.chainID,
		AssignmentID: assignment.AssignmentId,
		IntentID:     assignment.IntentId,
		AgentID:      assignment.AgentId,
		BidID:        assignment.BidId,
		MatcherID:    assignment.MatcherId,
		Status:       assignment.Status.String(),
	}

	data, _ := json.Marshal(canonical)
	return data
}

func (ms *MessageSigner) createCanonicalMatchingResult(result *pb.MatchingResult) []byte {
	canonical := struct {
		ChainID        string   `json:"chain_id"`
		IntentID       string   `json:"intent_id"`
		WinningBidID   string   `json:"winning_bid_id"`
		WinningAgentID string   `json:"winning_agent_id"`
		MatcherID      string   `json:"matcher_id"`
		MatchedAt      int64    `json:"matched_at"`
		RunnerUpBids   []string `json:"runner_up_bids"`
	}{
		ChainID:        ms.chainID,
		IntentID:       result.IntentId,
		WinningBidID:   result.WinningBidId,
		WinningAgentID: result.WinningAgentId,
		MatcherID:      result.MatcherId,
		MatchedAt:      result.MatchedAt,
		RunnerUpBids:   result.RunnerUpBidIds,
	}

	data, _ := json.Marshal(canonical)
	return data
}

func (ms *MessageSigner) createCanonicalExecutionReport(report *pb.ExecutionReport) []byte {
	canonical := struct {
		ChainID      string `json:"chain_id"`
		AssignmentID string `json:"assignment_id"`
		IntentID     string `json:"intent_id"`
		AgentID      string `json:"agent_id"`
		Status       string `json:"status"`
		Timestamp    int64  `json:"timestamp"`
		ResultHash   string `json:"result_hash"`
	}{
		ChainID:      ms.chainID,
		AssignmentID: report.AssignmentId,
		IntentID:     report.IntentId,
		AgentID:      report.AgentId,
		Status:       report.Status.String(),
		Timestamp:    report.Timestamp,
		ResultHash:   hex.EncodeToString(report.ResultData), // Using ResultData as hash for now
	}

	data, _ := json.Marshal(canonical)
	return data
}

// MessageVerifier handles verification of protocol messages
type MessageVerifier struct {
	verifier ExtendedVerifier
	chainID  string
}

// NewMessageVerifier creates a new message verifier
func NewMessageVerifier(verifier ExtendedVerifier, chainID string) *MessageVerifier {
	return &MessageVerifier{
		verifier: verifier,
		chainID:  chainID,
	}
}

// VerifyBid verifies a bid signature
func (mv *MessageVerifier) VerifyBid(bid *pb.Bid, pubKey []byte) bool {
	if bid == nil || len(bid.Signature) == 0 {
		return false
	}

	signer := &MessageSigner{chainID: mv.chainID}
	canonicalBid := signer.createCanonicalBid(bid)

	return mv.verifier.VerifyMessage(pubKey, canonicalBid, bid.Signature)
}

// VerifyAssignment verifies an assignment signature
func (mv *MessageVerifier) VerifyAssignment(assignment *rootpb.Assignment, pubKey []byte) bool {
	if assignment == nil || len(assignment.Signature) == 0 {
		return false
	}

	signer := &MessageSigner{chainID: mv.chainID}
	canonicalAssignment := signer.createCanonicalAssignment(assignment)

	return mv.verifier.VerifyMessage(pubKey, canonicalAssignment, assignment.Signature)
}

// VerifyMatchingResult verifies a matching result signature
func (mv *MessageVerifier) VerifyMatchingResult(result *pb.MatchingResult, pubKey []byte) bool {
	if result == nil || len(result.MatcherSignature) == 0 {
		return false
	}

	signer := &MessageSigner{chainID: mv.chainID}
	canonicalResult := signer.createCanonicalMatchingResult(result)

	return mv.verifier.VerifyMessage(pubKey, canonicalResult, result.MatcherSignature)
}

// VerifyExecutionReport verifies an execution report signature
func (mv *MessageVerifier) VerifyExecutionReport(report *pb.ExecutionReport, pubKey []byte) bool {
	if report == nil || len(report.Signature) == 0 {
		return false
	}

	signer := &MessageSigner{chainID: mv.chainID}
	canonicalReport := signer.createCanonicalExecutionReport(report)

	return mv.verifier.VerifyMessage(pubKey, canonicalReport, report.Signature)
}

// GenerateNonce generates a random nonce for replay protection
func GenerateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GetTimestamp returns current Unix timestamp
func GetTimestamp() int64 {
	return time.Now().Unix()
}