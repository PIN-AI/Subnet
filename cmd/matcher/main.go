package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"subnet/internal/blockchain"
	"subnet/internal/config"
	"subnet/internal/logging"
	"subnet/internal/matcher"
)

func main() {
	var (
		grpcAddr         = flag.String("grpc", ":8090", "gRPC server address")
		matcherID        = flag.String("id", "", "Matcher ID (overrides config file)")
		biddingWindowSec = flag.Int("window", 0, "Bidding window in seconds (overrides config file)")
		configFile       = flag.String("config", "", "Path to configuration file")
	)
	flag.Parse()

	// Setup logger
	logger := logging.NewDefaultLogger()
	logger.Info("Starting Matcher (with bidding windows)")

	// Create matcher configuration
	cfg := matcher.DefaultConfig()

	// Load configuration from file if specified
	if *configFile != "" {
		viper.SetConfigFile(*configFile)
		viper.AutomaticEnv()
		if err := viper.ReadInConfig(); err != nil {
			logger.Warnf("Failed to read config file %s: %v, using defaults", *configFile, err)
		} else {
			// Load identity configuration (CRITICAL)
			if cfg.Identity == nil {
				cfg.Identity = &config.IdentityConfig{}
			}
			if viper.IsSet("identity.subnet_id") {
				cfg.Identity.SubnetID = viper.GetString("identity.subnet_id")
			}
			if viper.IsSet("identity.matcher_id") {
				cfg.Identity.MatcherID = viper.GetString("identity.matcher_id")
			}

			// Load matcher specific configuration
			if viper.IsSet("agent.matcher.id") {
				cfg.MatcherID = viper.GetString("agent.matcher.id")
			}
			if viper.IsSet("agent.matcher.config.default_lease_seconds") {
				cfg.BiddingWindowSec = viper.GetInt("agent.matcher.config.default_lease_seconds") / 30 // Approximate
			}
			if viper.IsSet("agent.matcher.strategy") {
				cfg.MatchingStrategy = viper.GetString("agent.matcher.strategy")
			}
			if viper.IsSet("agent.matcher.signer.private_key") {
				cfg.PrivateKey = viper.GetString("agent.matcher.signer.private_key")
			}
			var rootlayerEndpoint string
			if viper.IsSet("rootlayer.grpc_endpoint") {
				rootlayerEndpoint = viper.GetString("rootlayer.grpc_endpoint")
			} else if viper.IsSet("rootlayer.http_url") {
				rootlayerEndpoint = viper.GetString("rootlayer.http_url")
			}
			if rootlayerEndpoint != "" {
				cfg.RootLayerEndpoint = rootlayerEndpoint
			}
			logger.Infof("Loaded configuration from %s", *configFile)
		}
	}

	blockchainCfg, err := loadBlockchainConfig(logger, *configFile)
	if err != nil {
		log.Fatalf("Failed to load blockchain config: %v", err)
	}

	// Override with command line arguments if provided
	if *matcherID != "" {
		cfg.MatcherID = *matcherID
	}
	if *biddingWindowSec > 0 {
		cfg.BiddingWindowSec = *biddingWindowSec
	}

	// Create matcher server
	matcherServer, err := matcher.NewServer(cfg, logger)
	if err != nil {
		log.Fatalf("Failed to create matcher server: %v", err)
	}

	var chainVerifier *blockchain.ParticipantVerifier
	if blockchainCfg.Enabled {
		addr := blockchainCfg.SubnetAddress()
		if !common.IsHexAddress(addr) {
			log.Fatalf("Invalid subnet contract address: %s", addr)
		}
		verifierCfg := blockchain.VerifierConfig{
			CacheTTL:       blockchainCfg.CacheTTL,
			CacheSize:      blockchainCfg.CacheSize,
			EnableFallback: blockchainCfg.EnableFallback,
		}
		verifier, verr := blockchain.NewParticipantVerifier(blockchainCfg.RPCURL, common.HexToAddress(addr), verifierCfg, logger)
		if verr != nil {
			log.Fatalf("Failed to initialise blockchain verifier: %v", verr)
		}
		matcherServer.AttachBlockchainVerifier(verifier, blockchainCfg.AllowUnverified)
		chainVerifier = verifier
		logger.Infof("On-chain agent verification enabled (fallback=%v allow_unverified=%v)",
			blockchainCfg.EnableFallback,
			blockchainCfg.AllowUnverified,
		)
	} else {
		logger.Info("On-chain agent verification disabled")
	}

	// Start matcher service
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := matcherServer.Start(ctx); err != nil {
		if chainVerifier != nil {
			chainVerifier.Close()
		}
		log.Fatalf("Failed to start matcher service: %v", err)
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Register matcher service
	matcherServer.RegisterGRPC(grpcServer)

	// Register reflection for grpcurl debugging
	reflection.Register(grpcServer)

	// Start listening
	listener, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", *grpcAddr, err)
	}

	// Start gRPC server
	go func() {
		logger.Infof("Starting gRPC server on %s", *grpcAddr)
		if err := grpcServer.Serve(listener); err != nil {
			logger.Errorf("gRPC server error: %v", err)
		}
	}()

	// The Matcher will now pull intents from RootLayer (mock or real)

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Matcher is running. Press Ctrl+C to stop.")
	logger.Infof("Matcher ID: %s", cfg.MatcherID)
	logger.Infof("Bidding window: %d seconds", cfg.BiddingWindowSec)
	if cfg.RootLayerEndpoint != "" {
		logger.Infof("Connected to RootLayer: %s", cfg.RootLayerEndpoint)
	} else {
		logger.Info("Connected to RootLayer (mock) - intents will be pulled automatically")
	}
	logger.Info("")
	logger.Info("Agents can connect to submit bids:")
	logger.Infof("  gRPC endpoint: %s", *grpcAddr)
	logger.Info("")

	<-sigCh

	logger.Info("Shutting down matcher service...")

	// Graceful shutdown
	grpcServer.GracefulStop()
	if err := matcherServer.Stop(); err != nil {
		logger.Errorf("Error stopping matcher: %v", err)
	}

	logger.Info("Matcher stopped")
}

