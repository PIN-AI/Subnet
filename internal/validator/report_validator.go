package validator

import (
	"errors"
	"fmt"
	"time"

	pb "subnet/proto/subnet"
	"subnet/internal/crypto"
)

// ReportValidationRules defines all validation rules for execution reports
type ReportValidationRules struct {
	// Maximum time allowed between assignment and report
	MaxExecutionTime time.Duration

	// Maximum time difference allowed from current time
	MaxTimeDrift time.Duration

	// Minimum required evidence fields
	RequireEvidence bool

	// Require signature verification
	RequireSignature bool

	// Verifier service for signature checks
	verifier *crypto.VerifierService
}

// NewReportValidationRules creates default validation rules
func NewReportValidationRules() *ReportValidationRules {
	return &ReportValidationRules{
		MaxExecutionTime: 30 * time.Minute,
		MaxTimeDrift:     5 * time.Minute,
		RequireEvidence:  true,
		RequireSignature: true,
		verifier:         crypto.NewVerifierService(),
	}
}

// ValidateExecutionReport performs comprehensive validation of an execution report
func (r *ReportValidationRules) ValidateExecutionReport(report *pb.ExecutionReport, assignment *pb.ExecutionTask) error {
	if report == nil {
		return errors.New("execution report is nil")
	}

	// 1. Validate basic fields
	if err := r.validateBasicFields(report); err != nil {
		return fmt.Errorf("basic field validation failed: %w", err)
	}

	// 2. Validate report matches assignment
	if assignment != nil {
		if err := r.validateReportMatchesAssignment(report, assignment); err != nil {
			return fmt.Errorf("report-assignment mismatch: %w", err)
		}
	}

	// 3. Validate timestamps
	if err := r.validateTimestamps(report, assignment); err != nil {
		return fmt.Errorf("timestamp validation failed: %w", err)
	}

	// 4. Validate status and result consistency
	if err := r.validateStatusConsistency(report); err != nil {
		return fmt.Errorf("status consistency validation failed: %w", err)
	}

	// 5. Validate evidence if required
	if r.RequireEvidence {
		if err := r.validateEvidence(report); err != nil {
			return fmt.Errorf("evidence validation failed: %w", err)
		}
	}

	// 6. Validate signature if required
	if r.RequireSignature {
		if err := r.validateSignature(report); err != nil {
			return fmt.Errorf("signature validation failed: %w", err)
		}
	}

	return nil
}

// validateBasicFields checks all required fields are present and valid
func (r *ReportValidationRules) validateBasicFields(report *pb.ExecutionReport) error {
	if report.ReportId == "" {
		return errors.New("report ID is empty")
	}
	if report.AssignmentId == "" {
		return errors.New("assignment ID is empty")
	}
	if report.IntentId == "" {
		return errors.New("intent ID is empty")
	}
	if report.AgentId == "" {
		return errors.New("agent ID is empty")
	}
	if report.Status == pb.ExecutionReport_STATUS_UNSPECIFIED {
		return errors.New("status is unspecified")
	}
	if report.Timestamp == 0 {
		return errors.New("timestamp is zero")
	}

	return nil
}

// validateReportMatchesAssignment ensures the report corresponds to the correct assignment
func (r *ReportValidationRules) validateReportMatchesAssignment(report *pb.ExecutionReport, assignment *pb.ExecutionTask) error {
	if report.AssignmentId != assignment.TaskId {
		return fmt.Errorf("assignment ID mismatch: report has %s, expected %s",
			report.AssignmentId, assignment.TaskId)
	}
	if report.IntentId != assignment.IntentId {
		return fmt.Errorf("intent ID mismatch: report has %s, expected %s",
			report.IntentId, assignment.IntentId)
	}
	if report.AgentId != assignment.AgentId {
		return fmt.Errorf("agent ID mismatch: report has %s, expected %s",
			report.AgentId, assignment.AgentId)
	}

	return nil
}

// validateTimestamps checks timestamp validity and ordering
func (r *ReportValidationRules) validateTimestamps(report *pb.ExecutionReport, assignment *pb.ExecutionTask) error {
	reportTime := time.Unix(report.Timestamp, 0)
	now := time.Now()

	// Check timestamp is not in the future
	if reportTime.After(now.Add(r.MaxTimeDrift)) {
		return fmt.Errorf("report timestamp is too far in the future: %v", reportTime)
	}

	// Check timestamp is not too old
	if reportTime.Before(now.Add(-r.MaxTimeDrift - r.MaxExecutionTime)) {
		return fmt.Errorf("report timestamp is too old: %v", reportTime)
	}

	// If assignment is provided, check execution time
	if assignment != nil && assignment.CreatedAt > 0 {
		assignmentTime := time.Unix(assignment.CreatedAt, 0)

		// Report must come after assignment
		if reportTime.Before(assignmentTime) {
			return fmt.Errorf("report timestamp %v is before assignment timestamp %v",
				reportTime, assignmentTime)
		}

		// Check execution didn't take too long
		executionDuration := reportTime.Sub(assignmentTime)
		if executionDuration > r.MaxExecutionTime {
			return fmt.Errorf("execution took too long: %v (max: %v)",
				executionDuration, r.MaxExecutionTime)
		}
	}

	return nil
}

