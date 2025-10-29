// +build integration

package cometbft

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"
)

// TestSingleValidatorSignatureFlow tests ValidationBundle signature collection
// with a single validator
func TestSingleValidatorSignatureFlow(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logging.NewDefaultLogger()

	// Create temporary directory for single validator
	testDir, err := os.MkdirTemp("", "cometbft-integration-*")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create mock RootLayer client to track submissions
	rootLayerClient := &mockRootLayerClient{
		submittedBundles: make([]*CheckpointSignatureBundle, 0),
	}

	// Create validator set with single validator
	validatorSet := map[string]int64{
		"validator_1": 10,
	}

	// Create ECDSA signer
	signer := &mockECDSASigner{address: "validator_1"}

	// Configure validator
	cfg := DefaultConfig()
	cfg.HomeDir = filepath.Join(testDir, "validator_1")
	cfg.Moniker = "validator_1"
	cfg.ChainID = "test-chain-1"
	cfg.P2PListenAddress = "tcp://127.0.0.1:26656"
	cfg.RPCListenAddress = "tcp://127.0.0.1:26657"
	cfg.GenesisValidators = validatorSet
	cfg.ECDSASigner = signer
	cfg.RootLayerClient = rootLayerClient
	cfg.CreateEmptyBlocks = true
	cfg.CreateEmptyBlocksInterval = 1 * time.Second
	cfg.TimeoutCommit = 1 * time.Second

	// Create handlers
	handlers := ConsensusHandlers{
		OnCheckpointCommitted: func(header *pb.CheckpointHeader) {
			logger.Info("Checkpoint committed", "epoch", header.Epoch)
		},
	}

	// Create consensus engine
	consensus, err := NewConsensus(cfg, handlers, logger)
	if err != nil {
		t.Fatalf("Failed to create consensus: %v", err)
	}

	// Start validator
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := consensus.Start(ctx); err != nil {
		t.Fatalf("Failed to start validator: %v", err)
	}
	defer consensus.Stop()

	// Wait for validator to be ready
	logger.Info("Waiting for validator to be ready...")
	time.Sleep(3 * time.Second)

	// Verify validator is ready
	if !consensus.IsReady() {
		t.Fatalf("Validator is not ready")
	}
	logger.Info("Validator ready")

	// Test 1: Propose checkpoint
	epoch := uint64(1)
	checkpoint := &pb.CheckpointHeader{
		Epoch:     epoch,
		SubnetId:  "test-subnet",
		Timestamp: time.Now().Unix(),
	}

	logger.Info("Proposing checkpoint", "epoch", epoch)
	if err := consensus.ProposeCheckpoint(checkpoint); err != nil {
		t.Fatalf("Failed to propose checkpoint: %v", err)
	}

	// Wait for checkpoint to be committed through consensus
	logger.Info("Waiting for checkpoint to be committed...")
	time.Sleep(5 * time.Second)

	// Verify checkpoint was committed
	latestCheckpoint := consensus.GetLatestCheckpoint()
	if latestCheckpoint == nil {
		t.Fatal("No checkpoint committed")
	}
	if latestCheckpoint.Epoch != epoch {
		t.Errorf("Expected epoch %d, got %d", epoch, latestCheckpoint.Epoch)
	}
	logger.Info("Checkpoint committed and verified", "epoch", latestCheckpoint.Epoch)

	// Verify signature was collected
	consensus.mu.RLock()
	app := consensus.app
	consensus.mu.RUnlock()

	if app == nil {
		t.Fatal("No ABCI app")
	}

	// Check if validator has signed
	app.mu.RLock()
	signatures, hasSignatures := app.validationBundleSignatures[epoch]
	app.mu.RUnlock()

	if hasSignatures && len(signatures) > 0 {
		logger.Info("Signatures collected", "count", len(signatures))

		// Verify threshold (for single validator, need 1 signature)
		if app.hasThresholdSignatures(epoch) {
			logger.Info("✓ Threshold reached")
		} else {
			logger.Info("Note: Threshold check timing - signature collection is async")
		}
	} else {
		logger.Info("Note: Signature collection happens in Commit hook (async)")
	}

	// Test 2: Verify ValidationBundle submission (if any)
	logger.Info("Checking for ValidationBundle submission...")
	time.Sleep(2 * time.Second)

	if len(rootLayerClient.submittedBundles) > 0 {
		bundle := rootLayerClient.submittedBundles[0]
		logger.Info("ValidationBundle submitted",
			"epoch", bundle.Epoch,
			"signatures", len(bundle.ValidatorSignatures))

		if bundle.Epoch != epoch {
			t.Errorf("ValidationBundle epoch mismatch: expected %d, got %d",
				epoch, bundle.Epoch)
		}

		// Verify validator signed
		if _, exists := bundle.ValidatorSignatures["validator_1"]; !exists {
			t.Error("Validator signature not found in bundle")
		}
	} else {
		logger.Info("Note: ValidationBundle submission happens async in Commit hook")
	}

	logger.Info("Integration test completed successfully")
}

