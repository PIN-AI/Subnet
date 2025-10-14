package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"gopkg.in/yaml.v3"
)

// Config represents the YAML configuration structure
type Config struct {
	SubnetID string `yaml:"subnet_id"`
	Agent    struct {
		Matcher struct {
			Signer struct {
				PrivateKey string `yaml:"private_key"`
			} `yaml:"signer"`
		} `yaml:"matcher"`
	} `yaml:"agent"`
	Blockchain struct {
		RPCURL         string `yaml:"rpc_url"`
		SubnetContract string `yaml:"subnet_contract"`
	} `yaml:"blockchain"`
}

func main() {
	// Command line flags
	var (
		configPath      = flag.String("config", "./config/config.yaml", "Path to config file")
		network         = flag.String("network", "base_sepolia", "Network name (base, base_sepolia, local)")
		rpcURL          = flag.String("rpc", "", "RPC URL (overrides config)")
		subnetContract  = flag.String("subnet", "", "Subnet contract address (overrides config)")
		privateKeyHex   = flag.String("key", "", "Private key hex (overrides config)")

		// Participant details
		domain          = flag.String("domain", "subnet.example.com", "Participant domain")
		validatorPort   = flag.String("validator-port", "9090", "Validator endpoint port")
		matcherPort     = flag.String("matcher-port", "8090", "Matcher endpoint port")
		agentPort       = flag.String("agent-port", "7070", "Agent endpoint port")
		metadataURI     = flag.String("metadata", "", "Metadata URI (optional)")

		// Staking options
		useERC20        = flag.Bool("erc20", false, "Use ERC20 staking instead of ETH")
		validatorStake  = flag.String("validator-stake", "0.1", "Validator stake amount in ETH")
		matcherStake    = flag.String("matcher-stake", "0.05", "Matcher stake amount in ETH")
		agentStake      = flag.String("agent-stake", "0.05", "Agent stake amount in ETH")

		// Registration options
		skipValidator   = flag.Bool("skip-validator", false, "Skip validator registration")
		skipMatcher     = flag.Bool("skip-matcher", false, "Skip matcher registration")
		skipAgent       = flag.Bool("skip-agent", false, "Skip agent registration")
		checkOnly       = flag.Bool("check", false, "Only check registration status")
		dryRun          = flag.Bool("dry-run", false, "Dry run (don't submit transactions)")
	)
	flag.Parse()

	ctx := context.Background()

	// Load config file
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override with command line flags
	if *rpcURL == "" {
		*rpcURL = cfg.Blockchain.RPCURL
	}
	if *subnetContract == "" {
		*subnetContract = cfg.Blockchain.SubnetContract
	}
	if *privateKeyHex == "" {
		*privateKeyHex = cfg.Agent.Matcher.Signer.PrivateKey
	}

	// Validate required parameters
	if *rpcURL == "" {
		log.Fatal("RPC URL is required (use -rpc or set blockchain.rpc_url in config)")
	}
	if *subnetContract == "" {
		log.Fatal("Subnet contract address is required (use -subnet or set blockchain.subnet_contract in config)")
	}
	if *privateKeyHex == "" {
		log.Fatal("Private key is required (use -key or set agent.matcher.signer.private_key in config)")
	}

	// Ensure private key has 0x prefix
	if len(*privateKeyHex) > 0 && (*privateKeyHex)[0:2] != "0x" {
		*privateKeyHex = "0x" + *privateKeyHex
	}

	log.Printf("🚀 Starting participant registration script")
	log.Printf("   Network: %s", *network)
	log.Printf("   RPC URL: %s", *rpcURL)
	log.Printf("   Subnet Contract: %s", *subnetContract)
	log.Printf("   Domain: %s", *domain)

	// Connect to Ethereum node
	client, err := ethclient.Dial(*rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum node: %v", err)
	}
	defer client.Close()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		log.Fatalf("Failed to get chain ID: %v", err)
	}
	log.Printf("   Chain ID: %s", chainID.String())

	// Initialize SDK client
	sdkClient, err := sdk.NewClient(ctx, sdk.Config{
		RPCURL:        *rpcURL,
		PrivateKeyHex: *privateKeyHex,
		Network:       *network,
	})
	if err != nil {
		log.Fatalf("Failed to create SDK client: %v", err)
	}
	defer sdkClient.Close()

	signerAddr := sdkClient.Signer.Address()
	log.Printf("   Signer Address: %s", signerAddr.Hex())

	// Check balance
	balance, err := client.BalanceAt(ctx, signerAddr, nil)
	if err != nil {
		log.Fatalf("Failed to get balance: %v", err)
	}
	log.Printf("   Balance: %s ETH", weiToEth(balance))

	// Get subnet service
	subnetAddr := common.HexToAddress(*subnetContract)
	subnetSvc, err := sdkClient.SubnetServiceByAddress(subnetAddr)
	if err != nil {
		log.Fatalf("Failed to get subnet service: %v", err)
	}

	// Query subnet info
	subnetInfo, err := subnetSvc.GetSubnetInfo(ctx)
	if err != nil {
		log.Fatalf("Failed to get subnet info: %v", err)
	}
	log.Printf("   Subnet Name: %s", subnetInfo.CanonicalName)
	log.Printf("   Subnet Owner: %s", subnetInfo.Owner.Hex())

	// Query stake governance config
	stakeConfig := subnetInfo.StakeCfg
	log.Printf("\n📊 Stake Requirements:")
	log.Printf("   Min Validator Stake: %s ETH", weiToEth(stakeConfig.MinValidatorStake))
	log.Printf("   Min Matcher Stake: %s ETH", weiToEth(stakeConfig.MinMatcherStake))
	log.Printf("   Min Agent Stake: %s ETH", weiToEth(stakeConfig.MinAgentStake))
	log.Printf("   Auto Approve: %t", subnetInfo.AutoApprove)
	log.Printf("   Unstake Lock Period: %s", formatDuration(stakeConfig.UnstakeLockPeriod))

	// Check current registration status
	log.Printf("\n🔍 Checking current registration status...")

	isValidator, err := subnetSvc.IsActiveParticipant(ctx, signerAddr, sdk.ParticipantValidator)
	if err != nil {
		log.Printf("   ⚠️  Failed to check validator status: %v", err)
	} else {
		if isValidator {
			log.Printf("   ✅ Already registered as Validator")
		} else {
			log.Printf("   ❌ Not registered as Validator")
		}
	}

	isMatcher, err := subnetSvc.IsActiveParticipant(ctx, signerAddr, sdk.ParticipantMatcher)
	if err != nil {
		log.Printf("   ⚠️  Failed to check matcher status: %v", err)
	} else {
		if isMatcher {
			log.Printf("   ✅ Already registered as Matcher")
		} else {
			log.Printf("   ❌ Not registered as Matcher")
		}
	}

	isAgent, err := subnetSvc.IsActiveParticipant(ctx, signerAddr, sdk.ParticipantAgent)
	if err != nil {
		log.Printf("   ⚠️  Failed to check agent status: %v", err)
	} else {
		if isAgent {
			log.Printf("   ✅ Already registered as Agent")
		} else {
			log.Printf("   ❌ Not registered as Agent")
		}
	}

	// If check-only mode, exit here
	if *checkOnly {
		log.Printf("\n✅ Check complete (use without -check to register)")
		return
	}

	// If dry-run mode, print what would be done
	if *dryRun {
		log.Printf("\n🧪 DRY RUN MODE - No transactions will be submitted")
	}

	// Parse stake amounts
	validatorStakeWei, _ := ethToWei(*validatorStake)
	matcherStakeWei, _ := ethToWei(*matcherStake)
	agentStakeWei, _ := ethToWei(*agentStake)

	// Register participants
	log.Printf("\n🔐 Starting registration process...")

	// Register as Validator
	if !*skipValidator && !isValidator {
		log.Printf("\n📝 Registering as Validator...")
		if err := registerValidator(ctx, subnetSvc, *domain, *validatorPort, *metadataURI, validatorStakeWei, *useERC20, *dryRun); err != nil {
			log.Fatalf("Failed to register validator: %v", err)
		}
		log.Printf("   ✅ Validator registration completed")
	} else if *skipValidator {
		log.Printf("\n⏭️  Skipping Validator registration")
	} else {
		log.Printf("\n✅ Already registered as Validator")
	}

	// Register as Matcher
	if !*skipMatcher && !isMatcher {
		log.Printf("\n📝 Registering as Matcher...")
		if err := registerMatcher(ctx, subnetSvc, *domain, *matcherPort, *metadataURI, matcherStakeWei, *useERC20, *dryRun); err != nil {
			log.Fatalf("Failed to register matcher: %v", err)
		}
		log.Printf("   ✅ Matcher registration completed")
	} else if *skipMatcher {
		log.Printf("\n⏭️  Skipping Matcher registration")
	} else {
		log.Printf("\n✅ Already registered as Matcher")
	}

	// Register as Agent
	if !*skipAgent && !isAgent {
		log.Printf("\n📝 Registering as Agent...")
		if err := registerAgent(ctx, subnetSvc, *domain, *agentPort, *metadataURI, agentStakeWei, *useERC20, *dryRun); err != nil {
			log.Fatalf("Failed to register agent: %v", err)
		}
		log.Printf("   ✅ Agent registration completed")
	} else if *skipAgent {
		log.Printf("\n⏭️  Skipping Agent registration")
	} else {
		log.Printf("\n✅ Already registered as Agent")
	}

	log.Printf("\n🎉 Registration process completed!")
}

