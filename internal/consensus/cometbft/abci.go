package cometbft

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	abci "github.com/cometbft/cometbft/abci/types"
	"google.golang.org/protobuf/proto"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"
)

// TxType represents the type of transaction
type TxType byte

const (
	TxTypeCheckpoint      TxType = 0x01
	TxTypeExecutionReport TxType = 0x02
)

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
	checkpoints        map[uint64]*pb.CheckpointHeader
	pendingReports     map[string]*pb.ExecutionReport
	latestBlockHeight  int64
	latestAppHash      []byte

	// Validator set (validator_id -> voting_power)
	validatorSet map[string]int64
}

// NewSubnetABCIApp creates a new ABCI application for subnet consensus
func NewSubnetABCIApp(handlers ConsensusHandlers, validatorSet map[string]int64, logger logging.Logger) *SubnetABCIApp {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	if validatorSet == nil {
		validatorSet = make(map[string]int64)
	}

	return &SubnetABCIApp{
		logger:         logger,
		handlers:       handlers,
		checkpoints:    make(map[uint64]*pb.CheckpointHeader),
		pendingReports: make(map[string]*pb.ExecutionReport),
		validatorSet:   validatorSet,
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

	// Initialize validator set from genesis
	for _, v := range req.Validators {
		validatorID := string(v.PubKey.GetEd25519())
		app.validatorSet[validatorID] = v.Power
		app.logger.Debug("Added validator", "id", validatorID, "power", v.Power)
	}

	return &abci.ResponseInitChain{}, nil
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
		if header.Epoch == 0 {
			return &abci.ResponseCheckTx{Code: 1, Log: "invalid epoch"}, nil
		}
		return &abci.ResponseCheckTx{Code: 0}, nil

	case TxTypeExecutionReport:
		report := &pb.ExecutionReport{}
		if err := proto.Unmarshal(payload, report); err != nil {
			return &abci.ResponseCheckTx{Code: 1, Log: fmt.Sprintf("invalid report: %v", err)}, nil
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

		default:
			txResults[i] = &abci.ExecTxResult{Code: 1, Log: "unknown transaction type"}
		}
	}

	return &abci.ResponseFinalizeBlock{
		TxResults: txResults,
	}, nil
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

// Commit persists the application state
func (app *SubnetABCIApp) Commit(ctx context.Context, req *abci.RequestCommit) (*abci.ResponseCommit, error) {
	app.mu.Lock()
	defer app.mu.Unlock()

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
	return &abci.ResponsePrepareProposal{Txs: req.Txs}, nil
}

func (app *SubnetABCIApp) ProcessProposal(ctx context.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
	return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
}

func (app *SubnetABCIApp) ExtendVote(ctx context.Context, req *abci.RequestExtendVote) (*abci.ResponseExtendVote, error) {
	return &abci.ResponseExtendVote{}, nil
}

func (app *SubnetABCIApp) VerifyVoteExtension(ctx context.Context, req *abci.RequestVerifyVoteExtension) (*abci.ResponseVerifyVoteExtension, error) {
	return &abci.ResponseVerifyVoteExtension{Status: abci.ResponseVerifyVoteExtension_ACCEPT}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
