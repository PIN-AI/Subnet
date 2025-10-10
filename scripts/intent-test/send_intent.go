package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	rootpb "rootlayer/proto"
)

func main() {
	endpoint := flag.String("endpoint", envOr("ROOTLAYER_GRPC", "3.17.208.238:9001"), "RootLayer gRPC endpoint")
	intentID := flag.String("intent", "", "intent id (0x32 bytes, randomly generated if empty)")
	subnetID := flag.String("subnet", envOr("SUBNET_ID", "0x0000000000000000000000000000000000000000000000000000000000000000"), "subnet id (0x32 bytes)")
	requester := flag.String("requester", envOr("REQUESTER", "0x0000000000000000000000000000000000000000"), "requester address")
	settleChain := flag.String("settle-chain", envOr("SETTLE_CHAIN", "base_sepolia"), "settle chain name")
	intentType := flag.String("type", envOr("INTENT_TYPE", "demo-intent"), "intent type")
	paramsRaw := flag.String("params-raw", envOr("PARAMS_RAW", `{"task":"demo"}`), "intent payload (JSON string)")
	metadataRaw := flag.String("metadata", envOr("PARAMS_METADATA", ""), "metadata payload (JSON string)")
	tips := flag.String("tips", envOr("TIPS", "0"), "tips amount (uint256 decimal)")
	budget := flag.String("budget", envOr("BUDGET", "0"), "budget amount (uint256 decimal)")
	signature := flag.String("signature", envOr("INTENT_SIGNATURE", ""), "signature bytes (0x hex, optional)")
	deadlineOffset := flag.Int64("deadline-offset", envInt64("DEADLINE_OFFSET", 3600), "deadline offset in seconds from now")

	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial rootlayer: %v", err)
	}
	defer conn.Close()

	req := &rootpb.SubmitIntentRequest{
		IntentId:    mustBytes32(*intentID),
		SubnetId:    mustBytes32(*subnetID),
		Requester:   *requester,
		SettleChain: *settleChain,
		IntentType:  *intentType,
		Params: &rootpb.IntentParams{
			IntentRaw: []byte(*paramsRaw),
			Metadata:  []byte(*metadataRaw),
		},
		TipsToken:   "0x0000000000000000000000000000000000000000",
		Tips:        *tips,
		BudgetToken: "0x0000000000000000000000000000000000000000",
		Budget:      *budget,
		Deadline:    time.Now().Add(time.Duration(*deadlineOffset) * time.Second).Unix(),
		Signature:   mustHexBytes(*signature),
	}

	client := rootpb.NewIntentPoolServiceClient(conn)
	resp, err := client.SubmitIntent(ctx, req)
	if err != nil {
		log.Fatalf("submit intent: %v", err)
	}
	if !resp.Ok {
		log.Fatalf("intent rejected: %s", resp.Msg)
	}

	log.Printf("intent accepted: id=%s params_hash=%x expires=%d", resp.IntentId, resp.ParamsHash, resp.IntentExpiration)
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		var out int64
		if _, err := fmt.Sscanf(v, "%d", &out); err == nil {
			return out
		}
	}
	return fallback
}

func mustBytes32(hexStr string) string {
	trimmed := strings.TrimSpace(hexStr)
	if trimmed == "" {
		buf := make([]byte, 32)
		now := time.Now().UnixNano()
		for i := range buf {
			buf[i] = byte(now >> (uint(i%8) * 8))
		}
		return "0x" + hex.EncodeToString(buf)
	}
	if !strings.HasPrefix(trimmed, "0x") || len(trimmed) != 66 {
		log.Fatalf("expected 32-byte hex value (0x + 64 chars), got %s", hexStr)
	}
	if _, err := hex.DecodeString(trimmed[2:]); err != nil {
		log.Fatalf("decode %s: %v", hexStr, err)
	}
	return trimmed
}

func mustHexBytes(hexStr string) []byte {
	trimmed := strings.TrimSpace(hexStr)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "0x") {
		trimmed = trimmed[2:]
	}
	out, err := hex.DecodeString(trimmed)
	if err != nil {
		log.Fatalf("decode signature: %v", err)
	}
	return out
}
