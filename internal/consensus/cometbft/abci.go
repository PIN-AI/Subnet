package cometbft

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"google.golang.org/protobuf/proto"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"
)

// TxType represents the type of transaction
type TxType byte

const (
	TxTypeCheckpoint                TxType = 0x01
	TxTypeExecutionReport           TxType = 0x02
	TxTypeValidationBundleSignature TxType = 0x03
)

// ValidationBundleSignatureTx represents a validator's ECDSA signature for ValidationBundle
type ValidationBundleSignatureTx struct {
	Epoch          uint64 `json:"epoch"`
	CheckpointHash []byte `json:"checkpoint_hash"`
	ValidatorAddr  string `json:"validator_addr"`
	Signature      []byte `json:"signature"` // ECDSA signature
}

// VoteExtension represents the ECDSA signature attached to each vote
type VoteExtension struct {
	Epoch          uint64 `json:"epoch"`
	CheckpointHash []byte `json:"checkpoint_hash"`
	ValidatorAddr  string `json:"validator_addr"`
	Signature      []byte `json:"signature"` // ECDSA signature
}

// ConsensusHandlers defines callbacks for consensus events
type ConsensusHandlers struct {
	OnCheckpointCommitted      func(header *pb.CheckpointHeader)
	OnExecutionReportCommitted func(report *pb.ExecutionReport, reportKey string)
	OnCheckpointFinalized      func(header *pb.CheckpointHeader, signatures []*pb.Signature)
}

// SubnetABCIApp implements the ABCI Application interface for Subnet consensus
type SubnetABCIApp struct {
	mu sync.RWMutex

	logger   logging.Logger
	handlers ConsensusHandlers

	// State
	currentEpoch       uint64
	lastProcessedEpoch uint64
	checkpoints        map[uint64]*pb.CheckpointHeader
	pendingReports     map[string]*pb.ExecutionReport
	latestBlockHeight  int64
	latestAppHash      []byte

	// Validator set (validator_id -> voting_power)
	validatorSet map[string]int64

	// ValidationBundle signature collection (epoch -> (validator_addr -> signature))
	validationBundleSignatures map[uint64]map[string][]byte

	// ECDSA signer for ValidationBundle (can be nil if not configured)
	ecdsaSigner ECDSASigner

	// RootLayer client for ValidationBundle submission (can be nil if not configured)
	rootlayerClient RootLayerClient

	// Mempool broadcaster for signature transactions
	mempoolBroadcaster MempoolBroadcaster
}

// ECDSASigner interface for signing ValidationBundle checkpoints
type ECDSASigner interface {
	Sign(hash []byte) ([]byte, error)
	Address() string
}

// CheckpointSignatureBundle represents collected signatures for a checkpoint
type CheckpointSignatureBundle struct {
	Epoch               uint64
	CheckpointHash      []byte
	ValidatorSignatures map[string][]byte // validator_addr -> signature
	Timestamp           int64
	IntentReports       []*pb.ExecutionReport // execution reports included in this checkpoint
}

// RootLayerClient interface for submitting CheckpointSignatureBundles
type RootLayerClient interface {
	SubmitCheckpointSignatures(bundle *CheckpointSignatureBundle) error
}

// MempoolBroadcaster interface for broadcasting transactions to mempool
type MempoolBroadcaster interface {
	BroadcastTxSync(tx []byte) error
}

// SubnetABCIAppConfig contains configuration for creating a SubnetABCIApp
type SubnetABCIAppConfig struct {
	Handlers           ConsensusHandlers
	ValidatorSet       map[string]int64
	ECDSASigner        ECDSASigner
	RootLayerClient    RootLayerClient
	MempoolBroadcaster MempoolBroadcaster
	Logger             logging.Logger
}

// NewSubnetABCIApp creates a new ABCI application for subnet consensus
func NewSubnetABCIApp(handlers ConsensusHandlers, validatorSet map[string]int64, logger logging.Logger) *SubnetABCIApp {
	return NewSubnetABCIAppWithConfig(SubnetABCIAppConfig{
		Handlers:     handlers,
		ValidatorSet: validatorSet,
		Logger:       logger,
	})
}

