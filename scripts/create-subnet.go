package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	subnetfactory "github.com/PIN-AI/intent-protocol-contract-sdk/contracts/subnetfactory"
	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"gopkg.in/yaml.v3"
)

// Config represents the YAML configuration structure
type Config struct {
	Agent struct {
		Matcher struct {
			Signer struct {
				PrivateKey string `yaml:"private_key"`
			} `yaml:"signer"`
		} `yaml:"matcher"`
	} `yaml:"agent"`
	Blockchain struct {
		RPCURL string `yaml:"rpc_url"`
	} `yaml:"blockchain"`
}

func main() {
	// Command line flags
	var (
		configPath     = flag.String("config", "./config/config.yaml", "Path to config file")
		network        = flag.String("network", "base_sepolia", "Network name")
		rpcURL         = flag.String("rpc", "", "RPC URL (overrides config)")
		privateKeyHex  = flag.String("key", "", "Private key hex (overrides config)")
		subnetName     = flag.String("name", "My Test Subnet", "Subnet canonical name")
		metadataURI    = flag.String("metadata", "", "Metadata URI (optional)")
		minValidatorStake = flag.String("min-validator-stake", "0.0001", "Min validator stake in ETH")
		minMatcherStake   = flag.String("min-matcher-stake", "0.0001", "Min matcher stake in ETH")
		minAgentStake     = flag.String("min-agent-stake", "0.0001", "Min agent stake in ETH")
		autoApprove    = flag.Bool("auto-approve", true, "Auto approve participants")
		requireKYC     = flag.Bool("require-kyc", false, "Require KYC for participants")
		thresholdNum   = flag.Int64("threshold-num", 3, "Signature threshold numerator")
		thresholdDenom = flag.Int64("threshold-denom", 4, "Signature threshold denominator")
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
	if *privateKeyHex == "" {
		*privateKeyHex = cfg.Agent.Matcher.Signer.PrivateKey
	}

	// Validate required parameters
	if *rpcURL == "" {
		log.Fatal("RPC URL is required")
	}
	if *privateKeyHex == "" {
		log.Fatal("Private key is required")
	}

	// Ensure private key has 0x prefix
	if len(*privateKeyHex) > 0 && (*privateKeyHex)[0:2] != "0x" {
		*privateKeyHex = "0x" + *privateKeyHex
	}

	log.Printf("🚀 Starting Subnet creation process")
	log.Printf("   Network: %s", *network)
	log.Printf("   RPC URL: %s", *rpcURL)
	log.Printf("   Subnet Name: %s", *subnetName)

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

	// Check minimum stake requirement
	minStake, err := sdkClient.SubnetFactory.GetMinStakeCreateSubnet(ctx)
	if err != nil {
		log.Fatalf("Failed to get min stake: %v", err)
	}
	log.Printf("   Min Stake Required: %s ETH", weiToEth(minStake))

	if balance.Cmp(minStake) < 0 {
		log.Fatalf("❌ Insufficient balance. Need at least %s ETH", weiToEth(minStake))
	}

	log.Printf("\n📋 Subnet Configuration:")
	log.Printf("   Name: %s", *subnetName)
	log.Printf("   Owner: %s", signerAddr.Hex())
	log.Printf("   Auto Approve: %t", *autoApprove)
	log.Printf("   Require KYC: %t", *requireKYC)
	log.Printf("   Signature Threshold: %d/%d", *thresholdNum, *thresholdDenom)

	// Parse stake amounts
	minValidatorStakeWei, _ := ethToWei(*minValidatorStake)
	minMatcherStakeWei, _ := ethToWei(*minMatcherStake)
	minAgentStakeWei, _ := ethToWei(*minAgentStake)

	// Create subnet configuration
	createInfo := subnetfactory.DataStructuresCreateSubnetInfo{
		CanonicalName: *subnetName,
		Owner:         signerAddr,
		Version:       1,
		DaKind:        "simple",
		SigScheme:     "ecdsa",
		CpPolicy: subnetfactory.DataStructuresCheckpointPolicy{
			ChallengeWindow:  big.NewInt(300000), // 5 minutes in milliseconds
			MinEpochInterval: 10,
			MaxEpochInterval: 100,
		},
		SigThreshold: subnetfactory.DataStructuresSignatureThreshold{
			ThresholdNumerator:   uint32(*thresholdNum),
			ThresholdDenominator: uint32(*thresholdDenom),
		},
		StakeCfg: subnetfactory.DataStructuresStakeGovernanceConfig{
			MinValidatorStake: minValidatorStakeWei,
			MinAgentStake:     minAgentStakeWei,
			MinMatcherStake:   minMatcherStakeWei,
			MaxValidators:     big.NewInt(1000),
			MaxAgents:         big.NewInt(10000),
			MaxMatchers:       big.NewInt(100),
			UnstakeLockPeriod: big.NewInt(7 * 24 * 3600), // 7 days
			SlashingRates:     []*big.Int{big.NewInt(10), big.NewInt(20), big.NewInt(50)},
		},
		MetadataUri:       *metadataURI,
		BidFrequencyLimit: big.NewInt(100),
		RequireKYC:        *requireKYC,
		AutoApprove:       *autoApprove,
	}

	log.Printf("\n🔐 Creating subnet on-chain...")

	// We need to send the min stake as value, but SDK's CreateSubnet doesn't support it
	// So we use txManager.Send directly
	tx, err := sdkClient.TxManager.Send(ctx, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		opts.Context = ctx
		opts.Value = minStake // Send the minimum stake requirement as ETH
		// Get the contract binding
		factoryContract, err := subnetfactory.NewSubnetFactory(sdkClient.Addresses.SubnetFactory, sdkClient.Backend)
		if err != nil {
			return nil, err
		}
		return factoryContract.CreateSubnet(opts, createInfo)
	})
	if err != nil {
		log.Fatalf("Failed to create subnet: %v", err)
	}

	log.Printf("   📤 Transaction submitted: %s", tx.Hash().Hex())
	log.Printf("   ⏳ Waiting for confirmation...")

	// Wait for transaction confirmation
	time.Sleep(5 * time.Second)

	// Get the subnets owned by this address
	subnetIDs, err := sdkClient.SubnetFactory.GetSubnetsByOwner(ctx, signerAddr)
	if err != nil {
		log.Fatalf("Failed to get owned subnets: %v", err)
	}

	if len(subnetIDs) == 0 {
		log.Fatalf("No subnets found for owner %s", signerAddr.Hex())
	}

	// Get the most recently created subnet (last in the list)
	subnetID := subnetIDs[len(subnetIDs)-1]

	// Get subnet contract address
	subnetAddr, err := sdkClient.SubnetFactory.GetSubnetContract(ctx, subnetID)
	if err != nil {
		log.Fatalf("Failed to get subnet contract address: %v", err)
	}

	log.Printf("\n🎉 Subnet created successfully!")
	log.Printf("   Subnet ID: 0x%s", hex.EncodeToString(subnetID[:]))
	log.Printf("   Contract Address: %s", subnetAddr.Hex())
	log.Printf("   Transaction: %s", tx.Hash().Hex())
	log.Printf("   View on Basescan: https://sepolia.basescan.org/tx/%s", tx.Hash().Hex())

	// Save subnet info to file for later use
	subnetInfo := fmt.Sprintf(`# Created Subnet Information

Subnet ID: 0x%s
Contract Address: %s
Owner: %s
Name: %s
Transaction: %s

Created at: %s

# To register participants on this subnet:
PIN_BASE_SEPOLIA_INTENT_MANAGER="0xD04d23775D3B8e028e6104E31eb0F6c07206EB46" \
PIN_BASE_SEPOLIA_SUBNET_FACTORY="0x493c5B1c7Ee9eDe75bf2e57e5250E695F929A796" \
PIN_BASE_SEPOLIA_STAKING_MANAGER="0xAc11AE66c7831A70Bea940b0AE16c967f940cB65" \
PIN_BASE_SEPOLIA_CHECKPOINT_MANAGER="0xe947c9C4183D583fB2E500aD05B105Fa01abE57e" \
./scripts/register.sh --subnet %s
`,
		hex.EncodeToString(subnetID[:]),
		subnetAddr.Hex(),
		signerAddr.Hex(),
		*subnetName,
		tx.Hash().Hex(),
		time.Now().Format(time.RFC3339),
		subnetAddr.Hex(),
	)

	infoFile := fmt.Sprintf("./subnet-info-%s.txt", time.Now().Format("20060102-150405"))
	if err := os.WriteFile(infoFile, []byte(subnetInfo), 0644); err != nil {
		log.Printf("⚠️  Failed to save subnet info to file: %v", err)
	} else {
		log.Printf("\n📄 Subnet info saved to: %s", infoFile)
	}
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

