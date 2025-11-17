package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "subnet/proto/subnet"
)

func main() {
	matcherAddr := flag.String("matcher", "localhost:8090", "Matcher gRPC address")
	validatorAddr := flag.String("validator", "localhost:9090", "Validator gRPC address")
	agentID := flag.String("agent-id", "test-agent-validator", "Agent ID")
	agentChainAddress := flag.String("chain-address", "0xfc5A111b714547fc2D1D796EAAbb68264ed4A132", "Agent blockchain address")
	subnetID := flag.String("subnet-id", "0x0000000000000000000000000000000000000000000000000000000000000003", "Subnet ID to subscribe to")
	flag.Parse()

	log.Printf("Starting validator test agent: %s", *agentID)
	log.Printf("Chain address: %s", *agentChainAddress)
	log.Printf("Connecting to matcher at: %s", *matcherAddr)
	log.Printf("Connecting to validator at: %s", *validatorAddr)

	// Connect to matcher
	matcherConn, err := grpc.Dial(*matcherAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to connect to matcher: %v", err)
	}
	defer matcherConn.Close()

	matcherClient := pb.NewMatcherServiceClient(matcherConn)
	log.Printf("✓ Connected to matcher successfully")

	// Connect to validator
	validatorConn, err := grpc.Dial(*validatorAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to connect to validator: %v", err)
	}
	defer validatorConn.Close()

	validatorClient := pb.NewValidatorServiceClient(validatorConn)
	log.Printf("✓ Connected to validator successfully")

	ctx := context.Background()

	// Subscribe to tasks (assignments) first
	taskStream, err := matcherClient.StreamTasks(ctx, &pb.StreamTasksRequest{
		AgentId: *agentID,
	})
	if err != nil {
		log.Fatalf("Failed to stream tasks: %v", err)
	}
	log.Printf("✓ Subscribed to task stream")

	// Start task listener goroutine
	go func() {
		for {
			task, err := taskStream.Recv()
			if err != nil {
				log.Printf("❌ Task stream error: %v", err)
				return
			}

			log.Printf("📋 Received assignment: %s", task.TaskId)
			log.Printf("   Intent: %s", task.IntentId)
			log.Printf("   Type: %s", task.IntentType)
			log.Printf("   Intent Data: %s", string(task.IntentData))

			// Mock execution
			log.Printf("⚙️  Executing task %s...", task.TaskId)
			time.Sleep(2 * time.Second) // Simulate work

			// Create execution result
			resultData := fmt.Sprintf(`{"status":"success","task_id":"%s","result":"Task completed successfully","timestamp":%d}`,
				task.TaskId, time.Now().Unix())

			// Submit ExecutionReport to Validator
			// IMPORTANT: AgentId must be Ethereum address format for RootLayer validation
			report := &pb.ExecutionReport{
				ReportId:     fmt.Sprintf("report-%s-%d", *agentID, time.Now().Unix()),
				AssignmentId: task.TaskId,
				IntentId:     task.IntentId,
				AgentId:      *agentChainAddress, // Use chain address for RootLayer compatibility
				Status:       pb.ExecutionReport_SUCCESS,
				ResultData:   []byte(resultData),
				Timestamp:    time.Now().Unix(),
				Evidence:     nil,      // In production would include verification evidence
				Signature:    []byte{}, // In production would sign the report
			}

			log.Printf("📤 Submitting ExecutionReport to Validator...")
			log.Printf("   Report ID: %s", report.ReportId)
			log.Printf("   Result: %s", resultData)

			receipt, err := validatorClient.SubmitExecutionReport(ctx, report)
			if err != nil {
				log.Printf("❌ Failed to submit execution report: %v", err)
				continue
			}

			log.Printf("✅ ExecutionReport submitted to Validator!")
			log.Printf("   Intent ID: %s", receipt.IntentId)
			log.Printf("   Report ID: %s", receipt.ReportId)
			log.Printf("   Status: %s", receipt.Status)
			log.Printf("   Phase: %s", receipt.Phase)
			if receipt.ScoreHint > 0 {
				log.Printf("   Score Hint: %d", receipt.ScoreHint)
			}
		}
	}()

	// Subscribe to intents
	intentStream, err := matcherClient.StreamIntents(ctx, &pb.StreamIntentsRequest{
		SubnetId: *subnetID,
	})
	if err != nil {
		log.Fatalf("Failed to stream intents: %v", err)
	}

	log.Printf("✓ Subscribed to intents stream")
	log.Println("Waiting for intents...")

	// Listen for intents and submit bids
	for {
		intentUpdate, err := intentStream.Recv()
		if err != nil {
			log.Printf("Intent stream error: %v", err)
			break
		}

		intentID := intentUpdate.IntentId
		if intentID == "" {
			continue
		}

		// Only bid on NEW intents, not MATCHED or WINDOW_CLOSED
		if intentUpdate.UpdateType != "NEW_INTENT" && intentUpdate.UpdateType != "EXISTING_INTENT" {
			log.Printf("📥 Received intent update: %s (type: %s) - skipping bid", intentID, intentUpdate.UpdateType)
			continue
		}

		log.Printf("📥 Received intent update: %s", intentID)
		log.Printf("   Update type: %s", intentUpdate.UpdateType)

		// Generate bid ID as 32-byte hex (hash of intent + agent + timestamp)
		bidData := fmt.Sprintf("%s:%s:%d", intentID, *agentID, time.Now().UnixNano())
		bidHash := sha256.Sum256([]byte(bidData))
		bidID := "0x" + hex.EncodeToString(bidHash[:])

		// Submit a bid
		// IMPORTANT: For blockchain compatibility, agents must provide their Ethereum address
		// in the "chain_address" metadata field for EIP-191 signature generation
		bid := &pb.Bid{
			BidId:       bidID,
			IntentId:    intentID,
			AgentId:     *agentID,
			Price:       100, // uint64
			Token:       "PIN",
			SubmittedAt: time.Now().Unix(),
			Metadata: map[string]string{
				"chain_address": *agentChainAddress, // Agent's blockchain address
			},
		}

		log.Printf("💰 Submitting bid: %s (price: %d %s)", bid.BidId, bid.Price, bid.Token)

		bidResp, err := matcherClient.SubmitBid(ctx, &pb.SubmitBidRequest{Bid: bid})
		if err != nil {
			log.Printf("❌ Failed to submit bid: %v", err)
			continue
		}

		ack := bidResp.Ack
		if ack != nil && ack.Accepted {
			log.Printf("✓ Bid accepted: %s", bid.BidId)
		} else if ack != nil {
			log.Printf("✗ Bid rejected: %s - %s", bid.BidId, ack.Reason)
		}
	}

	log.Println("Intent stream closed, exiting...")
}
