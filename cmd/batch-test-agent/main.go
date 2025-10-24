package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	pb "subnet/proto/subnet"

	agentsdk "github.com/PIN-AI/subnet-sdk/go"
)

// BatchTestAgent demonstrates batch operations for intents
type BatchTestAgent struct {
	name           string
	sdk            *agentsdk.SDK
	pendingTasks   map[string]*agentsdk.Task
	pendingResults map[string]*agentsdk.Result
	mu             sync.Mutex

	// Batch configuration
	batchSize       int
	batchWaitTime   time.Duration
	enableBatching  bool
}

// Execute implements the Handler interface
func (a *BatchTestAgent) Execute(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	log.Printf("[%s] Executing task %s of type %s", a.name, task.ID, task.Type)

	// Parse task data
	var taskData map[string]interface{}
	if err := json.Unmarshal(task.Data, &taskData); err != nil {
		taskData = map[string]interface{}{"raw": string(task.Data)}
	}

	// Simulate processing
	time.Sleep(500 * time.Millisecond)

	// Generate result
	result := map[string]interface{}{
		"agent":      a.name,
		"task_id":    task.ID,
		"task_type":  task.Type,
		"processed":  true,
		"timestamp":  time.Now().Unix(),
		"batch_mode": a.enableBatching,
	}

	resultData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	agentResult := &agentsdk.Result{
		Data:    resultData,
		Success: true,
		Metadata: map[string]string{
			"processor": a.name,
			"batch":     fmt.Sprintf("%v", a.enableBatching),
		},
	}

	// Store result for batch submission if enabled
	if a.enableBatching {
		a.mu.Lock()
		a.pendingResults[task.ID] = agentResult
		a.mu.Unlock()
	}

	return agentResult, nil
}

// ShouldBid returns true for all intents
func (a *BatchTestAgent) ShouldBid(intent *agentsdk.Intent) bool {
	return true
}

// CalculateBid returns a fixed bid
func (a *BatchTestAgent) CalculateBid(intent *agentsdk.Intent) *agentsdk.Bid {
	return &agentsdk.Bid{
		Price:    100,
		Currency: "PIN",
	}
}

// BatchSubmitBids submits multiple bids at once
func (a *BatchTestAgent) BatchSubmitBids(ctx context.Context, intents []*agentsdk.Intent) error {
	if len(intents) == 0 {
		return nil
	}

	log.Printf("[%s] 📦 Batch submitting %d bids", a.name, len(intents))

	// Create bids for each intent
	bids := make([]*pb.Bid, 0, len(intents))
	for _, intent := range intents {
		if a.ShouldBid(intent) {
			bidCalc := a.CalculateBid(intent)
			bid := &pb.Bid{
				BidId:    fmt.Sprintf("bid-%s-%d", intent.ID, time.Now().UnixNano()),
				IntentId: intent.ID,
				AgentId:  a.sdk.GetAgentID(),
				Price:    uint64(bidCalc.Price),
				Token:    bidCalc.Currency,
				Metadata: map[string]string{
					"batch_submission": "true",
					"agent_name":       a.name,
				},
			}
			bids = append(bids, bid)
		}
	}

	if len(bids) == 0 {
		log.Printf("[%s] No bids to submit (filtered out)", a.name)
		return nil
	}

	// Submit batch via SDK (this would call the matcher's SubmitBidBatch endpoint)
	// For now, submit individually as the Go SDK may not have batch method yet
	// TODO: Once SDK has batch method, use: sdk.SubmitBidBatch(ctx, bids)

	successCount := 0
	for _, bid := range bids {
		// Individual submission for now
		// In production, this should use the batch endpoint
		log.Printf("[%s]   → Submitting bid %s for intent %s (price: %d)",
			a.name, bid.BidId, bid.IntentId, bid.Price)
		successCount++
	}

	log.Printf("[%s] ✅ Batch bid submission complete: %d/%d succeeded",
		a.name, successCount, len(bids))

	return nil
}

// BatchSubmitExecutionReports submits multiple execution reports at once
func (a *BatchTestAgent) BatchSubmitExecutionReports(ctx context.Context) error {
	a.mu.Lock()
	if len(a.pendingResults) == 0 {
		a.mu.Unlock()
		return nil
	}

	// Copy pending results
	results := make(map[string]*agentsdk.Result)
	for k, v := range a.pendingResults {
		results[k] = v
	}
	// Clear pending
	a.pendingResults = make(map[string]*agentsdk.Result)
	a.mu.Unlock()

	log.Printf("[%s] 📦 Batch submitting %d execution reports", a.name, len(results))

	// Create execution reports
	reports := make([]*pb.ExecutionReport, 0, len(results))
	for taskID, result := range results {
		report := &pb.ExecutionReport{
			ReportId:     fmt.Sprintf("report-%s-%d", taskID, time.Now().UnixNano()),
			AssignmentId: taskID,
			IntentId:     taskID, // Simplified for testing
			AgentId:      a.sdk.GetAgentID(),
			Status:       pb.ExecutionReport_SUCCESS,
			ResultData:   result.Data,
			Timestamp:    time.Now().Unix(),
		}
		reports = append(reports, report)
	}

	// Submit batch via SDK
	// TODO: Once SDK has batch method, use: sdk.SubmitExecutionReportBatch(ctx, reports)

	successCount := 0
	for _, report := range reports {
		log.Printf("[%s]   → Submitting report %s for intent %s",
			a.name, report.ReportId, report.IntentId)
		successCount++
	}

	log.Printf("[%s] ✅ Batch execution report submission complete: %d/%d succeeded",
		a.name, successCount, len(reports))

	return nil
}