func loadBlockchainConfig(logger logging.Logger, configPath string) (*config.BlockchainConfig, error) {
	cfg := config.DefaultBlockchainConfig()
	if configPath != "" {
		if sub := viper.Sub("blockchain"); sub != nil {
			if err := sub.Unmarshal(cfg); err != nil {
				return nil, fmt.Errorf("unmarshal blockchain config: %w", err)
			}
		}
	}

	applyBlockchainEnvOverrides(cfg)
	if err := cfg.Normalize(); err != nil {
		if cfg.Enabled {
			return nil, err
		}
		logger.Warnf("Blockchain verification disabled due to config error: %v", err)
		defaults := config.DefaultBlockchainConfig()
		_ = defaults.Normalize()
		cfg.Enabled = false
		cfg.CacheTTL = defaults.CacheTTL
		cfg.CacheSize = defaults.CacheSize
		cfg.EnableFallback = defaults.EnableFallback
		cfg.AllowUnverified = defaults.AllowUnverified
	}
	return cfg, nil
}

func applyBlockchainEnvOverrides(cfg *config.BlockchainConfig) {
	if cfg == nil {
		return
	}
	if val, ok := lookupEnvBool("CHAIN_ENABLED"); ok {
		cfg.Enabled = val
	}
	if val, ok := os.LookupEnv("CHAIN_RPC_URL"); ok && val != "" {
		cfg.RPCURL = val
	}
	if val, ok := os.LookupEnv("SUBNET_CONTRACT_ADDRESS"); ok && val != "" {
		cfg.SubnetContract = val
	}
	if val, ok := os.LookupEnv("CHAIN_CACHE_TTL"); ok && val != "" {
		cfg.CacheTTLRaw = val
	}
	if val, ok := os.LookupEnv("CHAIN_CACHE_SIZE"); ok && val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			cfg.CacheSize = size
		}
	}
	if val, ok := lookupEnvBool("CHAIN_ENABLE_FALLBACK"); ok {
		cfg.EnableFallback = val
	}
	if val, ok := lookupEnvBool("ALLOW_UNVERIFIED_AGENTS"); ok {
		cfg.AllowUnverified = val
	}
}

func lookupEnvBool(key string) (bool, bool) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return false, false
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return false, false
	}
	return parsed, true
}
