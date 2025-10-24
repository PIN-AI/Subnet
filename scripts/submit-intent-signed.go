package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"github.com/PIN-AI/intent-protocol-contract-sdk/sdk/addressbook"
	sdkcrypto "github.com/PIN-AI/intent-protocol-contract-sdk/sdk/crypto"
)

func main() {
	// Command line flags
	var (
		rootLayerHTTP     = flag.String("rootlayer-http", envOr("ROOTLAYER_HTTP", "http://3.17.208.238:8081/api/v1"), "RootLayer HTTP API endpoint")
		rpcURL            = flag.String("rpc", envOr("RPC_URL", "https://sepolia.base.org"), "Blockchain RPC URL")
		privateKey        = flag.String("key", envOr("PRIVATE_KEY", ""), "Private key hex")
		network           = flag.String("network", envOr("PIN_NETWORK", "base_sepolia"), "Network name")
		intentManagerAddr = flag.String("intent-manager", envOr("INTENT_MANAGER_ADDR", envOr("PIN_BASE_SEPOLIA_INTENT_MANAGER", "")), "IntentManager contract address")

		subnetIDHex   = flag.String("subnet", envOr("SUBNET_ID", "0x0000000000000000000000000000000000000000000000000000000000000002"), "Subnet ID (0x64 hex)")
		intentType    = flag.String("type", envOr("INTENT_TYPE", "e2e-test"), "Intent type")
		paramsJSON    = flag.String("params", envOr("PARAMS_JSON", `{"task":"test"}`), "Intent params JSON")

		amountWei     = flag.String("amount", envOr("AMOUNT_WEI", "100000000000000"), "Amount in wei")
		deadlineHours = flag.Int64("deadline-hours", envInt64("DEADLINE_HOURS", 1), "Deadline offset in hours from now")
	)
	flag.Parse()

	if *privateKey == "" {
		log.Fatal("PRIVATE_KEY is required to generate signature")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Parse subnet ID
	subnetID, err := parseBytes32(*subnetIDHex)
	if err != nil {
		log.Fatalf("Invalid subnet ID: %v", err)
	}

	// Generate intent ID
	intentIDBytes := make([]byte, 32)
	if _, err := rand.Read(intentIDBytes); err != nil {
		log.Fatalf("Failed to generate intent ID: %v", err)
	}
	var intentID [32]byte
	copy(intentID[:], intentIDBytes)

	intentIDHex := "0x" + hex.EncodeToString(intentID[:])
	log.Printf("📋 Generated Intent ID: %s", intentIDHex)

	// Parse amount
	amount, ok := new(big.Int).SetString(*amountWei, 10)
	if !ok {
		log.Fatalf("Invalid amount: %s", *amountWei)
	}

	// Compute params hash = keccak256(intentRaw || metadata)
	// Must match what we send to RootLayer in the payload
	// Try EMPTY metadata first (like SDK example)
	metadata := ""
	intentRaw := []byte(*paramsJSON)
	metadataBytes := []byte(metadata)

	// Concatenate intentRaw + metadata before hashing
	combined := append(intentRaw, metadataBytes...)
	paramsHash := sdkcrypto.HashBytes(combined)

	deadline := big.NewInt(time.Now().Add(time.Duration(*deadlineHours) * time.Hour).Unix())

	log.Printf("📦 Intent Details:")
	log.Printf("   Subnet: %s", *subnetIDHex)
	log.Printf("   Type: %s", *intentType)
	log.Printf("   Params: %s", *paramsJSON)
	log.Printf("   ParamsHash: 0x%s", hex.EncodeToString(paramsHash[:]))
	log.Printf("   Amount: %s wei", amount.String())
	log.Printf("   Deadline: %s (%d)", time.Unix(deadline.Int64(), 0).Format(time.RFC3339), deadline.Int64())

	// Initialize SDK client for signing
	sdkConfig := sdk.Config{
		RPCURL:        *rpcURL,
		PrivateKeyHex: *privateKey,
		Network:       *network,
	}

	// Add IntentManager address if provided
	if *intentManagerAddr != "" {
		sdkConfig.Addresses = &addressbook.Addresses{
			IntentManager: common.HexToAddress(*intentManagerAddr),
		}
	}

	client, err := sdk.NewClient(ctx, sdkConfig)
	if err != nil {
		log.Fatalf("Failed to init SDK client: %v", err)
	}
	defer client.Close()

	requester := client.Signer.Address()
	log.Printf("   Requester: %s", requester.Hex())

	// Compute digest and sign
	input := sdkcrypto.SignedIntentInput{
		IntentID:     intentID,
		SubnetID:     subnetID,
		Requester:    requester,
		IntentType:   *intentType,
		ParamsHash:   paramsHash,
		Deadline:     deadline,
		PaymentToken: common.Address{}, // Zero address = native token
		Amount:       amount,
	}

	digest, err := client.Intent.ComputeDigest(input)
	if err != nil {
		log.Fatalf("Failed to compute digest: %v", err)
	}

	log.Printf("   Digest: 0x%s", hex.EncodeToString(digest[:]))
	log.Printf("   IntentManager: %s", client.Addresses.IntentManager.Hex())
	log.Printf("   Chain ID: %s", client.ChainID.String())

	signature, err := client.Intent.SignDigest(digest)
	if err != nil {
		log.Fatalf("Failed to sign digest: %v", err)
	}

	signatureHex := hex.EncodeToString(signature)
	log.Printf("   Signature (hex): %s (len=%d)", signatureHex, len(signature))

	// Verify signature locally by recovering signer
	if err := verifySignature(digest, signature, requester); err != nil {
		log.Printf("   ⚠️  Local signature verification failed: %v", err)
	} else {
		log.Printf("   ✅ Local signature verification passed")
	}

	// Encode signature as base64 for proto bytes field
	signatureB64 := base64.StdEncoding.EncodeToString(signature)
	log.Printf("   Signature (base64): %s", signatureB64)

	// Step 1: Submit to blockchain FIRST (dual submission requirement)
	log.Printf("\n⛓️  Submitting Intent to blockchain (IntentManager)...")
	submitParams := sdk.SubmitIntentParams{
		IntentID:     intentID,
		SubnetID:     subnetID,
		IntentType:   *intentType,
		ParamsHash:   paramsHash,
		Deadline:     deadline,
		PaymentToken: common.Address{}, // Zero address = native token
		Amount:       amount,
		Value:        amount, // For ETH payment
	}

	tx, err := client.Intent.SubmitIntent(ctx, submitParams)
	if err != nil {
		log.Fatalf("Failed to submit intent to blockchain: %v", err)
	}

	log.Printf("   ✅ Intent submitted to blockchain!")
	log.Printf("   Transaction hash: %s", tx.Hash().Hex())
	log.Printf("   Waiting for confirmation...")

	// Wait for transaction to be mined
	receipt, err := bind.WaitMined(ctx, client.Backend, tx)
	if err != nil {
		log.Fatalf("Failed to get transaction receipt: %v", err)
	}

	if receipt.Status == 0 {
		log.Fatalf("Transaction failed on-chain (status=0)")
	}

	log.Printf("   ✅ Transaction confirmed in block %d", receipt.BlockNumber.Uint64())

	// Step 2: Submit to RootLayer for distribution
	log.Printf("\n🚀 Submitting Intent to RootLayer (for distribution)...")
	if err := submitToRootLayer(ctx, *rootLayerHTTP, intentIDHex, *subnetIDHex, requester.Hex(), *intentType, *paramsJSON, metadata, deadline, amount, signatureB64); err != nil {
		log.Fatalf("Failed to submit to RootLayer: %v", err)
	}

	log.Printf("\n✅ Dual submission complete! Intent is now on-chain and distributed to RootLayer.")
	log.Printf("   Intent ID: %s", intentIDHex)
	log.Printf("   Blockchain TX: %s", tx.Hash().Hex())
}

// submitToRootLayer submits intent via RootLayer HTTP API with signature
func submitToRootLayer(ctx context.Context, endpoint, intentID, subnetID, requester, intentType, paramsJSON, metadata string, deadline, amount *big.Int, signature string) error {
	// Encode params as base64 (RootLayer expects base64)
	intentRawB64 := base64.StdEncoding.EncodeToString([]byte(paramsJSON))
	metadataB64 := base64.StdEncoding.EncodeToString([]byte(metadata))

	// Build request payload
	payload := map[string]interface{}{
		"intentId":    intentID,
		"subnetId":    subnetID,
		"requester":   requester,
		"settleChain": "base_sepolia",
		"intentType":  intentType,
		"params": map[string]interface{}{
			"intentRaw": intentRawB64,
			"metadata":  metadataB64,
		},
		"tipsToken":   "0x0000000000000000000000000000000000000000",
		"tips":        "100",
		"budgetToken": "0x0000000000000000000000000000000000000000",
		"budget":      amount.String(),
		"deadline":    deadline.Int64(),
		"signature":   signature, // Real EIP-191 signature
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	log.Printf("   Payload: %s", string(jsonData)[:200]+"...")

	// Submit via HTTP
	url := endpoint + "/intents/submit"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	log.Printf("   HTTP Status: %d", resp.StatusCode)
	log.Printf("   Response: %s", string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func parseBytes32(hexStr string) ([32]byte, error) {
	var result [32]byte
	trimmed := strings.TrimPrefix(strings.TrimSpace(hexStr), "0x")
	if len(trimmed) != 64 {
		return result, fmt.Errorf("expected 64 hex chars, got %d", len(trimmed))
	}
	data, err := hex.DecodeString(trimmed)
	if err != nil {
		return result, err
	}
	copy(result[:], data)
	return result, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		var out int64
		if _, err := fmt.Sscanf(v, "%d", &out); err == nil {
			return out
		}
	}
	return fallback
}

// verifySignature verifies that the signature correctly recovers to the expected signer
func verifySignature(digest [32]byte, signature []byte, expectedSigner common.Address) error {
	if len(signature) != 65 {
		return fmt.Errorf("invalid signature length: %d", len(signature))
	}

	// Create a copy of signature and adjust v value for recovery (go-ethereum expects 0/1)
	sig := make([]byte, 65)
	copy(sig, signature)
	if sig[64] >= 27 {
		sig[64] -= 27
	}

	// Apply EIP-191 prefix and recover public key
	msgHash := accounts.TextHash(digest[:])
	pubKey, err := crypto.SigToPub(msgHash, sig)
	if err != nil {
		return fmt.Errorf("failed to recover public key: %w", err)
	}

	// Derive address from public key
	recoveredAddr := crypto.PubkeyToAddress(*pubKey)

	if recoveredAddr != expectedSigner {
		return fmt.Errorf("recovered address %s does not match expected %s", recoveredAddr.Hex(), expectedSigner.Hex())
	}

	return nil
}
