package blockchain

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestVerifierConfigNormalizeDefaults(t *testing.T) {
	cfg := VerifierConfig{}
	cfg.normalize()
	if cfg.CacheTTL <= 0 {
		t.Fatalf("expected CacheTTL to be positive, got %v", cfg.CacheTTL)
	}
	if cfg.CacheSize <= 0 {
		t.Fatalf("expected CacheSize to be positive, got %d", cfg.CacheSize)
	}
}

func TestNormalizeAddress(t *testing.T) {
	v := &ParticipantVerifier{cacheTTL: time.Minute}
	addr, err := v.normalizeAddress("0xABC1230000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := common.HexToAddress("0xabc1230000000000000000000000000000000000")
	if addr != expected {
		t.Fatalf("expected %s, got %s", expected.Hex(), addr.Hex())
	}
}

func TestNormalizeAddressAddsPrefix(t *testing.T) {
	v := &ParticipantVerifier{}
	addr, err := v.normalizeAddress("abc1230000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := common.HexToAddress("0xabc1230000000000000000000000000000000000")
	if addr != expected {
		t.Fatalf("unexpected address: %s", addr.Hex())
	}
}

func TestNormalizeAddressMissing(t *testing.T) {
	v := &ParticipantVerifier{}
	_, err := v.normalizeAddress("")
	if err == nil {
		t.Fatal("expected error for missing address")
	}
	if err != errMissingAddress {
		t.Fatalf("expected errMissingAddress, got %v", err)
	}
}
