package cometbft

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cmtcfg "github.com/cometbft/cometbft/config"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	cmtmempool "github.com/cometbft/cometbft/mempool"
	cmtnode "github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	cmttypes "github.com/cometbft/cometbft/types"
	"google.golang.org/protobuf/proto"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"
)

// Consensus implements ConsensusEngine using CometBFT (Tendermint)
type Consensus struct {
	mu sync.RWMutex

	config   *Config
	logger   logging.Logger
	handlers ConsensusHandlers

	// CometBFT components
	node *cmtnode.Node
	app  *SubnetABCIApp

	// Runtime state
	ctx      context.Context
	cancel   context.CancelFunc
	ready    bool
	nodeID   string
	isLeader bool
}

// NewConsensus creates a new CometBFT consensus engine
func NewConsensus(
	cfg *Config,
	handlers ConsensusHandlers,
	logger logging.Logger,
) (*Consensus, error) {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cometbft config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Consensus{
		config:   cfg,
		logger:   logger,
		handlers: handlers,
		ctx:      ctx,
		cancel:   cancel,
	}

	return c, nil
}

// Start initializes and starts the CometBFT node
func (c *Consensus) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ready {
		return nil
	}

	c.logger.Info("Starting CometBFT consensus engine", "home", c.config.HomeDir)

	// Create home directory
	if err := os.MkdirAll(c.config.HomeDir, 0755); err != nil {
		return fmt.Errorf("create home dir: %w", err)
	}

	// Initialize CometBFT config
	cmtConfig := c.buildCometBFTConfig()

	// Create ABCI application with full configuration
	appConfig := SubnetABCIAppConfig{
		Handlers:           c.handlers,
		ValidatorSet:       c.config.GenesisValidators,
		ECDSASigner:        c.config.ECDSASigner,
		RootLayerClient:    c.config.RootLayerClient,
		MempoolBroadcaster: nil, // Will be set after node is created
		Logger:             c.logger,
	}
	c.app = NewSubnetABCIAppWithConfig(appConfig)

	// Create node key (for P2P)
	nodeKeyFile := filepath.Join(c.config.HomeDir, "config", "node_key.json")
	if err := os.MkdirAll(filepath.Dir(nodeKeyFile), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	nodeKey, err := p2p.LoadOrGenNodeKey(nodeKeyFile)
	if err != nil {
		return fmt.Errorf("load or generate node key: %w", err)
	}
	c.nodeID = string(nodeKey.ID())

	// Create or load private validator
	privValKeyFile := filepath.Join(c.config.HomeDir, "config", "priv_validator_key.json")
	privValStateFile := filepath.Join(c.config.HomeDir, "data", "priv_validator_state.json")

	// Ensure both config and data directories exist
	if err := os.MkdirAll(filepath.Dir(privValKeyFile), 0755); err != nil {
		return fmt.Errorf("create config dir for priv validator: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(privValStateFile), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// LoadOrGenFilePV will generate the files if they don't exist
	pv := privval.LoadOrGenFilePV(privValKeyFile, privValStateFile)

	// Create genesis file if not exists
	genesisFile := filepath.Join(c.config.HomeDir, "config", "genesis.json")
	if _, err := os.Stat(genesisFile); os.IsNotExist(err) {
		if err := c.generateGenesis(genesisFile, pv); err != nil {
			return fmt.Errorf("generate genesis: %w", err)
		}
	}

	// Create CometBFT logger
	cmtLogger := cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout))
	if c.config.LogFormat == "json" {
		cmtLogger = cmtlog.NewTMJSONLogger(cmtlog.NewSyncWriter(os.Stdout))
	}
	// Filter out debug logs to reduce noise
	cmtLogger = cmtlog.NewFilter(cmtLogger, cmtlog.AllowError())

	// Create node
	node, err := cmtnode.NewNode(
		cmtConfig,
		pv,
		nodeKey,
		proxy.NewLocalClientCreator(c.app),
		cmtnode.DefaultGenesisDocProviderFunc(cmtConfig),
		cmtcfg.DefaultDBProvider,
		cmtnode.DefaultMetricsProvider(cmtConfig.Instrumentation),
		cmtLogger,
	)
	if err != nil {
		return fmt.Errorf("create cometbft node: %w", err)
	}

	c.node = node

	// Set mempool broadcaster in ABCI app (now that node is created)
	c.app.mempoolBroadcaster = &mempoolBroadcasterAdapter{
		mempool: c.node.Mempool(),
		logger:  c.logger,
	}

	// Start node
	if err := c.node.Start(); err != nil {
		return fmt.Errorf("start node: %w", err)
	}

	c.ready = true
	c.logger.Info("CometBFT consensus engine started", "node_id", c.nodeID)

	// Monitor leadership (proposer in CometBFT rotates)
	go c.monitorLeadership()

	return nil
}

