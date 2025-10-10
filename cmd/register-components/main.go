package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
)

func main() {
	role := flag.String("role", "validator", "Role to register (validator|matcher)")
	rpcURL := flag.String("rpc", envOrDefault("CHAIN_RPC_URL", ""), "RPC URL for the blockchain node")
	privateKey := flag.String("private-key", envOrDefault("REGISTER_PRIVATE_KEY", ""), "Hex-encoded private key")
	network := flag.String("network", envOrDefault("PIN_NETWORK", ""), "SDK network hint (optional)")
	subnetAddr := flag.String("subnet", envOrDefault("SUBNET_CONTRACT_ADDRESS", ""), "Subnet contract address")
	dryRun := flag.Bool("dry-run", false, "If set, do not broadcast the transaction")
	flag.Parse()

	if *rpcURL == "" {
		log.Fatal("RPC URL is required (set CHAIN_RPC_URL or --rpc)")
	}
	if *privateKey == "" {
		log.Fatal("Private key is required (set REGISTER_PRIVATE_KEY or --private-key)")
	}
	if !common.IsHexAddress(*subnetAddr) {
		log.Fatalf("Invalid subnet contract address: %s", *subnetAddr)
	}

	cfg := sdk.Config{
		RPCURL:        *rpcURL,
		PrivateKeyHex: *privateKey,
		Network:       *network,
	}
	if *dryRun {
		cfg.Tx = &sdk.TxOptions{NoSend: boolPtr(true)}
	}

	ctx := context.Background()
	client, err := sdk.NewClient(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to create SDK client: %v", err)
	}
	defer client.Close()

	subnetService, err := client.SubnetServiceByAddress(common.HexToAddress(*subnetAddr))
	if err != nil {
		log.Fatalf("failed to bind subnet contract: %v", err)
	}

	params, err := loadParticipantParams(*role)
	if err != nil {
		log.Fatalf("failed to load participant parameters: %v", err)
	}

	switch strings.ToLower(*role) {
	case "validator":
		tx, err := subnetService.RegisterValidator(ctx, params)
		handleResult(tx, err)
	case "matcher":
		tx, err := subnetService.RegisterMatcher(ctx, params)
		handleResult(tx, err)
	default:
		log.Fatalf("unsupported role %s", *role)
	}
}

func loadParticipantParams(role string) (sdk.RegisterParticipantParams, error) {
	prefix := strings.ToUpper(role)
	domain := os.Getenv(prefix + "_DOMAIN")
	endpoint := os.Getenv(prefix + "_ENDPOINT")
	metadata := os.Getenv(prefix + "_METADATA_URI")
	valueRaw := os.Getenv(prefix + "_STAKE_AMOUNT")

	if strings.TrimSpace(domain) == "" {
		return sdk.RegisterParticipantParams{}, fmt.Errorf("%s_DOMAIN is required", prefix)
	}
	if strings.TrimSpace(endpoint) == "" {
		return sdk.RegisterParticipantParams{}, fmt.Errorf("%s_ENDPOINT is required", prefix)
	}

	var amount *big.Int
	if strings.TrimSpace(valueRaw) != "" {
		v, ok := new(big.Int).SetString(valueRaw, 10)
		if !ok {
			return sdk.RegisterParticipantParams{}, fmt.Errorf("invalid %s_STAKE_AMOUNT", prefix)
		}
		amount = v
	}

	return sdk.RegisterParticipantParams{
		Domain:      domain,
		Endpoint:    endpoint,
		MetadataURI: metadata,
		Value:       amount,
	}, nil
}

func handleResult(tx *types.Transaction, err error) {
	if err != nil {
		log.Fatalf("registration failed: %v", err)
	}
	if tx == nil {
		log.Println("registration simulated (dry-run)")
		return
	}
	log.Printf("registration submitted: %s", tx.Hash())
}

func envOrDefault(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func boolPtr(v bool) *bool {
	return &v
}
