package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"subnet/internal/blockchain"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/registry"
	"subnet/internal/types"
	"subnet/internal/validator"
	pb "subnet/proto/subnet"
)

func main() {
	var (
		validatorID      = flag.String("id", "", "Validator ID")
		privateKey       = flag.String("key", "", "Private key hex (without 0x)")
		grpcPort         = flag.Int("grpc", 9090, "gRPC server port")
		natsURL          = flag.String("nats", "nats://127.0.0.1:4222", "NATS server URL")
		storagePath      = flag.String("storage", "./data", "Storage path for LevelDB")
		metricsPort      = flag.Int("metrics", 9095, "Metrics server port")
		registryGRPC     = flag.String("registry-grpc", ":8091", "Registry gRPC listen address (empty to disable)")
		registryHTTP     = flag.String("registry-http", ":8092", "Registry HTTP listen address (empty to disable)")
		registryEndpoint = flag.String("registry-endpoint", "", "Validator endpoint advertised via registry")

		// Validator set configuration
		validatorCount = flag.Int("validators", 4, "Total number of validators")
		thresholdNum   = flag.Int("threshold-num", 3, "Threshold numerator")
		thresholdDenom = flag.Int("threshold-denom", 4, "Threshold denominator")

		chainEnabled    = flag.Bool("chain-enabled", false, "Enable on-chain agent verification")
		chainRPCURL     = flag.String("chain-rpc", "", "RPC URL for on-chain verification")
		subnetContract  = flag.String("chain-subnet-contract", "", "Subnet contract address for verification")
		chainCacheTTL   = flag.String("chain-cache-ttl", "5m", "Cache TTL for verification cache")
		chainCacheSize  = flag.Int("chain-cache-size", 1024, "Verification cache size")
		chainFallback   = flag.Bool("chain-fallback", true, "Allow operations when chain verification fails")
		allowUnverified = flag.Bool("allow-unverified-agents", true, "Allow agents that fail verification")
	)
	flag.Parse()

	if *registryEndpoint == "" {
		port := *metricsPort
		if port <= 0 {
			port = *grpcPort
		}
		*registryEndpoint = fmt.Sprintf("127.0.0.1:%d", port)
	}

	// Validate required flags
	if *validatorID == "" {
		log.Fatal("Validator ID is required (--id)")
	}
	if *privateKey == "" {
		// Generate a new key if not provided
		signer, err := crypto.GenerateECDSAKey()
		if err != nil {
			log.Fatalf("Failed to generate key: %v", err)
		}
		*privateKey = signer.GetPrivateKeyHex()
		fmt.Printf("Generated new private key: %s\n", *privateKey)
	}

	// Setup logger
	logger := logging.NewDefaultLogger()
	logger.Info("Starting validator node",
		"id", *validatorID,
		"grpc_port", *grpcPort,
		"nats_url", *natsURL)

	// Create validator set (simplified for MVP)
	validatorSet := createValidatorSet(*validatorCount, *thresholdNum, *thresholdDenom, *validatorID, *privateKey)

	// Create validator configuration
	config := &validator.Config{
		ValidatorID:      *validatorID,
		PrivateKey:       *privateKey,
		ValidatorSet:     validatorSet,
		StoragePath:      *storagePath,
		NATSUrl:          *natsURL,
		GRPCPort:         *grpcPort,
		MetricsPort:      *metricsPort,
		RegistryEndpoint: *registryEndpoint,

		// Consensus timing
		ProposeTimeout:  5 * time.Second,
		CollectTimeout:  10 * time.Second,
		FinalizeTimeout: 2 * time.Second,

		// Execution report config
		MaxReportsPerEpoch: 100,
		ReportScoreDecay:   0.9,
	}

	var chainVerifier *blockchain.ParticipantVerifier
	if *chainEnabled {
		if *chainRPCURL == "" {
			log.Fatal("--chain-rpc is required when --chain-enabled is true")
		}
		if !common.IsHexAddress(*subnetContract) {
			log.Fatalf("Invalid subnet contract address: %s", *subnetContract)
		}
		ttl, err := time.ParseDuration(*chainCacheTTL)
		if err != nil {
			log.Fatalf("Invalid --chain-cache-ttl value: %v", err)
		}
		if *chainCacheSize <= 0 {
			log.Fatal("--chain-cache-size must be positive")
		}
		verifierCfg := blockchain.VerifierConfig{
			CacheTTL:       ttl,
			CacheSize:      *chainCacheSize,
			EnableFallback: *chainFallback,
		}
		verifier, err := blockchain.NewParticipantVerifier(*chainRPCURL, common.HexToAddress(*subnetContract), verifierCfg, logger)
		if err != nil {
			log.Fatalf("Failed to initialise blockchain verifier: %v", err)
		}
		chainVerifier = verifier
		logger.Infof("On-chain agent verification enabled (fallback=%v allow_unverified=%v)", *chainFallback, *allowUnverified)
	} else {
		logger.Info("On-chain agent verification disabled")
	}

	// Start registry service so agents can register
	regService := registry.NewService(*registryGRPC, *registryHTTP)
	if err := regService.Start(); err != nil {
		log.Fatalf("Failed to start registry service: %v", err)
	}
	defer regService.Stop()

	agentRegistry := regService.GetRegistry()
	if *registryEndpoint != "" {
		_ = agentRegistry.RegisterValidator(&registry.ValidatorInfo{
			ID:       *validatorID,
			Endpoint: *registryEndpoint,
			LastSeen: time.Now(),
			Status:   pb.AgentStatus_AGENT_STATUS_ACTIVE,
		})
	}

	// Create validator node with registry
	node, err := validator.NewNode(config, logger, agentRegistry)
	if err != nil {
		log.Fatalf("Failed to create validator node: %v", err)
	}

	// Create and start gRPC server
	server := validator.NewServer(node, logger)
	if chainVerifier != nil {
		server.AttachBlockchainVerifier(chainVerifier, *allowUnverified)
	}
	if err := server.Start(*grpcPort); err != nil {
		if chainVerifier != nil {
			chainVerifier.Close()
		}
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
	defer server.Stop()

	// Start validator node
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := node.Start(ctx); err != nil {
		log.Fatalf("Failed to start validator node: %v", err)
	}
	defer node.Stop()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	logger.Info("Validator node is running. Press Ctrl+C to stop.")
	<-sigCh

	logger.Info("Shutting down validator node...")

	// Stop components
	if err := node.Stop(); err != nil {
		logger.Error("Error stopping validator node", "error", err)
	}
	server.Stop()

	logger.Info("Validator node shutdown complete")
}

// createValidatorSet creates a simple validator set for MVP testing
func createValidatorSet(count, thresholdNum, thresholdDenom int, myID, myPrivKey string) *types.ValidatorSet {
	validators := make([]types.Validator, 0, count)

	// Add ourselves
	mySigner, _ := crypto.NewECDSASignerFromHex(myPrivKey)
	validators = append(validators, types.Validator{
		ID:     myID,
		PubKey: mySigner.PublicKey(),
		Weight: 1,
	})

	// Add other validators (simplified for testing)
	for i := 2; i <= count; i++ {
		id := fmt.Sprintf("validator-%d", i)
		// Generate dummy keys for other validators
		dummySigner, _ := crypto.GenerateECDSAKey()
		validators = append(validators, types.Validator{
			ID:     id,
			PubKey: dummySigner.PublicKey(),
			Weight: 1,
		})
	}

	return &types.ValidatorSet{
		Validators:     validators,
		MinValidators:  4,
		ThresholdNum:   thresholdNum,
		ThresholdDenom: thresholdDenom,
	}
}