// NewSubnetABCIAppWithConfig creates a new ABCI application with full configuration
func NewSubnetABCIAppWithConfig(config SubnetABCIAppConfig) *SubnetABCIApp {
	if config.Logger == nil {
		config.Logger = logging.NewDefaultLogger()
	}

	if config.ValidatorSet == nil {
		config.ValidatorSet = make(map[string]int64)
	}

	return &SubnetABCIApp{
		logger:                     config.Logger,
		handlers:                   config.Handlers,
		checkpoints:                make(map[uint64]*pb.CheckpointHeader),
		pendingReports:             make(map[string]*pb.ExecutionReport),
		validatorSet:               config.ValidatorSet,
		validationBundleSignatures: make(map[uint64]map[string][]byte),
		ecdsaSigner:                config.ECDSASigner,
		rootlayerClient:            config.RootLayerClient,
		mempoolBroadcaster:         config.MempoolBroadcaster,
	}
}

// Info returns information about the application state
func (app *SubnetABCIApp) Info(ctx context.Context, req *abci.RequestInfo) (*abci.ResponseInfo, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()

	return &abci.ResponseInfo{
		Data:             "subnet-consensus",
		Version:          "1.0.0",
		AppVersion:       1,
		LastBlockHeight:  app.latestBlockHeight,
		LastBlockAppHash: app.latestAppHash,
	}, nil
}

// InitChain initializes the blockchain with validators
func (app *SubnetABCIApp) InitChain(ctx context.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	app.mu.Lock()
	defer app.mu.Unlock()

	app.logger.Info("Initializing CometBFT chain",
		"chain_id", req.ChainId,
		"validators", len(req.Validators))

	// Initialize validator set from genesis (clear existing first to avoid accumulation)
	app.validatorSet = make(map[string]int64)
	for _, v := range req.Validators {
		validatorID := string(v.PubKey.GetEd25519())
		app.validatorSet[validatorID] = v.Power
		app.logger.Debug("Added validator", "id", validatorID, "power", v.Power)
	}

	// Enable vote extensions from height 2
	// This allows validators to attach ECDSA signatures to their votes
	voteExtensionsEnableHeight := int64(2)

	app.logger.Info("Enabling vote extensions", "from_height", voteExtensionsEnableHeight)

	return &abci.ResponseInitChain{
		ConsensusParams: &cmtproto.ConsensusParams{
			Abci: &cmtproto.ABCIParams{
				VoteExtensionsEnableHeight: voteExtensionsEnableHeight,
			},
		},
	}, nil
}

// CheckTx validates a transaction before adding it to mempool
func (app *SubnetABCIApp) CheckTx(ctx context.Context, req *abci.RequestCheckTx) (*abci.ResponseCheckTx, error) {
	if len(req.Tx) < 1 {
		return &abci.ResponseCheckTx{Code: 1, Log: "empty transaction"}, nil
	}

	txType := TxType(req.Tx[0])
	payload := req.Tx[1:]

	switch txType {
	case TxTypeCheckpoint:
		header := &pb.CheckpointHeader{}
		if err := proto.Unmarshal(payload, header); err != nil {
			return &abci.ResponseCheckTx{Code: 1, Log: fmt.Sprintf("invalid checkpoint: %v", err)}, nil
		}
		// Epoch 0 is valid (genesis epoch)
		return &abci.ResponseCheckTx{Code: 0}, nil

	case TxTypeExecutionReport:
		report := &pb.ExecutionReport{}
		if err := proto.Unmarshal(payload, report); err != nil {
			return &abci.ResponseCheckTx{Code: 1, Log: fmt.Sprintf("invalid report: %v", err)}, nil
		}
		return &abci.ResponseCheckTx{Code: 0}, nil

	case TxTypeValidationBundleSignature:
		var sigTx ValidationBundleSignatureTx
		if err := json.Unmarshal(payload, &sigTx); err != nil {
			return &abci.ResponseCheckTx{Code: 1, Log: fmt.Sprintf("invalid signature tx: %v", err)}, nil
		}
		if sigTx.Epoch == 0 {
			return &abci.ResponseCheckTx{Code: 1, Log: "invalid epoch"}, nil
		}
		if len(sigTx.Signature) == 0 {
			return &abci.ResponseCheckTx{Code: 1, Log: "empty signature"}, nil
		}
		return &abci.ResponseCheckTx{Code: 0}, nil

	default:
		return &abci.ResponseCheckTx{Code: 1, Log: "unknown transaction type"}, nil
	}
}

