package main

import (
	"context"
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
	flag.Parse()

	log.Printf("Starting validator test agent: %s", *agentID)
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
			report := &pb.ExecutionReport{
				ReportId:     fmt.Sprintf("report-%s-%d", *agentID, time.Now().Unix()),
				AssignmentId: task.TaskId,
				IntentId:     task.IntentId,
				AgentId:      *agentID,
				Status:       pb.ExecutionReport_SUCCESS,
				ResultData:   []byte(resultData),
				Timestamp:    time.Now().Unix(),
				Evidence:     nil, // In production would include verification evidence
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
		SubnetId: "0x1111111111111111111111111111111111111111111111111111111111111111",
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

		// Submit a bid
		bid := &pb.Bid{
			BidId:       fmt.Sprintf("bid-%s-%d", *agentID, time.Now().Unix()),
			IntentId:    intentID,
			AgentId:     *agentID,
			Price:       100,  // uint64
			Token:       "PIN",
			SubmittedAt: time.Now().Unix(),
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
