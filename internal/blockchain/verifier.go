package blockchain

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	subnetcontract "github.com/PIN-AI/intent-protocol-contract-sdk/contracts/subnet"
	"subnet/internal/logging"
)

// ParticipantType mirrors the on-chain role enum.
type ParticipantType uint8

const (
	ParticipantValidator ParticipantType = 0
	ParticipantAgent     ParticipantType = 1
	ParticipantMatcher   ParticipantType = 2
)

var errMissingAddress = errors.New("blockchain: chain address missing")

type cacheEntry struct {
	key      string
	isActive bool
	expires  time.Time
}

// ParticipantVerifier performs cached, read-only checks against the Subnet contract.
type ParticipantVerifier struct {
	client   *ethclient.Client
	contract *subnetcontract.Subnet

	cache     map[string]*list.Element
	order     *list.List
	cacheMu   sync.Mutex
	cacheTTL  time.Duration
	cacheSize int

	fallback bool
	logger   logging.Logger
}

// NewParticipantVerifier constructs a verifier using read-only RPC access.
func NewParticipantVerifier(rpcURL string, subnetAddr common.Address, cfg VerifierConfig, logger logging.Logger) (*ParticipantVerifier, error) {
	if strings.TrimSpace(rpcURL) == "" {
		return nil, fmt.Errorf("blockchain: rpc url required")
	}
	if logger == nil {
		logger = logging.NewDefaultLogger()
	}
	cfg.normalize()

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("blockchain: connect rpc: %w", err)
	}

	contract, err := subnetcontract.NewSubnet(subnetAddr, client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("blockchain: bind subnet contract: %w", err)
	}

	return &ParticipantVerifier{
		client:    client,
		contract:  contract,
		cache:     make(map[string]*list.Element, cfg.CacheSize),
		order:     list.New(),
		cacheTTL:  cfg.CacheTTL,
		cacheSize: cfg.CacheSize,
		fallback:  cfg.EnableFallback,
		logger:    logger,
	}, nil
}

// Close releases the underlying RPC connection.
func (v *ParticipantVerifier) Close() {
	if v == nil {
		return
	}
	if v.client != nil {
		v.client.Close()
	}
}

// VerifyAgent checks whether the provided address is an active agent.
func (v *ParticipantVerifier) VerifyAgent(ctx context.Context, addr string) (bool, error) {
	return v.verify(ctx, addr, ParticipantAgent, "agent")
}

// VerifyMatcher checks whether the provided address is an active matcher.
func (v *ParticipantVerifier) VerifyMatcher(ctx context.Context, addr string) (bool, error) {
	return v.verify(ctx, addr, ParticipantMatcher, "matcher")
}

// VerifyValidator checks whether the provided address is an active validator.
func (v *ParticipantVerifier) VerifyValidator(ctx context.Context, addr string) (bool, error) {
	return v.verify(ctx, addr, ParticipantValidator, "validator")
}

func (v *ParticipantVerifier) verify(ctx context.Context, rawAddr string, participantType ParticipantType, roleLabel string) (bool, error) {
	if v == nil {
		return false, errors.New("blockchain: verifier not initialised")
	}

	addr, err := v.normalizeAddress(rawAddr)
	if err != nil {
		observeMissingAddress(roleLabel)
		if v.fallback {
			v.logger.Warnf("chain verification missing address for %s: %v", roleLabel, err)
			observeVerification(roleLabel, "fallback")
			return true, nil
		}
		observeVerification(roleLabel, "error")
		return false, errMissingAddress
	}

	cacheKey := cacheKeyFor(roleLabel, participantType, addr)
	if ok, hit := v.tryCache(cacheKey, roleLabel); hit {
		return ok, nil
	}

	callOpts := &bind.CallOpts{Context: ctx}
	active, err := v.contract.IsActiveParticipant(callOpts, addr, uint8(participantType))
	if err != nil {
		v.logger.Warnf("chain verification error for %s %s: %v", roleLabel, addr.Hex(), err)
		observeVerification(roleLabel, "error")
		if v.fallback {
			observeVerification(roleLabel, "fallback")
			return true, nil
		}
		return false, err
	}

	v.updateCache(cacheKey, active)
	if active {
		observeVerification(roleLabel, "success")
	} else {
		observeVerification(roleLabel, "inactive")
	}
	return active, nil
}

func (v *ParticipantVerifier) tryCache(key, role string) (bool, bool) {
	v.cacheMu.Lock()
	defer v.cacheMu.Unlock()

	element, ok := v.cache[key]
	if !ok {
		observeCacheEvent(role, "miss")
		return false, false
	}

	entry := element.Value.(*cacheEntry)
	if time.Now().After(entry.expires) {
		observeCacheEvent(role, "expired")
		v.removeElement(element)
		return false, false
	}

	v.order.MoveToFront(element)
	observeCacheEvent(role, "hit")
	if entry.isActive {
		observeVerification(role, "success_cached")
	} else {
		observeVerification(role, "inactive_cached")
	}
	return entry.isActive, true
}

func (v *ParticipantVerifier) updateCache(key string, active bool) {
	v.cacheMu.Lock()
	defer v.cacheMu.Unlock()

	if element, ok := v.cache[key]; ok {
		entry := element.Value.(*cacheEntry)
		entry.isActive = active
		entry.expires = time.Now().Add(v.cacheTTL)
		v.order.MoveToFront(element)
		return
	}

	entry := &cacheEntry{
		key:      key,
		isActive: active,
		expires:  time.Now().Add(v.cacheTTL),
	}
	element := v.order.PushFront(entry)
	v.cache[key] = element

	if v.order.Len() > v.cacheSize {
		v.evictOldest()
	}
}

func (v *ParticipantVerifier) evictOldest() {
	oldest := v.order.Back()
	if oldest == nil {
		return
	}
	v.removeElement(oldest)
}

func (v *ParticipantVerifier) removeElement(el *list.Element) {
	v.order.Remove(el)
	entry := el.Value.(*cacheEntry)
	delete(v.cache, entry.key)
}

func (v *ParticipantVerifier) normalizeAddress(addr string) (common.Address, error) {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return common.Address{}, errMissingAddress
	}
	if !strings.HasPrefix(trimmed, "0x") {
		trimmed = "0x" + trimmed
	}
	if !common.IsHexAddress(trimmed) {
		return common.Address{}, fmt.Errorf("invalid address: %s", addr)
	}
	return common.HexToAddress(trimmed), nil
}

func cacheKeyFor(role string, p ParticipantType, addr common.Address) string {
	return fmt.Sprintf("%s:%d:%s", role, p, strings.ToLower(addr.Hex()))
}
