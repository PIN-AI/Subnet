package cometbft

import (
	"context"
	"encoding/json"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"google.golang.org/protobuf/proto"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"
)

// Mock implementations for testing

type mockECDSASigner struct {
	address string
}

func (m *mockECDSASigner) Sign(hash []byte) ([]byte, error) {
	// Return a mock signature
	return []byte("mock_signature_" + m.address), nil
}

func (m *mockECDSASigner) Address() string {
	return m.address
}

type mockRootLayerClient struct {
	submittedBundles []*CheckpointSignatureBundle
}

func (m *mockRootLayerClient) SubmitCheckpointSignatures(bundle *CheckpointSignatureBundle) error {
	m.submittedBundles = append(m.submittedBundles, bundle)
	return nil
}

type mockMempoolBroadcaster struct {
	broadcasts [][]byte
}

func (m *mockMempoolBroadcaster) BroadcastTxSync(tx []byte) error {
	m.broadcasts = append(m.broadcasts, tx)
	return nil
}

// Test threshold signature checking
func TestHasThresholdSignatures(t *testing.T) {
	logger := logging.NewDefaultLogger()

	tests := []struct {
		name            string
		validatorCount  int
		signatureCount  int
		expectedResult  bool
		expectedThreshold int
	}{
		{
			name:            "3 validators, 3 signatures (meets threshold)",
			validatorCount:  3,
			signatureCount:  3,
			expectedResult:  true,
			expectedThreshold: 3, // (3*2)/3 + 1 = 3
		},
		{
			name:            "3 validators, 2 signatures (not enough)",
			validatorCount:  3,
			signatureCount:  2,
			expectedResult:  false,
			expectedThreshold: 3,
		},
		{
			name:            "3 validators, 1 signature (not enough)",
			validatorCount:  3,
			signatureCount:  1,
			expectedResult:  false,
			expectedThreshold: 3,
		},
		{
			name:            "4 validators, 3 signatures (meets threshold)",
			validatorCount:  4,
			signatureCount:  3,
			expectedResult:  true,
			expectedThreshold: 3, // (4*2)/3 + 1 = 3
		},
		{
			name:            "4 validators, 2 signatures (not enough)",
			validatorCount:  4,
			signatureCount:  2,
			expectedResult:  false,
			expectedThreshold: 3,
		},
		{
			name:            "5 validators, 4 signatures (meets threshold)",
			validatorCount:  5,
			signatureCount:  4,
			expectedResult:  true,
			expectedThreshold: 4, // (5*2)/3 + 1 = 4
		},
		{
			name:            "5 validators, 3 signatures (not enough)",
			validatorCount:  5,
			signatureCount:  3,
			expectedResult:  false,
			expectedThreshold: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator set
			validatorSet := make(map[string]int64)
			for i := 0; i < tt.validatorCount; i++ {
				validatorSet[string(rune('A'+i))] = 10
			}

			// Create app
			app := NewSubnetABCIApp(ConsensusHandlers{}, validatorSet, logger)

			// Add signatures
			epoch := uint64(1)
			app.validationBundleSignatures[epoch] = make(map[string][]byte)
			for i := 0; i < tt.signatureCount; i++ {
				validatorAddr := string(rune('A' + i))
				app.validationBundleSignatures[epoch][validatorAddr] = []byte("signature_" + validatorAddr)
			}

			// Test threshold
			result := app.hasThresholdSignatures(epoch)
			if result != tt.expectedResult {
				t.Errorf("hasThresholdSignatures() = %v, want %v (validators=%d, signatures=%d, threshold=%d)",
					result, tt.expectedResult, tt.validatorCount, tt.signatureCount, tt.expectedThreshold)
			}
		})
	}
}

