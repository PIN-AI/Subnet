package consensus

import (
	"subnet/internal/types"
	"testing"
	"time"
)

func TestLeaderTrackerFailover(t *testing.T) {
	set := &types.ValidatorSet{MinValidators: 3, ThresholdNum: 2, ThresholdDenom: 3,
		Validators: []types.Validator{{ID: "v1"}, {ID: "v2"}, {ID: "v3"}}}
	tracker := NewLeaderTracker(set, 50*time.Millisecond)

	idx, leader := tracker.Leader(1)
	if idx != 1%3 || leader.ID != "v2" {
		t.Fatalf("unexpected leader: idx=%d id=%s", idx, leader.ID)
	}
	tracker.RecordActivity(1, time.Now())
	if tracker.ShouldFailover(1, time.Now().Add(30*time.Millisecond)) {
		t.Fatalf("should not failover yet")
	}
	if !tracker.ShouldFailover(1, time.Now().Add(80*time.Millisecond)) {
		t.Fatalf("expected failover after timeout")
	}

	idx2, backup := tracker.Backup(1)
	if idx2 != (idx+1)%3 || backup.ID != "v3" {
		t.Fatalf("unexpected backup: idx=%d id=%s", idx2, backup.ID)
	}
}
