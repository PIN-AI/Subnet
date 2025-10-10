package rootlayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"subnet/internal/crypto"
)

// AuthInterceptor handles request signing for RootLayer calls
type AuthInterceptor struct {
	signer  crypto.ExtendedSigner
	chainID string
}

// UnaryClientInterceptor signs outgoing unary RPC calls
func (a *AuthInterceptor) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		// Sign and add metadata
		ctx = a.signRequest(ctx, method)

		// Invoke the RPC
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor signs outgoing streaming RPC calls
func (a *AuthInterceptor) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {

		// Sign and add metadata
		ctx = a.signRequest(ctx, method)

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// signRequest adds signature metadata to the request context
func (a *AuthInterceptor) signRequest(ctx context.Context, method string) context.Context {
	if a.signer == nil {
		return ctx
	}

	// Create timestamp
	timestamp := time.Now().Unix()

	// Create canonical message for signing
	canonical := fmt.Sprintf("%s:%s:%d", a.chainID, method, timestamp)

	// Sign the message
	signature, err := a.signer.SignMessage([]byte(canonical))
	if err != nil {
		// Log error but continue without signature
		return ctx
	}

	// Add metadata to context
	md := metadata.Pairs(
		"x-chain-id", a.chainID,
		"x-signer-id", a.signer.GetAddress(),
		"x-timestamp", fmt.Sprintf("%d", timestamp),
		"x-signature", hex.EncodeToString(signature),
	)

	return metadata.NewOutgoingContext(ctx, md)
}