// Test signature transaction validation (CheckTx)
func TestCheckTx_ValidationBundleSignature(t *testing.T) {
	logger := logging.NewDefaultLogger()
	app := NewSubnetABCIApp(ConsensusHandlers{}, nil, logger)

	tests := []struct {
		name        string
		tx          ValidationBundleSignatureTx
		expectError bool
	}{
		{
			name: "valid signature tx",
			tx: ValidationBundleSignatureTx{
				Epoch:          1,
				CheckpointHash: []byte("checkpoint_hash"),
				ValidatorAddr:  "validator_1",
				Signature:      []byte("signature_data"),
			},
			expectError: false,
		},
		{
			name: "invalid epoch (zero)",
			tx: ValidationBundleSignatureTx{
				Epoch:          0,
				CheckpointHash: []byte("checkpoint_hash"),
				ValidatorAddr:  "validator_1",
				Signature:      []byte("signature_data"),
			},
			expectError: true,
		},
		{
			name: "empty signature",
			tx: ValidationBundleSignatureTx{
				Epoch:          1,
				CheckpointHash: []byte("checkpoint_hash"),
				ValidatorAddr:  "validator_1",
				Signature:      []byte{},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode transaction
			payload, err := json.Marshal(tt.tx)
			if err != nil {
				t.Fatalf("Failed to marshal tx: %v", err)
			}

			tx := append([]byte{byte(TxTypeValidationBundleSignature)}, payload...)

			// Call CheckTx
			resp, err := app.CheckTx(context.Background(), &abci.RequestCheckTx{Tx: tx})
			if err != nil {
				t.Fatalf("CheckTx returned error: %v", err)
			}

			if tt.expectError {
				if resp.Code == 0 {
					t.Errorf("Expected CheckTx to fail, but it succeeded")
				}
			} else {
				if resp.Code != 0 {
					t.Errorf("Expected CheckTx to succeed, but it failed: %s", resp.Log)
				}
			}
		})
	}
}

// Test signature collection (FinalizeBlock + deliverValidationBundleSignature)
func TestSignatureCollection(t *testing.T) {
	logger := logging.NewDefaultLogger()

	validatorSet := map[string]int64{
		"validator_1": 10,
		"validator_2": 10,
		"validator_3": 10,
	}

	app := NewSubnetABCIApp(ConsensusHandlers{}, validatorSet, logger)

	epoch := uint64(1)

	// Simulate 3 validators sending signatures
	signatures := []ValidationBundleSignatureTx{
		{
			Epoch:          epoch,
			CheckpointHash: []byte("checkpoint_hash_1"),
			ValidatorAddr:  "validator_1",
			Signature:      []byte("sig_1"),
		},
		{
			Epoch:          epoch,
			CheckpointHash: []byte("checkpoint_hash_1"),
			ValidatorAddr:  "validator_2",
			Signature:      []byte("sig_2"),
		},
		{
			Epoch:          epoch,
			CheckpointHash: []byte("checkpoint_hash_1"),
			ValidatorAddr:  "validator_3",
			Signature:      []byte("sig_3"),
		},
	}

	// Encode and deliver each signature
	var txs [][]byte
	for _, sigTx := range signatures {
		payload, _ := json.Marshal(sigTx)
		tx := append([]byte{byte(TxTypeValidationBundleSignature)}, payload...)
		txs = append(txs, tx)
	}

	// Call FinalizeBlock with all signatures
	resp, err := app.FinalizeBlock(context.Background(), &abci.RequestFinalizeBlock{
		Height: 1,
		Txs:    txs,
	})

	if err != nil {
		t.Fatalf("FinalizeBlock failed: %v", err)
	}

	// Check all transactions succeeded
	for i, result := range resp.TxResults {
		if result.Code != 0 {
			t.Errorf("Transaction %d failed: %s", i, result.Log)
		}
	}

	// Verify signatures were stored
	if len(app.validationBundleSignatures[epoch]) != 3 {
		t.Errorf("Expected 3 signatures, got %d", len(app.validationBundleSignatures[epoch]))
	}

	// Verify specific signatures
	for _, sigTx := range signatures {
		storedSig, exists := app.validationBundleSignatures[epoch][sigTx.ValidatorAddr]
		if !exists {
			t.Errorf("Signature from %s not found", sigTx.ValidatorAddr)
		}
		if string(storedSig) != string(sigTx.Signature) {
			t.Errorf("Stored signature mismatch for %s", sigTx.ValidatorAddr)
		}
	}

	// Verify threshold is reached
	if !app.hasThresholdSignatures(epoch) {
		t.Error("Threshold should be reached with 3/3 signatures")
	}
}

