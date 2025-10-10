package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	agentsdk "subnet/sdk/go"
)

func main() {
	var (
		agentID      = flag.String("id", "", "Agent ID (auto-generated if empty)")
		agentName    = flag.String("name", "ExampleAgent", "Agent name")
		matcherAddr  = flag.String("matcher", "localhost:8090", "Matcher gRPC address")
		capabilities = flag.String("capabilities", "general.processing,data.transform", "Comma-separated capabilities")
		minBid       = flag.Uint64("min-bid", 50, "Minimum bid price")
		maxBid       = flag.Uint64("max-bid", 500, "Maximum bid price")
		maxTasks     = flag.Int("max-tasks", 5, "Maximum concurrent tasks")
	)
	flag.Parse()

	log.Println("=================================================")
	log.Printf("Starting Example Agent")
	log.Println("=================================================")

	// Parse capabilities
	capList := strings.Split(*capabilities, ",")
	for i, cap := range capList {
		capList[i] = strings.TrimSpace(cap)
	}

	// Create SDK configuration
	config := &agentsdk.Config{
		AgentID:            *agentID,
		Owner:              "0x1234567890abcdef", // Example owner address
		MatcherAddr:        *matcherAddr,
		Capabilities:       capList,
		MaxConcurrentTasks: *maxTasks,
		MinBidPrice:        *minBid,
		MaxBidPrice:        *maxBid,
		LogLevel:           "info",
	}

	// Create SDK instance
	sdk, err := agentsdk.New(config)
	if err != nil {
		log.Fatalf("Failed to create SDK: %v", err)
	}

	// Register a simple handler
	handler := &SimpleHandler{name: *agentName}
	sdk.RegisterHandler(handler)

	// Note: BiddingStrategy is not part of the new SDK API
	// Bidding logic should be handled within the handler's Execute method

	// Start the agent
	if err := sdk.Start(); err != nil {
		log.Fatalf("Failed to start SDK: %v", err)
	}

	log.Printf("Agent '%s' started", *agentName)
	log.Printf("Agent ID: %s", config.AgentID)
	log.Printf("Capabilities: %v", capList)
	log.Printf("Matcher: %s", *matcherAddr)
	log.Printf("Price range: %d-%d", *minBid, *maxBid)
	log.Println("Press Ctrl+C to stop.")
	log.Println("=================================================")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("\nShutting down agent...")
	if err := sdk.Stop(); err != nil {
		log.Printf("Error stopping SDK: %v", err)
	}
	log.Println("Agent stopped")
}

// SimpleHandler implements the Handler interface
type SimpleHandler struct {
	name string
}

func (h *SimpleHandler) Execute(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	log.Printf("[%s] Executing task %s", h.name, task.ID)

	// Simulate some work
	// In a real implementation, you would process the task here

	return &agentsdk.Result{
		Data:    []byte(`{"status": "completed"}`),
		Success: true,
		Metadata: map[string]string{
			"processor": h.name,
		},
	}, nil
}

// SimpleBiddingStrategy implements a basic bidding strategy
type SimpleBiddingStrategy struct {
	basePrice uint64
}

func (s *SimpleBiddingStrategy) ShouldBid(intent *agentsdk.Intent) bool {
	// Bid on all intents for demo
	return true
}

func (s *SimpleBiddingStrategy) CalculateBid(intent *agentsdk.Intent) *agentsdk.Bid {
	return &agentsdk.Bid{
		Price:    s.basePrice,
		Currency: "PIN",
	}
}