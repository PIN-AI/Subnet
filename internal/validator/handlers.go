package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	pb "subnet/proto/subnet"
	"subnet/internal/crypto"
	"subnet/internal/types"
)

// ProcessExecutionReport processes an execution report from an agent
func (n *Node) ProcessExecutionReport(report *pb.ExecutionReport) (*pb.Receipt, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if we already have this report (idempotency)
	reportID := n.generateReportID(report)
	if _, exists := n.pendingReports[reportID]; exists {
		return &pb.Receipt{
			ReportId:   reportID,
			ReceivedTs: time.Now().Unix(),
			Status:     "duplicate",
		}, nil
	}

	// Store the report
	n.pendingReports[reportID] = report

	// Score the report (for now, simple scoring)
	score := n.scoreReport(report)
	n.reportScores[reportID] = score

	// Create receipt
	receipt := &pb.Receipt{
		ReportId:    reportID,
		ValidatorId: n.id,
		IntentId:    report.IntentId,
		ReceivedTs:  time.Now().Unix(),
		Status:      "accepted",
		ScoreHint:   uint32(score),
	}

	n.logger.Debug("Processed execution report",
		"report_id", reportID,
		"assignment", report.AssignmentId,
		"score", score)

	return receipt, nil
}

// GetCheckpoint retrieves a checkpoint by epoch
func (n *Node) GetCheckpoint(epoch uint64) (*pb.CheckpointHeader, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.currentCheckpoint != nil && n.currentCheckpoint.Epoch == epoch {
		return n.currentCheckpoint, nil
	}

	// Load from storage
	checkpoint, err := n.chain.GetCheckpoint(epoch)
	if err != nil {
		return nil, fmt.Errorf("checkpoint not found for epoch %d", epoch)
	}

	return checkpoint, nil
}

// IsLeader checks if this node is the leader for given epoch
func (n *Node) IsLeader(epoch uint64) bool {
	_, leader := n.leaderTracker.Leader(epoch)
	return leader != nil && leader.ID == n.id
}

// HandleProposal handles a checkpoint proposal
func (n *Node) HandleProposal(header *pb.CheckpointHeader) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Validate proposal
	if err := n.validateProposal(header); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}

	// Update FSM
	if err := n.fsm.ProposeHeader(header); err != nil {
		return fmt.Errorf("FSM error: %w", err)
	}

	// Sign the proposal
	sig, err := n.signCheckpoint(header)
	if err != nil {
		return fmt.Errorf("failed to sign: %w", err)
	}

	n.signatures[n.id] = sig
	n.currentCheckpoint = header

	// Broadcast signature
	go n.broadcastSignature(sig)

	return nil
}

// AddSignature adds a signature to current checkpoint
func (n *Node) AddSignature(sig *pb.Signature) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Verify signature corresponds to current checkpoint
	if n.currentCheckpoint == nil {
		return fmt.Errorf("no active checkpoint")
	}

	// Verify signature
	if err := n.verifySignature(sig); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	// Check for double sign
	if existingSig, exists := n.signatures[sig.SignerId]; exists {
		// Validator already signed for this epoch
		if !bytes.Equal(existingSig.Der, sig.Der) {
			// Different signature for same epoch - this is double signing!
			n.detectDoubleSign(sig.SignerId, n.currentCheckpoint.Epoch, existingSig, sig)
			return fmt.Errorf("double sign detected for validator %s at epoch %d",
				sig.SignerId, n.currentCheckpoint.Epoch)
		}
		// Same signature, just ignore duplicate
		return nil
	}

	// Add to FSM
	if err := n.fsm.AddSignature(sig); err != nil {
		return err
	}

	// Store signature using SignerId field
	n.signatures[sig.SignerId] = sig

	return nil
}

// GetSignatures returns signatures for an epoch
func (n *Node) GetSignatures(epoch uint64) []*pb.Signature {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.currentCheckpoint == nil || n.currentCheckpoint.Epoch != epoch {
		return nil
	}

	sigs := make([]*pb.Signature, 0, len(n.signatures))
	for _, sig := range n.signatures {
		sigs = append(sigs, sig)
	}

	return sigs
}

// GetValidatorSet returns the current validator set
func (n *Node) GetValidatorSet() *types.ValidatorSet {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.validatorSet
}

