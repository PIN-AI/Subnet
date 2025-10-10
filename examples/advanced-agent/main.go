package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	agentsdk "subnet/sdk/go"
)

// AdvancedAgent demonstrates all SDK features
type AdvancedAgent struct {
	name            string
	processedTasks  int64
	totalEarnings   uint64
	lastHealthCheck time.Time
}

// Execute implements the Handler interface
func (a *AdvancedAgent) Execute(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	log.Printf("[%s] Executing task %s (type: %s)", a.name, task.ID, task.Type)

	// Parse task data
	var taskData map[string]interface{}
	if err := json.Unmarshal(task.Data, &taskData); err != nil {
		return &agentsdk.Result{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse task data: %v", err),
		}, nil
	}

	// Simulate processing based on task type
	processingTime := 1 * time.Second
	if task.Type == "complex.compute" {
		processingTime = 3 * time.Second
	} else if task.Type == "ai.inference" {
		processingTime = 5 * time.Second
	}

	select {
	case <-time.After(processingTime):
		// Processing complete
		atomic.AddInt64(&a.processedTasks, 1)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Generate result
	result := map[string]interface{}{
		"task_id":   task.ID,
		"processed": true,
		"output": map[string]interface{}{
			"computation": "completed",
			"metrics": map[string]interface{}{
				"processing_time": processingTime.Seconds(),
				"complexity":      "medium",
			},
		},
		"timestamp": time.Now().Unix(),
	}

	resultData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return &agentsdk.Result{
		Data:    resultData,
		Success: true,
		Metadata: map[string]string{
			"processor":       a.name,
			"processing_time": processingTime.String(),
		},
	}, nil
}

// CustomBiddingStrategy implements intelligent bidding
type CustomBiddingStrategy struct {
	basePrice      uint64
	surgeMultiplier float64
	currentLoad    int32
	maxLoad        int32
}

// ShouldBid decides whether to bid on an intent
func (s *CustomBiddingStrategy) ShouldBid(intent *agentsdk.Intent) bool {
	// Don't bid if at max capacity
	load := atomic.LoadInt32(&s.currentLoad)
	if load >= s.maxLoad {
		log.Printf("At max capacity (%d/%d), skipping bid", load, s.maxLoad)
		return false
	}

	// Bid on specific types we support
	supportedTypes := map[string]bool{
		"general.processing": true,
		"complex.compute":    true,
		"ai.inference":       true,
		"data.transform":     true,
	}

	return supportedTypes[intent.Type]
}

// CalculateBid calculates dynamic pricing
func (s *CustomBiddingStrategy) CalculateBid(intent *agentsdk.Intent) *agentsdk.Bid {
	price := s.basePrice

	// Adjust price based on type
	switch intent.Type {
	case "ai.inference":
		price = uint64(float64(price) * 2.5) // AI tasks cost more
	case "complex.compute":
		price = uint64(float64(price) * 1.8)
	}

	// Apply surge pricing based on load
	load := atomic.LoadInt32(&s.currentLoad)
	loadRatio := float64(load) / float64(s.maxLoad)
	if loadRatio > 0.7 {
		price = uint64(float64(price) * s.surgeMultiplier)
		log.Printf("Applying surge pricing (load: %.1f%%)", loadRatio*100)
	}

	return &agentsdk.Bid{
		Price:    price,
		Currency: "PIN",
	}
}

// AgentCallbacks implements lifecycle callbacks
type AgentCallbacks struct {
	agent           *AdvancedAgent
	biddingStrategy *CustomBiddingStrategy
	startTime       time.Time
}

func (c *AgentCallbacks) OnStart() error {
	c.startTime = time.Now()
	log.Printf("=== Agent '%s' starting up ===", c.agent.name)
	log.Println("Performing health checks...")

	// Simulate startup checks
	time.Sleep(500 * time.Millisecond)

	c.agent.lastHealthCheck = time.Now()
	log.Println("Health checks passed")
	return nil
}

func (c *AgentCallbacks) OnStop() error {
	uptime := time.Since(c.startTime)
	log.Printf("=== Agent '%s' shutting down ===", c.agent.name)
	log.Printf("Total uptime: %v", uptime)
	log.Printf("Tasks processed: %d", atomic.LoadInt64(&c.agent.processedTasks))
	log.Printf("Total earnings: %d PIN", c.agent.totalEarnings)

	// Cleanup operations
	log.Println("Saving state...")
	time.Sleep(200 * time.Millisecond)

	return nil
}

func (c *AgentCallbacks) OnTaskAccepted(task *agentsdk.Task) {
	atomic.AddInt32(&c.biddingStrategy.currentLoad, 1)
	log.Printf("Task %s accepted (load: %d/%d)",
		task.ID,
		atomic.LoadInt32(&c.biddingStrategy.currentLoad),
		c.biddingStrategy.maxLoad)
}

func (c *AgentCallbacks) OnTaskRejected(task *agentsdk.Task, reason string) {
	log.Printf("Task %s rejected: %s", task.ID, reason)
}

func (c *AgentCallbacks) OnTaskCompleted(task *agentsdk.Task, result *agentsdk.Result, err error) {
	atomic.AddInt32(&c.biddingStrategy.currentLoad, -1)

	if err != nil {
		log.Printf("Task %s failed: %v", task.ID, err)
	} else if result.Success {
		log.Printf("Task %s completed successfully (load: %d/%d)",
			task.ID,
			atomic.LoadInt32(&c.biddingStrategy.currentLoad),
			c.biddingStrategy.maxLoad)
	} else {
		log.Printf("Task %s completed with error: %s", task.ID, result.Error)
	}
}

func (c *AgentCallbacks) OnBidSubmitted(intent *agentsdk.Intent, bid *agentsdk.Bid) {
	log.Printf("Bid submitted for intent %s: %d %s", intent.ID, bid.Price, bid.Currency)
}

func (c *AgentCallbacks) OnBidWon(intentID string) {
	log.Printf("🎉 Won bid for intent %s", intentID)
	// In real implementation, would track earnings
	c.agent.totalEarnings += 100 // Simplified
}

func (c *AgentCallbacks) OnBidLost(intentID string) {
	log.Printf("Lost bid for intent %s", intentID)
}

func (c *AgentCallbacks) OnError(err error) {
	log.Printf("⚠️ Error occurred: %v", err)
}

func main() {
	// Parse flags
	var (
		agentID       = flag.String("id", "", "Agent ID (auto-generated if empty)")
		agentName     = flag.String("name", "AdvancedAgent", "Agent name")
		matcherAddr   = flag.String("matcher", "localhost:8090", "Matcher address")
		rootLayerRPC  = flag.String("rootlayer", "", "RootLayer RPC endpoint")
		owner         = flag.String("owner", "0x1234...abcd", "Agent owner address")
		stakeAmount   = flag.Uint64("stake", 10000, "Stake amount in PIN")
		useTLS        = flag.Bool("tls", false, "Enable TLS")
		certFile      = flag.String("cert", "", "TLS certificate file")
		keyFile       = flag.String("key", "", "TLS key file")
		dataDir       = flag.String("data-dir", "./agent-data", "Data directory")
	)
	flag.Parse()

	// Create SDK configuration
	config := &agentsdk.Config{
		AgentID:     *agentID,
		Owner:       *owner,
		MatcherAddr: *matcherAddr,
		Capabilities: []string{
			"general.processing",
			"complex.compute",
			"ai.inference",
			"data.transform",
		},
		MaxConcurrentTasks: 5,
		MinBidPrice:        50,
		MaxBidPrice:        500,
		StakeAmount:        *stakeAmount,
		TaskTimeout:        30 * time.Second,
		BidTimeout:         5 * time.Second,
		DataDir:            *dataDir,
		LogLevel:           "info",
		UseTLS:             *useTLS,
		CertFile:           *certFile,
		KeyFile:            *keyFile,
	}

	// Add validator addresses if available
	if envValidators := os.Getenv("VALIDATOR_ADDRS"); envValidators != "" {
		// Parse comma-separated addresses
		// config.ValidatorAddrs = strings.Split(envValidators, ",")
	}

	// Create SDK instance
	sdk, err := agentsdk.New(config)
	if err != nil {
		log.Fatalf("Failed to create SDK: %v", err)
	}

	// Create agent handler
	agent := &AdvancedAgent{
		name: *agentName,
	}

	// Create bidding strategy
	biddingStrategy := &CustomBiddingStrategy{
		basePrice:       100,
		surgeMultiplier: 1.5,
		maxLoad:         int32(config.MaxConcurrentTasks),
	}

	// Note: Callbacks are not part of the new SDK API
	// Event handling should be integrated within the handler

	// Register components
	sdk.RegisterHandler(agent)
	// Note: BiddingStrategy and Callbacks are not part of the new SDK API
	// Bidding logic should be handled within the handler's Execute method

	// Start the agent
	if err := sdk.Start(); err != nil {
		log.Fatalf("Failed to start SDK: %v", err)
	}

	log.Println(strings.Repeat("=", 50))
	log.Printf("Advanced Agent '%s' started", *agentName)
	log.Printf("Agent ID: %s", config.AgentID)
	log.Printf("Owner: %s", config.Owner)
	log.Printf("Capabilities: %v", config.Capabilities)
	log.Printf("Matcher: %s", *matcherAddr)
	if *rootLayerRPC != "" {
		log.Printf("RootLayer: %s", *rootLayerRPC)
	}
	log.Printf("Stake: %d PIN", config.StakeAmount)
	log.Printf("Price range: %d-%d PIN", config.MinBidPrice, config.MaxBidPrice)
	log.Printf("Max concurrent tasks: %d", config.MaxConcurrentTasks)
	log.Printf("TLS: %v", config.UseTLS)
	log.Println(strings.Repeat("=", 50))

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Periodic reporting
	reportTicker := time.NewTicker(60 * time.Second)
	defer reportTicker.Stop()

	// Health check ticker
	healthTicker := time.NewTicker(30 * time.Second)
	defer healthTicker.Stop()

	// Main loop
	for {
		select {
		case <-sigCh:
			log.Println("\n" + strings.Repeat("=", 50))
			log.Println("Received shutdown signal")
			if err := sdk.Stop(); err != nil {
				log.Printf("Error stopping SDK: %v", err)
			}
			return

		case <-reportTicker.C:
			metrics := sdk.GetMetrics()
			// Note: GetStakeStatus is not available in the new SDK
			stakeRemaining, stakeTotal := uint64(1000000), uint64(1000000) // Mock values

			log.Println("\n--- Performance Report ---")
			log.Printf("Tasks - Completed: %d, Failed: %d, Current: %d",
				metrics.TasksCompleted,
				metrics.TasksFailed,
				metrics.CurrentTasks)
			log.Printf("Bids - Total: %d, Won: %d (%.1f%% win rate)",
				metrics.TotalBids,
				metrics.SuccessfulBids,
				float64(metrics.SuccessfulBids)/float64(metrics.TotalBids)*100)
			log.Printf("Average execution time: %v", metrics.AverageExecTime)
			log.Printf("Total earnings: %d PIN", metrics.TotalEarnings)
			log.Printf("Stake status: %d/%d PIN", stakeRemaining, stakeTotal)
			log.Printf("Current load: %d/%d",
				atomic.LoadInt32(&biddingStrategy.currentLoad),
				biddingStrategy.maxLoad)
			log.Println("-------------------------")

		case <-healthTicker.C:
			// Perform health checks
			agent.lastHealthCheck = time.Now()
			// Note: GetAgentInfo is not available in the new SDK
			info := struct{ ID string; Capabilities []string; Status string }{
				ID:           *agentID,
				Capabilities: config.Capabilities,
				Status:       "active",
			}
			if info.Status != "active" {
				log.Printf("⚠️ Agent status: %s", info.Status)
			}
		}
	}
}