// FinalizeBlock executes transactions and updates state (replaces DeliverTx in ABCI 2.0)
func (app *SubnetABCIApp) FinalizeBlock(ctx context.Context, req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
	app.mu.Lock()
	defer app.mu.Unlock()

	app.latestBlockHeight = req.Height

	txResults := make([]*abci.ExecTxResult, len(req.Txs))

	for i, tx := range req.Txs {
		if len(tx) < 1 {
			txResults[i] = &abci.ExecTxResult{Code: 1, Log: "empty transaction"}
			continue
		}

		txType := TxType(tx[0])
		payload := tx[1:]

		switch txType {
		case TxTypeCheckpoint:
			result := app.deliverCheckpoint(payload)
			txResults[i] = &result

		case TxTypeExecutionReport:
			result := app.deliverExecutionReport(payload)
			txResults[i] = &result

		case TxTypeValidationBundleSignature:
			result := app.deliverValidationBundleSignature(payload)
			txResults[i] = &result

		default:
			txResults[i] = &abci.ExecTxResult{Code: 1, Log: "unknown transaction type"}
		}
	}

	return &abci.ResponseFinalizeBlock{
		TxResults: txResults,
	}, nil
}

// processVoteExtensions extracts ECDSA signatures from vote extensions and submits ValidationBundle when threshold is reached
func (app *SubnetABCIApp) processVoteExtensions(commit abci.ExtendedCommitInfo) {
	if len(commit.Votes) == 0 {
		return
	}

	// Extract ECDSA signatures from vote extensions
	for _, vote := range commit.Votes {
		// Skip votes without extensions
		if len(vote.VoteExtension) == 0 {
			continue
		}

		// Deserialize vote extension
		var voteExt VoteExtension
		if err := json.Unmarshal(vote.VoteExtension, &voteExt); err != nil {
			app.logger.Warn("Failed to unmarshal vote extension in PrepareProposal", "error", err)
			continue
		}

		// Initialize epoch map if needed
		if app.validationBundleSignatures[voteExt.Epoch] == nil {
			app.validationBundleSignatures[voteExt.Epoch] = make(map[string][]byte)
		}

		// Store signature (only if not already present)
		if _, exists := app.validationBundleSignatures[voteExt.Epoch][voteExt.ValidatorAddr]; !exists {
			app.validationBundleSignatures[voteExt.Epoch][voteExt.ValidatorAddr] = voteExt.Signature

			app.logger.Info("Collected ECDSA signature from vote extension",
				"epoch", voteExt.Epoch,
				"validator", voteExt.ValidatorAddr,
				"total_signatures", len(app.validationBundleSignatures[voteExt.Epoch]))
		}
	}

	// Check if we have enough signatures for any epoch
	// We check all epochs that have signatures collected
	for epoch := range app.validationBundleSignatures {
		go app.checkAndSubmitValidationBundle(epoch)
	}
}

// deliverCheckpoint processes a checkpoint transaction
func (app *SubnetABCIApp) deliverCheckpoint(payload []byte) abci.ExecTxResult {
	header := &pb.CheckpointHeader{}
	if err := proto.Unmarshal(payload, header); err != nil {
		return abci.ExecTxResult{Code: 1, Log: err.Error()}
	}

	// Store checkpoint
	app.checkpoints[header.Epoch] = header
	app.currentEpoch = header.Epoch

	// Notify handler (non-blocking)
	if app.handlers.OnCheckpointCommitted != nil {
		go app.handlers.OnCheckpointCommitted(header)
	}

	app.logger.Info("Delivered checkpoint", "epoch", header.Epoch)
	return abci.ExecTxResult{Code: 0}
}

