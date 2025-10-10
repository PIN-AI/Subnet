package matcher

import (
	"testing"

	pb "subnet/proto/subnet"
)

func TestExtractChainAddressFromBidMetadata(t *testing.T) {
	bid := &pb.Bid{
		AgentId: "agent-1",
		Metadata: map[string]string{
			chainAddressMetadataKey: " 0xAbc000000000000000000000000000000000000 ",
		},
	}
	addr := extractChainAddressFromBid(bid)
	expected := "0xAbc000000000000000000000000000000000000"
	if addr != expected {
		t.Fatalf("expected %s, got %s", expected, addr)
	}
}

func TestExtractChainAddressFromBidFallback(t *testing.T) {
	bid := &pb.Bid{AgentId: "0xabc1230000000000000000000000000000000000"}
	addr := extractChainAddressFromBid(bid)
	if addr != "0xabc1230000000000000000000000000000000000" {
		t.Fatalf("unexpected address %s", addr)
	}
}

func TestExtractChainAddressFromBidEmpty(t *testing.T) {
	addr := extractChainAddressFromBid(nil)
	if addr != "" {
		t.Fatalf("expected empty string, got %s", addr)
	}
}
