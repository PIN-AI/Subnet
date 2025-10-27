package main

import (
	"context"
	"encoding/hex"
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
		natsURL          = flag.String("nats", "", "NATS server URL (deprecated - use Raft instead)")
		storagePath      = flag.String("storage", "./data", "Storage path for LevelDB")
		metricsPort      = flag.Int("metrics", 9095, "Metrics server port")
		registryGRPC     = flag.String("registry-grpc", ":8091", "Registry gRPC listen address (empty to disable)")
		registryHTTP     = flag.String("registry-http", ":8092", "Registry HTTP listen address (empty to disable)")
		registryEndpoint = flag.String("registry-endpoint", "", "Validator endpoint advertised via registry")

		// Raft consensus configuration
		raftEnable    = flag.Bool("raft-enable", false, "Enable Raft consensus")
		raftBootstrap = flag.Bool("raft-bootstrap", false, "Bootstrap Raft cluster (first node only)")
		raftBind      = flag.String("raft-bind", "127.0.0.1:7000", "Raft bind address")
		raftDataDir   = flag.String("raft-data-dir", "./data/raft", "Raft data directory")
		raftPeers     = flag.String("raft-peers", "", "Comma-separated list of validator_id:raft_address pairs (e.g. 'v1:127.0.0.1:7000,v2:127.0.0.1:7001')")

		// Validator endpoints for execution report forwarding
		validatorEndpoints = flag.String("validator-endpoints", "", "Comma-separated list of validator_id:grpc_address pairs (e.g. 'v1:127.0.0.1:9201,v2:127.0.0.1:9202')")

		// Gossip configuration
		gossipEnable = flag.Bool("gossip-enable", false, "Enable Gossip for signature propagation")
		gossipBind   = flag.String("gossip-bind", "127.0.0.1", "Gossip bind address")
		gossipPort   = flag.Int("gossip-port", 7946, "Gossip port")
	gossipSeeds  = flag.String("gossip-seeds", "", "Comma-separated list of seed nodes (e.g. '127.0.0.1:6001,127.0.0.1:6002')")

		// Validator set configuration
		validatorCount  = flag.Int("validators", 4, "Total number of validators")
		thresholdNum    = flag.Int("threshold-num", 3, "Threshold numerator")
		thresholdDenom  = flag.Int("threshold-denom", 4, "Threshold denominator")
		validatorPubkey = flag.String("validator-pubkeys", "", "Comma-separated list of validator_id:pubkey_hex pairs (e.g. 'val1:04abc...,val2:04def...')")

		chainEnabled    = flag.Bool("chain-enabled", false, "Enable on-chain agent verification")
		chainRPCURL     = flag.String("chain-rpc", "", "RPC URL for on-chain verification")
		subnetContract  = flag.String("chain-subnet-contract", "", "Subnet contract address for verification")
		chainCacheTTL   = flag.String("chain-cache-ttl", "5m", "Cache TTL for verification cache")
		chainCacheSize  = flag.Int("chain-cache-size", 1024, "Verification cache size")
		chainFallback   = flag.Bool("chain-fallback", true, "Allow operations when chain verification fails")
		allowUnverified = flag.Bool("allow-unverified-agents", true, "Allow agents that fail verification")

		// RootLayer configuration
		rootlayerEndpoint = flag.String("rootlayer-endpoint", "", "RootLayer gRPC endpoint (e.g. 3.17.208.238:9001)")
		enableRootlayer   = flag.Bool("enable-rootlayer", false, "Enable ValidationBundle submission to RootLayer")

		// Subnet configuration
		subnetID = flag.String("subnet-id", "0x1111111111111111111111111111111111111111111111111111111111111111", "Subnet ID (32-byte hex string)")

		// Blockchain configuration for ValidationBundle signing
		enableChainSubmit  = flag.Bool("enable-chain-submit", false, "Enable on-chain ValidationBundle submission")
		valChainRPCURL     = flag.String("chain-rpc-url", "", "Blockchain RPC URL for ValidationBundle signing")
		valChainNetwork    = flag.String("chain-network", "", "Network name (e.g., base_sepolia)")
		intentManagerAddr  = flag.String("intent-manager-addr", "", "IntentManager contract address")
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
	if *raftEnable {
		logger.Info("Starting validator node",
			"id", *validatorID,
			"grpc_port", *grpcPort,
			"consensus", "Raft+Gossip",
			"raft_bind", *raftBind)
	} else if *natsURL != "" {
		logger.Warn("NATS mode is deprecated - please use Raft consensus",
			"id", *validatorID,
			"grpc_port", *grpcPort)
	} else {
		logger.Info("Starting validator node",
			"id", *validatorID,
			"grpc_port", *grpcPort)
	}

	// Create validator set (simplified for MVP)
	validatorSet := createValidatorSet(*validatorCount, *thresholdNum, *thresholdDenom, *validatorID, *privateKey, *validatorPubkey)

	// Create validator configuration
	config := &validator.Config{
		ValidatorID:      *validatorID,
		SubnetID:         *subnetID,
		PrivateKey:       *privateKey,
		ValidatorSet:     validatorSet,
		StoragePath:      *storagePath,
		NATSUrl:          *natsURL,  // Deprecated but kept for backward compatibility
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

		// RootLayer configuration
		RootLayerEndpoint:     *rootlayerEndpoint,
		EnableRootLayerSubmit: *enableRootlayer,

		// Blockchain configuration
		EnableChainSubmit:  *enableChainSubmit,
		ChainRPCURL:        *valChainRPCURL,
		ChainNetwork:       *valChainNetwork,
		IntentManagerAddr:  *intentManagerAddr,

		// Raft consensus configuration
		Raft: &validator.RaftConfig{
			Enable:           *raftEnable,
			DataDir:          *raftDataDir,
			BindAddress:      *raftBind,
			Bootstrap:        *raftBootstrap,
			Peers:            parseRaftPeers(*raftPeers),
			HeartbeatTimeout: 1 * time.Second,
			ElectionTimeout:  2 * time.Second,
			CommitTimeout:    500 * time.Millisecond,
		},

		// Gossip configuration
		Gossip: &validator.GossipConfig{
			Enable:         *gossipEnable,
			BindAddress:    *gossipBind,
			BindPort:       *gossipPort,
			Seeds:          parseGossipSeeds(*gossipSeeds),
			GossipInterval: 200 * time.Millisecond,
			ProbeInterval:  1 * time.Second,
		},

		// Validator endpoints mapping for execution report forwarding
		ValidatorEndpoints: parseValidatorEndpoints(*validatorEndpoints),
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
// IMPORTANT: All validators must use the SAME order for consensus to work correctly!
func createValidatorSet(count, thresholdNum, thresholdDenom int, myID, myPrivKey, validatorPubkeys string) *types.ValidatorSet {
	// Parse validator pubkeys from the comma-separated list
	pubkeyMap := make(map[string][]byte)
	if validatorPubkeys != "" {
		pairs := splitAndTrim(validatorPubkeys, ",")
		for _, pair := range pairs {
			parts := splitAndTrim(pair, ":")
			if len(parts) == 2 {
				id := parts[0]
				pubkeyHex := parts[1]
				// Remove 0x prefix if present
				if len(pubkeyHex) >= 2 && pubkeyHex[:2] == "0x" {
					pubkeyHex = pubkeyHex[2:]
				}
				pubkeyBytes, err := hex.DecodeString(pubkeyHex)
				if err != nil {
					log.Printf("Warning: invalid pubkey hex for %s: %v", id, err)
					continue
				}
				pubkeyMap[id] = pubkeyBytes
			}
		}
	}

	// Get my signer
	mySigner, _ := crypto.NewECDSASignerFromHex(myPrivKey)

	// Create validators in FIXED order (sorted by ID) so all validators agree on leader
	validators := make([]types.Validator, 0, count)
	for i := 1; i <= count; i++ {
		id := fmt.Sprintf("validator-e2e-%d", i)

		if id == myID {
			// Use my actual signer
			validators = append(validators, types.Validator{
				ID:     id,
				PubKey: mySigner.PublicKey(),
				Weight: 1,
			})
		} else {
			// Use provided pubkey if available, otherwise generate dummy key
			if pubkey, ok := pubkeyMap[id]; ok {
				validators = append(validators, types.Validator{
					ID:     id,
					PubKey: pubkey,
					Weight: 1,
				})
			} else {
				log.Printf("Warning: no pubkey found for %s, using dummy key", id)
				dummySigner, _ := crypto.GenerateECDSAKey()
				validators = append(validators, types.Validator{
					ID:     id,
					PubKey: dummySigner.PublicKey(),
					Weight: 1,
				})
			}
		}
	}

	return &types.ValidatorSet{
		Validators:     validators,
		MinValidators:  count,
		ThresholdNum:   thresholdNum,
		ThresholdDenom: thresholdDenom,
	}
}

// parseRaftPeers parses comma-separated raft peer config: "id1:addr1:port1,id2:addr2:port2,..."
func parseRaftPeers(peerStr string) []validator.RaftPeerConfig {
	if peerStr == "" {
		return nil
	}

	peers := []validator.RaftPeerConfig{}
	for _, pair := range splitAndTrim(peerStr, ",") {
		parts := splitAndTrim(pair, ":")
		if len(parts) >= 2 {
			// First part is ID, rest is address (which may contain colons for host:port)
			id := parts[0]
			// Rejoin remaining parts with ":" to reconstruct address
			address := parts[1]
			for i := 2; i < len(parts); i++ {
				address += ":" + parts[i]
			}
			peers = append(peers, validator.RaftPeerConfig{
				ID:      id,
				Address: address,
			})
		} else {
			log.Printf("Warning: invalid raft peer format '%s', expected 'id:address:port'", pair)
		}
	}
	return peers
}

// parseValidatorEndpoints parses comma-separated validator endpoint config: "id1:addr1:port1,id2:addr2:port2,..."
func parseValidatorEndpoints(endpointStr string) map[string]string {
	endpoints := make(map[string]string)
	if endpointStr == "" {
		return endpoints
	}

	for _, pair := range splitAndTrim(endpointStr, ",") {
		parts := splitAndTrim(pair, ":")
		if len(parts) >= 2 {
			// First part is ID, rest is address (which may contain colons for host:port)
			id := parts[0]
			// Rejoin remaining parts with ":" to reconstruct address
			address := parts[1]
			for i := 2; i < len(parts); i++ {
				address += ":" + parts[i]
			}
			endpoints[id] = address
		} else {
			log.Printf("Warning: invalid validator endpoint format '%s', expected 'id:address:port'", pair)
		}
	}
	return endpoints
}

// parseGossipSeeds parses comma-separated gossip seed nodes: "host1:port1,host2:port2,..."
func parseGossipSeeds(seedsStr string) []string {
	if seedsStr == "" {
		return []string{}
	}
	return splitAndTrim(seedsStr, ",")
}

// splitAndTrim splits a string by delimiter and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	parts := []string{}
	for _, p := range splitString(s, sep) {
		trimmed := trimWhitespace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// splitString splits a string by a separator
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

// trimWhitespace trims whitespace from a string
func trimWhitespace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