// deliverExecutionReport processes an execution report transaction
func (app *SubnetABCIApp) deliverExecutionReport(payload []byte) abci.ExecTxResult {
	report := &pb.ExecutionReport{}
	if err := proto.Unmarshal(payload, report); err != nil {
		return abci.ExecTxResult{Code: 1, Log: err.Error()}
	}

	// Store report
	reportKey := fmt.Sprintf("%s-%s", report.IntentId, report.AssignmentId)
	app.pendingReports[reportKey] = report

	// Notify handler (non-blocking)
	if app.handlers.OnExecutionReportCommitted != nil {
		go app.handlers.OnExecutionReportCommitted(report, reportKey)
	}

	app.logger.Debug("Delivered execution report", "intent_id", report.IntentId)
	return abci.ExecTxResult{Code: 0}
}

// deliverValidationBundleSignature processes a ValidationBundle signature transaction
func (app *SubnetABCIApp) deliverValidationBundleSignature(payload []byte) abci.ExecTxResult {
	var sigTx ValidationBundleSignatureTx
	if err := json.Unmarshal(payload, &sigTx); err != nil {
		return abci.ExecTxResult{Code: 1, Log: err.Error()}
	}

	// Initialize epoch map if needed
	if app.validationBundleSignatures[sigTx.Epoch] == nil {
		app.validationBundleSignatures[sigTx.Epoch] = make(map[string][]byte)
	}

	// Store signature
	app.validationBundleSignatures[sigTx.Epoch][sigTx.ValidatorAddr] = sigTx.Signature

	app.logger.Info("Received ValidationBundle signature",
		"epoch", sigTx.Epoch,
		"validator", sigTx.ValidatorAddr,
		"sig_count", len(app.validationBundleSignatures[sigTx.Epoch]))

	// Check if we reached threshold after receiving this signature
	go app.checkAndSubmitValidationBundle(sigTx.Epoch)

	return abci.ExecTxResult{Code: 0}
}

// Commit persists the application state and triggers ValidationBundle signing
func (app *SubnetABCIApp) Commit(ctx context.Context, req *abci.RequestCommit) (*abci.ResponseCommit, error) {
	app.mu.Lock()
	defer app.mu.Unlock()

	// NOTE: We no longer call processValidationBundleSignature here
	// ECDSA signatures are now collected via vote extensions in ExtendVote/FinalizeBlock

	// Compute app hash (simple version: hash of current state)
	stateData := map[string]interface{}{
		"epoch":       app.currentEpoch,
		"checkpoints": len(app.checkpoints),
		"reports":     len(app.pendingReports),
	}
	stateBytes, _ := json.Marshal(stateData)
	app.latestAppHash = stateBytes // In production, use proper hash function

	app.logger.Debug("Committed state",
		"epoch", app.currentEpoch,
		"height", app.latestBlockHeight,
		"hash", fmt.Sprintf("%x", app.latestAppHash[:min(8, len(app.latestAppHash))]))

	return &abci.ResponseCommit{
		RetainHeight: 0,
	}, nil
}

