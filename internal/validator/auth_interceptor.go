package validator

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"subnet/internal/crypto"
	"subnet/internal/logging"
	"subnet/internal/rootlayer"
)

// AuthInterceptor handles authentication for gRPC calls
type AuthInterceptor struct {
	logger          logging.Logger
	rootLayerClient rootlayer.Client
	verifier        *crypto.ECDSAVerifier

	// Cache for verified agents
	agentCache     map[string]*CachedAgent
	cacheMu        sync.RWMutex

	// Nonce tracking for replay protection
	usedNonces     map[string]time.Time
	nonceMu        sync.RWMutex
}

// CachedAgent stores verified agent information
type CachedAgent struct {
	AgentID    string
	PublicKey  []byte
	Status     string
	Stake      uint64
	VerifiedAt time.Time
}

// NewAuthInterceptor creates a new authentication interceptor
func NewAuthInterceptor(logger logging.Logger, rootLayerClient rootlayer.Client) *AuthInterceptor {
	return &AuthInterceptor{
		logger:          logger,
		rootLayerClient: rootLayerClient,
		verifier:        &crypto.ECDSAVerifier{},
		agentCache:      make(map[string]*CachedAgent),
		usedNonces:      make(map[string]time.Time),
	}
}

// UnaryServerInterceptor returns the gRPC interceptor function
func (a *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for health checks
		if a.shouldSkipAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract and verify authentication
		agentID, err := a.verifyRequest(ctx)
		if err != nil {
			a.logger.Warn("Authentication failed",
				"method", info.FullMethod,
				"error", err)
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Add agent ID to context
		ctx = context.WithValue(ctx, "agent_id", agentID)

		// Continue with the handler
		return handler(ctx, req)
	}
}

// verifyRequest verifies the authentication metadata
func (a *AuthInterceptor) verifyRequest(ctx context.Context) (string, error) {
	// Extract metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("missing metadata")
	}

	// Extract authentication fields
	agentID := a.getMetadataValue(md, "agent-id")
	if agentID == "" {
		return "", fmt.Errorf("missing agent-id")
	}

	timestampStr := a.getMetadataValue(md, "timestamp")
	if timestampStr == "" {
		return "", fmt.Errorf("missing timestamp")
	}

	nonce := a.getMetadataValue(md, "nonce")
	if nonce == "" {
		return "", fmt.Errorf("missing nonce")
	}

	signatureHex := a.getMetadataValue(md, "signature")
	if signatureHex == "" {
		return "", fmt.Errorf("missing signature")
	}

	// Parse and verify timestamp (5 minute window)
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp: %w", err)
	}

	timeDiff := time.Now().Unix() - timestamp
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	if timeDiff > 300 { // 5 minutes
		return "", fmt.Errorf("timestamp too old or too far in future")
	}

	// Check nonce for replay protection
	if err := a.checkAndStoreNonce(nonce); err != nil {
		return "", fmt.Errorf("nonce check failed: %w", err)
	}

	// Get agent's public key
	publicKey, err := a.getAgentPublicKey(agentID, md)
	if err != nil {
		return "", fmt.Errorf("failed to get agent public key: %w", err)
	}

	// Decode signature
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return "", fmt.Errorf("invalid signature format: %w", err)
	}

	// Construct the message that was signed
	// Format: "agent_id:timestamp:nonce:method"
	method := a.getMetadataValue(md, "method")
	message := fmt.Sprintf("%s:%s:%s:%s", agentID, timestampStr, nonce, method)
	msgHash := crypto.HashMessage([]byte(message))

	// Verify signature using Ethereum-style recovery
	// The SDK uses crypto.Sign which produces Ethereum-style signatures (65 bytes with recovery ID)
	// We need to use SigToPub to recover and verify
	recoveredPub, err := ethcrypto.SigToPub(msgHash[:], signature)
	if err != nil {
		return "", fmt.Errorf("failed to recover public key from signature: %w", err)
	}

	// Compare recovered public key with expected public key
	recoveredPubBytes := ethcrypto.FromECDSAPub(recoveredPub)
	if !bytes.Equal(recoveredPubBytes, publicKey) {
		return "", fmt.Errorf("signature verification failed: public key mismatch")
	}

	a.logger.Debug("Request authenticated successfully", "agent_id", agentID)
	return agentID, nil
}

// getAgentPublicKey retrieves the agent's public key
func (a *AuthInterceptor) getAgentPublicKey(agentID string, md metadata.MD) ([]byte, error) {
	// Check cache first
	a.cacheMu.RLock()
	if cached, ok := a.agentCache[agentID]; ok {
		// Cache valid for 5 minutes
		if time.Since(cached.VerifiedAt) < 5*time.Minute {
			a.cacheMu.RUnlock()
			return cached.PublicKey, nil
		}
	}
	a.cacheMu.RUnlock()

	// Check if public key is provided in metadata (first request)
	publicKeyHex := a.getMetadataValue(md, "public-key")
	if publicKeyHex != "" {
		publicKey, err := hex.DecodeString(publicKeyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid public key format: %w", err)
		}

		// Verify that public key hash matches agent ID
		hash := ethcrypto.Keccak256Hash(publicKey)
		if hex.EncodeToString(hash.Bytes()) != agentID {
			return nil, fmt.Errorf("public key does not match agent ID")
		}

		// Cache it
		a.cacheAgent(agentID, publicKey)
		return publicKey, nil
	}

	// Query RootLayer for agent information
	// For now, we'll return an error. In production, implement RootLayer query
	return nil, fmt.Errorf("agent not in cache and no public key provided")
}

// cacheAgent caches agent information
func (a *AuthInterceptor) cacheAgent(agentID string, publicKey []byte) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()

	a.agentCache[agentID] = &CachedAgent{
		AgentID:    agentID,
		PublicKey:  publicKey,
		Status:     "active",
		VerifiedAt: time.Now(),
	}
}

// checkAndStoreNonce checks if nonce was already used
func (a *AuthInterceptor) checkAndStoreNonce(nonce string) error {
	a.nonceMu.Lock()
	defer a.nonceMu.Unlock()

	// Clean old nonces (older than 10 minutes)
	cutoff := time.Now().Add(-10 * time.Minute)
	for n, t := range a.usedNonces {
		if t.Before(cutoff) {
			delete(a.usedNonces, n)
		}
	}

	// Check if nonce exists
	if _, exists := a.usedNonces[nonce]; exists {
		return fmt.Errorf("nonce already used")
	}

	// Store nonce
	a.usedNonces[nonce] = time.Now()
	return nil
}

// getMetadataValue gets a value from metadata
func (a *AuthInterceptor) getMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// shouldSkipAuth checks if authentication should be skipped
func (a *AuthInterceptor) shouldSkipAuth(method string) bool {
	// Skip auth for these methods
	skipMethods := []string{
		"/grpc.health.v1.Health/Check",
		"/subnet.ValidatorService/GetStatus",
	}

	for _, skip := range skipMethods {
		if method == skip {
			return true
		}
	}
	return false
}