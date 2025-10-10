package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
)

// Basic identifiers use strings for MVP; replace with fixed-size types as needed.

type Intent struct {
	IntentID     string
	SubnetID     string
	Requester    string
	IntentType   string
	CreatedAt    int64
	Deadline     int64
	ParamsHash   string
	Budget       uint64
	PaymentToken string
	Status       string

	// Bidding window fields
	BiddingStartTime int64
	BiddingEndTime   int64

	// Additional fields for matching
	IntentData []byte // Raw intent data
	Token      string // Payment token address
}

// Bid represents a bid from an agent for an intent
type Bid struct {
	BidID       string
	IntentID    string
	AgentID     string
	Price       uint64
	Token       string
	SubmittedAt int64
	Nonce       string
	Signature   string
	Metadata    []byte
}

// Assignment represents task assignment to an agent
type Assignment struct {
	AssignmentID string
	IntentID     string
	AgentID      string
	BidID        string
	AssignedAt   int64
	LeaseTTL     int64  // Lease duration in seconds
	Status       string
	MatcherID    string
	Signature    string
}

// MatchingResult represents the result of matching process
type MatchingResult struct {
	IntentID       string
	WinningBid     *Bid
	WinningAgentID string
	RunnerUpBids   []*Bid
	MatchedAt      int64
	MatcherID      string
	ResultHash     string
	Signature      string
}

type RootLayerRef struct {
	Height uint64
	Root   string
	Proof  []byte // optional
}

type Report struct {
	IntentID     string
	AssignmentID string
	RootRef      RootLayerRef
	ExecutedAt   int64
	ReceivedTS   int64

	// Agent-related (future-ready)
	AgentID         string
	AgentEndpoint   string
	AgentSignature  []byte
	AgentPubKey     []byte
	AgentNonce      uint64
	EnvFingerprint  []byte
	ResultHash      []byte
	ResultData      []byte
	ResultURI       string
	ProofExec       []byte
	ConfidenceScore uint32
	CostEstimate    uint64
	ValidatorHints  []byte
	ResourceUsage   *ResourceUsage // Resource usage information
}

// ResourceUsage contains resource consumption information
type ResourceUsage struct {
	CPUMs        uint64 // CPU milliseconds used
	MemoryMB     uint64 // Memory in megabytes
	IOOps        uint64 // IO operations count
	NetworkBytes uint64 // Network bytes transferred
}

type Receipt struct {
	IntentID    string
	ValidatorID string
	ReceivedTS  int64
	Status      string
	ScoreHint   uint32
}

type VerificationVerdict string

const (
	VerdictUnspecified VerificationVerdict = "unspecified"
	VerdictPass        VerificationVerdict = "pass"
	VerdictFail        VerificationVerdict = "fail"
	VerdictSkip        VerificationVerdict = "skip"
)

type VerificationRecord struct {
	RecordID           string
	IntentID           string
	AssignmentID       string
	AgentID            string
	ReportID           string
	PolicyID           string
	ValidatorID        string
	Verdict            VerificationVerdict
	Confidence         float64
	Reason             string
	EvidenceChecked    []string
	Timestamp          int64
	ValidatorSignature []byte
	Metadata           map[string]interface{} // Additional metadata
}

type CommitmentRoots struct {
	AgentRoot        string
	AgentServiceRoot string
	RankRoot         string
	MetricsRoot      string
	DataUsageRoot    string
	StateRoot        string
	EventRoot        string
	CrossSubnetRoot  string
}

type DACommitment struct {
	Kind          string
	Pointer       string
	SizeHint      uint64
	SegmentHashes [][]byte
	Expiry        int64
}

type EpochSlot struct {
	Epoch uint64
	Slot  uint64
}

type CheckpointSignatures struct {
	ECDSASignatures [][]byte
	SignersBitmap   []byte
	SignatureCount  uint32
	TotalWeight     uint64
}

type CheckpointHeader struct {
	SubnetID             string
	Epoch                uint64
	ParentCPHash         string
	Timestamp            int64
	Version              uint16
	ParamsHash           string
	Roots                CommitmentRoots
	DACommitments        []DACommitment
	ValidatorSetID       string
	Stats                []byte
	AuxHash              string
	EpochSlot            EpochSlot
	AssignmentsRoot      string
	ValidationCommitment string
	PolicyRoot           string
	Signatures           *CheckpointSignatures
}

type Signature struct {
	Algo     string // "ECDSA-secp256k1"
	DER      []byte
	PubKey   []byte
	MsgHash  []byte
	SignerID string // validator id / hotkey
}

type Validator struct {
	ID       string
	PubKey   []byte
	Weight   uint64
	Endpoint string
}

type ValidatorSet struct {
	Validators     []Validator
	MinValidators  int
	ThresholdNum   int
	ThresholdDenom int
	EffectiveEpoch uint64
}

// Validate checks if the validator set configuration is valid
func (vs *ValidatorSet) Validate() error {
	if len(vs.Validators) < vs.MinValidators {
		return fmt.Errorf("insufficient validators: got %d, need %d", len(vs.Validators), vs.MinValidators)
	}
	if vs.ThresholdNum <= 0 || vs.ThresholdDenom <= 0 {
		return fmt.Errorf("invalid threshold: %d/%d", vs.ThresholdNum, vs.ThresholdDenom)
	}
	if vs.ThresholdNum > vs.ThresholdDenom {
		return fmt.Errorf("invalid threshold: numerator %d > denominator %d", vs.ThresholdNum, vs.ThresholdDenom)
	}
	// Check for duplicate validator IDs
	seen := make(map[string]bool)
	for _, v := range vs.Validators {
		if seen[v.ID] {
			return fmt.Errorf("duplicate validator ID: %s", v.ID)
		}
		seen[v.ID] = true
	}
	return nil
}

