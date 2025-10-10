package blockchain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	lru "github.com/hashicorp/golang-lru/v2"

	sdk "github.com/PIN-AI/intent-protocol-contract-sdk/sdk"
	"subnet/internal/logging"
)

// ParticipantVerifierSDK 使用 SDK 的只读验证器
type ParticipantVerifierSDK struct {
	client    *sdk.Client
	subnetSvc *sdk.SubnetService

	cache    *lru.Cache[string, cacheEntry]
	cacheTTL time.Duration

	fallback bool
	logger   logging.Logger
}

// NewParticipantVerifierSDK 使用 SDK 创建验证器（使用虚拟私钥）
func NewParticipantVerifierSDK(rpcURL string, subnetID [32]byte, cfg VerifierConfig, logger logging.Logger) (*ParticipantVerifierSDK, error) {
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}
	cfg.normalize()

	// 生成一个临时的只读私钥（不会用于发送交易）
	readOnlyKey := generateReadOnlyKey()

	ctx := context.Background()
	client, err := sdk.NewClient(ctx, sdk.Config{
		RPCURL:        rpcURL,
		PrivateKeyHex: readOnlyKey,
		Tx: &sdk.TxOptions{
			NoSend: boolPtr(true), // 确保不发送任何交易
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create SDK client: %w", err)
	}

	subnetSvc, err := client.SubnetServiceByID(ctx, subnetID)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("get subnet service: %w", err)
	}

	cache, err := lru.New[string, cacheEntry](cfg.CacheSize)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("create cache: %w", err)
	}

	return &ParticipantVerifierSDK{
		client:    client,
		subnetSvc: subnetSvc,
		cache:     cache,
		cacheTTL:  cfg.CacheTTL,
		fallback:  cfg.EnableFallback,
		logger:    logger,
	}, nil
}

// VerifyAgent 使用 SDK 验证 Agent
func (v *ParticipantVerifierSDK) VerifyAgent(ctx context.Context, addr string) (bool, error) {
	normalized, err := v.normalizeAddress(addr)
	if err != nil {
		if v.fallback {
			v.logger.Warnf("Invalid address %s, using fallback", addr)
			return true, nil
		}
		return false, err
	}

	// 检查缓存
	cacheKey := fmt.Sprintf("agent:%s", normalized.Hex())
	if cached, found := v.cache.Get(cacheKey); found {
		entry := cached
		if time.Now().Before(entry.expires) {
			return entry.isActive, nil
		}
		v.cache.Remove(cacheKey)
	}

	// 使用 SDK 查询
	isActive, err := v.subnetSvc.IsActiveParticipant(ctx, normalized, sdk.ParticipantAgent)
	if err != nil {
		v.logger.Warnf("SDK verification failed: %v", err)
		if v.fallback {
			return true, nil
		}
		return false, err
	}

	// 更新缓存
	v.cache.Add(cacheKey, cacheEntry{
		isActive: isActive,
		expires:  time.Now().Add(v.cacheTTL),
	})

	return isActive, nil
}

// normalizeAddress 标准化地址
func (v *ParticipantVerifierSDK) normalizeAddress(addr string) (common.Address, error) {
	trimmed := strings.TrimSpace(addr)
	if !common.IsHexAddress(trimmed) {
		return common.Address{}, fmt.Errorf("invalid address: %s", addr)
	}
	return common.HexToAddress(trimmed), nil
}

// Close 关闭 SDK 客户端
func (v *ParticipantVerifierSDK) Close() {
	if v.client != nil {
		v.client.Close()
	}
}

// generateReadOnlyKey 生成一个仅用于只读操作的临时私钥
func generateReadOnlyKey() string {
	// 方案1：使用固定的测试私钥（不含真实资金）
	// return "0x0000000000000000000000000000000000000000000000000000000000000001"

	// 方案2：每次生成随机私钥（更安全）
	key, _ := crypto.GenerateKey()
	return fmt.Sprintf("0x%x", crypto.FromECDSA(key))
}

func boolPtr(b bool) *bool {
	return &b
}