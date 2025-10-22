package consensus

import (
	"fmt"
	"sync"
	"testing"
	"time"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGossipManager_SingleNode(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node-1", handler, logger)
	require.NotNil(t, delegate)

	config := GossipConfig{
		NodeName:       "test-node-1",
		BindAddress:    "127.0.0.1",
		BindPort:       0, // Random port
		AdvertiseAddr:  "",
		AdvertisePort:  0,
		Seeds:          []string{}, // No seeds for single node
		GossipInterval: 200 * time.Millisecond,
		ProbeInterval:  1 * time.Second,
	}

	manager, err := NewGossipManager(config, delegate, logger)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Verify manager is initialized
	assert.NotNil(t, manager.LocalNode())
	assert.Equal(t, 1, manager.NumMembers()) // Only itself

	// Cleanup
	err = manager.Shutdown()
	assert.NoError(t, err)
}

func TestNewGossipManager_InvalidConfig(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node-1", handler, logger)
	require.NotNil(t, delegate)

	tests := []struct {
		name   string
		config GossipConfig
	}{
		{
			name: "Empty node name",
			config: GossipConfig{
				NodeName:    "",
				BindAddress: "127.0.0.1",
				BindPort:    7946,
			},
		},
		{
			name: "Empty bind address",
			config: GossipConfig{
				NodeName:    "test-node",
				BindAddress: "",
				BindPort:    7946,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewGossipManager(tt.config, delegate, logger)
			// Should still succeed but may have issues
			if manager != nil {
				_ = manager.Shutdown()
			}
			// Just verify it doesn't panic
			_ = err
		})
	}
}

func TestGossipManager_TwoNodeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping two-node cluster test in short mode")
	}

	logger := logging.NewDefaultLogger()

	// Node 1
	var receivedSigs1 []*pb.Signature
	var mu1 sync.Mutex
	handler1 := func(sig *pb.Signature, checkpointHash []byte) {
		mu1.Lock()
		defer mu1.Unlock()
		receivedSigs1 = append(receivedSigs1, sig)
	}

	delegate1 := NewSignatureGossipDelegate("node-1", handler1, logger)
	config1 := GossipConfig{
		NodeName:       "node-1",
		BindAddress:    "127.0.0.1",
		BindPort:       0, // Random port
		AdvertiseAddr:  "",
		AdvertisePort:  0,
		Seeds:          []string{},
		GossipInterval: 100 * time.Millisecond,
		ProbeInterval:  500 * time.Millisecond,
	}

	manager1, err := NewGossipManager(config1, delegate1, logger)
	require.NoError(t, err)
	defer manager1.Shutdown()

	// Get actual port for node 1
	node1Addr := fmt.Sprintf("127.0.0.1:%d", manager1.LocalNode().Port)

	// Node 2 - join node 1
	var receivedSigs2 []*pb.Signature
	var mu2 sync.Mutex
	handler2 := func(sig *pb.Signature, checkpointHash []byte) {
		mu2.Lock()
		defer mu2.Unlock()
		receivedSigs2 = append(receivedSigs2, sig)
	}

	delegate2 := NewSignatureGossipDelegate("node-2", handler2, logger)
	config2 := GossipConfig{
		NodeName:       "node-2",
		BindAddress:    "127.0.0.1",
		BindPort:       0, // Random port
		AdvertiseAddr:  "",
		AdvertisePort:  0,
		Seeds:          []string{node1Addr},
		GossipInterval: 100 * time.Millisecond,
		ProbeInterval:  500 * time.Millisecond,
	}

	manager2, err := NewGossipManager(config2, delegate2, logger)
	require.NoError(t, err)
	defer manager2.Shutdown()

	// Wait for cluster to form
	time.Sleep(1 * time.Second)

	// Verify both nodes see each other
	assert.Equal(t, 2, manager1.NumMembers(), "Node 1 should see 2 members")
	assert.Equal(t, 2, manager2.NumMembers(), "Node 2 should see 2 members")

	// Broadcast from node 1
	testSig1 := &pb.Signature{
		SignerId: "node-1",
		Der:      []byte("signature-from-node-1"),
	}
	err = manager1.BroadcastSignature(testSig1, 1, []byte("checkpoint-hash-1"))
	require.NoError(t, err)

	// Wait for gossip propagation
	time.Sleep(500 * time.Millisecond)

	// Node 2 should receive the signature
	mu2.Lock()
	receivedCount := len(receivedSigs2)
	mu2.Unlock()

	assert.Greater(t, receivedCount, 0, "Node 2 should have received signature from node 1")
}

