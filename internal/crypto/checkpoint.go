package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"sort"

	pb "subnet/proto/subnet"
)

// CheckpointHasher provides canonical hashing for checkpoints
type CheckpointHasher struct {
	includeSignatures bool // Whether to include signatures in hash (only for verification)
}

// NewCheckpointHasher creates a new checkpoint hasher
func NewCheckpointHasher() *CheckpointHasher {
	return &CheckpointHasher{
		includeSignatures: false,
	}
}

// ComputeHash computes the canonical hash of a checkpoint header
// This hash is used for signing and verification
func (ch *CheckpointHasher) ComputeHash(header *pb.CheckpointHeader) [32]byte {
	if header == nil {
		return [32]byte{}
	}

	h := sha256.New()

	// 1. Write version (for future compatibility)
	ch.writeUint32(h, 1) // Version 1

	// 2. Write epoch (most important field)
	ch.writeUint64(h, header.Epoch)

	// 3. Write subnet ID (fixed length or length-prefixed)
	ch.writeLengthPrefixedString(h, header.SubnetId)

	// 4. Write timestamp
	ch.writeInt64(h, header.Timestamp)

	// 5. Write parent checkpoint hash (32 bytes or empty)
	if len(header.ParentCpHash) == 32 {
		h.Write(header.ParentCpHash)
	} else {
		h.Write(make([]byte, 32)) // Write zeros if no parent
	}

	// 6. Write merkle roots in deterministic order
	ch.writeMerkleRoots(h, header.Roots)

	// 7. Write additional metadata if present
	ch.writeMetadata(h, header)

	// Note: NEVER include signatures in the hash for signing
	// Signatures are only included when verifying the complete checkpoint

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// ComputeHashForVerification computes hash including signatures
// This is used to verify the complete checkpoint integrity
func (ch *CheckpointHasher) ComputeHashForVerification(header *pb.CheckpointHeader) [32]byte {
	// First compute base hash
	baseHash := ch.ComputeHash(header)

	// Then hash the base hash with signatures
	h := sha256.New()
	h.Write(baseHash[:])

	// Add signatures in deterministic order
	if header.Signatures != nil {
		ch.writeSignatures(h, header.Signatures)
	}

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// writeMerkleRoots writes merkle roots in deterministic order
func (ch *CheckpointHasher) writeMerkleRoots(h hash.Hash, roots *pb.CommitmentRoots) {
	if roots == nil {
		// Write zeros for each root
		h.Write(make([]byte, 32)) // StateRoot
		h.Write(make([]byte, 32)) // AgentRoot
		h.Write(make([]byte, 32)) // EventRoot
		return
	}

	// Write each root (32 bytes each or zeros)
	ch.writeFixedBytes(h, roots.StateRoot, 32)
	ch.writeFixedBytes(h, roots.AgentRoot, 32)
	ch.writeFixedBytes(h, roots.EventRoot, 32)
}

// writeSignatures writes signatures in deterministic order
func (ch *CheckpointHasher) writeSignatures(h hash.Hash, sigs *pb.CheckpointSignatures) {
	if sigs == nil {
		return
	}

	// Write number of signatures
	ch.writeUint32(h, uint32(len(sigs.EcdsaSignatures)))

	// Sort signatures by signer ID for determinism
	sortedSigs := ch.sortSignatures(sigs.EcdsaSignatures)

	// Write each signature
	for _, sig := range sortedSigs {
		h.Write(sig)
	}

	// Write signers bitmap
	if sigs.SignersBitmap != nil {
		ch.writeLengthPrefixedBytes(h, sigs.SignersBitmap)
	}
}

// writeMetadata writes additional checkpoint metadata
func (ch *CheckpointHasher) writeMetadata(h hash.Hash, header *pb.CheckpointHeader) {
	// Add any additional fields that should be included in the hash
	// For now, we don't have additional metadata
}

// Helper functions for deterministic encoding

func (ch *CheckpointHasher) writeUint32(h hash.Hash, v uint32) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	h.Write(buf)
}

func (ch *CheckpointHasher) writeUint64(h hash.Hash, v uint64) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	h.Write(buf)
}