func registerValidator(ctx context.Context, svc *sdk.SubnetService, domain, port, metadataURI string, stakeAmount *big.Int, useERC20, dryRun bool) error {
	endpoint := fmt.Sprintf("https://%s:%s", domain, port)

	log.Printf("   Domain: %s", domain)
	log.Printf("   Endpoint: %s", endpoint)
	log.Printf("   Stake: %s ETH", weiToEth(stakeAmount))

	if dryRun {
		log.Printf("   🧪 DRY RUN - Transaction not submitted")
		return nil
	}

	var tx interface{ Hash() common.Hash }
	var err error

	if useERC20 {
		tx, err = svc.RegisterValidatorERC20(ctx, sdk.RegisterParticipantERC20Params{
			Amount:      stakeAmount,
			Domain:      domain,
			Endpoint:    endpoint,
			MetadataURI: metadataURI,
		})
	} else {
		tx, err = svc.RegisterValidator(ctx, sdk.RegisterParticipantParams{
			Domain:      domain,
			Endpoint:    endpoint,
			MetadataURI: metadataURI,
			Value:       stakeAmount,
		})
	}

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	log.Printf("   📤 Transaction submitted: %s", tx.Hash().Hex())
	log.Printf("   ⏳ Waiting for confirmation...")

	time.Sleep(3 * time.Second) // Simple wait for demo

	return nil
}