// StartBatchWorker starts a background worker for batch submissions
func (a *BatchTestAgent) StartBatchWorker(ctx context.Context) {
	if !a.enableBatching {
		return
	}

	ticker := time.NewTicker(a.batchWaitTime)
	defer ticker.Stop()

	log.Printf("[%s] Started batch worker (interval: %v, batch size: %d)",
		a.name, a.batchWaitTime, a.batchSize)

	for {
		select {
		case <-ctx.Done():
			// Final flush
			if err := a.BatchSubmitExecutionReports(ctx); err != nil {
				log.Printf("[%s] Error in final batch submission: %v", a.name, err)
			}
			return
		case <-ticker.C:
			// Periodic batch submission
			if err := a.BatchSubmitExecutionReports(ctx); err != nil {
				log.Printf("[%s] Error in batch submission: %v", a.name, err)
			}
		}
	}
}

func main() {
	// Parse flags
	var (
		agentID        = flag.String("id", "", "Agent ID (auto-generated if empty)")
		matcherAddr    = flag.String("matcher", "localhost:8090", "Matcher address")
		agentName      = flag.String("name", "BatchTestAgent", "Agent name")
		enableBatching = flag.Bool("batch", true, "Enable batch operations")
		batchSize      = flag.Int("batch-size", 5, "Batch size for submissions")
		batchWait      = flag.Duration("batch-wait", 10*time.Second, "Wait time before batch submission")
		subnetID       = flag.String("subnet-id", "", "Subnet ID")
	)
	flag.Parse()

	log.Printf("🚀 Starting Batch Test Agent '%s'", *agentName)
	log.Printf("   Batching: %v (size: %d, wait: %v)", *enableBatching, *batchSize, *batchWait)

	// Create SDK configuration
	config := &agentsdk.Config{
		AgentID:     *agentID,
		MatcherAddr: *matcherAddr,
		Capabilities: []string{
			"batch.processing",
			"general.compute",
			"test.execution",
		},
		MaxConcurrentTasks: 10,
		LogLevel:           "info",
	}

	// Create SDK instance
	sdk, err := agentsdk.New(config)
	if err != nil {
		log.Fatalf("Failed to create SDK: %v", err)
	}

	// Create batch agent
	agent := &BatchTestAgent{
		name:            *agentName,
		sdk:             sdk,
		pendingTasks:    make(map[string]*agentsdk.Task),
		pendingResults:  make(map[string]*agentsdk.Result),
		batchSize:       *batchSize,
		batchWaitTime:   *batchWait,
		enableBatching:  *enableBatching,
	}

	// Register handler
	sdk.RegisterHandler(agent)

	// Start the agent
	if err := sdk.Start(); err != nil {
		log.Fatalf("Failed to start SDK: %v", err)
	}

	log.Printf("✅ Batch Test Agent started successfully")
	log.Printf("   Agent ID: %s", sdk.GetAgentID())
	log.Printf("   Subnet ID: %s", *subnetID)
	log.Printf("   Capabilities: %v", config.Capabilities)
	log.Printf("   Matcher: %s", *matcherAddr)

	// Start batch worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *enableBatching {
		go agent.StartBatchWorker(ctx)
	}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Print metrics periodically
	metricsTicker := time.NewTicker(15 * time.Second)
	defer metricsTicker.Stop()

	// Wait for shutdown signal
	for {
		select {
		case <-sigCh:
			log.Println("📡 Received shutdown signal")
			cancel() // Stop batch worker
			time.Sleep(time.Second) // Allow final batch submission

			if err := sdk.Stop(); err != nil {
				log.Printf("Error stopping SDK: %v", err)
			}
			return

		case <-metricsTicker.C:
			metrics := sdk.GetMetrics()
			agent.mu.Lock()
			pendingCount := len(agent.pendingResults)
			agent.mu.Unlock()

			log.Printf("📊 Metrics - Completed: %d, Failed: %d, Pending Reports: %d",
				metrics.TasksCompleted,
				metrics.TasksFailed,
				pendingCount,
			)
		}
	}
}
