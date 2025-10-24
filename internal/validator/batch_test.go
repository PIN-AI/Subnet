package validator

import (
	"context"
	"testing"
	"time"

	"subnet/internal/logging"
	pb "subnet/proto/subnet"
)

func TestSubmitExecutionReportBatch(t *testing.T) {
	// Create test logger
	logger := logging.NewDefaultLogger()

	// Create test server
	node := &Node{
		id:             "test-validator",
		pendingReports: make(map[string]*pb.ExecutionReport),
		reportScores:   make(map[string]int32),
		logger:         logger,
		config:         &Config{}, // Add empty config to avoid nil pointer
	}

	server := &Server{
		node:            node,
		logger:          logger,
		reportRateLimit: make(map[string]time.Time),
	}

	ctx := context.Background()

	// Test 1: Empty batch
	t.Run("empty_batch", func(t *testing.T) {
		req := &pb.ExecutionReportBatchRequest{
			Reports: []*pb.ExecutionReport{},
		}
		_, err := server.SubmitExecutionReportBatch(ctx, req)
		if err == nil {
			t.Error("Expected error for empty batch")
		}
	})

	// Test 2: Valid batch
	t.Run("valid_batch", func(t *testing.T) {
		req := &pb.ExecutionReportBatchRequest{
			Reports: []*pb.ExecutionReport{
				{
					ReportId:     "report-1",
					AssignmentId: "assignment-1",
					IntentId:     "intent-1",
					AgentId:      "agent-1",
					Timestamp:    time.Now().Unix(),
				},
				{
					ReportId:     "report-2",
					AssignmentId: "assignment-2",
					IntentId:     "intent-2",
					AgentId:      "agent-2",
					Timestamp:    time.Now().Unix(),
				},
			},
			BatchId: "batch-1",
		}

		resp, err := server.SubmitExecutionReportBatch(ctx, req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(resp.Receipts) != 2 {
			t.Errorf("Expected 2 receipts, got %d", len(resp.Receipts))
		}

		if resp.Success != 2 {
			t.Errorf("Expected success=2, got %d", resp.Success)
		}

		if resp.Failed != 0 {
			t.Errorf("Expected failed=0, got %d", resp.Failed)
		}
	})

	// Test 3: Partial success with partial_ok=false (stop on first error)
	t.Run("stop_on_error", func(t *testing.T) {
		partialOk := false
		req := &pb.ExecutionReportBatchRequest{
			Reports: []*pb.ExecutionReport{
				{
					ReportId:     "report-3",
					AssignmentId: "assignment-3",
					IntentId:     "intent-3",
					AgentId:      "agent-3",
					Timestamp:    time.Now().Unix(),
				},
				{
					ReportId:     "report-4",
					AssignmentId: "", // Invalid - missing assignment ID
					IntentId:     "intent-4",
					AgentId:      "agent-4",
					Timestamp:    time.Now().Unix(),
				},
				{
					ReportId:     "report-5",
					AssignmentId: "assignment-5",
					IntentId:     "intent-5",
					AgentId:      "agent-5",
					Timestamp:    time.Now().Unix(),
				},
			},
			BatchId:   "batch-2",
			PartialOk: &partialOk,
		}

		resp, err := server.SubmitExecutionReportBatch(ctx, req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// First should succeed, second should fail, third should be rejected
		if resp.Success != 1 {
			t.Errorf("Expected success=1, got %d", resp.Success)
		}

		if resp.Failed != 2 {
			t.Errorf("Expected failed=2, got %d", resp.Failed)
		}
	})

	// Test 4: Partial success with partial_ok=true (continue on error)
	t.Run("continue_on_error", func(t *testing.T) {
		partialOk := true
		req := &pb.ExecutionReportBatchRequest{
			Reports: []*pb.ExecutionReport{
				{
					ReportId:     "report-6",
					AssignmentId: "assignment-6",
					IntentId:     "intent-6",
					AgentId:      "agent-6",
					Timestamp:    time.Now().Unix(),
				},
				{
					ReportId:     "report-7",
					AssignmentId: "", // Invalid - missing assignment ID
					IntentId:     "intent-7",
					AgentId:      "agent-7",
					Timestamp:    time.Now().Unix(),
				},
				{
					ReportId:     "report-8",
					AssignmentId: "assignment-8",
					IntentId:     "intent-8",
					AgentId:      "agent-8",
					Timestamp:    time.Now().Unix(),
				},
			},
			BatchId:   "batch-3",
			PartialOk: &partialOk,
		}

		resp, err := server.SubmitExecutionReportBatch(ctx, req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// First and third should succeed, second should fail
		if resp.Success != 2 {
			t.Errorf("Expected success=2, got %d", resp.Success)
		}

		if resp.Failed != 1 {
			t.Errorf("Expected failed=1, got %d", resp.Failed)
		}
	})
}
