package consensus

import "subnet/internal/types"

// SelectLeader returns the index of the leader validator for the given epoch.
func SelectLeader(epoch uint64, vs *types.ValidatorSet) int {
	n := len(vs.Validators)
	if n == 0 {
		return -1
	}
	return int(epoch % uint64(n))
}

// SelectBackup returns the index of the backup leader (next in rotation).
func SelectBackup(epoch uint64, vs *types.ValidatorSet) int {
	n := len(vs.Validators)
	if n == 0 {
		return -1
	}
	leader := SelectLeader(epoch, vs)
	return (leader + 1) % n
}
