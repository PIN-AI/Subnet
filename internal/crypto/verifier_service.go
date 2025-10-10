package crypto

import (
	"errors"
	"fmt"
	"sync"

	pb "subnet/proto/subnet"
)

// VerifierService provides a unified interface for all signature verification in the system
type VerifierService struct {
	verifier     ExtendedVerifier
	hasher       *CheckpointHasher
	msgSigner    *MessageSigner
	trustedKeys  map[string][]byte // Map of entity ID to public key
	mu           sync.RWMutex
}

// NewVerifierService creates a new verifier service
func NewVerifierService() *VerifierService {
	return &VerifierService{
		verifier:    NewExtendedVerifier(),
		hasher:      NewCheckpointHasher(),
		trustedKeys: make(map[string][]byte),
	}
}

// AddTrustedKey adds a trusted public key for an entity
func (vs *VerifierService) AddTrustedKey(entityID string, pubKey []byte) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.trustedKeys[entityID] = pubKey
}

// GetTrustedKey retrieves a trusted public key for an entity
func (vs *VerifierService) GetTrustedKey(entityID string) ([]byte, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	pubKey, exists := vs.trustedKeys[entityID]
	if !exists {
		return nil, fmt.Errorf("no trusted key for entity: %s", entityID)
	}
	return pubKey, nil
}

// VerifyCheckpointSignature verifies a signature on a checkpoint header
func (vs *VerifierService) VerifyCheckpointSignature(header *pb.CheckpointHeader, signature []byte, signerID string) error {
	if header == nil {
		return errors.New("checkpoint header is nil")
	}
	if len(signature) == 0 {
		return errors.New("signature is empty")
	}

	// Get trusted public key
	pubKey, err := vs.GetTrustedKey(signerID)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Compute canonical hash
	hash := vs.hasher.ComputeHash(header)

	// Verify signature
	if !vs.verifier.Verify(pubKey, hash[:], signature) {
		return fmt.Errorf("signature verification failed for signer %s", signerID)
	}

	return nil
}

// VerifyBidSignature verifies a signature on a bid
func (vs *VerifierService) VerifyBidSignature(bid *pb.Bid) error {
	if bid == nil {
		return errors.New("bid is nil")
	}
	if len(bid.Signature) == 0 {
		return errors.New("bid signature is empty")
	}

	// Get trusted public key for agent
	pubKey, err := vs.GetTrustedKey(bid.AgentId)
	if err != nil {
		return fmt.Errorf("failed to get agent public key: %w", err)
	}

	// Create canonical bid message
	canonicalBid := createCanonicalBid(bid)

	// Hash the message
	hash := HashMessage(canonicalBid)

	// Verify signature
	if !vs.verifier.Verify(pubKey, hash[:], bid.Signature) {
		return fmt.Errorf("bid signature verification failed for agent %s", bid.AgentId)
	}

	return nil
}


// VerifyExecutionTaskSignature verifies the matcher's signature on an execution task
func (vs *VerifierService) VerifyExecutionTaskSignature(task *pb.ExecutionTask) error {
	if task == nil {
		return errors.New("execution task is nil")
	}

	// ExecutionTask signature verification would require the matcher's signature
	// which should be stored in the task or passed separately
	// For now, return nil as ExecutionTask doesn't have a signature field in proto
	return nil
}

// VerifyExecutionReportSignature verifies a signature on an execution report
func (vs *VerifierService) VerifyExecutionReportSignature(report *pb.ExecutionReport) error {
	if report == nil {
		return errors.New("execution report is nil")
	}

	// Get agent signature from report
	if len(report.Signature) == 0 {
		return errors.New("execution report missing signature")
	}

	// Get trusted public key for agent
	pubKey, err := vs.GetTrustedKey(report.AgentId)
	if err != nil {
		return fmt.Errorf("failed to get agent public key: %w", err)
	}

	// Create canonical report message
	canonicalReport := createCanonicalExecutionReport(report)

	// Hash the message
	hash := HashMessage(canonicalReport)

	// Verify signature
	if !vs.verifier.Verify(pubKey, hash[:], report.Signature) {
		return fmt.Errorf("execution report signature verification failed for agent %s", report.AgentId)
	}

	return nil
}

// VerifyMessageSignature verifies a generic message signature
func (vs *VerifierService) VerifyMessageSignature(message []byte, signature []byte, signerID string) error {
	if len(message) == 0 {
		return errors.New("message is empty")
	}
	if len(signature) == 0 {
		return errors.New("signature is empty")
	}

	// Get trusted public key
	pubKey, err := vs.GetTrustedKey(signerID)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Verify signature on message
	if !vs.verifier.VerifyMessage(pubKey, message, signature) {
		return fmt.Errorf("message signature verification failed for signer %s", signerID)
	}

	return nil
}

// BatchVerifyCheckpointSignatures verifies multiple signatures on the same checkpoint
func (vs *VerifierService) BatchVerifyCheckpointSignatures(header *pb.CheckpointHeader, signatures []*pb.Signature) ([]bool, error) {
	if header == nil {
		return nil, errors.New("checkpoint header is nil")
	}

	// Compute canonical hash once
	hash := vs.hasher.ComputeHash(header)

	results := make([]bool, len(signatures))

	for i, sig := range signatures {
		if sig == nil || len(sig.Der) == 0 {
			results[i] = false
			continue
		}

		// Get trusted public key
		pubKey, err := vs.GetTrustedKey(sig.SignerId)
		if err != nil {
			results[i] = false
			continue
		}

		// Verify signature
		results[i] = vs.verifier.Verify(pubKey, hash[:], sig.Der)
	}

	return results, nil
}

// Helper functions to create canonical messages (matching MessageSigner)

func createCanonicalBid(bid *pb.Bid) []byte {
	// Create deterministic canonical form using available fields
	canonical := fmt.Sprintf("bid:v1|%s|%s|%s|%d|%s|%s|%d|%s",
		bid.BidId,
		bid.IntentId,
		bid.AgentId,
		bid.Price,
		bid.Token,
		bid.SettleChain,
		bid.SubmittedAt,
		bid.Nonce,
	)
	return []byte(canonical)
}

func createCanonicalExecutionReport(report *pb.ExecutionReport) []byte {
	// Create deterministic canonical form using actual fields
	canonical := fmt.Sprintf("report:v1|%s|%s|%s|%s|%s|%d",
		report.ReportId,
		report.AssignmentId,
		report.IntentId,
		report.AgentId,
		report.Status.String(),
		report.Timestamp,
	)
	return []byte(canonical)
}

// Global verifier service instance (optional)
var globalVerifierService = NewVerifierService()

// GetGlobalVerifierService returns the global verifier service
func GetGlobalVerifierService() *VerifierService {
	return globalVerifierService
}

// InitializeTrustedKeys initializes trusted keys for all known entities
func InitializeTrustedKeys(keys map[string][]byte) {
	for id, pubKey := range keys {
		globalVerifierService.AddTrustedKey(id, pubKey)
	}
}