func registerMatcher(ctx context.Context, svc *sdk.SubnetService, domain, port, metadataURI string, stakeAmount *big.Int, useERC20, dryRun bool) error {
	endpoint := fmt.Sprintf("https://%s:%s", domain, port)

	log.Printf("   Domain: %s", domain)
	log.Printf("   Endpoint: %s", endpoint)
	log.Printf("   Stake: %s ETH", weiToEth(stakeAmount))

	if dryRun {
		log.Printf("   🧪 DRY RUN - Transaction not submitted")
		return nil
	}

	var tx interface{ Hash() common.Hash }
	var err error

	if useERC20 {
		tx, err = svc.RegisterMatcherERC20(ctx, sdk.RegisterParticipantERC20Params{
			Amount:      stakeAmount,
			Domain:      domain,
			Endpoint:    endpoint,
			MetadataURI: metadataURI,
		})
	} else {
		tx, err = svc.RegisterMatcher(ctx, sdk.RegisterParticipantParams{
			Domain:      domain,
			Endpoint:    endpoint,
			MetadataURI: metadataURI,
			Value:       stakeAmount,
		})
	}

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	log.Printf("   📤 Transaction submitted: %s", tx.Hash().Hex())
	log.Printf("   ⏳ Waiting for confirmation...")

	time.Sleep(3 * time.Second)

	return nil
}

func registerAgent(ctx context.Context, svc *sdk.SubnetService, domain, port, metadataURI string, stakeAmount *big.Int, useERC20, dryRun bool) error {
	endpoint := fmt.Sprintf("https://%s:%s", domain, port)

	log.Printf("   Domain: %s", domain)
	log.Printf("   Endpoint: %s", endpoint)
	log.Printf("   Stake: %s ETH", weiToEth(stakeAmount))

	if dryRun {
		log.Printf("   🧪 DRY RUN - Transaction not submitted")
		return nil
	}

	var tx interface{ Hash() common.Hash }
	var err error

	if useERC20 {
		tx, err = svc.RegisterAgentERC20(ctx, sdk.RegisterParticipantERC20Params{
			Amount:      stakeAmount,
			Domain:      domain,
			Endpoint:    endpoint,
			MetadataURI: metadataURI,
		})
	} else {
		tx, err = svc.RegisterAgent(ctx, sdk.RegisterParticipantParams{
			Domain:      domain,
			Endpoint:    endpoint,
			MetadataURI: metadataURI,
			Value:       stakeAmount,
		})
	}

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	log.Printf("   📤 Transaction submitted: %s", tx.Hash().Hex())
	log.Printf("   ⏳ Waiting for confirmation...")

	time.Sleep(3 * time.Second)

	return nil
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func weiToEth(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	eth := new(big.Float).SetInt(wei)
	eth.Quo(eth, big.NewFloat(1e18))
	return eth.Text('f', 6)
}

func ethToWei(eth string) (*big.Int, error) {
	flt, _, err := big.ParseFloat(eth, 10, 256, big.ToNearestEven)
	if err != nil {
		return nil, err
	}
	flt.Mul(flt, big.NewFloat(1e18))
	wei, _ := flt.Int(nil)
	return wei, nil
}

func formatDuration(seconds *big.Int) string {
	if seconds == nil {
		return "0s"
	}
	secs := seconds.Int64()
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm", secs/60)
	}
	if secs < 86400 {
		return fmt.Sprintf("%dh", secs/3600)
	}
	return fmt.Sprintf("%dd", secs/86400)
}
