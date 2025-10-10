package types

import "testing"

func TestValidatorSetValidate(t *testing.T) {
	vs := &ValidatorSet{
		MinValidators:  3,
		ThresholdNum:   2,
		ThresholdDenom: 3,
		Validators:     []Validator{{ID: "v1"}, {ID: "v2"}, {ID: "v3"}},
	}
	if err := vs.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatorSetValidateFailsDuplicate(t *testing.T) {
	vs := &ValidatorSet{MinValidators: 1, ThresholdNum: 1, ThresholdDenom: 1, Validators: []Validator{{ID: "v1"}, {ID: "v1"}}}
	if err := vs.Validate(); err == nil {
		t.Fatalf("expected duplicate error")
	}
}

func TestRequiredSignatures(t *testing.T) {
	vs := &ValidatorSet{MinValidators: 1, ThresholdNum: 3, ThresholdDenom: 4, Validators: []Validator{{ID: "v1"}, {ID: "v2"}, {ID: "v3"}, {ID: "v4"}}}
	if got := vs.RequiredSignatures(); got != 3 {
		t.Fatalf("expected 3 required signatures, got %d", got)
	}
	if !vs.CheckThreshold(3) {
		t.Fatalf("threshold check should pass for 3 signatures")
	}
	if vs.CheckThreshold(2) {
		t.Fatalf("threshold check should fail for 2 signatures")
	}
}

func TestUpsertAndRemove(t *testing.T) {
	vs := &ValidatorSet{MinValidators: 1, ThresholdNum: 1, ThresholdDenom: 1}
	vs.UpsertValidator(Validator{ID: "v1", Weight: 5})
	vs.UpsertValidator(Validator{ID: "v2", Weight: 3})
	if len(vs.Validators) != 2 {
		t.Fatalf("expected 2 validators")
	}
	total := vs.TotalWeight()
	if total != 8 {
		t.Fatalf("unexpected total weight %d", total)
	}
	vs.UpsertValidator(Validator{ID: "v1", Weight: 10})
	if vs.GetValidator("v1").Weight != 10 {
		t.Fatalf("upsert should replace existing validator")
	}
	if !vs.RemoveValidator("v2") {
		t.Fatalf("expected remove success")
	}
	if vs.RemoveValidator("v2") {
		t.Fatalf("expected remove to fail for missing validator")
	}
}

func TestCloneAndSort(t *testing.T) {
	vs := &ValidatorSet{MinValidators: 1, ThresholdNum: 1, ThresholdDenom: 1, Validators: []Validator{{ID: "b"}, {ID: "a"}}}
	clone := vs.Clone()
	if clone == vs || &clone.Validators[0] == &vs.Validators[0] {
		t.Fatalf("clone should deep copy slice")
	}
	vs.SortValidators()
	if vs.Validators[0].ID != "a" {
		t.Fatalf("expected sorted order")
	}
}
