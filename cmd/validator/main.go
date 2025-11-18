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
	"subnet/internal/config"
	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/registry"
	"subnet/internal/types"
	"subnet/internal/validator"
	pb "subnet/proto/subnet"
)

func main() {
	// Configuration file flag (must be defined first)
	configFile := flag.String("config", "", "Path to config file (YAML)")

	var (
		validatorID      = flag.String("id", "", "Validator ID")
		privateKey       = flag.String("key", "", "Private key hex (without 0x)")
		grpcPort         = flag.Int("grpc", 9090, "gRPC server port")
		storagePath      = flag.String("storage", "./data", "Storage path for LevelDB")
		metricsPort      = flag.Int("metrics", 9095, "Metrics server port")
		registryGRPC     = flag.String("registry-grpc", ":8091", "Registry gRPC listen address (empty to disable)")
		registryHTTP     = flag.String("registry-http", ":8092", "Registry HTTP listen address (empty to disable)")
		registryEndpoint = flag.String("registry-endpoint", "", "Validator endpoint advertised via registry")

		// Raft consensus configuration
		raftEnable    = flag.Bool("raft-enable", false, "Enable Raft consensus")
		raftBootstrap = flag.Bool("raft-bootstrap", false, "Bootstrap Raft cluster (first node only)")
		raftBind      = flag.String("raft-bind", "127.0.0.1:7000", "Raft bind address")
		raftAdvertise = flag.String("raft-advertise", "", "Raft advertise address (defaults to raft-bind if empty)")
		raftDataDir   = flag.String("raft-data-dir", "./data/raft", "Raft data directory")
		raftPeers     = flag.String("raft-peers", "", "Comma-separated list of validator_id:raft_address pairs (e.g. 'v1:127.0.0.1:7000,v2:127.0.0.1:7001')")

		// Validator endpoints for execution report forwarding
		validatorEndpoints = flag.String("validator-endpoints", "", "Comma-separated list of validator_id:grpc_address pairs (e.g. 'v1:127.0.0.1:9201,v2:127.0.0.1:9202')")

		// Gossip configuration
		gossipEnable = flag.Bool("gossip-enable", false, "Enable Gossip for signature propagation")
		gossipBind   = flag.String("gossip-bind", "127.0.0.1", "Gossip bind address")
		gossipPort   = flag.Int("gossip-port", 7946, "Gossip port")
		gossipSeeds  = flag.String("gossip-seeds", "", "Comma-separated list of seed nodes (e.g. '127.0.0.1:6001,127.0.0.1:6002')")

		// Consensus type selection (user-facing values: raft, cometbft)
		consensusTypeFlag = flag.String("consensus-type", "raft", "Consensus engine type: 'raft' or 'cometbft'")

		// CometBFT configuration
		cometbftHome             = flag.String("cometbft-home", "./cometbft-data", "CometBFT home directory")
		cometbftMoniker          = flag.String("cometbft-moniker", "", "Node moniker for CometBFT (default: validator-id)")
		cometbftP2PPort          = flag.Int("cometbft-p2p-port", 26656, "CometBFT P2P port")
		cometbftRPCPort          = flag.Int("cometbft-rpc-port", 26657, "CometBFT RPC port")
		cometbftProxyPort        = flag.Int("cometbft-proxy-port", 26658, "ABCI proxy app port")
		cometbftSeeds            = flag.String("cometbft-seeds", "", "Comma-separated list of seed nodes for CometBFT (e.g. 'node_id@host:port')")
		cometbftPersistentPeers  = flag.String("cometbft-persistent-peers", "", "Comma-separated list of persistent peers (e.g. 'node_id@host:port')")
		cometbftGenesisFile      = flag.String("cometbft-genesis-file", "", "Path to genesis.json file (required for CometBFT)")
		cometbftPrivValidatorKey = flag.String("cometbft-priv-validator-key", "", "Path to priv_validator_key.json (required for CometBFT)")
		cometbftNodeKey          = flag.String("cometbft-node-key", "", "Path to node_key.json (required for CometBFT)")

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
		subnetID = flag.String("subnet-id", "0x0000000000000000000000000000000000000000000000000000000000000003", "Subnet ID (32-byte hex string)")

		// Blockchain configuration for ValidationBundle signing
		enableChainSubmit = flag.Bool("enable-chain-submit", false, "Enable on-chain ValidationBundle submission")
		valChainRPCURL    = flag.String("chain-rpc-url", "", "Blockchain RPC URL for ValidationBundle signing")
		valChainNetwork   = flag.String("chain-network", "", "Network name (e.g., base_sepolia)")
		intentManagerAddr = flag.String("intent-manager-addr", "", "IntentManager contract address")
	)
	flag.Parse()

	// Load configuration file if specified
	var cfg *config.AppConfig
	if *configFile != "" {
		var err error
		cfg, err = config.Load(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config file %s: %v", *configFile, err)
		}

		// Apply config file defaults (only if not set via command line)
		if *validatorID == "" && cfg.Identity.ValidatorID != "" {
			*validatorID = cfg.Identity.ValidatorID
		}
		if *subnetID == "0x0000000000000000000000000000000000000000000000000000000000000003" && cfg.SubnetID != "" {
			*subnetID = cfg.SubnetID
		}
		if *storagePath == "./data" && cfg.Storage.LevelDBPath != "" {
			*storagePath = cfg.Storage.LevelDBPath
		}
		if *validatorCount == 4 && cfg.ValidatorSet.MinValidators > 0 {
			*validatorCount = cfg.ValidatorSet.MinValidators
		}
		if *thresholdNum == 3 && cfg.ValidatorSet.ThresholdNum > 0 {
			*thresholdNum = cfg.ValidatorSet.ThresholdNum
		}
		if *thresholdDenom == 4 && cfg.ValidatorSet.ThresholdDenom > 0 {
			*thresholdDenom = cfg.ValidatorSet.ThresholdDenom
		}
		// Build validator-endpoints from config file validator set
		if *validatorEndpoints == "" && len(cfg.ValidatorSet.Validators) > 0 {
			endpoints := ""
			for i, v := range cfg.ValidatorSet.Validators {
				if i > 0 {
					endpoints += ","
				}
				endpoints += v.ID + ":" + v.Endpoint
			}
			*validatorEndpoints = endpoints
		}
		// Apply network configuration from config file
		if cfg.Network.ValidatorGRPCPort != "" && *grpcPort == 9090 {
			// Parse port from ":9090" format
			var port int
			fmt.Sscanf(cfg.Network.ValidatorGRPCPort, ":%d", &port)
			if port > 0 {
				*grpcPort = port
			}
		}
		if cfg.Network.MetricsPort != "" && *metricsPort == 9095 {
			// Parse port from ":9095" format
			var port int
			fmt.Sscanf(cfg.Network.MetricsPort, ":%d", &port)
			if port > 0 {
				*metricsPort = port
			}
		}

		// Apply RootLayer configuration from config file
		if *rootlayerEndpoint == "" {
			// Try grpc_endpoint first, then grpc_addr
			if cfg.RootLayer.GRPCEndpoint != "" {
				*rootlayerEndpoint = cfg.RootLayer.GRPCEndpoint
			} else if cfg.RootLayer.GRPCAddr != "" {
				*rootlayerEndpoint = cfg.RootLayer.GRPCAddr
			}
		}
		if !*enableRootlayer {
			// Enable if either grpc_endpoint or grpc_addr is set
			if cfg.RootLayer.GRPCEndpoint != "" || cfg.RootLayer.GRPCAddr != "" {
				*enableRootlayer = true
			}
		}
		if cfg.Blockchain != nil && cfg.Blockchain.Enabled {
			if !*chainEnabled {
				*chainEnabled = true
			}
			if *chainRPCURL == "" && cfg.Blockchain.RPCURL != "" {
				*chainRPCURL = cfg.Blockchain.RPCURL
			}
			if *subnetContract == "" && cfg.Blockchain.SubnetContract != "" {
				*subnetContract = cfg.Blockchain.SubnetContract
			}
		}

		// Apply RPC (Registry) configuration from config file
		if cfg.RPC.ListenAddr != "" {
			// If listen_addr is empty string, disable registry
			// Otherwise use it as the HTTP registry address
			if *registryHTTP == ":8092" {
				*registryHTTP = cfg.RPC.ListenAddr
			}
		}

		// Apply Raft configuration from config file
		if cfg.Raft.Enable {
			if !*raftEnable {
				*raftEnable = true
			}
			if !*raftBootstrap && cfg.Raft.Bootstrap {
				*raftBootstrap = cfg.Raft.Bootstrap
			}
			if *raftBind == "127.0.0.1:7000" && cfg.Raft.Bind != "" {
				*raftBind = cfg.Raft.Bind
			}
			if *raftAdvertise == "" && cfg.Raft.Advertise != "" {
				*raftAdvertise = cfg.Raft.Advertise
			}
			if *raftDataDir == "./data/raft" && cfg.Raft.DataDir != "" {
				*raftDataDir = cfg.Raft.DataDir
			}
			// Build raft peers string from config file peers list
			if *raftPeers == "" && len(cfg.Raft.Peers) > 0 {
				peers := ""
				for i, peer := range cfg.Raft.Peers {
					if i > 0 {
						peers += ","
					}
					peers += peer.ID + ":" + peer.Address
				}
				*raftPeers = peers
			}
		}

		// Apply Gossip configuration from config file
		if cfg.Gossip.Enable {
			if !*gossipEnable {
				*gossipEnable = true
			}
			if *gossipBind == "127.0.0.1" && cfg.Gossip.BindAddress != "" {
				*gossipBind = cfg.Gossip.BindAddress
			}
			if *gossipPort == 7946 && cfg.Gossip.BindPort > 0 {
				*gossipPort = cfg.Gossip.BindPort
			}
			// Build gossip seeds string from config file seeds list
			if *gossipSeeds == "" && len(cfg.Gossip.Seeds) > 0 {
				seeds := ""
				for i, seed := range cfg.Gossip.Seeds {
					if i > 0 {
						seeds += ","
					}
					seeds += seed
				}
				*gossipSeeds = seeds
			}
		}
	}

	consensusType := *consensusTypeFlag
	switch consensusType {
	case "raft":
		consensusType = "raft-gossip"
	case "raft-gossip":
		// legacy name still accepted
	case "cometbft":
	default:
		log.Fatalf("Invalid consensus type: %s (must be 'raft' or 'cometbft')", consensusType)
	}

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

	// Set CometBFT defaults
	if consensusType == "cometbft" {
		// Set default moniker to validator ID if not specified
		if *cometbftMoniker == "" {
			*cometbftMoniker = *validatorID
		}
		// Optional files: if not specified, CometBFT will auto-generate them
		// Genesis file, node_key, and priv_validator_key are automatically created if missing
	}

	// Setup logger
	logger := logging.NewDefaultLogger()
	if consensusType == "cometbft" {
		logger.Info("Starting validator node",
			"id", *validatorID,
			"grpc_port", *grpcPort,
			"consensus", "CometBFT",
			"p2p_port", *cometbftP2PPort,
			"rpc_port", *cometbftRPCPort)
	} else if *raftEnable {
		logger.Info("Starting validator node",
			"id", *validatorID,
			"grpc_port", *grpcPort,
			"consensus", "Raft+Gossip",
			"raft_bind", *raftBind)
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
		EnableChainSubmit: *enableChainSubmit,
		ChainRPCURL:       *valChainRPCURL,
		ChainNetwork:      *valChainNetwork,
		IntentManagerAddr: *intentManagerAddr,

		// Raft consensus configuration
		Raft: &validator.RaftConfig{
			Enable:           *raftEnable,
			DataDir:          *raftDataDir,
			BindAddress:      *raftBind,
			AdvertiseAddress: *raftAdvertise,
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

		// CometBFT consensus configuration
		CometBFT: &validator.CometBFTConfig{
			Enable:            consensusType == "cometbft",
			HomeDir:           *cometbftHome,
			Moniker:           *cometbftMoniker,
			ChainID:           "", // Will be auto-generated from subnet ID (shortened to meet 50-char limit)
			P2PPort:           *cometbftP2PPort,
			RPCPort:           *cometbftRPCPort,
			ProxyPort:         *cometbftProxyPort,
			Seeds:             *cometbftSeeds,
			PersistentPeers:   *cometbftPersistentPeers,
			GenesisFile:       *cometbftGenesisFile,
			PrivValidatorKey:  *cometbftPrivValidatorKey,
			NodeKey:           *cometbftNodeKey,
			GenesisValidators: parseGenesisValidators(*validatorPubkey, validatorSet),
		},

		// Consensus type selection
		ConsensusType: consensusType,

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
	// Start registry service if configured
	var agentRegistry *registry.Registry
	if *registryGRPC != "" || *registryHTTP != "" {
		regService := registry.NewService(*registryGRPC, *registryHTTP)
		if err := regService.Start(); err != nil {
			log.Fatalf("Failed to start registry service: %v", err)
		}
		defer regService.Stop()

		agentRegistry = regService.GetRegistry()
		if *registryEndpoint != "" {
			_ = agentRegistry.RegisterValidator(&registry.ValidatorInfo{
				ID:       *validatorID,
				Endpoint: *registryEndpoint,
				LastSeen: time.Now(),
				Status:   pb.AgentStatus_AGENT_STATUS_ACTIVE,
			})
		}
	} else {
		// Create empty in-memory registry when registry service is disabled
		agentRegistry = registry.New()
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
	validatorIDs := make([]string, 0, count)

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
				validatorIDs = append(validatorIDs, id)
			}
		}
	}

	// Get my signer
	mySigner, _ := crypto.NewECDSASignerFromHex(myPrivKey)

	// If no pubkeys provided, fall back to legacy e2e format
	if len(validatorIDs) == 0 {
		for i := 1; i <= count; i++ {
			validatorIDs = append(validatorIDs, fmt.Sprintf("validator-e2e-%d", i))
		}
	}

	// Create validators using the IDs from the pubkey map
	// Keep them in sorted order for consistent leader election
	validators := make([]types.Validator, 0, len(validatorIDs))
	for _, id := range validatorIDs {
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
				log.Printf("Warning: no pubkey found for %s, generating dummy key", id)
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

// parseGenesisValidators creates a genesis validator set from validatorSet
// For CometBFT, we need a map of validator addresses to voting power
func parseGenesisValidators(pubkeysStr string, validatorSet *types.ValidatorSet) map[string]int64 {
	result := make(map[string]int64)

	// Use the validator set to determine voting power
	// For simplicity, give each validator equal voting power of 10
	for _, validator := range validatorSet.Validators {
		result[validator.ID] = 10
	}

	return result
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