// SortValidators sorts validators by ID for consistent ordering
func (vs *ValidatorSet) SortValidators() {
	sort.Slice(vs.Validators, func(i, j int) bool {
		return vs.Validators[i].ID < vs.Validators[j].ID
	})
}

// IndexByID returns the index of a validator by ID, or -1 if not found
func (vs *ValidatorSet) IndexByID(id string) int {
	for i, v := range vs.Validators {
		if v.ID == id {
			return i
		}
	}
	return -1
}

// RequiredSignatures calculates the minimum number of signatures needed
func (vs *ValidatorSet) RequiredSignatures() int {
	totalValidators := len(vs.Validators)
	required := (totalValidators * vs.ThresholdNum) / vs.ThresholdDenom
	// Round up if there's a remainder
	if (totalValidators*vs.ThresholdNum)%vs.ThresholdDenom > 0 {
		required++
	}
	// Ensure at least 1 signature is required
	if required < 1 {
		required = 1
	}
	return required
}

// CheckThreshold checks if the given number of signatures meets threshold
func (vs *ValidatorSet) CheckThreshold(numSignatures int) bool {
	return numSignatures >= vs.RequiredSignatures()
}

// TotalWeight calculates the total weight of all validators
func (vs *ValidatorSet) TotalWeight() uint64 {
	var total uint64
	for _, v := range vs.Validators {
		total += v.Weight
	}
	return total
}

// GetValidator returns a validator by ID or nil if not found
func (vs *ValidatorSet) GetValidator(id string) *Validator {
	for i := range vs.Validators {
		if vs.Validators[i].ID == id {
			return &vs.Validators[i]
		}
	}
	return nil
}

// UpsertValidator adds or updates a validator
func (vs *ValidatorSet) UpsertValidator(v Validator) {
	for i := range vs.Validators {
		if vs.Validators[i].ID == v.ID {
			vs.Validators[i] = v
			return
		}
	}
	vs.Validators = append(vs.Validators, v)
}

// RemoveValidator removes a validator by ID, returns true if removed
func (vs *ValidatorSet) RemoveValidator(id string) bool {
	for i, v := range vs.Validators {
		if v.ID == id {
			vs.Validators = append(vs.Validators[:i], vs.Validators[i+1:]...)
			return true
		}
	}
	return false
}

// Clone creates a deep copy of the validator set
func (vs *ValidatorSet) Clone() *ValidatorSet {
	clone := &ValidatorSet{
		MinValidators:  vs.MinValidators,
		ThresholdNum:   vs.ThresholdNum,
		ThresholdDenom: vs.ThresholdDenom,
		EffectiveEpoch: vs.EffectiveEpoch,
	}
	clone.Validators = make([]Validator, len(vs.Validators))
	copy(clone.Validators, vs.Validators)
	return clone
}

func (h *CheckpointHeader) CanonicalHash() []byte {
	// Domain-separated, length-prefixed canonical encoding over stable ordered fields.
	// domain: "checkpoint:v1"
	var buf bytes.Buffer
	buf.WriteString("checkpoint:v1|")
	// version
	vb := make([]byte, 4)
	binary.BigEndian.PutUint32(vb, uint32(h.Version))
	buf.Write(vb)
	// subnet_id
	buf.WriteString("|")
	buf.WriteString(h.SubnetID)
	// epoch
	eb := make([]byte, 8)
	binary.BigEndian.PutUint64(eb, h.Epoch)
	buf.Write(eb)
	// parent_cp_hash (as hex string already in struct)
	buf.WriteString("|")
	buf.WriteString(h.ParentCPHash)
	// timestamp
	tb := make([]byte, 8)
	binary.BigEndian.PutUint64(tb, uint64(h.Timestamp))
	buf.Write(tb)
	// validator_set_id
	buf.WriteString("|")
	buf.WriteString(h.ValidatorSetID)
	// params_hash
	buf.WriteString("|")
	buf.WriteString(h.ParamsHash)
	// roots (stable order)
	buf.WriteString("|")
	buf.WriteString(h.Roots.AgentRoot)
	buf.WriteString("|")
	buf.WriteString(h.Roots.AgentServiceRoot)
	buf.WriteString("|")
	buf.WriteString(h.Roots.RankRoot)
	buf.WriteString("|")
	buf.WriteString(h.Roots.MetricsRoot)
	buf.WriteString("|")
	buf.WriteString(h.Roots.DataUsageRoot)
	buf.WriteString("|")
	buf.WriteString(h.Roots.StateRoot)
	buf.WriteString("|")
	buf.WriteString(h.Roots.EventRoot)
	buf.WriteString("|")
	buf.WriteString(h.Roots.CrossSubnetRoot)
	buf.WriteString("|")
	buf.WriteString(h.AssignmentsRoot)
	buf.WriteString("|")
	buf.WriteString(h.ValidationCommitment)
	buf.WriteString("|")
	buf.WriteString(h.PolicyRoot)
	// aux hash
	buf.WriteString("|")
	buf.WriteString(h.AuxHash)
	// epoch_slot
	es := make([]byte, 16)
	binary.BigEndian.PutUint64(es[0:8], h.EpochSlot.Epoch)
	binary.BigEndian.PutUint64(es[8:16], h.EpochSlot.Slot)
	buf.Write(es)

	sum := sha256.Sum256(buf.Bytes())
	return sum[:]
}

func (h *CheckpointHeader) CanonicalHashHex() string {
	return hex.EncodeToString(h.CanonicalHash())
}
