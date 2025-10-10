package validator

import (
	"testing"

	pb "subnet/proto/subnet"
)

func TestExtractChainAddressFromReport(t *testing.T) {
	report := &pb.ExecutionReport{AgentId: " 0xAbc000000000000000000000000000000000000 "}
	addr := extractChainAddressFromReport(report)
	expected := "0xAbc000000000000000000000000000000000000"
	if addr != expected {
		t.Fatalf("expected %s, got %s", expected, addr)
	}
}

func TestExtractChainAddressFromReportNil(t *testing.T) {
	addr := extractChainAddressFromReport(nil)
	if addr != "" {
		t.Fatalf("expected empty address, got %s", addr)
	}
}