// processValidationBundleSignature handles the signing and submission of ValidationBundle
func (app *SubnetABCIApp) processValidationBundleSignature(epoch uint64) {
	// Check if ECDSA signer is configured
	if app.ecdsaSigner == nil {
		app.logger.Debug("ECDSA signer not configured, skipping ValidationBundle signing")
		return
	}

	checkpoint, exists := app.checkpoints[epoch]
	if !exists {
		app.logger.Warn("Checkpoint not found for epoch", "epoch", epoch)
		return
	}

	// Check if we already signed this epoch
	myAddress := app.ecdsaSigner.Address()
	if app.validationBundleSignatures[epoch] != nil {
		if _, alreadySigned := app.validationBundleSignatures[epoch][myAddress]; alreadySigned {
			app.logger.Debug("Already signed this epoch", "epoch", epoch)
			return
		}
	}

	// Compute checkpoint hash
	checkpointHash := app.computeCheckpointHash(checkpoint)

	// Sign with ECDSA key
	signature, err := app.ecdsaSigner.Sign(checkpointHash)
	if err != nil {
		app.logger.Error("Failed to sign checkpoint", "error", err, "epoch", epoch)
		return
	}

	app.logger.Info("Signed checkpoint with ECDSA",
		"epoch", epoch,
		"validator", app.ecdsaSigner.Address())

	// Broadcast signature as transaction
	sigTx := ValidationBundleSignatureTx{
		Epoch:          epoch,
		CheckpointHash: checkpointHash,
		ValidatorAddr:  app.ecdsaSigner.Address(),
		Signature:      signature,
	}

	if err := app.broadcastSignature(sigTx); err != nil {
		app.logger.Error("Failed to broadcast signature", "error", err)
		return
	}

}

// checkAndSubmitValidationBundle checks if threshold is reached and submits ValidationBundle
func (app *SubnetABCIApp) checkAndSubmitValidationBundle(epoch uint64) {
	app.mu.Lock()
	defer app.mu.Unlock()

	// Check if already submitted
	if app.lastProcessedEpoch >= epoch {
		return
	}

	if !app.hasThresholdSignatures(epoch) {
		return
	}

	app.logger.Info("Threshold signatures reached, constructing ValidationBundle",
		"epoch", epoch,
		"signatures", len(app.validationBundleSignatures[epoch]))

	bundle := app.constructValidationBundle(epoch)
	if bundle == nil {
		app.logger.Error("Failed to construct ValidationBundle")
		return
	}

	if err := app.submitToRootLayer(bundle); err != nil {
		app.logger.Error("Failed to submit ValidationBundle to RootLayer", "error", err)
		return
	}

	app.lastProcessedEpoch = epoch
	app.logger.Info("Successfully submitted ValidationBundle to RootLayer", "epoch", epoch)
}

// computeCheckpointHash computes the hash of a checkpoint header
func (app *SubnetABCIApp) computeCheckpointHash(checkpoint *pb.CheckpointHeader) []byte {
	// Serialize checkpoint to bytes
	data, err := proto.Marshal(checkpoint)
	if err != nil {
		app.logger.Error("Failed to marshal checkpoint", "error", err)
		return nil
	}

	// In production, use proper crypto hash (e.g., Keccak256 for Ethereum compatibility)
	// For now, just return the marshaled data
	return data
}

// broadcastSignature broadcasts a signature transaction to the mempool
func (app *SubnetABCIApp) broadcastSignature(sigTx ValidationBundleSignatureTx) error {
	if app.mempoolBroadcaster == nil {
		app.logger.Warn("Mempool broadcaster not configured, cannot broadcast signature")
		return fmt.Errorf("mempool broadcaster not configured")
	}

	// Serialize signature transaction
	payload, err := json.Marshal(sigTx)
	if err != nil {
		return fmt.Errorf("failed to marshal signature tx: %w", err)
	}

	// Prepend transaction type
	tx := append([]byte{byte(TxTypeValidationBundleSignature)}, payload...)

	// Broadcast to mempool
	if err := app.mempoolBroadcaster.BroadcastTxSync(tx); err != nil {
		return fmt.Errorf("failed to broadcast tx: %w", err)
	}

	return nil
}

