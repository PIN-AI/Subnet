package consensus

import "subnet/internal/types"

// RequiredSigs computes floor(N * num / denom) with min 1.
func RequiredSigs(vs *types.ValidatorSet) int {
	if vs == nil {
		return 0
	}
	return vs.RequiredSignatures()
}

func CheckThreshold(sigCount int, vs *types.ValidatorSet) bool {
	return sigCount >= RequiredSigs(vs)
}