// validateStatusConsistency ensures status and result data are consistent
func (r *ReportValidationRules) validateStatusConsistency(report *pb.ExecutionReport) error {
	switch report.Status {
	case pb.ExecutionReport_SUCCESS:
		// Success reports should have result data
		if len(report.ResultData) == 0 {
			return errors.New("success report missing result data")
		}
		// Success reports should not have error info
		if report.Error != nil {
			return errors.New("success report should not have error info")
		}

	case pb.ExecutionReport_FAILED:
		// Failure reports should have error info
		if report.Error == nil {
			return errors.New("failure report missing error info")
		}
		// Validate error info
		if report.Error.Code == "" && report.Error.Message == "" {
			return errors.New("failure report has empty error info")
		}

	case pb.ExecutionReport_PARTIAL:
		// Partial reports can have both result data and error info
		if len(report.ResultData) == 0 && report.Error == nil {
			return errors.New("partial report must have either result data or error info")
		}

	default:
		return fmt.Errorf("unknown execution status: %v", report.Status)
	}

	return nil
}

// validateEvidence checks that required evidence is present and valid
func (r *ReportValidationRules) validateEvidence(report *pb.ExecutionReport) error {
	if report.Evidence == nil {
		return errors.New("evidence is required but missing")
	}

	evidence := report.Evidence

	// Check execution proof
	if len(evidence.ProofExec) == 0 {
		return errors.New("execution proof is required but missing")
	}

	// Check environment fingerprint
	if len(evidence.EnvFingerprint) == 0 {
		return errors.New("environment fingerprint is required but missing")
	}

	// Check input/output hashes
	if len(evidence.InputsHash) == 0 {
		return errors.New("inputs hash is required but missing")
	}
	if len(evidence.OutputsHash) == 0 {
		return errors.New("outputs hash is required but missing")
	}

	// Check transcript root for large results
	if len(report.ResultData) > 1024*1024 { // > 1MB
		if len(evidence.TranscriptRoot) == 0 {
			return errors.New("transcript root required for large results")
		}
		// External references should point to data availability
		if len(evidence.ExternalRefs) == 0 {
			return errors.New("external references required for large results")
		}
	}

	// Validate resource usage if present
	if evidence.ResourceUsage != nil {
		if err := r.validateResourceUsage(evidence.ResourceUsage); err != nil {
			return fmt.Errorf("resource usage validation failed: %w", err)
		}
	}

	return nil
}

// validateResourceUsage validates resource usage metrics
func (r *ReportValidationRules) validateResourceUsage(usage *pb.ResourceUsage) error {
	if usage == nil {
		return errors.New("resource usage is nil")
	}

	// Check for unrealistic values (potential overflow or error)
	const maxReasonableCpuMs = 1000000000    // ~11.5 days of CPU time
	const maxReasonableMemoryMb = 1000000000 // 1 petabyte
	const maxReasonableIoOps = 1000000000    // 1 billion I/O ops
	const maxReasonableNetworkBytes = 1 << 50 // 1 petabyte

	if usage.CpuMs > maxReasonableCpuMs {
		return fmt.Errorf("unrealistic CPU usage: %d ms", usage.CpuMs)
	}
	if usage.MemoryMb > maxReasonableMemoryMb {
		return fmt.Errorf("unrealistic memory usage: %d MB", usage.MemoryMb)
	}
	if usage.IoOps > maxReasonableIoOps {
		return fmt.Errorf("unrealistic I/O operations: %d", usage.IoOps)
	}
	if usage.NetworkBytes > maxReasonableNetworkBytes {
		return fmt.Errorf("unrealistic network usage: %d bytes", usage.NetworkBytes)
	}

	return nil
}

// validateSignature verifies the agent's signature on the report
func (r *ReportValidationRules) validateSignature(report *pb.ExecutionReport) error {
	if len(report.Signature) == 0 {
		return errors.New("report signature is required but missing")
	}

	if r.verifier != nil {
		err := r.verifier.VerifyExecutionReportSignature(report)
		if err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	return nil
}

// ValidateBatch validates multiple execution reports
func (r *ReportValidationRules) ValidateBatch(reports []*pb.ExecutionReport, assignments map[string]*pb.ExecutionTask) ([]error, error) {
	if len(reports) == 0 {
		return nil, errors.New("no reports to validate")
	}

	results := make([]error, len(reports))
	hasErrors := false

	for i, report := range reports {
		var assignment *pb.ExecutionTask
		if assignments != nil && report != nil {
			assignment = assignments[report.AssignmentId]
		}

		err := r.ValidateExecutionReport(report, assignment)
		results[i] = err
		if err != nil {
			hasErrors = true
		}
	}

	if hasErrors {
		return results, fmt.Errorf("batch validation failed: some reports invalid")
	}

	return results, nil
}

// SetTrustedAgent adds a trusted agent's public key for signature verification
func (r *ReportValidationRules) SetTrustedAgent(agentID string, pubKey []byte) {
	if r.verifier != nil {
		r.verifier.AddTrustedKey(agentID, pubKey)
	}
}

// ReportValidator is the main validator for execution reports
type ReportValidator struct {
	rules *ReportValidationRules
}

// NewReportValidator creates a new report validator with default rules
func NewReportValidator() *ReportValidator {
	return &ReportValidator{
		rules: NewReportValidationRules(),
	}
}

// ValidateReport validates a single execution report
func (rv *ReportValidator) ValidateReport(report *pb.ExecutionReport, assignment *pb.ExecutionTask) error {
	return rv.rules.ValidateExecutionReport(report, assignment)
}

// ValidateReportBatch validates multiple execution reports
func (rv *ReportValidator) ValidateReportBatch(reports []*pb.ExecutionReport, assignments map[string]*pb.ExecutionTask) ([]error, error) {
	return rv.rules.ValidateBatch(reports, assignments)
}

// ConfigureRules allows customization of validation rules
func (rv *ReportValidator) ConfigureRules(configureFn func(*ReportValidationRules)) {
	if configureFn != nil {
		configureFn(rv.rules)
	}
}