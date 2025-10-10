package consensus

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sync"

	pb "subnet/proto/subnet"
	"subnet/internal/crypto"
	"subnet/internal/storage"
	"subnet/internal/types"
)

// Chain manages the checkpoint chain
type Chain struct {
	mu    sync.RWMutex
	store storage.Store

	// In-memory cache
	checkpoints map[uint64]*pb.CheckpointHeader
	latest      *pb.CheckpointHeader
}

// NewChain creates a new chain manager
func NewChain() *Chain {
	return &Chain{
		checkpoints: make(map[uint64]*pb.CheckpointHeader),
	}
}

// SetStore sets the storage backend
func (c *Chain) SetStore(store storage.Store) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = store
}

// AddCheckpoint adds a new checkpoint to the chain
func (c *Chain) AddCheckpoint(header *pb.CheckpointHeader) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate continuity
	if c.latest != nil {
		if header.Epoch != c.latest.Epoch+1 {
			return fmt.Errorf("invalid epoch: expected %d, got %d",
				c.latest.Epoch+1, header.Epoch)
		}

		// Verify parent checkpoint hash to ensure chain continuity
		// This ensures checkpoints form an immutable chain
		latestHash := c.computeHeaderHash(c.latest)
		if header.ParentCpHash != nil && len(header.ParentCpHash) > 0 {
			if !bytes.Equal(header.ParentCpHash, latestHash) {
				return fmt.Errorf("invalid parent checkpoint hash: expected %x, got %x",
					latestHash, header.ParentCpHash)
			}
		} else {
			// Auto-set parent checkpoint hash for chain continuity
			header.ParentCpHash = latestHash
		}
	}

	// Store in memory
	c.checkpoints[header.Epoch] = header
	c.latest = header

	// Persist if store is available
	if c.store != nil {
		// Convert pb.CheckpointHeader to types.CheckpointHeader for storage
		typesHeader := c.convertToTypesHeader(header)
		if err := c.store.SaveHeader(typesHeader); err != nil {
			// Log error but don't fail the operation
			// Storage failure shouldn't block consensus
			return fmt.Errorf("failed to persist checkpoint: %w", err)
		}
	}

	return nil
}

// GetCheckpoint retrieves a checkpoint by epoch
func (c *Chain) GetCheckpoint(epoch uint64) (*pb.CheckpointHeader, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check in-memory cache first
	if cp, ok := c.checkpoints[epoch]; ok {
		return cp, nil
	}

	// Load from storage if not in cache
	if c.store != nil {
		typesHeader, err := c.store.GetHeader(epoch)
		if err == nil && typesHeader != nil {
			// Convert from storage format to proto format
			pbHeader := c.convertFromTypesHeader(typesHeader)
			// Cache for future use
			c.checkpoints[epoch] = pbHeader
			return pbHeader, nil
		}
	}

	return nil, fmt.Errorf("checkpoint not found for epoch %d", epoch)
}

// GetLatest returns the latest checkpoint
func (c *Chain) GetLatest() *pb.CheckpointHeader {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// computeHeaderHash computes the hash of a checkpoint header
func (c *Chain) computeHeaderHash(header *pb.CheckpointHeader) []byte {
	if header == nil {
		return nil
	}

	// Use the canonical checkpoint hasher for consistent hashing across all validators
	hasher := crypto.NewCheckpointHasher()

	// For verification, include signatures
	// This ensures the hash includes all checkpoint data
	hash := hasher.ComputeHashForVerification(header)
	return hash[:]
}

// GetHeight returns the current chain height (latest epoch)
func (c *Chain) GetHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.latest == nil {
		return 0
	}
	return c.latest.Epoch
}

// HasCheckpoint checks if a checkpoint exists for the given epoch
func (c *Chain) HasCheckpoint(epoch uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.checkpoints[epoch]
	return exists
}

// GetChainSegment returns a segment of the chain from startEpoch to endEpoch
func (c *Chain) GetChainSegment(startEpoch, endEpoch uint64) ([]*pb.CheckpointHeader, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if startEpoch > endEpoch {
		return nil, fmt.Errorf("invalid range: start %d > end %d", startEpoch, endEpoch)
	}

	var headers []*pb.CheckpointHeader
	for epoch := startEpoch; epoch <= endEpoch; epoch++ {
		if cp, ok := c.checkpoints[epoch]; ok {
			headers = append(headers, cp)
		}
	}

	return headers, nil
}

// convertToTypesHeader converts pb.CheckpointHeader to types.CheckpointHeader for storage
func (c *Chain) convertToTypesHeader(pbHeader *pb.CheckpointHeader) *types.CheckpointHeader {
	if pbHeader == nil {
		return nil
	}

	typesHeader := &types.CheckpointHeader{
		Epoch:        pbHeader.Epoch,
		SubnetID:     pbHeader.SubnetId,
		Timestamp:    pbHeader.Timestamp,
		ParentCPHash: hex.EncodeToString(pbHeader.ParentCpHash),
	}

	// Convert merkle roots
	if pbHeader.Roots != nil {
		typesHeader.Roots = types.CommitmentRoots{
			StateRoot: hex.EncodeToString(pbHeader.Roots.StateRoot),
			AgentRoot: hex.EncodeToString(pbHeader.Roots.AgentRoot),
			EventRoot: hex.EncodeToString(pbHeader.Roots.EventRoot),
		}
	}

	// Convert signatures
	if pbHeader.Signatures != nil {
		typesHeader.Signatures = &types.CheckpointSignatures{
			ECDSASignatures: pbHeader.Signatures.EcdsaSignatures,
			SignersBitmap:   pbHeader.Signatures.SignersBitmap,
			TotalWeight:     pbHeader.Signatures.TotalWeight,
		}
	}

	return typesHeader
}

// convertFromTypesHeader converts types.CheckpointHeader to pb.CheckpointHeader
func (c *Chain) convertFromTypesHeader(typesHeader *types.CheckpointHeader) *pb.CheckpointHeader {
	if typesHeader == nil {
		return nil
	}

	// Decode ParentCPHash from hex string
	parentCpHash, _ := hex.DecodeString(typesHeader.ParentCPHash)

	pbHeader := &pb.CheckpointHeader{
		Epoch:        typesHeader.Epoch,
		SubnetId:     typesHeader.SubnetID,
		Timestamp:    typesHeader.Timestamp,
		ParentCpHash: parentCpHash,
	}

	// Convert merkle roots
	if typesHeader.Roots.StateRoot != "" || typesHeader.Roots.AgentRoot != "" || typesHeader.Roots.EventRoot != "" {
		stateRoot, _ := hex.DecodeString(typesHeader.Roots.StateRoot)
		agentRoot, _ := hex.DecodeString(typesHeader.Roots.AgentRoot)
		eventRoot, _ := hex.DecodeString(typesHeader.Roots.EventRoot)

		pbHeader.Roots = &pb.CommitmentRoots{
			StateRoot: stateRoot,
			AgentRoot: agentRoot,
			EventRoot: eventRoot,
		}
	}

	// Convert signatures
	if typesHeader.Signatures != nil {
		pbHeader.Signatures = &pb.CheckpointSignatures{
			EcdsaSignatures: typesHeader.Signatures.ECDSASignatures,
			SignersBitmap:   typesHeader.Signatures.SignersBitmap,
			TotalWeight:     typesHeader.Signatures.TotalWeight,
		}
	}

	return pbHeader
}