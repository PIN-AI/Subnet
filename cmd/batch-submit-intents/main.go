package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
)

// BatchIntentSubmitter submits multiple intents in parallel
type BatchIntentSubmitter struct {
	client     *sdk.Client
	privateKey *ecdsa.PrivateKey
	address    common.Address
}

// IntentSubmissionResult tracks the result of each intent submission
type IntentSubmissionResult struct {
	Index     int
	IntentID  string
	TxHash    string
	Success   bool
	Error     error
	Duration  time.Duration
}

// NewBatchIntentSubmitter creates a new batch submitter
func NewBatchIntentSubmitter(rpcURL, privateKeyHex string, network sdk.Network) (*BatchIntentSubmitter, error) {
	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	// Derive address
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Create SDK client
	client, err := sdk.NewClient(rpcURL, privateKeyHex, network)
	if err != nil {
		return nil, fmt.Errorf("failed to create SDK client: %w", err)
	}

	return &BatchIntentSubmitter{
		client:     client,
		privateKey: privateKey,
		address:    address,
	}, nil
}

// SubmitBatch submits multiple intents in parallel
func (b *BatchIntentSubmitter) SubmitBatch(ctx context.Context, count int, subnetID string, params *sdk.IntentParams) ([]*IntentSubmissionResult, error) {
	log.Printf("📦 Submitting %d intents in parallel...", count)

	results := make([]*IntentSubmissionResult, count)
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit concurrent submissions

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Rate limiting
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := b.submitSingleIntent(ctx, index, subnetID, params)
			results[index] = result
		}(i)
	}

	wg.Wait()
	return results, nil
}

// submitSingleIntent submits a single intent
func (b *BatchIntentSubmitter) submitSingleIntent(ctx context.Context, index int, subnetID string, baseParams *sdk.IntentParams) *IntentSubmissionResult {
	start := time.Now()
	result := &IntentSubmissionResult{Index: index}

	// Generate unique intent ID
	intentID := common.BytesToHash(crypto.Keccak256([]byte(fmt.Sprintf("batch-intent-%d-%d", index, time.Now().UnixNano()))))
	result.IntentID = intentID.Hex()

	// Customize params for this intent
	params := *baseParams
	params.IntentID = intentID.Hex()

	// Add index to task data
	var taskData map[string]interface{}
	if err := json.Unmarshal([]byte(params.ParamsJSON), &taskData); err != nil {
		taskData = make(map[string]interface{})
	}
	taskData["batch_index"] = index
	taskData["timestamp"] = time.Now().Unix()

	paramsJSON, _ := json.Marshal(taskData)
	params.ParamsJSON = string(paramsJSON)

	// Calculate params hash
	params.ParamsHash = common.BytesToHash(crypto.Keccak256(paramsJSON)).Hex()

	// Convert subnetID to common.Hash
	subnetHash := common.HexToHash(subnetID)

	// Submit intent
	log.Printf("  [%d] Submitting intent %s...", index, result.IntentID[:10])

	tx, err := b.client.IntentManager.SubmitIntent(ctx, sdk.SubmitIntentParams{
		IntentID:     intentID,
		SubnetID:     subnetHash,
		IntentType:   params.IntentType,
		ParamsHash:   common.HexToHash(params.ParamsHash),
		Deadline:     params.Deadline,
		PaymentToken: common.HexToAddress(params.PaymentToken),
		Amount:       params.Amount,
		Value:        params.Amount, // ETH payment
	})

	result.Duration = time.Since(start)

	if err != nil {
		result.Success = false
		result.Error = err
		log.Printf("  [%d] ❌ Failed: %v", index, err)
		return result
	}

	result.Success = true
	result.TxHash = tx.Hash().Hex()
	log.Printf("  [%d] ✅ Submitted (tx: %s, duration: %v)", index, result.TxHash[:10], result.Duration)

	return result
}

