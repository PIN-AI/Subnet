package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentsdk "github.com/PIN-AI/subnet-sdk/go"
)

// SimpleAgent is a basic agent implementation
type SimpleAgent struct {
	name string
}

// Execute implements the Handler interface
func (a *SimpleAgent) Execute(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	log.Printf("[%s] Executing task %s of type %s", a.name, task.ID, task.Type)

	// Parse task data
	var taskData map[string]interface{}
	if err := json.Unmarshal(task.Data, &taskData); err != nil {
		// If not JSON, treat as string
		taskData = map[string]interface{}{
			"raw": string(task.Data),
		}
	}

	log.Printf("[%s] Task data: %v", a.name, taskData)

	// Simulate some processing
	select {
	case <-time.After(1 * time.Second):
		// Processing complete
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Generate result
	result := map[string]interface{}{
		"agent":      a.name,
		"task_id":    task.ID,
		"task_type":  task.Type,
		"processed":  true,
		"timestamp":  time.Now().Unix(),
		"input_data": taskData,
		"output":     fmt.Sprintf("Processed by %s", a.name),
	}

	resultData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	log.Printf("[%s] Task %s completed successfully", a.name, task.ID)

	return &agentsdk.Result{
		Data:    resultData,
		Success: true,
		Metadata: map[string]string{
			"processor": a.name,
			"duration":  "1s",
		},
	}, nil
}

// ShouldBid implements optional bidding logic
func (a *SimpleAgent) ShouldBid(intent *agentsdk.Intent) bool {
	// Bid on all intents for demo
	return true
}

// CalculateBid implements optional bid calculation
func (a *SimpleAgent) CalculateBid(intent *agentsdk.Intent) *agentsdk.Bid {
	// Simple fixed bidding
	return &agentsdk.Bid{
		Price:    100,
		Currency: "PIN",
	}
}

func main() {
	// Parse flags
	var (
		agentID     = flag.String("id", "", "Agent ID (auto-generated if empty)")
		matcherAddr = flag.String("matcher", "localhost:8090", "Matcher address")
		agentName   = flag.String("name", "SimpleAgent", "Agent name")
	)
	flag.Parse()

	// Create SDK configuration
	config := &agentsdk.Config{
		AgentID:     *agentID,
		MatcherAddr: *matcherAddr,
		Capabilities: []string{
			"general.processing",
			"data.transform",
			"simple.compute",
		},
		MaxConcurrentTasks: 3,
		LogLevel:           "info",
	}

	// Create SDK instance
	sdk, err := agentsdk.New(config)
	if err != nil {
		log.Fatalf("Failed to create SDK: %v", err)
	}

	// Create and register handler
	handler := &SimpleAgent{
		name: *agentName,
	}
	sdk.RegisterHandler(handler)

	// Start the agent
	if err := sdk.Start(); err != nil {
		log.Fatalf("Failed to start SDK: %v", err)
	}

	log.Printf("Simple Agent '%s' started successfully", *agentName)
	log.Printf("Agent ID: %s", config.AgentID)
	log.Printf("Capabilities: %v", config.Capabilities)
	log.Printf("Connected to matcher at: %s", *matcherAddr)

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Print metrics periodically
	metricsTicker := time.NewTicker(30 * time.Second)
	defer metricsTicker.Stop()

	// Wait for shutdown signal
	for {
		select {
		case <-sigCh:
			log.Println("Received shutdown signal")
			if err := sdk.Stop(); err != nil {
				log.Printf("Error stopping SDK: %v", err)
			}
			return

		case <-metricsTicker.C:
			metrics := sdk.GetMetrics()
			log.Printf("Metrics - Completed: %d, Failed: %d, Current: %d, Avg Time: %v",
				metrics.TasksCompleted,
				metrics.TasksFailed,
				metrics.CurrentTasks,
				metrics.AverageExecTime,
			)
		}
	}
}