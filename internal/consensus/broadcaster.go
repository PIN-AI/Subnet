package consensus

import (
	pb "subnet/proto/subnet"
)

// Broadcaster defines the interface for broadcasting consensus messages
type Broadcaster interface {
	// Checkpoint related broadcasts
	BroadcastProposal(header *pb.CheckpointHeader) error
	BroadcastSignature(sig *pb.Signature) error
	BroadcastFinalized(header *pb.CheckpointHeader) error // Broadcast finalized checkpoint

	// Execution report broadcast
	BroadcastExecutionReport(report *pb.ExecutionReport) error

	// Subscriptions
	SubscribeToProposals(handler func(*pb.CheckpointHeader)) error
	SubscribeToSignatures(handler func(*pb.Signature)) error
	SubscribeToFinalized(handler func(*pb.CheckpointHeader)) error // Subscribe to finalized checkpoints
	SubscribeToExecutionReports(handler func(*pb.ExecutionReport)) error
	SubscribeToReadiness(handler func(string)) error
	BroadcastReadiness(validatorID string) error

	// Lifecycle
	Close() error
	IsConnected() bool
	Flush() error
}