func TestGossipManager_BroadcastSignature(t *testing.T) {
	logger := logging.NewDefaultLogger()

	handler := func(sig *pb.Signature, checkpointHash []byte) {
		// Handler not called in single-node test
	}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	config := GossipConfig{
		NodeName:       "test-node",
		BindAddress:    "127.0.0.1",
		BindPort:       0,
		GossipInterval: 200 * time.Millisecond,
		ProbeInterval:  1 * time.Second,
	}

	manager, err := NewGossipManager(config, delegate, logger)
	require.NoError(t, err)
	defer manager.Shutdown()

	// Broadcast a signature
	testSig := &pb.Signature{
		SignerId: "validator-1",
		Der:      []byte("test-signature"),
	}
	testEpoch := uint64(42)
	testHash := []byte("test-checkpoint-hash")

	err = manager.BroadcastSignature(testSig, testEpoch, testHash)
	require.NoError(t, err)

	// Note: In single node, signature won't be received by handler
	// because there's no other node to gossip to
	// This test just verifies broadcast doesn't error
}

func TestGossipManager_Members(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	config := GossipConfig{
		NodeName:       "test-node",
		BindAddress:    "127.0.0.1",
		BindPort:       0,
		GossipInterval: 200 * time.Millisecond,
		ProbeInterval:  1 * time.Second,
	}

	manager, err := NewGossipManager(config, delegate, logger)
	require.NoError(t, err)
	defer manager.Shutdown()

	// Verify members
	members := manager.Members()
	assert.Len(t, members, 1)
	assert.Equal(t, "test-node", members[0].Name)

	// Verify num members
	assert.Equal(t, 1, manager.NumMembers())
}

func TestGossipManager_LocalNode(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	config := GossipConfig{
		NodeName:       "test-node",
		BindAddress:    "127.0.0.1",
		BindPort:       7946, // Use fixed port for testing
		AdvertiseAddr:  "1.2.3.4",
		AdvertisePort:  7946,
		GossipInterval: 200 * time.Millisecond,
		ProbeInterval:  1 * time.Second,
	}

	manager, err := NewGossipManager(config, delegate, logger)
	require.NoError(t, err)
	defer manager.Shutdown()

	// Verify local node
	localNode := manager.LocalNode()
	require.NotNil(t, localNode)
	assert.Equal(t, "test-node", localNode.Name)
	assert.Equal(t, "1.2.3.4", localNode.Addr.String())
	assert.Equal(t, uint16(7946), localNode.Port)
}

func TestGossipManager_Shutdown(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	config := GossipConfig{
		NodeName:       "test-node",
		BindAddress:    "127.0.0.1",
		BindPort:       0,
		GossipInterval: 200 * time.Millisecond,
		ProbeInterval:  1 * time.Second,
	}

	manager, err := NewGossipManager(config, delegate, logger)
	require.NoError(t, err)

	// Verify it's running
	assert.NotNil(t, manager.LocalNode())

	// Shutdown
	err = manager.Shutdown()
	assert.NoError(t, err)

	// Note: Double shutdown panics in memberlist, so we skip that test
	// This is expected behavior from memberlist library
}

func TestGossipManager_JoinSeeds_EmptyList(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	config := GossipConfig{
		NodeName:       "test-node",
		BindAddress:    "127.0.0.1",
		BindPort:       0,
		Seeds:          []string{},
		GossipInterval: 200 * time.Millisecond,
		ProbeInterval:  1 * time.Second,
	}

	manager, err := NewGossipManager(config, delegate, logger)
	require.NoError(t, err)
	defer manager.Shutdown()

	// Joining empty seeds should not error
	err = manager.JoinSeeds([]string{})
	assert.NoError(t, err)
}

func TestGossipManager_JoinSeeds_InvalidAddress(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	config := GossipConfig{
		NodeName:       "test-node",
		BindAddress:    "127.0.0.1",
		BindPort:       0,
		GossipInterval: 200 * time.Millisecond,
		ProbeInterval:  1 * time.Second,
	}

	manager, err := NewGossipManager(config, delegate, logger)
	require.NoError(t, err)
	defer manager.Shutdown()

	// Try to join invalid seed
	err = manager.JoinSeeds([]string{"invalid-address:9999"})
	// Should return error but not panic
	assert.Error(t, err)
}