func (ch *CheckpointHasher) writeInt64(h hash.Hash, v int64) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(v))
	h.Write(buf)
}

func (ch *CheckpointHasher) writeLengthPrefixedString(h hash.Hash, s string) {
	data := []byte(s)
	ch.writeUint32(h, uint32(len(data)))
	h.Write(data)
}

func (ch *CheckpointHasher) writeLengthPrefixedBytes(h hash.Hash, data []byte) {
	ch.writeUint32(h, uint32(len(data)))
	h.Write(data)
}

func (ch *CheckpointHasher) writeFixedBytes(h hash.Hash, data []byte, expectedLen int) {
	if len(data) == expectedLen {
		h.Write(data)
	} else {
		h.Write(make([]byte, expectedLen))
	}
}

func (ch *CheckpointHasher) sortSignatures(sigs [][]byte) [][]byte {
	// Create a copy to avoid modifying original
	sorted := make([][]byte, len(sigs))
	copy(sorted, sigs)

	// Sort by byte comparison
	sort.Slice(sorted, func(i, j int) bool {
		return string(sorted[i]) < string(sorted[j])
	})

	return sorted
}

// ComputeParentHash computes the hash of a parent checkpoint
// This should be used to set ParentCpHash field
func ComputeParentHash(parent *pb.CheckpointHeader) []byte {
	if parent == nil {
		return nil
	}

	hasher := NewCheckpointHasher()
	hash := hasher.ComputeHashForVerification(parent)
	return hash[:]
}

// ComputeMerkleRoot computes a merkle root from a list of hashes
func ComputeMerkleRoot(hashes [][]byte) []byte {
	if len(hashes) == 0 {
		return make([]byte, 32) // Empty root
	}

	// Simple merkle tree implementation
	// In production, use a proper merkle tree library
	current := hashes

	for len(current) > 1 {
		next := make([][]byte, 0)

		for i := 0; i < len(current); i += 2 {
			h := sha256.New()

			h.Write(current[i])
			if i+1 < len(current) {
				h.Write(current[i+1])
			} else {
				h.Write(current[i]) // Duplicate last element if odd
			}

			next = append(next, h.Sum(nil))
		}

		current = next
	}

	return current[0]
}

// ComputeStateRoot computes the state merkle root
func ComputeStateRoot(validators []string, balances []uint64) []byte {
	if len(validators) != len(balances) {
		return make([]byte, 32)
	}

	hashes := make([][]byte, len(validators))
	for i, validator := range validators {
		h := sha256.New()
		h.Write([]byte(validator))
		binary.Write(h, binary.BigEndian, balances[i])
		hashes[i] = h.Sum(nil)
	}

	return ComputeMerkleRoot(hashes)
}

// ComputeAgentRoot computes the agent merkle root
func ComputeAgentRoot(agents []string, statuses []string) []byte {
	if len(agents) != len(statuses) {
		return make([]byte, 32)
	}

	hashes := make([][]byte, len(agents))
	for i, agent := range agents {
		h := sha256.New()
		h.Write([]byte(agent))
		h.Write([]byte(statuses[i]))
		hashes[i] = h.Sum(nil)
	}

	return ComputeMerkleRoot(hashes)
}

// ComputeEventRoot computes the event/report merkle root
func ComputeEventRoot(reports []*pb.ExecutionReport) []byte {
	hashes := make([][]byte, len(reports))

	for i, report := range reports {
		h := sha256.New()
		h.Write([]byte(report.AssignmentId))
		h.Write([]byte(report.IntentId))
		h.Write([]byte(report.AgentId))
		binary.Write(h, binary.BigEndian, report.Timestamp)
		hashes[i] = h.Sum(nil)
	}

	return ComputeMerkleRoot(hashes)
}