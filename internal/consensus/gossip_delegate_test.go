package consensus

import (
	"sync"
	"testing"
	"time"

	pb "subnet/proto/subnet"
	"subnet/internal/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSignatureGossipDelegate_BroadcastSignature(t *testing.T) {
	logger := logging.NewDefaultLogger()

	handler := func(sig *pb.Signature, checkpointHash []byte) {
		// Handler not called in this test
	}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	require.NotNil(t, delegate)

	// Create test signature
	testSig := &pb.Signature{
		SignerId: "validator-1",
		Der:      []byte("test-signature-data"),
	}
	testEpoch := uint64(42)
	testHash := []byte("test-checkpoint-hash")

	// Broadcast signature
	err := delegate.BroadcastSignature(testSig, testEpoch, testHash)
	require.NoError(t, err)

	// Get broadcasts (should return the queued message)
	broadcasts := delegate.GetBroadcasts(1, 1024)
	require.Len(t, broadcasts, 1)

	// Verify broadcast contains our signature
	assert.NotNil(t, broadcasts[0])
}

func TestSignatureGossipDelegate_NotifyMsg(t *testing.T) {
	logger := logging.NewDefaultLogger()

	var receivedSig *pb.Signature
	var receivedHash []byte
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)

	handler := func(sig *pb.Signature, checkpointHash []byte) {
		mu.Lock()
		defer mu.Unlock()
		receivedSig = sig
		receivedHash = checkpointHash
		wg.Done()
	}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	require.NotNil(t, delegate)

	// Create a gossip message
	testSig := &pb.CheckpointSignature{
		Epoch:          42,
		ValidatorId:    "validator-1",
		SignatureDer:   []byte("test-signature-data"),
		Timestamp:      time.Now().Unix(),
		CheckpointHash: []byte("test-checkpoint-hash"),
	}

	gossipMsg := &pb.GossipMessage{
		Payload: &pb.GossipMessage_Signature{
			Signature: testSig,
		},
	}

	// Marshal the message
	data, err := proto.Marshal(gossipMsg)
	require.NoError(t, err)

	// Notify the delegate
	delegate.NotifyMsg(data)

	// Wait for handler to be called
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Handler was not called within timeout")
	}

	// Verify received signature
	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, receivedSig)
	assert.Equal(t, "validator-1", receivedSig.SignerId)
	assert.Equal(t, []byte("test-signature-data"), receivedSig.Der)
	assert.Equal(t, []byte("test-checkpoint-hash"), receivedHash)
}

func TestSignatureGossipDelegate_NotifyMsg_InvalidData(t *testing.T) {
	logger := logging.NewDefaultLogger()

	handlerCalled := false
	handler := func(sig *pb.Signature, checkpointHash []byte) {
		handlerCalled = true
	}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	require.NotNil(t, delegate)

	// Send invalid data
	delegate.NotifyMsg([]byte("invalid-protobuf-data"))

	// Give it a moment
	time.Sleep(100 * time.Millisecond)

	// Handler should not be called
	assert.False(t, handlerCalled, "Handler should not be called for invalid data")
}

func TestSignatureGossipDelegate_GetBroadcasts_Limit(t *testing.T) {
	logger := logging.NewDefaultLogger()

	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	require.NotNil(t, delegate)

	// Queue multiple signatures
	for i := 0; i < 5; i++ {
		testSig := &pb.Signature{
			SignerId: "validator-1",
			Der:      []byte("test-signature-data"),
		}
		err := delegate.BroadcastSignature(testSig, uint64(i), []byte("test-hash"))
		require.NoError(t, err)
	}

	// GetBroadcasts returns all pending broadcasts and clears the queue
	broadcasts := delegate.GetBroadcasts(0, 1024*1024)
	assert.Len(t, broadcasts, 5)

	// Second call should return empty
	broadcasts = delegate.GetBroadcasts(0, 1024*1024)
	assert.Len(t, broadcasts, 0)
}

func TestSignatureGossipDelegate_NodeMeta(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	require.NotNil(t, delegate)

	meta := delegate.NodeMeta(1024)
	assert.NotNil(t, meta)
	assert.LessOrEqual(t, len(meta), 1024)
}

func TestSignatureGossipDelegate_EventCallbacks(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	require.NotNil(t, delegate)

	// Test event callbacks (should not panic)
	delegate.NotifyJoin(nil)
	delegate.NotifyLeave(nil)
	delegate.NotifyUpdate(nil)
}

func TestSignatureGossipDelegate_ConcurrentBroadcasts(t *testing.T) {
	logger := logging.NewDefaultLogger()
	handler := func(sig *pb.Signature, checkpointHash []byte) {}

	delegate := NewSignatureGossipDelegate("test-node", handler, logger)
	require.NotNil(t, delegate)

	// Concurrent broadcasts
	var wg sync.WaitGroup
	numGoroutines := 10
	broadcastsPerGoroutine := 10

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < broadcastsPerGoroutine; j++ {
				testSig := &pb.Signature{
					SignerId: "validator-1",
					Der:      []byte("test-signature-data"),
				}
				err := delegate.BroadcastSignature(testSig, uint64(id*100+j), []byte("test-hash"))
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all broadcasts are queued
	totalBroadcasts := numGoroutines * broadcastsPerGoroutine
	broadcasts := delegate.GetBroadcasts(totalBroadcasts+10, 1024*1024)
	assert.Equal(t, totalBroadcasts, len(broadcasts))
}
