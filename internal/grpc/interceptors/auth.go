package interceptors

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"subnet/internal/crypto"
)

const (
	// Metadata keys for authentication
	MetadataKeySignature  = "x-signature"
	MetadataKeySignerID   = "x-signer-id"
	MetadataKeyTimestamp  = "x-timestamp"
	MetadataKeyNonce      = "x-nonce"
	MetadataKeyChainID    = "x-chain-id"

	// Maximum time difference allowed (5 minutes)
	MaxTimeDrift = 5 * time.Minute
)

// AuthInterceptor handles signing and verification of gRPC calls
type AuthInterceptor struct {
	signer   crypto.ExtendedSigner
	verifier crypto.ExtendedVerifier
	chainID  string
	// Map of trusted public keys by signer ID
	trustedKeys map[string][]byte
	// Whether to enforce signature verification
	enforceVerification bool
}

// NewAuthInterceptor creates a new auth interceptor
func NewAuthInterceptor(signer crypto.ExtendedSigner, chainID string, enforceVerification bool) *AuthInterceptor {
	return &AuthInterceptor{
		signer:              signer,
		verifier:            crypto.NewExtendedVerifier(),
		chainID:             chainID,
		trustedKeys:         make(map[string][]byte),
		enforceVerification: enforceVerification,
	}
}

// AddTrustedKey adds a trusted public key for a signer ID
func (a *AuthInterceptor) AddTrustedKey(signerID string, pubKey []byte) {
	a.trustedKeys[signerID] = pubKey
}

// UnaryClientInterceptor signs outgoing unary RPC calls
func (a *AuthInterceptor) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		// Sign the request
		signedCtx, err := a.signRequest(ctx, method, req)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to sign request: %v", err)
		}

		// Invoke the RPC with signed context
		return invoker(signedCtx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor signs outgoing streaming RPC calls
func (a *AuthInterceptor) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {

		// Sign the initial request
		signedCtx, err := a.signRequest(ctx, method, nil)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to sign stream request: %v", err)
		}

		return streamer(signedCtx, desc, cc, method, opts...)
	}
}

// UnaryServerInterceptor verifies incoming unary RPC calls
func (a *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (interface{}, error) {

		// Verify the request signature
		if err := a.verifyRequest(ctx, info.FullMethod, req); err != nil {
			if a.enforceVerification {
				return nil, status.Errorf(codes.Unauthenticated, "signature verification failed: %v", err)
			}
			// Log warning but continue if not enforcing
			// logger.Warn("Signature verification failed", "error", err)
		}

		// Call the handler
		return handler(ctx, req)
	}
}

// StreamServerInterceptor verifies incoming streaming RPC calls
func (a *AuthInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {

		// Verify the initial stream request
		if err := a.verifyRequest(ss.Context(), info.FullMethod, nil); err != nil {
			if a.enforceVerification {
				return status.Errorf(codes.Unauthenticated, "stream signature verification failed: %v", err)
			}
		}

		return handler(srv, ss)
	}
}

// signRequest signs an RPC request and adds signature to metadata
func (a *AuthInterceptor) signRequest(ctx context.Context, method string, req interface{}) (context.Context, error) {
	if a.signer == nil {
		return ctx, nil // No signer configured, skip signing
	}

	// Generate timestamp and nonce
	timestamp := time.Now().Unix()
	nonce := crypto.GenerateNonce()

	// Create canonical message
	canonical := a.createCanonicalMessage(method, req, timestamp, nonce)

	// Sign the message
	signature, err := a.signer.SignMessage(canonical)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	// Add signature and metadata to context
	md := metadata.Pairs(
		MetadataKeySignature, hex.EncodeToString(signature),
		MetadataKeySignerID, a.signer.GetAddress(),
		MetadataKeyTimestamp, fmt.Sprintf("%d", timestamp),
		MetadataKeyNonce, nonce,
		MetadataKeyChainID, a.chainID,
	)

	return metadata.NewOutgoingContext(ctx, md), nil
}

// verifyRequest verifies the signature of an incoming RPC request
func (a *AuthInterceptor) verifyRequest(ctx context.Context, method string, req interface{}) error {
	// Extract metadata from context
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("no metadata in context")
	}

	// Extract signature fields
	signatures := md.Get(MetadataKeySignature)
	signerIDs := md.Get(MetadataKeySignerID)
	timestamps := md.Get(MetadataKeyTimestamp)
	nonces := md.Get(MetadataKeyNonce)
	chainIDs := md.Get(MetadataKeyChainID)

	if len(signatures) == 0 || len(signerIDs) == 0 || len(timestamps) == 0 {
		return fmt.Errorf("missing signature metadata")
	}

	// Parse signature
	signature, err := hex.DecodeString(signatures[0])
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	// Verify chain ID
	if len(chainIDs) > 0 && chainIDs[0] != a.chainID {
		return fmt.Errorf("chain ID mismatch: expected %s, got %s", a.chainID, chainIDs[0])
	}

	// Parse and verify timestamp
	var timestamp int64
	if _, err := fmt.Sscanf(timestamps[0], "%d", &timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if err := a.verifyTimestamp(timestamp); err != nil {
		return err
	}

	// Get public key for signer
	signerID := signerIDs[0]
	pubKey, ok := a.trustedKeys[signerID]
	if !ok {
		return fmt.Errorf("unknown signer: %s", signerID)
	}

	// Recreate canonical message
	var nonce string
	if len(nonces) > 0 {
		nonce = nonces[0]
	}
	canonical := a.createCanonicalMessage(method, req, timestamp, nonce)

	// Verify signature
	if !a.verifier.VerifyMessage(pubKey, canonical, signature) {
		return fmt.Errorf("signature verification failed for signer %s", signerID)
	}

	return nil
}

// createCanonicalMessage creates a canonical message for signing
func (a *AuthInterceptor) createCanonicalMessage(method string, req interface{}, timestamp int64, nonce string) []byte {
	// Create deterministic canonical form
	canonical := struct {
		ChainID   string      `json:"chain_id"`
		Method    string      `json:"method"`
		Timestamp int64       `json:"timestamp"`
		Nonce     string      `json:"nonce"`
		Request   interface{} `json:"request,omitempty"`
	}{
		ChainID:   a.chainID,
		Method:    method,
		Timestamp: timestamp,
		Nonce:     nonce,
		Request:   req,
	}

	data, _ := json.Marshal(canonical)
	return data
}

// verifyTimestamp checks if the timestamp is within acceptable range
func (a *AuthInterceptor) verifyTimestamp(timestamp int64) error {
	now := time.Now().Unix()
	diff := time.Duration(abs(now-timestamp)) * time.Second

	if diff > MaxTimeDrift {
		return fmt.Errorf("timestamp too old or too far in future: %d seconds difference", int(diff.Seconds()))
	}

	return nil
}

// Helper function for absolute value
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// ClientInterceptorOptions returns gRPC dial options with auth interceptor
func (a *AuthInterceptor) ClientInterceptorOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(a.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(a.StreamClientInterceptor()),
	}
}

// ServerInterceptorOptions returns gRPC server options with auth interceptor
func (a *AuthInterceptor) ServerInterceptorOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(a.UnaryServerInterceptor()),
		grpc.StreamInterceptor(a.StreamServerInterceptor()),
	}
}