package storage

import (
	"testing"

	pb "subnet/proto/subnet"

	"google.golang.org/protobuf/proto"
)

func TestExecutionReportStorage(t *testing.T) {
	// Create in-memory store
	store := NewInMemory()

	// Create test execution report
	report := &pb.ExecutionReport{
		IntentId:     "test-intent-123",
		AssignmentId: "test-assignment-456",
		AgentId:      "test-agent-789",
		ResultData:   []byte(`{"status": "success", "result": "Task completed successfully"}`),
		Timestamp:    1234567890,
	}

	reportID := "test-intent-123:test-assignment-456:test-agent-789:1234567890"

	// Marshal report
	reportBytes, err := proto.Marshal(report)
	if err != nil {
		t.Fatalf("Failed to marshal report: %v", err)
	}

	// Test SaveExecutionReport
	t.Run("SaveExecutionReport", func(t *testing.T) {
		err := store.SaveExecutionReport(reportID, reportBytes)
		if err != nil {
			t.Errorf("SaveExecutionReport failed: %v", err)
		}
	})

	// Test GetExecutionReport
	t.Run("GetExecutionReport", func(t *testing.T) {
		retrievedBytes, err := store.GetExecutionReport(reportID)
		if err != nil {
			t.Fatalf("GetExecutionReport failed: %v", err)
		}

		// Unmarshal and verify
		var retrievedReport pb.ExecutionReport
		if err := proto.Unmarshal(retrievedBytes, &retrievedReport); err != nil {
			t.Fatalf("Failed to unmarshal retrieved report: %v", err)
		}

		if retrievedReport.IntentId != report.IntentId {
			t.Errorf("IntentId mismatch: got %s, want %s", retrievedReport.IntentId, report.IntentId)
		}
		if retrievedReport.AssignmentId != report.AssignmentId {
			t.Errorf("AssignmentId mismatch: got %s, want %s", retrievedReport.AssignmentId, report.AssignmentId)
		}
		if retrievedReport.AgentId != report.AgentId {
			t.Errorf("AgentId mismatch: got %s, want %s", retrievedReport.AgentId, report.AgentId)
		}
		if string(retrievedReport.ResultData) != string(report.ResultData) {
			t.Errorf("ResultData mismatch: got %s, want %s", retrievedReport.ResultData, report.ResultData)
		}
	})

	// Test GetExecutionReport - Not Found
	t.Run("GetExecutionReport_NotFound", func(t *testing.T) {
		_, err := store.GetExecutionReport("nonexistent-report-id")
		if err == nil {
			t.Error("Expected error for nonexistent report, got nil")
		}
	})

	// Test ListExecutionReports
	t.Run("ListExecutionReports", func(t *testing.T) {
		// Add another report
		report2 := &pb.ExecutionReport{
			IntentId:     "test-intent-456",
			AssignmentId: "test-assignment-789",
			AgentId:      "test-agent-000",
			ResultData:   []byte(`{"status": "failed", "error": "Task failed"}`),
			Timestamp:    1234567891,
		}
		reportID2 := "test-intent-456:test-assignment-789:test-agent-000:1234567891"
		reportBytes2, _ := proto.Marshal(report2)
		store.SaveExecutionReport(reportID2, reportBytes2)

		// List all reports
		reports, err := store.ListExecutionReports("", 10)
		if err != nil {
			t.Fatalf("ListExecutionReports failed: %v", err)
		}

		if len(reports) != 2 {
			t.Errorf("Expected 2 reports, got %d", len(reports))
		}

		// Verify both reports are present
		_, ok1 := reports[reportID]
		_, ok2 := reports[reportID2]
		if !ok1 || !ok2 {
			t.Error("Not all reports were returned")
		}
	})

	// Test ListExecutionReports with limit
	t.Run("ListExecutionReports_WithLimit", func(t *testing.T) {
		reports, err := store.ListExecutionReports("", 1)
		if err != nil {
			t.Fatalf("ListExecutionReports failed: %v", err)
		}

		if len(reports) != 1 {
			t.Errorf("Expected 1 report (limit=1), got %d", len(reports))
		}
	})
}
