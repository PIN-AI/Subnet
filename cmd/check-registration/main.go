package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"subnet/internal/blockchain"
)

func main() {
	role := flag.String("role", "validator", "Role to check (validator|agent|matcher)")
	rpcURL := flag.String("rpc", envOrDefault("CHAIN_RPC_URL", ""), "RPC URL for the blockchain node")
	subnetAddr := flag.String("subnet", envOrDefault("SUBNET_CONTRACT_ADDRESS", ""), "Subnet contract address")
	address := flag.String("address", "", "Address to check (0x...) ")
	flag.Parse()

	if *rpcURL == "" {
		log.Fatal("RPC URL is required (set CHAIN_RPC_URL or use --rpc)")
	}
	if !common.IsHexAddress(*subnetAddr) {
		log.Fatalf("Invalid subnet contract address: %s", *subnetAddr)
	}
	if !common.IsHexAddress(*address) {
		log.Fatalf("Invalid participant address: %s", *address)
	}

	verifierCfg := blockchain.VerifierConfig{
		CacheTTL:       time.Second,
		CacheSize:      4,
		EnableFallback: false,
	}

	verifier, err := blockchain.NewParticipantVerifier(*rpcURL, common.HexToAddress(*subnetAddr), verifierCfg, nil)
	if err != nil {
		log.Fatalf("failed to create verifier: %v", err)
	}
	defer verifier.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addr := strings.TrimSpace(*address)
	var (
		active    bool
		verifyErr error
	)

	switch strings.ToLower(*role) {
	case "validator":
		active, verifyErr = verifier.VerifyValidator(ctx, addr)
	case "agent":
		active, verifyErr = verifier.VerifyAgent(ctx, addr)
	case "matcher":
		active, verifyErr = verifier.VerifyMatcher(ctx, addr)
	default:
		log.Fatalf("unsupported role %s", *role)
	}

	if verifyErr != nil {
		log.Fatalf("verification failed: %v", verifyErr)
	}

	if active {
		fmt.Printf("%s is active for role %s\n", addr, strings.ToLower(*role))
	} else {
		fmt.Printf("%s is NOT active for role %s\n", addr, strings.ToLower(*role))
	}
}

func envOrDefault(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
