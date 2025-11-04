package cometbft

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	sdkCrypto "github.com/PIN-AI/intent-protocol-contract-sdk/sdk/crypto"

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

// SubmitCheckpointSignatures converts CheckpointSignatureBundle to ValidationBatchGroup and submits it
func (a *RootLayerClientAdapter) SubmitCheckpointSignatures(bundle *CheckpointSignatureBundle) error {
	if bundle == nil {
		return fmt.Errorf("checkpoint signature bundle is nil")
	}

	if len(bundle.IntentReports) == 0 {
		a.logger.Warn("No intent reports in checkpoint, skipping ValidationBatchGroup submission", "epoch", bundle.Epoch)
		return nil
	}

	a.logger.Info("Submitting ValidationBatchGroup to RootLayer",
		"epoch", bundle.Epoch,
		"intent_count", len(bundle.IntentReports),
		"validators", len(bundle.ValidatorSignatures),
		"subnet_id", a.subnetID)

	// Prepare ValidationItem array
	items := make([]*rootpb.ValidationItem, 0, len(bundle.IntentReports))
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

		items = append(items, &rootpb.ValidationItem{
			IntentId:     report.IntentId,
			AssignmentId: report.AssignmentId,
			AgentId:      report.AgentId,
			ExecutedAt:   report.Timestamp,
			ResultHash:   resultHash,
			ProofHash:    nil,
		})
	}

	// Prepare validator signatures
	signatures := make([]*rootpb.ValidationSignature, 0, len(bundle.ValidatorSignatures))
	for validatorAddr, sig := range bundle.ValidatorSignatures {
		// Ensure validator address is valid UTF-8 and properly formatted
		// If it's not already 0x-prefixed, format it
		validAddr := validatorAddr
		if !strings.HasPrefix(validAddr, "0x") {
			// If the address is binary, convert to hex
			validAddr = fmt.Sprintf("0x%x", []byte(validatorAddr))
			a.logger.Warn("Validator address was not 0x-prefixed, converted",
				"original", validatorAddr,
				"converted", validAddr)
		}

		signatures = append(signatures, &rootpb.ValidationSignature{
			Validator: validAddr,
			Signature: sig,
		})
	}

	// Compute items_hash using SDK crypto library (keccak256(abi.encode(items)))
	// This is required for ValidationBatch signature verification
	sdkItems := make([]sdkCrypto.ValidationItem, len(items))
	for i, item := range items {
		// Parse IDs from hex strings
		intentID := common.HexToHash(item.IntentId)
		assignmentID := common.HexToHash(item.AssignmentId)
		agentAddr := common.HexToAddress(item.AgentId)

		// Convert resultHash to [32]byte
		var resultHash [32]byte
		copy(resultHash[:], item.ResultHash)

		// Convert proofHash to [32]byte (or zero if nil)
		var proofHash [32]byte
		if len(item.ProofHash) >= 32 {
			copy(proofHash[:], item.ProofHash)
		}

		sdkItems[i] = sdkCrypto.ValidationItem{
			IntentID:     intentID,
			AssignmentID: assignmentID,
			Agent:        agentAddr,
			ResultHash:   resultHash,
			ProofHash:    proofHash,
		}
	}

	itemsHash, err := sdkCrypto.ComputeItemsHash(sdkItems)
	if err != nil {
		return fmt.Errorf("failed to compute items_hash: %w", err)
	}
	a.logger.Info("Computed items_hash for ValidationBatch",
		"items_count", len(sdkItems),
		"items_hash", fmt.Sprintf("0x%x", itemsHash))

	// Create ValidationBatchGroup with items_hash
	group := &rootpb.ValidationBatchGroup{
		SubnetId:     a.subnetID,
		RootHeight:   bundle.Epoch,
		RootHash:     fmt.Sprintf("0x%x", bundle.CheckpointHash),
		AggregatorId: "cometbft-consensus",
		CompletedAt:  bundle.Timestamp,
		Signatures:   signatures,
		SignerBitmap: nil,
		TotalWeight:  uint64(len(signatures)),
		Items:        items,
		ItemsHash:    itemsHash[:], // Set the computed items_hash
	}

	// Submit to RootLayer
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	batchID := fmt.Sprintf("cometbft-epoch-%d-%d", bundle.Epoch, time.Now().Unix())
	partialOk := true

	resp, err := a.client.SubmitValidationBundleBatch(ctx, []*rootpb.ValidationBatchGroup{group}, batchID, partialOk)
	if err != nil {
		return fmt.Errorf("failed to submit ValidationBatchGroup: %w", err)
	}

	a.logger.Info("ValidationBatchGroup submission complete",
		"epoch", bundle.Epoch,
		"batch_id", batchID,
		"items", len(items),
		"success", resp.Success,
		"failed", resp.Failed)

	if resp.Failed > 0 {
		for i, result := range resp.Results {
			if !result.Ok {
				a.logger.Warn("ValidationItem failed",
					"index", i,
					"intent_id", items[i].IntentId,
					"error", result.Msg)
			}
		}
	}

	if resp.Success == 0 {
		return fmt.Errorf("all ValidationItem submissions failed: %s", resp.Msg)
	}

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