// Stop gracefully stops the CometBFT node
func (c *Consensus) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.ready {
		return nil
	}

	c.logger.Info("Stopping CometBFT consensus engine")
	c.cancel()

	if c.node != nil {
		if err := c.node.Stop(); err != nil {
			return fmt.Errorf("stop node: %w", err)
		}
		c.node.Wait()
	}

	c.ready = false
	return nil
}

// IsReady returns whether the consensus engine is ready
func (c *Consensus) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// IsLeader returns whether this node is the current proposer
// In CometBFT, proposer rotates, so this is approximate
func (c *Consensus) IsLeader() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isLeader
}

// GetLeaderID returns the current leader's ID
func (c *Consensus) GetLeaderID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.isLeader {
		return c.nodeID
	}
	return ""
}

// GetLeaderAddress returns the current leader's address
func (c *Consensus) GetLeaderAddress() string {
	// CometBFT doesn't have a fixed leader, proposer rotates
	return ""
}

// ProposeCheckpoint submits a checkpoint proposal to CometBFT
func (c *Consensus) ProposeCheckpoint(header *pb.CheckpointHeader) error {
	if !c.IsReady() {
		return fmt.Errorf("consensus engine not ready")
	}

	// Encode checkpoint as transaction
	payload, err := proto.Marshal(header)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	tx := append([]byte{byte(TxTypeCheckpoint)}, payload...)

	// Broadcast transaction to mempool
	if err := c.node.Mempool().CheckTx(tx, nil, cmtmempool.TxInfo{}); err != nil {
		return fmt.Errorf("submit checkpoint tx: %w", err)
	}

	c.logger.Info("Proposed checkpoint", "epoch", header.Epoch)
	return nil
}

// GetLatestCheckpoint returns the latest committed checkpoint
func (c *Consensus) GetLatestCheckpoint() *pb.CheckpointHeader {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.app == nil {
		return nil
	}

	return c.app.checkpoints[c.app.currentEpoch]
}

// BroadcastSignature is a no-op for CometBFT (signatures are part of consensus)
func (c *Consensus) BroadcastSignature(sig *pb.Signature, epoch uint64, checkpointHash []byte) error {
	// In CometBFT, signatures are automatically collected during consensus voting
	// This is a no-op for compatibility with the interface
	c.logger.Debug("BroadcastSignature called (no-op in CometBFT)", "epoch", epoch)
	return nil
}

// GetSignatures returns signatures from the last committed block
// In CometBFT, signatures are stored in block commits
func (c *Consensus) GetSignatures(epoch uint64) []*pb.Signature {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.node == nil {
		return nil
	}

	// Get the latest block commit
	// Note: This is a simplified implementation
	// In production, you'd track which block corresponds to which epoch
	height := c.node.BlockStore().Height()
	if height == 0 {
		return nil
	}

	commit := c.node.BlockStore().LoadBlockCommit(height)
	if commit == nil {
		return nil
	}

	// Convert CometBFT signatures to our format
	signatures := make([]*pb.Signature, 0, len(commit.Signatures))
	for _, commitSig := range commit.Signatures {
		if commitSig.BlockIDFlag == cmttypes.BlockIDFlagAbsent {
			continue
		}

		sig := &pb.Signature{
			SignerId: string(commitSig.ValidatorAddress),
			Der:      commitSig.Signature,
			// Note: CometBFT uses a different signature format
			// You may need to convert or adapt the format
		}
		signatures = append(signatures, sig)
	}

	return signatures
}

