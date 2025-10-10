package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"subnet/internal/types"
)

// HashCheckpoint computes a simple hash of the header's canonical fields.
func HashCheckpoint(h *types.CheckpointHeader) []byte {
	// Delegate to types for now.
	return h.CanonicalHash()
}

func HashCheckpointHex(h *types.CheckpointHeader) string {
	sum := HashCheckpoint(h)
	return hex.EncodeToString(sum)
}

// Bitmap helpers (MVP simple byte slice) -------------------------------------

func SetBit(bitmap []byte, idx int) []byte {
	byteIdx := idx / 8
	bitIdx := uint(idx % 8)
	if byteIdx >= len(bitmap) {
		// grow
		nb := make([]byte, byteIdx+1)
		copy(nb, bitmap)
		bitmap = nb
	}
	bitmap[byteIdx] |= (1 << bitIdx)
	return bitmap
}

func BitIsSet(bitmap []byte, idx int) bool {
	byteIdx := idx / 8
	bitIdx := uint(idx % 8)
	if byteIdx >= len(bitmap) {
		return false
	}
	return (bitmap[byteIdx] & (1 << bitIdx)) != 0
}

func CountBits(bitmap []byte) int {
	c := 0
	for _, b := range bitmap {
		// Brian Kernighan’s algorithm
		v := b
		for v != 0 {
			v &= v - 1
			c++
		}
	}
	return c
}

// Minimal parent/child continuity check (hash equality of parent field)
func IsChildOf(child, parent *types.CheckpointHeader) bool {
	return child.ParentCPHash == hex.EncodeToString(parent.CanonicalHash())
}

// Utility to compute aux hash (optional statistics)
func ComputeAuxHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