// hasThresholdSignatures checks if we have enough signatures (2/3 threshold)
func (app *SubnetABCIApp) hasThresholdSignatures(epoch uint64) bool {
	signatures, exists := app.validationBundleSignatures[epoch]
	if !exists {
		app.logger.Info("No signatures exist for epoch", "epoch", epoch)
		return false
	}

	totalValidators := len(app.validatorSet)
	if totalValidators == 0 {
		app.logger.Warn("validatorSet is empty, cannot check threshold")
		return false
	}

	// For now, we count signatures by ECDSA address, not CometBFT validator pubkey
	// This is because ValidationBundle signatures use ECDSA addresses
	threshold := (totalValidators*2)/3 + 1

	app.logger.Info("Checking threshold",
		"epoch", epoch,
		"signatures", len(signatures),
		"threshold", threshold,
		"total_validators", totalValidators)

	return len(signatures) >= threshold
}

// constructValidationBundle constructs a CheckpointSignatureBundle from collected signatures
func (app *SubnetABCIApp) constructValidationBundle(epoch uint64) *CheckpointSignatureBundle {
	signatures, exists := app.validationBundleSignatures[epoch]
	if !exists {
		return nil
	}

	checkpoint, exists := app.checkpoints[epoch]
	if !exists {
		return nil
	}

	checkpointHash := app.computeCheckpointHash(checkpoint)

	// Collect all execution reports from pendingReports map
	intentReports := make([]*pb.ExecutionReport, 0, len(app.pendingReports))
	for _, report := range app.pendingReports {
		intentReports = append(intentReports, report)
	}

	return &CheckpointSignatureBundle{
		Epoch:               epoch,
		CheckpointHash:      checkpointHash,
		ValidatorSignatures: signatures,
		Timestamp:           checkpoint.Timestamp,
		IntentReports:       intentReports,
	}
}

// submitToRootLayer submits a CheckpointSignatureBundle to the RootLayer
func (app *SubnetABCIApp) submitToRootLayer(bundle *CheckpointSignatureBundle) error {
	if app.rootlayerClient == nil {
		app.logger.Warn("RootLayer client not configured, cannot submit CheckpointSignatureBundle")
		return fmt.Errorf("rootlayer client not configured")
	}

	return app.rootlayerClient.SubmitCheckpointSignatures(bundle)
}

// Query handles queries for application state
func (app *SubnetABCIApp) Query(ctx context.Context, req *abci.RequestQuery) (*abci.ResponseQuery, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()

	switch req.Path {
	case "/checkpoint/latest":
		if app.currentEpoch == 0 {
			return &abci.ResponseQuery{Code: 1, Log: "no checkpoints"}, nil
		}
		header := app.checkpoints[app.currentEpoch]
		data, _ := proto.Marshal(header)
		return &abci.ResponseQuery{Code: 0, Value: data}, nil

	case "/checkpoint/epoch":
		var epoch uint64
		if err := json.Unmarshal(req.Data, &epoch); err != nil {
			return &abci.ResponseQuery{Code: 1, Log: err.Error()}, nil
		}
		header, exists := app.checkpoints[epoch]
		if !exists {
			return &abci.ResponseQuery{Code: 1, Log: "checkpoint not found"}, nil
		}
		data, _ := proto.Marshal(header)
		return &abci.ResponseQuery{Code: 0, Value: data}, nil

	default:
		return &abci.ResponseQuery{Code: 1, Log: "unknown query path"}, nil
	}
}

// GetState returns the current application state (for testing/debugging)
func (app *SubnetABCIApp) GetState() map[string]interface{} {
	app.mu.RLock()
	defer app.mu.RUnlock()

	return map[string]interface{}{
		"current_epoch":   app.currentEpoch,
		"checkpoints":     len(app.checkpoints),
		"pending_reports": len(app.pendingReports),
		"validators":      len(app.validatorSet),
	}
}

// Additional ABCI 2.0 methods (stubs for now)

func (app *SubnetABCIApp) ListSnapshots(ctx context.Context, req *abci.RequestListSnapshots) (*abci.ResponseListSnapshots, error) {
	return &abci.ResponseListSnapshots{}, nil
}

func (app *SubnetABCIApp) OfferSnapshot(ctx context.Context, req *abci.RequestOfferSnapshot) (*abci.ResponseOfferSnapshot, error) {
	return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}, nil
}