// CheckThreshold checks if we have enough signatures (always true after commit in BFT)
func (c *Consensus) CheckThreshold(epoch uint64) bool {
	// In CometBFT, if a block is committed, it automatically has 2/3+ signatures
	// So we just check if the block for this epoch exists
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.app == nil {
		return false
	}

	_, exists := c.app.checkpoints[epoch]
	return exists
}

// ProposeExecutionReport submits an execution report
func (c *Consensus) ProposeExecutionReport(report *pb.ExecutionReport, reportKey string) error {
	if !c.IsReady() {
		return fmt.Errorf("consensus engine not ready")
	}

	payload, err := proto.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	tx := append([]byte{byte(TxTypeExecutionReport)}, payload...)

	if err := c.node.Mempool().CheckTx(tx, nil, cmtmempool.TxInfo{}); err != nil {
		return fmt.Errorf("submit report tx: %w", err)
	}

	return nil
}

// BroadcastValidationBundleSignature broadcasts a ValidationBundle signature via CometBFT transaction
func (c *Consensus) BroadcastValidationBundleSignature(vbSig *pb.ValidationBundleSignature) error {
	if !c.IsReady() {
		return fmt.Errorf("consensus engine not ready")
	}

	if c.node == nil {
		return fmt.Errorf("CometBFT node not initialized")
	}

	payload, err := proto.Marshal(vbSig)
	if err != nil {
		return fmt.Errorf("marshal ValidationBundleSignature: %w", err)
	}

	tx := append([]byte{byte(TxTypeValidationBundleSignature)}, payload...)

	// Use CheckTx to add transaction to mempool
	if err := c.node.Mempool().CheckTx(tx, nil, cmtmempool.TxInfo{}); err != nil {
		return fmt.Errorf("submit ValidationBundleSignature tx: %w", err)
	}

	c.logger.Debug("Broadcasted ValidationBundleSignature via CometBFT",
		"epoch", vbSig.Epoch,
		"validator", vbSig.ValidatorAddress)

	return nil
}

// GetPendingReports returns all pending execution reports
func (c *Consensus) GetPendingReports() map[string]*pb.ExecutionReport {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.app == nil {
		return nil
	}

	return c.app.pendingReports
}

// ClearPendingReports removes reports from pending state
func (c *Consensus) ClearPendingReports(keys []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.app == nil {
		return
	}

	for _, key := range keys {
		delete(c.app.pendingReports, key)
	}
}

// GetCurrentEpoch returns the current epoch
func (c *Consensus) GetCurrentEpoch() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.app == nil {
		return 0
	}

	return c.app.currentEpoch
}

// GetValidatorSet returns the current validator set
func (c *Consensus) GetValidatorSet() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.app == nil {
		return nil
	}

	validators := make([]string, 0, len(c.app.validatorSet))
	for validatorID := range c.app.validatorSet {
		validators = append(validators, validatorID)
	}
	return validators
}

// monitorLeadership monitors whether this node is the current proposer
func (c *Consensus) monitorLeadership() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			// In CometBFT, proposer rotates round-robin
			// This is simplified; real implementation would query consensus state
			// For now, we don't track leadership precisely
		}
	}
}