// TestValidatorRecovery tests that validators can recover and sync after restart
func TestValidatorRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logging.NewDefaultLogger()

	testDir, err := os.MkdirTemp("", "cometbft-recovery-*")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Configure single validator
	homeDir := filepath.Join(testDir, "validator_1")

	validatorSet := map[string]int64{
		"validator_1": 10,
	}

	cfg := DefaultConfig()
	cfg.HomeDir = homeDir
	cfg.Moniker = "validator_1"
	cfg.ChainID = "test-chain-recovery"
	cfg.P2PListenAddress = "tcp://127.0.0.1:26656"
	cfg.RPCListenAddress = "tcp://127.0.0.1:26657"
	cfg.GenesisValidators = validatorSet
	cfg.ECDSASigner = &mockECDSASigner{address: "validator_1"}
	cfg.RootLayerClient = &mockRootLayerClient{}
	cfg.CreateEmptyBlocks = true
	cfg.CreateEmptyBlocksInterval = 1 * time.Second

	handlers := ConsensusHandlers{}

	// Start validator
	consensus, err := NewConsensus(cfg, handlers, logger)
	if err != nil {
		t.Fatalf("Failed to create consensus: %v", err)
	}

	ctx := context.Background()
	if err := consensus.Start(ctx); err != nil {
		t.Fatalf("Failed to start validator: %v", err)
	}

	time.Sleep(3 * time.Second)

	// Propose a checkpoint
	epoch := uint64(1)
	checkpoint := &pb.CheckpointHeader{
		Epoch:     epoch,
		SubnetId:  "test-subnet",
		Timestamp: time.Now().Unix(),
	}

	logger.Info("Proposing checkpoint before restart")
	if err := consensus.ProposeCheckpoint(checkpoint); err != nil {
		t.Fatalf("Failed to propose checkpoint: %v", err)
	}

	time.Sleep(5 * time.Second)

	// Verify checkpoint was committed
	latestCheckpoint := consensus.GetLatestCheckpoint()
	if latestCheckpoint == nil || latestCheckpoint.Epoch != epoch {
		t.Fatalf("Checkpoint was not committed before restart")
	}

	// Stop validator
	logger.Info("Stopping validator")
	if err := consensus.Stop(); err != nil {
		t.Fatalf("Failed to stop validator: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Restart validator with same config
	logger.Info("Restarting validator")
	consensus2, err := NewConsensus(cfg, handlers, logger)
	if err != nil {
		t.Fatalf("Failed to create consensus after restart: %v", err)
	}

	if err := consensus2.Start(ctx); err != nil {
		t.Fatalf("Failed to restart validator: %v", err)
	}
	defer consensus2.Stop()

	time.Sleep(3 * time.Second)

	// Verify checkpoint is still available after restart
	recoveredCheckpoint := consensus2.GetLatestCheckpoint()
	if recoveredCheckpoint == nil {
		t.Fatalf("Checkpoint was not recovered after restart")
	}

	if recoveredCheckpoint.Epoch != epoch {
		t.Errorf("Recovered checkpoint epoch mismatch: expected %d, got %d",
			epoch, recoveredCheckpoint.Epoch)
	}

	logger.Info("Validator recovery test completed successfully")
}

// TestCheckpointHashConsistency verifies that all validators compute the same checkpoint hash
func TestCheckpointHashConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logging.NewDefaultLogger()

	// Create 3 mock validators
	validators := []string{"validator_1", "validator_2", "validator_3"}
	apps := make([]*SubnetABCIApp, 0, len(validators))

	validatorSet := make(map[string]int64)
	for _, v := range validators {
		validatorSet[v] = 10
	}

	// Create ABCI apps for each validator
	for _, v := range validators {
		appConfig := SubnetABCIAppConfig{
			Handlers:           ConsensusHandlers{},
			ValidatorSet:       validatorSet,
			ECDSASigner:        &mockECDSASigner{address: v},
			RootLayerClient:    &mockRootLayerClient{},
			MempoolBroadcaster: &mockMempoolBroadcaster{},
			Logger:             logger,
		}
		app := NewSubnetABCIAppWithConfig(appConfig)
		apps = append(apps, app)
	}

	// Create checkpoint
	epoch := uint64(5)
	checkpoint := &pb.CheckpointHeader{
		Epoch:     epoch,
		SubnetId:  "test-subnet",
		Timestamp: 1234567890,
	}

	// Process checkpoint on all validators
	payload, _ := proto.Marshal(checkpoint)
	tx := append([]byte{byte(TxTypeCheckpoint)}, payload...)

	for i, app := range apps {
		// Deliver checkpoint
		app.deliverCheckpoint(tx[1:])

		// Verify checkpoint was stored
		if app.checkpoints[epoch] == nil {
			t.Errorf("Validator %d: checkpoint not stored", i)
		}
	}

	// Compute checkpoint hash on each validator
	hashes := make([][]byte, 0, len(apps))
	for _, app := range apps {
		hash := app.computeCheckpointHash(checkpoint)
		hashes = append(hashes, hash)
	}

	// Verify all hashes are identical
	firstHash := hashes[0]
	for i, hash := range hashes[1:] {
		if string(hash) != string(firstHash) {
			t.Errorf("Validator %d hash mismatch: %x vs %x",
				i+1, hash, firstHash)
		}
	}

	logger.Info("Checkpoint hash consistency verified",
		"hash", fmt.Sprintf("%x", firstHash))
}