// PrintSummary prints the batch submission summary
func (b *BatchIntentSubmitter) PrintSummary(results []*IntentSubmissionResult) {
	fmt.Println("\n" + "=" * 60)
	fmt.Println("Batch Intent Submission Summary")
	fmt.Println("=" * 60)

	successCount := 0
	failedCount := 0
	totalDuration := time.Duration(0)

	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failedCount++
		}
		totalDuration += r.Duration
	}

	avgDuration := totalDuration / time.Duration(len(results))

	fmt.Printf("\nTotal Intents:   %d\n", len(results))
	fmt.Printf("✅ Successful:    %d (%.1f%%)\n", successCount, float64(successCount)*100/float64(len(results)))
	fmt.Printf("❌ Failed:        %d (%.1f%%)\n", failedCount, float64(failedCount)*100/float64(len(results)))
	fmt.Printf("\n⏱️  Total Time:    %v\n", totalDuration)
	fmt.Printf("⏱️  Average Time:  %v\n", avgDuration)
	fmt.Printf("⏱️  Rate:          %.2f intents/sec\n", float64(len(results))/totalDuration.Seconds())

	// Print successful intent IDs
	if successCount > 0 {
		fmt.Println("\n✅ Successful Intent IDs:")
		for _, r := range results {
			if r.Success {
				fmt.Printf("  [%d] %s (tx: %s)\n", r.Index, r.IntentID, r.TxHash[:10])
			}
		}
	}

	// Print failed intents
	if failedCount > 0 {
		fmt.Println("\n❌ Failed Intents:")
		for _, r := range results {
			if !r.Success {
				fmt.Printf("  [%d] Error: %v\n", r.Index, r.Error)
			}
		}
	}

	fmt.Println("\n" + "=" * 60)
}

func main() {
	// Parse flags
	var (
		rpcURL         = flag.String("rpc", os.Getenv("RPC_URL"), "Ethereum RPC URL")
		privateKey     = flag.String("key", os.Getenv("PRIVATE_KEY"), "Private key (hex, without 0x)")
		network        = flag.String("network", "base_sepolia", "Network name")
		subnetID       = flag.String("subnet-id", os.Getenv("SUBNET_ID"), "Subnet ID")
		count          = flag.Int("count", 5, "Number of intents to submit")
		intentType     = flag.String("type", "batch-test-intent", "Intent type")
		paramsJSON     = flag.String("params", `{"task":"batch test"}`, "Params JSON")
		deadlineOffset = flag.Duration("deadline", 24*time.Hour, "Deadline offset from now")
		amountWei      = flag.String("amount", "100000000000000", "Amount in wei")
	)
	flag.Parse()

	// Validation
	if *rpcURL == "" {
		log.Fatal("RPC URL is required (--rpc or RPC_URL env var)")
	}
	if *privateKey == "" {
		log.Fatal("Private key is required (--key or PRIVATE_KEY env var)")
	}
	if *subnetID == "" {
		log.Fatal("Subnet ID is required (--subnet-id or SUBNET_ID env var)")
	}

	// Parse network
	var netEnum sdk.Network
	switch *network {
	case "base_sepolia":
		netEnum = sdk.NetworkBaseSepolia
	default:
		log.Fatalf("Unknown network: %s", *network)
	}

	log.Printf("🚀 Batch Intent Submission Tool")
	log.Printf("   Network:   %s", *network)
	log.Printf("   RPC:       %s", *rpcURL)
	log.Printf("   Subnet:    %s", *subnetID)
	log.Printf("   Count:     %d", *count)
	log.Printf("   Type:      %s", *intentType)

	// Create submitter
	submitter, err := NewBatchIntentSubmitter(*rpcURL, *privateKey, netEnum)
	if err != nil {
		log.Fatalf("Failed to create submitter: %v", err)
	}

	log.Printf("   Submitter: %s", submitter.address.Hex())

	// Parse amount
	amount, ok := new(big.Int).SetString(*amountWei, 10)
	if !ok {
		log.Fatalf("Invalid amount: %s", *amountWei)
	}

	// Prepare base params
	baseParams := &sdk.IntentParams{
		IntentType:   *intentType,
		ParamsJSON:   *paramsJSON,
		Deadline:     time.Now().Add(*deadlineOffset),
		PaymentToken: "0x0000000000000000000000000000000000000000", // ETH
		Amount:       amount,
	}

	// Submit batch
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	results, err := submitter.SubmitBatch(ctx, *count, *subnetID, baseParams)
	if err != nil {
		log.Fatalf("Batch submission failed: %v", err)
	}

	// Print summary
	submitter.PrintSummary(results)

	// Exit with appropriate code
	failedCount := 0
	for _, r := range results {
		if !r.Success {
			failedCount++
		}
	}

	if failedCount > 0 {
		os.Exit(1)
	}
}
