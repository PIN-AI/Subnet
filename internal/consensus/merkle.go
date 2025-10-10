package consensus

import (
	"crypto/sha256"
	"sort"
)

// MerkleTree provides simple merkle tree functionality
type MerkleTree struct {
	leaves [][]byte
	root   []byte
}

// NewMerkleTree creates a new merkle tree from data
func NewMerkleTree(data [][]byte) *MerkleTree {
	if len(data) == 0 {
		return &MerkleTree{
			root: emptyHash(),
		}
	}

	// Create leaves
	leaves := make([][]byte, len(data))
	for i, d := range data {
		leaves[i] = hash(d)
	}

	tree := &MerkleTree{
		leaves: leaves,
	}
	tree.root = tree.computeRoot(leaves)
	return tree
}

// Root returns the merkle root
func (m *MerkleTree) Root() []byte {
	return m.root
}

// computeRoot recursively computes the merkle root
func (m *MerkleTree) computeRoot(nodes [][]byte) []byte {
	if len(nodes) == 0 {
		return emptyHash()
	}
	if len(nodes) == 1 {
		return nodes[0]
	}

	// Compute next level
	var nextLevel [][]byte
	for i := 0; i < len(nodes); i += 2 {
		if i+1 < len(nodes) {
			// Hash pair
			combined := append(nodes[i], nodes[i+1]...)
			nextLevel = append(nextLevel, hash(combined))
		} else {
			// Odd node, hash with itself
			combined := append(nodes[i], nodes[i]...)
			nextLevel = append(nextLevel, hash(combined))
		}
	}

	return m.computeRoot(nextLevel)
}

// ComputeMerkleRoot computes merkle root for a list of items
func ComputeMerkleRoot(items [][]byte) []byte {
	tree := NewMerkleTree(items)
	return tree.Root()
}

// ComputeSortedMerkleRoot computes merkle root after sorting items
func ComputeSortedMerkleRoot(items map[string][]byte) []byte {
	if len(items) == 0 {
		return emptyHash()
	}

	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build sorted data
	data := make([][]byte, len(keys))
	for i, k := range keys {
		// Include key in hash for uniqueness
		keyedData := append([]byte(k), items[k]...)
		data[i] = keyedData
	}

	return ComputeMerkleRoot(data)
}

// hash computes SHA256 hash
func hash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// emptyHash returns hash of empty data
func emptyHash() []byte {
	return hash([]byte{})
}