// buildCometBFTConfig converts our config to CometBFT config
func (c *Consensus) buildCometBFTConfig() *cmtcfg.Config {
	cfg := cmtcfg.DefaultConfig()
	cfg.SetRoot(c.config.HomeDir)

	// Basic config
	cfg.Moniker = c.config.Moniker
	cfg.DBBackend = c.config.DBBackend
	cfg.DBPath = c.config.DBDir
	if cfg.DBPath == "" {
		cfg.DBPath = "data"
	}
	cfg.LogLevel = c.config.LogLevel
	cfg.LogFormat = c.config.LogFormat

	// P2P config
	cfg.P2P.ListenAddress = c.config.P2PListenAddress
	cfg.P2P.Seeds = strings.Join(c.config.Seeds, ",")
	cfg.P2P.PersistentPeers = strings.Join(c.config.PersistentPeers, ",")
	cfg.P2P.AllowDuplicateIP = true // Allow localhost connections for testing
	cfg.P2P.AddrBookStrict = false  // Allow non-routable addresses (127.0.0.1)

	// RPC config
	cfg.RPC.ListenAddress = c.config.RPCListenAddress

	// Consensus config
	cfg.Consensus.TimeoutPropose = c.config.TimeoutPropose
	cfg.Consensus.TimeoutProposeDelta = c.config.TimeoutPropose / 2
	cfg.Consensus.TimeoutPrevote = c.config.TimeoutPrevote
	cfg.Consensus.TimeoutPrevoteDelta = c.config.TimeoutPrevote / 2
	cfg.Consensus.TimeoutPrecommit = c.config.TimeoutPrecommit
	cfg.Consensus.TimeoutPrecommitDelta = c.config.TimeoutPrecommit / 2
	cfg.Consensus.TimeoutCommit = c.config.TimeoutCommit
	cfg.Consensus.CreateEmptyBlocks = c.config.CreateEmptyBlocks
	cfg.Consensus.CreateEmptyBlocksInterval = c.config.CreateEmptyBlocksInterval

	// Mempool config
	cfg.Mempool.Size = c.config.MempoolSize
	cfg.Mempool.Recheck = c.config.MempoolRecheck
	cfg.Mempool.Broadcast = c.config.MempoolBroadcast

	// State sync
	cfg.StateSync.Enable = c.config.StateSyncEnable

	return cfg
}

// generateGenesis creates a genesis file for the chain
func (c *Consensus) generateGenesis(genesisFile string, pv *privval.FilePV) error {
	pubKey, err := pv.GetPubKey()
	if err != nil {
		return err
	}

	// Build validator list from config
	validators := make([]cmttypes.GenesisValidator, 0, len(c.config.GenesisValidators))

	// Add this node as a validator
	validators = append(validators, cmttypes.GenesisValidator{
		Address: pubKey.Address(),
		PubKey:  pubKey,
		Power:   10,
		Name:    c.config.Moniker,
	})

	genDoc := cmttypes.GenesisDoc{
		ChainID:         c.config.ChainID,
		GenesisTime:     time.Now(),
		ConsensusParams: cmttypes.DefaultConsensusParams(),
		Validators:      validators,
	}

	if err := genDoc.SaveAs(genesisFile); err != nil {
		return fmt.Errorf("save genesis: %w", err)
	}

	c.logger.Info("Generated genesis file", "file", genesisFile, "validators", len(validators))
	return nil
}

// mempoolBroadcasterAdapter adapts CometBFT mempool to MempoolBroadcaster interface
type mempoolBroadcasterAdapter struct {
	mempool cmtmempool.Mempool
	logger  logging.Logger
}

// BroadcastTxSync broadcasts a transaction to the mempool synchronously
func (m *mempoolBroadcasterAdapter) BroadcastTxSync(tx []byte) error {
	if m.mempool == nil {
		return fmt.Errorf("mempool not available")
	}

	// CheckTx adds the transaction to the mempool and broadcasts to peers
	err := m.mempool.CheckTx(tx, nil, cmtmempool.TxInfo{})
	if err != nil {
		m.logger.Error("Failed to broadcast transaction to mempool", "error", err)
		return fmt.Errorf("broadcast tx: %w", err)
	}

	m.logger.Debug("Successfully broadcast transaction to mempool", "tx_size", len(tx))
	return nil
}