// detectDoubleSign handles detection and storage of double sign evidence
func (n *Node) detectDoubleSign(validatorID string, epoch uint64, sig1, sig2 *pb.Signature) {
	// Create detailed evidence data
	evidenceDetail := struct {
		ValidatorID     string `json:"validator_id"`
		Epoch          uint64 `json:"epoch"`
		FirstSignature  []byte `json:"first_signature"`
		SecondSignature []byte `json:"second_signature"`
		DetectedAt      int64  `json:"detected_at"`
	}{
		ValidatorID:     validatorID,
		Epoch:          epoch,
		FirstSignature:  sig1.Der,
		SecondSignature: sig2.Der,
		DetectedAt:      time.Now().Unix(),
	}

	// Marshal evidence details
	evidenceData, _ := json.Marshal(evidenceDetail)

	// Create proto evidence (will be used when reporting to RootLayer)
	_ = &pb.DoubleSignEvidence{
		ValidatorId: validatorID,
		Epoch:      epoch,
		Evidence:   evidenceData,
	}

	// Store evidence
	if err := n.store.SaveDoubleSignEvidence(epoch, validatorID, evidenceData); err != nil {
		n.logger.Error("Failed to save double sign evidence",
			"validator", validatorID,
			"epoch", epoch,
			"error", err)
	}

	// Log the critical security event
	n.logger.Error("CRITICAL: DOUBLE SIGN DETECTED",
		"validator", validatorID,
		"epoch", epoch,
		"message", "Validator signed conflicting data for same epoch - evidence stored for slashing")

	// Notify RootLayer if connected
	if n.rootlayerClient != nil && n.rootlayerClient.IsConnected() {
		// This would be sent to RootLayer for slashing
		n.logger.Info("Reporting double sign to RootLayer",
			"validator", validatorID,
			"epoch", epoch)
		// In production, we would call:
		// n.rootlayerClient.ReportDoubleSign(evidence)
	}
}

// GetDoubleSignEvidences returns double sign evidences
func (n *Node) GetDoubleSignEvidences(validatorID string, startEpoch, endEpoch uint64) []*pb.DoubleSignEvidence {
	evidences := make([]*pb.DoubleSignEvidence, 0)

	// Query from storage
	var epochPtr *uint64
	if startEpoch == endEpoch && startEpoch > 0 {
		epochPtr = &startEpoch
	}

	items, err := n.store.ListDoubleSignEvidence(validatorID, epochPtr, 100)
	if err != nil {
		n.logger.Error("Failed to list double sign evidence", "error", err)
		return evidences
	}

	// Convert storage items to proto messages
	for _, item := range items {
		// Filter by epoch range if specified
		if startEpoch > 0 && item.Epoch < startEpoch {
			continue
		}
		if endEpoch > 0 && item.Epoch > endEpoch {
			continue
		}

		var evidence pb.DoubleSignEvidence
		if err := json.Unmarshal(item.Evidence, &evidence); err != nil {
			n.logger.Warn("Failed to unmarshal double sign evidence",
				"validator", item.ValidatorID,
				"epoch", item.Epoch,
				"error", err)
			continue
		}
		evidences = append(evidences, &evidence)
	}

	return evidences
}

// GetValidationPolicy returns the current validation policy
func (n *Node) GetValidationPolicy() *pb.ValidationPolicy {
	// Load from configuration if available
	if n.config.ValidationPolicy != nil {
		return &pb.ValidationPolicy{
			PolicyId: n.config.ValidationPolicy.PolicyID,
			Version:  n.config.ValidationPolicy.Version,
			Rules: &pb.ValidationPolicy_Rules{
				// Note: Proto definition for Rules may need adjustment
				// These are placeholders based on typical validation rules
			},
		}
	}

	// Default policy if not configured
	return &pb.ValidationPolicy{
		PolicyId: "default",
		Version:  "1.0.0",
		Rules:    &pb.ValidationPolicy_Rules{},
	}
}

// GetVerificationRecords returns verification records
func (n *Node) GetVerificationRecords(assignmentID string, startTime, endTime int64) []*pb.VerificationRecord {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var records []*pb.VerificationRecord

	for reportID, report := range n.pendingReports {
		if report.AssignmentId == assignmentID {
			// Use current time as approximation since ExecutedAt doesn't exist
			reportTime := time.Now().Unix()
			if reportTime >= startTime && reportTime <= endTime {
				records = append(records, &pb.VerificationRecord{
					RecordId:    reportID,
					IntentId:    report.IntentId,
					ValidatorId: n.id,
					Timestamp:   reportTime,
					// Remove Status field as it doesn't exist
				})
			}
		}
	}

	return records
}