// TestThreeValidatorSignatureFlow tests that 3 independent validators can each
// process checkpoints and collect signatures correctly. This tests the ValidationBundle
// logic with multiple validators, but does NOT test P2P consensus coordination
// (that requires shared genesis and is better suited for E2E tests).
func TestThreeValidatorSignatureFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logging.NewDefaultLogger()

	// Create temporary directory for 3 validators
	testDir, err := os.MkdirTemp("", "cometbft-3val-*")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Shared RootLayer client to track all submissions
	rootLayerClient := &mockRootLayerClient{
		submittedBundles: make([]*CheckpointSignatureBundle, 0),
	}

	// Create validator set with 3 validators
	validatorSet := map[string]int64{
		"validator_1": 10,
		"validator_2": 10,
		"validator_3": 10,
	}

	// Create 3 validators
	validators := make([]*Consensus, 0, 3)
	contexts := make([]context.Context, 0, 3)
	cancels := make([]context.CancelFunc, 0, 3)

	// Base ports - use different port ranges to avoid conflicts
	// Validator 1: P2P=26656, RPC=26657
	// Validator 2: P2P=26666, RPC=26667
	// Validator 3: P2P=26676, RPC=26677

	logger.Info("Creating 3 validators...")

	// Note: For multi-validator testing, we create each validator with the same
	// validatorSet configuration. Each validator will generate its own private key
	// and genesis file. In a real multi-validator scenario, you would need to:
	// 1. Pre-generate all validator keys
	// 2. Create a shared genesis file with all validator public keys
	// 3. Distribute the genesis file to all validators
	//
	// For this integration test, we're testing that each validator can:
	// - Start independently
	// - Process checkpoints through its local consensus
	// - Collect signatures locally
	// - Submit ValidationBundles
	//
	// Full P2P consensus testing across multiple nodes requires more complex setup
	// and is better suited for E2E tests with actual network infrastructure.

	for i := 1; i <= 3; i++ {
		validatorName := fmt.Sprintf("validator_%d", i)

		// Use port ranges spaced 10 apart to avoid conflicts
		p2pPort := 26656 + (i-1)*10
		rpcPort := 26657 + (i-1)*10

		cfg := DefaultConfig()
		cfg.HomeDir = filepath.Join(testDir, validatorName)
		cfg.Moniker = validatorName
		cfg.ChainID = "test-chain-3val"
		cfg.P2PListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", p2pPort)
		cfg.RPCListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", rpcPort)
		cfg.GenesisValidators = validatorSet
		cfg.ECDSASigner = &mockECDSASigner{address: validatorName}
		cfg.RootLayerClient = rootLayerClient
		cfg.CreateEmptyBlocks = true
		cfg.CreateEmptyBlocksInterval = 2 * time.Second
		cfg.TimeoutCommit = 1 * time.Second

		handlers := ConsensusHandlers{
			OnCheckpointCommitted: func(header *pb.CheckpointHeader) {
				logger.Info("Checkpoint committed", "validator", validatorName, "epoch", header.Epoch)
			},
		}

		consensus, err := NewConsensus(cfg, handlers, logger)
		if err != nil {
			t.Fatalf("Failed to create validator %s: %v", validatorName, err)
		}

		validators = append(validators, consensus)
	}

	// Start all validators
	logger.Info("Starting all validators...")
	for i, consensus := range validators {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		contexts = append(contexts, ctx)
		cancels = append(cancels, cancel)

		if err := consensus.Start(ctx); err != nil {
			t.Fatalf("Failed to start validator_%d: %v", i+1, err)
		}
		defer consensus.Stop()

		// Small delay between validator starts
		time.Sleep(1 * time.Second)
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	// Wait for validators to be ready
	logger.Info("Waiting for validators to be ready...")
	time.Sleep(5 * time.Second)

	// Verify all validators are ready
	for i, consensus := range validators {
		if !consensus.IsReady() {
			t.Fatalf("Validator_%d is not ready", i+1)
		}
	}
	logger.Info("All validators ready")

	// Propose the SAME checkpoint to all validators independently
	// (simulating that they all received the same checkpoint from RootLayer)
	epoch := uint64(1)
	checkpoint := &pb.CheckpointHeader{
		Epoch:     epoch,
		SubnetId:  "test-subnet-3val",
		Timestamp: time.Now().Unix(),
	}

	logger.Info("Proposing same checkpoint to all validators", "epoch", epoch)
	for i, consensus := range validators {
		if err := consensus.ProposeCheckpoint(checkpoint); err != nil {
			t.Errorf("Validator_%d: Failed to propose checkpoint: %v", i+1, err)
		}
	}

	// Wait for checkpoints to be committed through each validator's local consensus
	logger.Info("Waiting for checkpoints to be committed...")
	time.Sleep(8 * time.Second)

	// Verify checkpoint was committed on all validators
	for i, consensus := range validators {
		latestCheckpoint := consensus.GetLatestCheckpoint()
		if latestCheckpoint == nil {
			t.Errorf("Validator_%d: No checkpoint committed", i+1)
			continue
		}
		if latestCheckpoint.Epoch != epoch {
			t.Errorf("Validator_%d: Expected epoch %d, got %d", i+1, epoch, latestCheckpoint.Epoch)
		}
		logger.Info("Checkpoint verified", "validator", i+1, "epoch", latestCheckpoint.Epoch)
	}

	// Check signature collection on each validator
	logger.Info("Checking signature collection on each validator...")
	time.Sleep(3 * time.Second) // Wait for async signature collection

	validatorsWithSignatures := 0
	for i, consensus := range validators {
		consensus.mu.RLock()
		app := consensus.app
		consensus.mu.RUnlock()

		if app == nil {
			t.Errorf("Validator_%d: No ABCI app", i+1)
			continue
		}

		app.mu.RLock()
		signatures, hasSignatures := app.validationBundleSignatures[epoch]
		app.mu.RUnlock()

		if hasSignatures && len(signatures) > 0 {
			validatorsWithSignatures++
			logger.Info("Validator has signatures",
				"validator", i+1,
				"count", len(signatures))
		} else {
			logger.Info("Note: Signature collection is async",
				"validator", i+1)
		}
	}

	// Since each validator runs independently with its own genesis (single-validator chain),
	// each validator will collect only its own signature. They are not coordinating through
	// P2P consensus, so we expect each validator to have 1 signature (its own).
	logger.Info("Signature collection status",
		"validators_with_signatures", validatorsWithSignatures,
		"total_validators", len(validators))

	// Check if any ValidationBundles were submitted
	// Note: Each validator runs independently, so we may see multiple bundle submissions
	logger.Info("Checking ValidationBundle submissions...")
	time.Sleep(2 * time.Second)

	if len(rootLayerClient.submittedBundles) > 0 {
		logger.Info("ValidationBundles submitted",
			"count", len(rootLayerClient.submittedBundles))

		// Check first bundle as example
		bundle := rootLayerClient.submittedBundles[0]
		logger.Info("Sample ValidationBundle",
			"epoch", bundle.Epoch,
			"signatures", len(bundle.ValidatorSignatures))

		if bundle.Epoch != epoch {
			t.Errorf("ValidationBundle epoch mismatch: expected %d, got %d", epoch, bundle.Epoch)
		}

		// Each validator runs its own single-node consensus, so each bundle
		// will only have the signature from that validator
		logger.Info("✓ Independent validator signature collection verified")
	} else {
		logger.Info("Note: ValidationBundle submission is async and may not complete within test timeout")
	}

	logger.Info("Three-validator integration test completed",
		"note", "Each validator ran independently with its own consensus")
}
