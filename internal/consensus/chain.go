package consensus

import (
	"encoding/hex"
	"fmt"
	"subnet/internal/types"
)

// VerifyChainContinuity checks parent/child linkage and monotonic epochs.
// Returns nil if consistent across all consecutive headers.
func VerifyChainContinuity(headers []*types.CheckpointHeader) error {
	if len(headers) <= 1 {
		return nil
	}
	for i := 1; i < len(headers); i++ {
		parent := headers[i-1]
		child := headers[i]
		// epoch monotonic +1
		if child.Epoch != parent.Epoch+1 {
			return fmt.Errorf("epoch continuity broken at index %d: %d -> %d", i, parent.Epoch, child.Epoch)
		}
		// parent hash matches
		expected := hex.EncodeToString(parent.CanonicalHash())
		if child.ParentCPHash != expected {
			return fmt.Errorf("parent hash mismatch at epoch %d: got %s expected %s", child.Epoch, child.ParentCPHash, expected)
		}
	}
	return nil
}

// FindAnchorIndexByHash returns index of header with canonical hash hex equal to anchor.
func FindAnchorIndexByHash(headers []*types.CheckpointHeader, anchorHashHex string) int {
	if anchorHashHex == "" {
		return -1
	}
	for i, h := range headers {
		if hex.EncodeToString(h.CanonicalHash()) == anchorHashHex {
			return i
		}
	}
	return -1
}

// FindFirstDiscontinuityIndex returns the first index i where headers[i] is not a valid child of headers[i-1].
// Returns -1 if no discontinuity is found.
func FindFirstDiscontinuityIndex(headers []*types.CheckpointHeader) int {
	if len(headers) <= 1 {
		return -1
	}
	for i := 1; i < len(headers); i++ {
		parent := headers[i-1]
		child := headers[i]
		if child.Epoch != parent.Epoch+1 {
			return i
		}
		if hex.EncodeToString(parent.CanonicalHash()) != child.ParentCPHash {
			return i
		}
	}
	return -1
}