// GetMetrics returns validator metrics
func (n *Node) GetMetrics() *pb.ValidatorMetrics {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Return simplified metrics with available fields
	return &pb.ValidatorMetrics{
		ValidatorId: n.id,
		// Add other fields based on actual proto definition
	}
}

// Helper methods

func (n *Node) generateReportID(report *pb.ExecutionReport) string {
	h := sha256.New()
	h.Write([]byte(report.AssignmentId))
	h.Write([]byte(report.AgentId))
	h.Write([]byte(fmt.Sprintf("%d", time.Now().Unix())))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (n *Node) scoreReport(report *pb.ExecutionReport) int32 {
	// Implement scoring based on report quality and validation policy
	baseScore := int32(100)

	// Apply validation policy scoring if configured
	if n.config.ValidationPolicy != nil {
		policy := n.config.ValidationPolicy

		// Adjust score based on execution time
		if policy.MinExecutionTime > 0 || policy.MaxExecutionTime > 0 {
			executionTime := time.Now().Unix() - report.Timestamp
			optimalTime := (policy.MinExecutionTime + policy.MaxExecutionTime) / 2

			if executionTime >= policy.MinExecutionTime && executionTime <= policy.MaxExecutionTime {
				// Within acceptable range, calculate distance from optimal
				deviation := float32(executionTime - optimalTime)
				if optimalTime > 0 {
					penalty := int32(math.Abs(float64(deviation)) * 10 / float64(optimalTime))
					baseScore = baseScore - penalty
				}
			} else {
				// Outside acceptable range, heavy penalty
				baseScore = baseScore / 2
			}
		}

		// Bonus for providing proof of execution
		if policy.RequireProofOfExecution && len(report.ResultData) > 0 {
			baseScore += 20
		}

		// Apply confidence score if available
		if policy.MinConfidenceScore > 0 {
			// In production, extract confidence from report metadata
			// For now, use a default high confidence
			confidence := float32(0.95)
			if confidence >= policy.MinConfidenceScore {
				baseScore += int32(confidence * 10)
			} else {
				baseScore -= 30
			}
		}
	}

	// Ensure score is within reasonable bounds
	if baseScore < 0 {
		baseScore = 0
	} else if baseScore > 200 {
		baseScore = 200
	}

	return baseScore
}

func (n *Node) validateProposal(header *pb.CheckpointHeader) error {
	// Check epoch sequence
	if n.currentCheckpoint != nil && header.Epoch <= n.currentCheckpoint.Epoch {
		return fmt.Errorf("invalid epoch: %d <= %d", header.Epoch, n.currentCheckpoint.Epoch)
	}

	// TODO: Add more validation once proto fields are confirmed
	return nil
}

func (n *Node) verifySignature(sig *pb.Signature) error {
	// Find validator in the set
	var validator *types.Validator
	for _, v := range n.validatorSet.Validators {
		if v.ID == sig.SignerId {
			validator = &v
			break
		}
	}
	if validator == nil {
		return fmt.Errorf("unknown validator: %s", sig.SignerId)
	}

	// Check signature is not empty
	if len(sig.Der) == 0 {
		return fmt.Errorf("empty signature")
	}

	// Get the checkpoint header to verify
	header := n.currentCheckpoint
	if header == nil {
		return fmt.Errorf("no checkpoint header to verify")
	}

	// Use the canonical checkpoint hasher for consistent verification
	hasher := crypto.NewCheckpointHasher()
	msgHash := hasher.ComputeHash(header)

	// Verify signature using validator's public key
	verifier := crypto.NewExtendedVerifier()
	if !verifier.Verify(validator.PubKey, msgHash[:], sig.Der) {
		return fmt.Errorf("signature verification failed for validator %s", sig.SignerId)
	}

	return nil
}

func (n *Node) broadcastSignature(sig *pb.Signature) {
	if n.broadcaster != nil {
		if err := n.broadcaster.BroadcastSignature(sig); err != nil {
			n.logger.Error("Failed to broadcast signature",
				"error", err,
				"signer", sig.SignerId)
		} else {
			n.logger.Debug("Broadcasted signature", "signer", sig.SignerId)
		}
	} else {
		n.logger.Warn("No broadcaster configured, skipping signature broadcast")
	}
}