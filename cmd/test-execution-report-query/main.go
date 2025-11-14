package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "subnet/proto/subnet"
)

func main() {
	// Connect to validator
	validatorAddr := "localhost:9090"
	conn, err := grpc.Dial(validatorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to validator: %v", err)
	}
	defer conn.Close()

	client := pb.NewValidatorServiceClient(conn)
	ctx := context.Background()

	fmt.Println("=== Testing ExecutionReport Query API ===\n")

	// Step 1: Submit an execution report
	fmt.Println("Step 1: Submitting ExecutionReport...")
	resultData := map[string]interface{}{
		"status": "success",
		"message": "Task completed successfully",
		"data": map[string]interface{}{
			"result": 42,
			"computeTime": "120ms",
		},
	}
	resultBytes, _ := json.Marshal(resultData)

	report := &pb.ExecutionReport{
		IntentId:     "test-intent-" + time.Now().Format("20060102-150405"),
		AssignmentId: "test-assignment-001",
		AgentId:      "test-agent-query-api",
		ResultData:   resultBytes,
		Timestamp:    time.Now().Unix(),
	}

	receipt, err := client.SubmitExecutionReport(ctx, report)
	if err != nil {
		log.Fatalf("Failed to submit execution report: %v", err)
	}

	fmt.Printf("✅ Submitted successfully!\n")
	fmt.Printf("   Report ID: %s\n", receipt.ReportId)
	fmt.Printf("   Status: %s\n", receipt.Status)
	fmt.Printf("   Validator: %s\n\n", receipt.ValidatorId)

	// Wait a moment for report to be saved
	time.Sleep(500 * time.Millisecond)

	// Step 2: Query the specific execution report
	fmt.Println("Step 2: Querying ExecutionReport by ID...")
	getReq := &pb.GetExecutionReportRequest{
		ReportId: receipt.ReportId,
	}

	retrievedReport, err := client.GetExecutionReport(ctx, getReq)
	if err != nil {
		log.Fatalf("Failed to get execution report: %v", err)
	}

	fmt.Printf("✅ Retrieved successfully!\n")
	fmt.Printf("   Intent ID: %s\n", retrievedReport.IntentId)
	fmt.Printf("   Assignment ID: %s\n", retrievedReport.AssignmentId)
	fmt.Printf("   Agent ID: %s\n", retrievedReport.AgentId)
	fmt.Printf("   Timestamp: %d\n", retrievedReport.Timestamp)
	fmt.Printf("   Result Data: %s\n\n", string(retrievedReport.ResultData))

	// Step 3: List all execution reports
	fmt.Println("Step 3: Listing all ExecutionReports...")
	listReq := &pb.ListExecutionReportsRequest{
		Limit: 10,
	}

	listResp, err := client.ListExecutionReports(ctx, listReq)
	if err != nil {
		log.Fatalf("Failed to list execution reports: %v", err)
	}

	fmt.Printf("✅ Found %d execution reports:\n", listResp.Total)
	for i, entry := range listResp.Reports {
		fmt.Printf("\n   Report #%d:\n", i+1)
		fmt.Printf("   - Report ID: %s\n", entry.ReportId)
		fmt.Printf("   - Intent ID: %s\n", entry.Report.IntentId)
		fmt.Printf("   - Agent ID: %s\n", entry.Report.AgentId)

		// Parse and display result data
		var result map[string]interface{}
		if err := json.Unmarshal(entry.Report.ResultData, &result); err == nil {
			prettyResult, _ := json.MarshalIndent(result, "     ", "  ")
			fmt.Printf("   - Result:\n%s\n", string(prettyResult))
		} else {
			fmt.Printf("   - Result Data: %s\n", string(entry.Report.ResultData))
		}
	}

	fmt.Println("\n=== Test Completed Successfully! ===")
}
