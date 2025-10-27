package raft

import (
	"fmt"
	"sync"

	"github.com/hashicorp/memberlist"
	"google.golang.org/protobuf/proto"
	"subnet/internal/logging"
	pb "subnet/proto/subnet"
)

// SignatureHandler is called when a signature is received via gossip
type SignatureHandler func(*pb.Signature, []byte)

// ValidationBundleSignatureHandler is called when a ValidationBundle signature is received via gossip
type ValidationBundleSignatureHandler func(*pb.ValidationBundleSignature)

// SignatureGossipDelegate implements memberlist.Delegate for signature propagation
type SignatureGossipDelegate struct {
	mu                           sync.RWMutex
	broadcasts                   [][]byte                         // Queue of messages to broadcast
	signatureHandler             SignatureHandler                 // Callback when signature received
	validationBundleSigHandler   ValidationBundleSignatureHandler // Callback when ValidationBundle signature received
	nodeID                       string
	logger                       logging.Logger
}

// NewSignatureGossipDelegate creates a new gossip delegate
func NewSignatureGossipDelegate(nodeID string, handler SignatureHandler, logger logging.Logger) *SignatureGossipDelegate {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}
	return &SignatureGossipDelegate{
		broadcasts:       make([][]byte, 0),
		signatureHandler: handler,
		nodeID:          nodeID,
		logger:          logger,
	}
}

// NotifyMsg is invoked when a user-data message is received via gossip.
// This is called from memberlist's goroutine, so it must not block.
func (d *SignatureGossipDelegate) NotifyMsg(data []byte) {
	var msg pb.GossipMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		d.logger.Error("Failed to unmarshal gossip message", "error", err)
		return
	}

	if sig := msg.GetSignature(); sig != nil {
		// Skip our own signature (already handled locally)
		if sig.ValidatorId == d.nodeID {
			return
		}

		// Convert proto signature to internal format
		// Note: pb.Signature uses SignerId (not ValidatorId) and doesn't have Epoch/Timestamp
		internalSig := &pb.Signature{
			SignerId: sig.ValidatorId,
			Der:      sig.SignatureDer,
		}

		// Call handler with signature and checkpoint hash
		if d.signatureHandler != nil {
			d.signatureHandler(internalSig, sig.CheckpointHash)
		}

		d.logger.Debug("Received signature via gossip",
			"validator", sig.ValidatorId,
			"epoch", sig.Epoch)
	}

	if vbSig := msg.GetValidationBundleSignature(); vbSig != nil {
		// Skip our own signature (already handled locally)
		// Compare with validator address instead of nodeID
		// since ValidationBundleSignature uses validator address

		// Call handler with ValidationBundle signature
		if d.validationBundleSigHandler != nil {
			d.validationBundleSigHandler(vbSig)
		}

		d.logger.Debug("Received ValidationBundle signature via gossip",
			"validator", vbSig.ValidatorAddress,
			"intent_id", vbSig.IntentId,
			"epoch", vbSig.Epoch)
	}
}

// GetBroadcasts is called when memberlist needs messages to gossip.
// Returns messages to broadcast and clears the queue.
func (d *SignatureGossipDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.broadcasts) == 0 {
		return nil
	}

	// Return all pending broadcasts
	// memberlist will handle batching and retransmission
	broadcasts := d.broadcasts
	d.broadcasts = make([][]byte, 0)

	d.logger.Debug("Providing broadcasts to gossip",
		"count", len(broadcasts))

	return broadcasts
}

// LocalState is called to get the current state for anti-entropy.
// We don't use this for signature propagation (signatures are ephemeral).
func (d *SignatureGossipDelegate) LocalState(join bool) []byte {
	return nil
}

// MergeRemoteState is called to merge remote state.
// We don't use this for signature propagation.
func (d *SignatureGossipDelegate) MergeRemoteState(buf []byte, join bool) {
	// No-op: signatures are propagated via NotifyMsg
}

// NodeMeta returns metadata about this node (max 512 bytes).
// We include the node ID for debugging.
func (d *SignatureGossipDelegate) NodeMeta(limit int) []byte {
	meta := []byte(d.nodeID)
	if len(meta) > limit {
		meta = meta[:limit]
	}
	return meta
}

// BroadcastSignature queues a signature for gossip propagation
// epoch parameter is needed since pb.Signature doesn't contain epoch
func (d *SignatureGossipDelegate) BroadcastSignature(sig *pb.Signature, epoch uint64, checkpointHash []byte) error {
	// Build gossip message
	gossipSig := &pb.CheckpointSignature{
		Epoch:          epoch,
		ValidatorId:    sig.SignerId,
		SignatureDer:   sig.Der,
		Timestamp:      0, // Will be set by receiver based on receipt time
		CheckpointHash: checkpointHash,
	}

	msg := &pb.GossipMessage{
		Payload: &pb.GossipMessage_Signature{
			Signature: gossipSig,
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal gossip message: %w", err)
	}

	d.mu.Lock()
	d.broadcasts = append(d.broadcasts, data)
	d.mu.Unlock()

	d.logger.Debug("Queued signature for gossip broadcast",
		"validator", sig.SignerId,
		"epoch", epoch,
		"queue_size", len(d.broadcasts))

	return nil
}

// BroadcastValidationBundleSignature queues a ValidationBundle signature for gossip propagation
func (d *SignatureGossipDelegate) BroadcastValidationBundleSignature(vbSig *pb.ValidationBundleSignature) error {
	msg := &pb.GossipMessage{
		Payload: &pb.GossipMessage_ValidationBundleSignature{
			ValidationBundleSignature: vbSig,
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal ValidationBundle gossip message: %w", err)
	}

	d.mu.Lock()
	d.broadcasts = append(d.broadcasts, data)
	d.mu.Unlock()

	d.logger.Debug("Queued ValidationBundle signature for gossip broadcast",
		"validator", vbSig.ValidatorAddress,
		"intent_id", vbSig.IntentId,
		"epoch", vbSig.Epoch,
		"queue_size", len(d.broadcasts))

	return nil
}

// SetValidationBundleSignatureHandler sets the handler for ValidationBundle signatures
func (d *SignatureGossipDelegate) SetValidationBundleSignatureHandler(handler ValidationBundleSignatureHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.validationBundleSigHandler = handler
}

// NotifyJoin is called when a node joins the cluster
func (d *SignatureGossipDelegate) NotifyJoin(node *memberlist.Node) {
	if node == nil {
		return
	}
	d.logger.Info("Node joined gossip cluster",
		"node", node.Name,
		"addr", node.Address())
}

// NotifyLeave is called when a node leaves the cluster
func (d *SignatureGossipDelegate) NotifyLeave(node *memberlist.Node) {
	if node == nil {
		return
	}
	d.logger.Info("Node left gossip cluster",
		"node", node.Name,
		"addr", node.Address())
}

// NotifyUpdate is called when a node is updated
func (d *SignatureGossipDelegate) NotifyUpdate(node *memberlist.Node) {
	if node == nil {
		return
	}
	d.logger.Debug("Node updated in gossip cluster",
		"node", node.Name,
		"addr", node.Address())
}