// Test CheckpointSignatureBundle construction
func TestConstructValidationBundle(t *testing.T) {
	logger := logging.NewDefaultLogger()

	validatorSet := map[string]int64{
		"validator_1": 10,
		"validator_2": 10,
		"validator_3": 10,
	}

	app := NewSubnetABCIApp(ConsensusHandlers{}, validatorSet, logger)

	epoch := uint64(5)

	// Create a checkpoint
	checkpoint := &pb.CheckpointHeader{
		Epoch:     epoch,
		SubnetId:  "test_subnet",
		Timestamp: 1234567890,
	}
	app.checkpoints[epoch] = checkpoint

	// Add signatures
	app.validationBundleSignatures[epoch] = map[string][]byte{
		"validator_1": []byte("sig_1"),
		"validator_2": []byte("sig_2"),
		"validator_3": []byte("sig_3"),
	}

	// Construct bundle
	bundle := app.constructValidationBundle(epoch)

	if bundle == nil {
		t.Fatal("constructValidationBundle returned nil")
	}

	// Verify bundle fields
	if bundle.Epoch != epoch {
		t.Errorf("Bundle epoch = %d, want %d", bundle.Epoch, epoch)
	}

	if len(bundle.ValidatorSignatures) != 3 {
		t.Errorf("Bundle has %d signatures, want 3", len(bundle.ValidatorSignatures))
	}

	// Verify all validators are present
	validatorMap := make(map[string][]byte)
	for addr, sig := range bundle.ValidatorSignatures {
		validatorMap[addr] = sig
	}

	for _, expectedAddr := range []string{"validator_1", "validator_2", "validator_3"} {
		if _, exists := validatorMap[expectedAddr]; !exists {
			t.Errorf("Validator %s not found in bundle", expectedAddr)
		}
	}
}

// Test checkpoint transaction processing
func TestCheckpointProcessing(t *testing.T) {
	logger := logging.NewDefaultLogger()

	handlers := ConsensusHandlers{
		OnCheckpointCommitted: func(header *pb.CheckpointHeader) {
			// Checkpoint received callback
		},
	}

	app := NewSubnetABCIApp(handlers, nil, logger)

	// Create checkpoint
	checkpoint := &pb.CheckpointHeader{
		Epoch:     10,
		SubnetId:  "test_subnet",
		Timestamp: 1234567890,
	}

	// Encode as transaction
	payload, _ := proto.Marshal(checkpoint)
	tx := append([]byte{byte(TxTypeCheckpoint)}, payload...)

	// Process transaction
	resp, err := app.FinalizeBlock(context.Background(), &abci.RequestFinalizeBlock{
		Height: 1,
		Txs:    [][]byte{tx},
	})

	if err != nil {
		t.Fatalf("FinalizeBlock failed: %v", err)
	}

	if resp.TxResults[0].Code != 0 {
		t.Errorf("Checkpoint transaction failed: %s", resp.TxResults[0].Log)
	}

	// Verify checkpoint was stored
	if app.currentEpoch != checkpoint.Epoch {
		t.Errorf("Current epoch = %d, want %d", app.currentEpoch, checkpoint.Epoch)
	}

	storedCheckpoint := app.checkpoints[checkpoint.Epoch]
	if storedCheckpoint == nil {
		t.Fatal("Checkpoint was not stored")
	}

	if storedCheckpoint.Epoch != checkpoint.Epoch {
		t.Errorf("Stored checkpoint epoch = %d, want %d", storedCheckpoint.Epoch, checkpoint.Epoch)
	}
}
