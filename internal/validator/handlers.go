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
	"subnet/internal/consensus"
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

	n.logger.Info("Processed execution report",
		"report_id", reportID,
		"assignment", report.AssignmentId,
		"intent_id", report.IntentId,
		"score", score,
		"pending_reports_count", len(n.pendingReports))

	// Broadcast the execution report to all validators (including leader)
	// This ensures all nodes, especially the current leader, can include this report in the next checkpoint
	go n.broadcastExecutionReport(report)

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

	// Check if we are the leader for this epoch
	_, leader := n.leaderTracker.Leader(header.Epoch)
	isLeader := leader != nil && leader.ID == n.id

	n.logger.Info("HandleProposal called",
		"epoch", header.Epoch,
		"is_leader", isLeader,
		"fsm_state", n.fsm.GetState())

	// If we are the leader and the FSM is already in a non-idle state for this epoch,
	// this is our own proposal coming back via NATS - skip it to avoid "cannot propose in state proposed" error
	if isLeader {
		fsmState := n.fsm.GetState()
		if fsmState != consensus.StateIdle {
			n.logger.Info("Leader skipping own proposal received via broadcast",
				"epoch", header.Epoch, "fsm_state", fsmState)
			return nil
		}
	}

	// Validate proposal
	if err := n.validateProposal(header); err != nil {
		n.logger.Error("Proposal validation failed", "error", err, "epoch", header.Epoch)
		return fmt.Errorf("invalid proposal: %w", err)
	}

	// If we're a follower and not in idle state, this is a retry from the leader
	// We need to reset our FSM to accept the new proposal
	fsmState := n.fsm.GetState()
	if !isLeader && fsmState != consensus.StateIdle {
		n.logger.Info("Follower resetting state to accept leader's new proposal",
			"epoch", header.Epoch,
			"old_state", fsmState)
		n.fsm.Reset()
		n.signatures = make(map[string]*pb.Signature) // Clear old signatures
	}

	// Update FSM
	if err := n.fsm.ProposeHeader(header); err != nil {
		n.logger.Error("FSM ProposeHeader failed", "error", err, "epoch", header.Epoch)
		return fmt.Errorf("FSM error: %w", err)
	}

	// Sign the proposal
	sig, err := n.signCheckpoint(header)
	if err != nil {
		n.logger.Error("Failed to sign checkpoint", "error", err, "epoch", header.Epoch)
		return fmt.Errorf("failed to sign: %w", err)
	}

	// Add our own signature to both Node map and FSM
	n.signatures[n.id] = sig
	if err := n.fsm.AddSignature(sig); err != nil {
		n.logger.Error("Failed to add own signature to FSM", "error", err, "epoch", header.Epoch)
		return fmt.Errorf("FSM error: %w", err)
	}

	n.currentCheckpoint = header

	n.logger.Info("Follower signed checkpoint proposal, broadcasting signature",
		"epoch", header.Epoch,
		"validator", n.id)

	// Broadcast signature
	go n.broadcastSignature(sig)

	return nil
}

// AddSignature adds a signature to current checkpoint
func (n *Node) AddSignature(sig *pb.Signature) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.logger.Info("AddSignature called",
		"signer", sig.SignerId,
		"has_checkpoint", n.currentCheckpoint != nil)

	// Verify signature corresponds to current checkpoint
	if n.currentCheckpoint == nil {
		return fmt.Errorf("no active checkpoint")
	}

	// Verify signature
	if err := n.verifySignature(sig); err != nil {
		n.logger.Error("Signature verification failed", "error", err, "signer", sig.SignerId)
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
		n.logger.Debug("Ignoring duplicate signature", "signer", sig.SignerId)
		return nil
	}

	// Add to FSM
	if err := n.fsm.AddSignature(sig); err != nil {
		n.logger.Error("FSM AddSignature failed", "error", err, "signer", sig.SignerId)
		return err
	}

	// Store signature using SignerId field
	n.signatures[sig.SignerId] = sig

	// Check FSM state immediately after adding signature
	fsmState := n.fsm.GetState()

	// Get threshold info for debugging
	signCount, required, progress := n.fsm.GetProgress()

	// Get all signers for detailed logging
	signers := make([]string, 0, len(n.signatures))
	for signerID := range n.signatures {
		signers = append(signers, signerID)
	}

	n.logger.Info("Added signature to checkpoint",
		"signer", sig.SignerId,
		"epoch", n.currentCheckpoint.Epoch,
		"total_signatures", len(n.signatures),
		"fsm_state", fsmState,
		"fsm_sign_count", signCount,
		"required", required,
		"progress", progress,
		"all_signers", signers)

	// Check if we just reached threshold and trigger finalization immediately
	// This prevents race condition where ticker might check old state before threshold is processed
	if fsmState == consensus.StateThreshold {
		n.logger.Info("Threshold reached via signature, triggering finalization",
			"epoch", n.currentCheckpoint.Epoch,
			"signatures", len(n.signatures))
		// Trigger finalization in separate goroutine to avoid blocking signature processing
		go n.finalizeCheckpoint()
	}

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
	// Basic validation - only reject proposals from past epochs that we've already finalized
	if n.currentCheckpoint != nil && header.Epoch < n.currentCheckpoint.Epoch {
		return fmt.Errorf("invalid epoch: %d < %d (already finalized)", header.Epoch, n.currentCheckpoint.Epoch)
	}

	// Accept proposals for current or future epochs
	// The finalized broadcast mechanism will handle epoch synchronization

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

