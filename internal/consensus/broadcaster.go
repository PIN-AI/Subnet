package consensus

import (
	pb "subnet/proto/subnet"
)

// Broadcaster defines the interface for broadcasting consensus messages
type Broadcaster interface {
	// Checkpoint related broadcasts
	BroadcastProposal(header *pb.CheckpointHeader) error
	BroadcastSignature(sig *pb.Signature) error

	// Execution report broadcast
	BroadcastExecutionReport(report *pb.ExecutionReport) error

	// Subscriptions
	SubscribeToProposals(handler func(*pb.CheckpointHeader)) error
	SubscribeToSignatures(handler func(*pb.Signature)) error
	SubscribeToExecutionReports(handler func(*pb.ExecutionReport)) error

	// Lifecycle
	Close() error
	IsConnected() bool
	Flush() error
}