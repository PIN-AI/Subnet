package cometbft

import (
	"context"
	"fmt"
	"time"

	rootpb "subnet/proto/rootlayer"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/rootlayer"
)

// RootLayerClientAdapter adapts CompleteClient to CometBFT's RootLayerClient interface
type RootLayerClientAdapter struct {
	client   *rootlayer.CompleteClient
	subnetID string
	logger   logging.Logger
}

// NewRootLayerClientAdapter creates a new adapter
func NewRootLayerClientAdapter(
	client *rootlayer.CompleteClient,
	subnetID string,
	logger logging.Logger,
) *RootLayerClientAdapter {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	return &RootLayerClientAdapter{
		client:   client,
		subnetID: subnetID,
		logger:   logger,
	}
}

// SubmitCheckpointSignatures converts CheckpointSignatureBundle to ValidationBundles and submits them
func (a *RootLayerClientAdapter) SubmitCheckpointSignatures(bundle *CheckpointSignatureBundle) error {
	if bundle == nil {
		return fmt.Errorf("checkpoint signature bundle is nil")
	}

	if len(bundle.IntentReports) == 0 {
		a.logger.Warn("No intent reports in checkpoint, skipping ValidationBundle submission", "epoch", bundle.Epoch)
		return nil
	}

	a.logger.Info("Submitting ValidationBundles to RootLayer",
		"epoch", bundle.Epoch,
		"intent_count", len(bundle.IntentReports),
		"validators", len(bundle.ValidatorSignatures),
		"subnet_id", a.subnetID)

	// Submit one ValidationBundle per Intent
	successCount := 0
	var lastError error

	for _, report := range bundle.IntentReports {
		// Ensure checkpoint hash is exactly 32 bytes
		var resultHash []byte
		if len(bundle.CheckpointHash) == 32 {
			resultHash = bundle.CheckpointHash
		} else if len(bundle.CheckpointHash) < 32 {
			// Pad to 32 bytes
			resultHash = make([]byte, 32)
			copy(resultHash, bundle.CheckpointHash)
		} else {
			// Truncate to 32 bytes
			resultHash = bundle.CheckpointHash[:32]
		}

		vb := &rootpb.ValidationBundle{
			// Use real Intent ID from the execution report
			IntentId:     report.IntentId,
			SubnetId:     a.subnetID,
			AssignmentId: report.AssignmentId,
			AgentId:      report.AgentId,

			// Use checkpoint hash as the result hash (validators validated the entire checkpoint)
			ResultHash: resultHash,

			// Use timestamps from execution report and checkpoint
			ExecutedAt:  report.Timestamp,
			CompletedAt: bundle.Timestamp,

			// Convert validator signatures to ValidationSignature format
			// All validators sign the checkpoint, which covers all intents
			Signatures:   make([]*rootpb.ValidationSignature, 0, len(bundle.ValidatorSignatures)),
			TotalWeight:  uint64(len(bundle.ValidatorSignatures)),
			AggregatorId: "cometbft-consensus",
		}

		// Add each validator's signature (checkpoint signature covers all intents)
		for validatorAddr, sig := range bundle.ValidatorSignatures {
			vb.Signatures = append(vb.Signatures, &rootpb.ValidationSignature{
				Validator: validatorAddr,
				Signature: sig,
			})
		}

		// Submit to RootLayer with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		if err := a.client.SubmitValidationBundle(ctx, vb); err != nil {
			a.logger.Error("Failed to submit ValidationBundle",
				"intent_id", report.IntentId,
				"assignment_id", report.AssignmentId,
				"error", err)
			lastError = err
			cancel()
			continue
		}
		cancel()

		a.logger.Info("Successfully submitted ValidationBundle",
			"intent_id", report.IntentId,
			"assignment_id", report.AssignmentId)
		successCount++
	}

	if successCount == 0 && lastError != nil {
		return fmt.Errorf("failed to submit any ValidationBundles: %w", lastError)
	}

	a.logger.Info("ValidationBundle submission complete",
		"epoch", bundle.Epoch,
		"success", successCount,
		"total", len(bundle.IntentReports))

	return nil
}

// ECDSASignerAdapter adapts crypto.ExtendedSigner to CometBFT's ECDSASigner interface
type ECDSASignerAdapter struct {
	signer crypto.ExtendedSigner
	logger logging.Logger
}

// NewECDSASignerAdapter creates a new signer adapter
func NewECDSASignerAdapter(privateKeyHex string, logger logging.Logger) (*ECDSASignerAdapter, error) {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	signer, err := crypto.LoadSignerFromPrivateKey(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to load signer: %w", err)
	}

	return &ECDSASignerAdapter{
		signer: signer,
		logger: logger,
	}, nil
}

// Sign signs a hash and returns the signature
func (a *ECDSASignerAdapter) Sign(hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, fmt.Errorf("empty hash")
	}

	signature, err := a.signer.Sign(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	a.logger.Debug("Signed checkpoint hash", "hash_len", len(hash), "sig_len", len(signature))
	return signature, nil
}

// Address returns the Ethereum address of the signer
func (a *ECDSASignerAdapter) Address() string {
	return a.signer.GetAddress()
}