func (app *SubnetABCIApp) LoadSnapshotChunk(ctx context.Context, req *abci.RequestLoadSnapshotChunk) (*abci.ResponseLoadSnapshotChunk, error) {
	return &abci.ResponseLoadSnapshotChunk{}, nil
}

func (app *SubnetABCIApp) ApplySnapshotChunk(ctx context.Context, req *abci.RequestApplySnapshotChunk) (*abci.ResponseApplySnapshotChunk, error) {
	return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ABORT}, nil
}

func (app *SubnetABCIApp) PrepareProposal(ctx context.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	// Process vote extensions from LocalLastCommit to collect ECDSA signatures
	app.mu.Lock()
	app.processVoteExtensions(req.LocalLastCommit)
	app.mu.Unlock()

	return &abci.ResponsePrepareProposal{Txs: req.Txs}, nil
}

func (app *SubnetABCIApp) ProcessProposal(ctx context.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
	return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
}

func (app *SubnetABCIApp) ExtendVote(ctx context.Context, req *abci.RequestExtendVote) (*abci.ResponseExtendVote, error) {
	// If ECDSA signer not configured, return empty extension
	if app.ecdsaSigner == nil {
		return &abci.ResponseExtendVote{}, nil
	}

	app.mu.RLock()
	epoch := app.currentEpoch
	checkpoint, exists := app.checkpoints[epoch]
	app.mu.RUnlock()

	// If no checkpoint for current epoch, return empty extension
	if !exists {
		return &abci.ResponseExtendVote{}, nil
	}

	// Compute checkpoint hash
	checkpointHash := app.computeCheckpointHash(checkpoint)

	// Sign with ECDSA key
	signature, err := app.ecdsaSigner.Sign(checkpointHash)
	if err != nil {
		app.logger.Error("Failed to sign checkpoint in ExtendVote", "error", err, "epoch", epoch)
		return &abci.ResponseExtendVote{}, nil
	}

	// Create vote extension with ECDSA signature
	voteExt := VoteExtension{
		Epoch:          epoch,
		CheckpointHash: checkpointHash,
		ValidatorAddr:  app.ecdsaSigner.Address(),
		Signature:      signature,
	}

	// Serialize vote extension
	extBytes, err := json.Marshal(voteExt)
	if err != nil {
		app.logger.Error("Failed to marshal vote extension", "error", err)
		return &abci.ResponseExtendVote{}, nil
	}

	app.logger.Debug("Extended vote with ECDSA signature",
		"epoch", epoch,
		"validator", app.ecdsaSigner.Address(),
		"height", req.Height)

	return &abci.ResponseExtendVote{
		VoteExtension: extBytes,
	}, nil
}

func (app *SubnetABCIApp) VerifyVoteExtension(ctx context.Context, req *abci.RequestVerifyVoteExtension) (*abci.ResponseVerifyVoteExtension, error) {
	// Empty extensions are valid (validator might not have ECDSA signer configured)
	if len(req.VoteExtension) == 0 {
		return &abci.ResponseVerifyVoteExtension{Status: abci.ResponseVerifyVoteExtension_ACCEPT}, nil
	}

	// Deserialize vote extension
	var voteExt VoteExtension
	if err := json.Unmarshal(req.VoteExtension, &voteExt); err != nil {
		app.logger.Warn("Failed to unmarshal vote extension", "error", err)
		return &abci.ResponseVerifyVoteExtension{Status: abci.ResponseVerifyVoteExtension_REJECT}, nil
	}

	// TODO: Verify ECDSA signature
	// For now, we accept all vote extensions
	// In production, you would verify the signature using crypto/ecdsa

	app.logger.Debug("Verified vote extension",
		"epoch", voteExt.Epoch,
		"validator", voteExt.ValidatorAddr,
		"height", req.Height)

	return &abci.ResponseVerifyVoteExtension{Status: abci.ResponseVerifyVoteExtension_ACCEPT}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
