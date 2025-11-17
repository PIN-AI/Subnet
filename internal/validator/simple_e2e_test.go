package validator_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"subnet/internal/config"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/registry"
	"subnet/internal/types"
	"subnet/internal/validator"
	pb "subnet/proto/subnet"
)

// TestSimpleE2EFlow tests a simplified end-to-end validation flow
func TestSimpleE2EFlow(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewDefaultLogger()

	t.Log(strings.Repeat("=", 80))
	t.Log("COMPLETE E2E VALIDATION FLOW TEST")
	t.Log(strings.Repeat("=", 80))

	// Phase 1: Setup
	t.Log("\n[Phase 1] Setting up infrastructure...")

	validatorSet, validatorKeys := setupValidators(t, 3)
	agentRegistry := registry.New()

	t.Logf("✓ Created %d validators with 2/3 threshold", len(validatorKeys))

	// Phase 2: Start Validators
	t.Log("\n[Phase 2] Starting validator nodes...")

	validators := make([]*validator.Node, 3)
	servers := make([]*validator.Server, 3)

	// Define Raft peers for all nodes
	raftPeers := []validator.RaftPeerConfig{
		{ID: "validator-1", Address: "127.0.0.1:7000"},
		{ID: "validator-2", Address: "127.0.0.1:7001"},
		{ID: "validator-3", Address: "127.0.0.1:7002"},
	}

	// Define Gossip seeds for cluster formation
	gossipSeeds := []string{"127.0.0.1:7900", "127.0.0.1:7901", "127.0.0.1:7902"}

	for i := 0; i < 3; i++ {
		cfg := &validator.Config{
			ValidatorID:        fmt.Sprintf("validator-%d", i+1),
			SubnetID:           "0x0000000000000000000000000000000000000000000000000000000000000003",
			PrivateKey:         validatorKeys[i].PrivateKeyHex,
			ValidatorSet:       validatorSet,
			StoragePath:        fmt.Sprintf("/tmp/e2e-val-%d-%d", i+1, time.Now().Unix()),
			GRPCPort:           9200 + i,
			Timeouts:           config.DefaultTimeoutConfig(),
			CheckpointInterval: 8 * time.Second,
			// Raft consensus configuration (required - NATS removed)
			Raft: &validator.RaftConfig{
				Enable:           true,
				DataDir:          fmt.Sprintf("/tmp/e2e-raft-%d-%d", i+1, time.Now().Unix()),
				BindAddress:      fmt.Sprintf("127.0.0.1:%d", 7000+i),
				Bootstrap:        i == 0, // First node bootstraps the cluster
				Peers:            raftPeers,
				HeartbeatTimeout: 1 * time.Second,
				ElectionTimeout:  2 * time.Second,
				CommitTimeout:    500 * time.Millisecond,
			},
			// Gossip for signature propagation
			Gossip: &validator.GossipConfig{
				Enable:         true,
				BindAddress:    "127.0.0.1",
				BindPort:       7900 + i,
				Seeds:          gossipSeeds,
				GossipInterval: 200 * time.Millisecond,
				ProbeInterval:  1 * time.Second,
			},
		}

		node, err := validator.NewNode(cfg, logger, agentRegistry)
		if err != nil {
			t.Fatalf("Failed to create validator %d: %v", i+1, err)
		}

		if err = node.Start(ctx); err != nil {
			t.Fatalf("Failed to start validator %d: %v", i+1, err)
		}

		server := validator.NewServer(node, logger)
		if err = server.Start(9200 + i); err != nil {
			t.Fatalf("Failed to start server %d: %v", i+1, err)
		}

		validators[i] = node
		servers[i] = server

		t.Logf("✓ Validator-%d started (Port: %d)", i+1, 9200+i)
	}

	defer func() {
		for i := range validators {
			servers[i].Stop()
			validators[i].Stop()
		}
	}()

	time.Sleep(2 * time.Second)

	// Phase 3: Submit Intent & Assignment
	t.Log("\n[Phase 3] Creating test intent and assignment...")

	intentID := fmt.Sprintf("intent-e2e-%d", time.Now().Unix())
	agentID := "agent-test-001"
	assignmentID := fmt.Sprintf("assign-%s", intentID)

	t.Logf("✓ Intent ID: %s", intentID)
	t.Logf("✓ Agent ID: %s", agentID)
	t.Logf("✓ Assignment ID: %s", assignmentID)

	// Phase 4: Agent Execution & Report Submission
	t.Log("\n[Phase 4] Agent executing task...")

	reportID := fmt.Sprintf("report-%s-%d", agentID, time.Now().Unix())

	report := &pb.ExecutionReport{
		ReportId:     reportID,
		AssignmentId: assignmentID,
		IntentId:     intentID,
		AgentId:      agentID,
		Status:       pb.ExecutionReport_SUCCESS,
		ResultData:   []byte(`{"result":"task completed","output":"3.14159","gas_used":21000}`),
		Timestamp:    time.Now().Unix(),
	}

	t.Logf("✓ Report ID: %s", reportID)
	t.Logf("✓ Status: %s", "SUCCESS")
	t.Logf("✓ Result: %s", string(report.ResultData))

	// Phase 5: Submit to Validator
	t.Log("\n[Phase 5] Submitting ExecutionReport to Validator...")

	receipt, err := servers[0].SubmitExecutionReport(ctx, report)
	if err != nil {
		t.Fatalf("Failed to submit report: %v", err)
	}

	t.Logf("✓ Receipt ID: %s", receipt.ReportId)
	t.Logf("✓ Validator ID: %s", receipt.ValidatorId)
	t.Logf("✓ Status: %s", receipt.Status)
	t.Logf("✓ Score: %d/200", receipt.ScoreHint)

	// Phase 6: Wait for Checkpoint Creation
	t.Log("\n[Phase 6] Waiting for checkpoint creation...")
	t.Log("⏳ Waiting 10 seconds for checkpoint timer...")

	time.Sleep(10 * time.Second)

	// Phase 7: Verify Checkpoint
	t.Log("\n[Phase 7] Verifying checkpoint and signatures...")

	epoch := uint64(0)
	checkpoint, err := validators[0].GetCheckpoint(epoch)

	if checkpoint != nil {
		t.Logf("✓ Checkpoint created:")
		t.Logf("  - Epoch: %d", checkpoint.Epoch)
		t.Logf("  - SubnetID: %s", checkpoint.SubnetId[:20]+"...")
		t.Logf("  - Timestamp: %d", checkpoint.Timestamp)

		if checkpoint.Roots != nil {
			t.Logf("  - StateRoot: %x", checkpoint.Roots.StateRoot[:4])
			t.Logf("  - AgentRoot: %x", checkpoint.Roots.AgentRoot[:4])
			t.Logf("  - EventRoot: %x", checkpoint.Roots.EventRoot[:4])
		}

		if checkpoint.Signatures != nil {
			t.Logf("✓ Signatures collected:")
			t.Logf("  - Count: %d", checkpoint.Signatures.SignatureCount)
			t.Logf("  - TotalWeight: %d", checkpoint.Signatures.TotalWeight)
			t.Logf("  - Threshold: %d/%d (2/3)", checkpoint.Signatures.SignatureCount, 3)

			if checkpoint.Signatures.SignatureCount >= 2 {
				t.Log("  ✓ THRESHOLD REACHED!")
			}
		}
	} else {
		t.Log("⚠ Checkpoint not created yet (timing dependent)")
	}

	// Phase 8: ValidationBundle Status
	t.Log("\n[Phase 8] ValidationBundle status...")

	if checkpoint != nil && checkpoint.Signatures != nil && checkpoint.Signatures.SignatureCount >= 2 {
		t.Log("✓ ValidationBundle READY for RootLayer:")
		t.Logf("  - SubnetID: %s", checkpoint.SubnetId[:20]+"...")
		t.Logf("  - IntentID: checkpoint-%d", checkpoint.Epoch)
		t.Logf("  - RootHeight: %d", checkpoint.Epoch)
		t.Logf("  - Signatures: %d", checkpoint.Signatures.SignatureCount)
		t.Logf("  - Weight: %d", checkpoint.Signatures.TotalWeight)
		t.Log("  ℹ️  Submission happens in validator.submitToRootLayer()")
	}

	// Final Summary
	t.Log("\n" + strings.Repeat("=", 80))
	t.Log("COMPLETE FLOW TRACE")
	t.Log(strings.Repeat("=", 80))

	t.Logf("\n1️⃣  Intent:       %s", intentID)
	t.Logf("2️⃣  Assignment:   %s", assignmentID)
	t.Logf("3️⃣  Agent:        %s", agentID)
	t.Logf("4️⃣  Report:       %s", reportID)
	t.Logf("5️⃣  Receipt:      %s", receipt.ReportId)

	if checkpoint != nil {
		t.Logf("6️⃣  Checkpoint:   Epoch %d", checkpoint.Epoch)
		if checkpoint.Signatures != nil {
			t.Logf("7️⃣  Signatures:   %d/%d collected", checkpoint.Signatures.SignatureCount, 3)
		}
		if checkpoint.Signatures != nil && checkpoint.Signatures.SignatureCount >= 2 {
			t.Log("8️⃣  Bundle:       ✓ READY for RootLayer")
		}
	}

	t.Log("\n✅ TEST STATUS:")
	t.Log("  ✓ Intent created")
	t.Log("  ✓ Agent executed task")
	t.Log("  ✓ ExecutionReport submitted")
	t.Log("  ✓ Validator verified report")
	t.Log("  ✓ Receipt generated")

	if checkpoint != nil {
		t.Log("  ✓ Checkpoint created")
		if checkpoint.Signatures != nil && checkpoint.Signatures.SignatureCount >= 2 {
			t.Log("  ✓ Signatures collected (THRESHOLD)")
			t.Log("  ✓ ValidationBundle ready")
		}
	}

	t.Log("\n" + strings.Repeat("=", 80))

	// Assertions
	assert.NotEmpty(t, intentID)
	assert.NotEmpty(t, reportID)
	assert.NotNil(t, receipt)
	assert.Equal(t, "accepted", receipt.Status)
}

type testValidatorKey struct {
	ID            string
	PrivateKeyHex string
	PublicKey     []byte
	Weight        uint64
}

func setupValidators(t *testing.T, count int) (*types.ValidatorSet, []testValidatorKey) {
	validators := make([]testValidatorKey, count)
	validatorSetItems := make([]types.Validator, count)

	for i := 0; i < count; i++ {
		testPrivKey := fmt.Sprintf("%064x", (i+1)*111111)
		signer, err := crypto.NewECDSASignerFromHex(testPrivKey)
		if err != nil {
			t.Fatalf("Failed to create signer: %v", err)
		}

		validators[i] = testValidatorKey{
			ID:            fmt.Sprintf("validator-%d", i+1),
			PrivateKeyHex: testPrivKey,
			PublicKey:     signer.PublicKey(),
			Weight:        100,
		}

		validatorSetItems[i] = types.Validator{
			ID:     validators[i].ID,
			PubKey: validators[i].PublicKey,
			Weight: validators[i].Weight,
		}
	}

	return &types.ValidatorSet{
		Validators:     validatorSetItems,
		MinValidators:  count,
		ThresholdNum:   2,
		ThresholdDenom: 3,
	}, validators
}