func (n *Node) broadcastExecutionReport(report *pb.ExecutionReport) {
	if n.broadcaster != nil {
		if err := n.broadcaster.BroadcastExecutionReport(report); err != nil {
			n.logger.Error("Failed to broadcast execution report",
				"error", err,
				"intent_id", report.IntentId,
				"assignment_id", report.AssignmentId)
		} else {
			n.logger.Info("📡 Broadcasted execution report to all validators",
				"intent_id", report.IntentId,
				"assignment_id", report.AssignmentId,
				"agent_id", report.AgentId)
		}
	} else {
		n.logger.Warn("No broadcaster configured, skipping execution report broadcast")
	}
}

func (n *Node) broadcastFinalized(header *pb.CheckpointHeader) {
	if n.broadcaster != nil {
		if err := n.broadcaster.BroadcastFinalized(header); err != nil {
			n.logger.Error("Failed to broadcast finalized checkpoint",
				"error", err,
				"epoch", header.Epoch)
		} else {
			n.logger.Info("🏁 Broadcasted finalized checkpoint",
				"epoch", header.Epoch,
				"signatures", header.Signatures.SignatureCount)
		}
	} else {
		n.logger.Warn("No broadcaster configured, skipping finalized broadcast")
	}
}

// HandleFinalized handles a finalized checkpoint broadcast from another validator
func (n *Node) HandleFinalized(header *pb.CheckpointHeader) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.logger.Info("🏁 Handling finalized checkpoint",
		"received_epoch", header.Epoch,
		"current_epoch", n.currentEpoch,
		"current_state", n.fsm.GetState())

	// If we're behind, catch up
	if header.Epoch > n.currentEpoch {
		n.logger.Info("📈 Catching up to finalized epoch",
			"from_epoch", n.currentEpoch,
			"to_epoch", header.Epoch)

		// Store the finalized checkpoint
		n.currentCheckpoint = header
		n.currentEpoch = header.Epoch + 1 // Move to next epoch after finalized

		// Reset FSM to idle state for the new epoch
		n.fsm.Reset()
		n.signatures = make(map[string]*pb.Signature)

		// Update leadership status for new epoch
		_, leader := n.leaderTracker.Leader(n.currentEpoch)
		wasLeader := n.isLeader
		n.isLeader = leader != nil && leader.ID == n.id

		if wasLeader != n.isLeader {
			n.lastCheckpointAt = time.Time{} // Reset timer
			if n.isLeader {
				n.logger.Info("Became leader after catch-up", "epoch", n.currentEpoch)
			} else {
				n.logger.Info("Not leader after catch-up", "epoch", n.currentEpoch)
			}
		}

		// Store checkpoint in chain
		if err := n.chain.AddCheckpoint(header); err != nil {
			n.logger.Error("Failed to store finalized checkpoint", "error", err)
		}

		// Persist to storage
		if err := n.saveCheckpoint(header); err != nil {
			n.logger.Error("Failed to save finalized checkpoint", "error", err)
		}

		n.logger.Info("✅ Successfully caught up to finalized checkpoint",
			"new_epoch", n.currentEpoch,
			"is_leader", n.isLeader)
	} else if header.Epoch == n.currentEpoch {
		// We're at the same epoch but might have missed the finalization
		if n.fsm.GetState() != consensus.StateIdle {
			n.logger.Info("📊 Updating to finalized state for current epoch",
				"epoch", n.currentEpoch,
				"prev_state", n.fsm.GetState())

			// Reset to idle and move to next epoch
			n.currentCheckpoint = header
			n.currentEpoch++
			n.fsm.Reset()
			n.signatures = make(map[string]*pb.Signature)

			// Update leadership
			_, leader := n.leaderTracker.Leader(n.currentEpoch)
			n.isLeader = leader != nil && leader.ID == n.id
		}
	}
	// If header.Epoch < n.currentEpoch, we're ahead, ignore

	return nil
}