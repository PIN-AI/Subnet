package consensus

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"subnet/internal/logging"
	"subnet/internal/messaging"
	pb "subnet/proto/subnet"
)

type readinessMessage struct {
	ValidatorID string `json:"validator_id"`
	Timestamp   int64  `json:"timestamp"`
}

// NATSBroadcaster implements Broadcaster using NATS
type NATSBroadcaster struct {
	bus           *messaging.NATSBus
	nc            *nats.Conn
	validatorID   string
	subnetID      string
	logger        logging.Logger
	subscriptions map[string]*nats.Subscription
	handlers      map[string]func([]byte)
	mu            sync.RWMutex
	closed        bool
}

// NewNATSBroadcaster creates a new NATS-based broadcaster
func NewNATSBroadcaster(url string, validatorID, subnetID string, logger logging.Logger) (*NATSBroadcaster, error) {
	// Connect to NATS with options
	nc, err := nats.Connect(url,
		nats.Name(fmt.Sprintf("validator-%s", validatorID)),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(10),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Error("NATS error", "error", err)
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			logger.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create messaging bus
	bus, err := messaging.NewNATSBus(url)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create NATS bus: %w", err)
	}

	return &NATSBroadcaster{
		bus:           bus,
		nc:            nc,
		validatorID:   validatorID,
		subnetID:      subnetID,
		logger:        logger,
		subscriptions: make(map[string]*nats.Subscription),
		handlers:      make(map[string]func([]byte)),
	}, nil
}

// BroadcastProposal broadcasts a checkpoint proposal to all validators
func (b *NATSBroadcaster) BroadcastProposal(header *pb.CheckpointHeader) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broadcaster is closed")
	}
	b.mu.RUnlock()

	data, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	subject := fmt.Sprintf("subnet.%s.checkpoint.proposal", b.subnetID)
	if err := b.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish proposal: %w", err)
	}

	// Flush to ensure message is sent
	if err := b.nc.Flush(); err != nil {
		b.logger.Warn("Failed to flush after proposal broadcast", "error", err)
	}

	b.logger.Info("Broadcasted checkpoint proposal",
		"epoch", header.Epoch,
		"subject", subject,
		"data_size", len(data))

	return nil
}

// BroadcastSignature broadcasts a signature to all validators
func (b *NATSBroadcaster) BroadcastSignature(sig *pb.Signature) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broadcaster is closed")
	}
	b.mu.RUnlock()

	data, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("failed to marshal signature: %w", err)
	}

	subject := fmt.Sprintf("subnet.%s.checkpoint.signature", b.subnetID)
	if err := b.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish signature: %w", err)
	}

	// Flush to ensure message is sent
	if err := b.nc.Flush(); err != nil {
		b.logger.Warn("Failed to flush after signature broadcast", "error", err)
	}

	b.logger.Info("Broadcasted signature",
		"signer", sig.SignerId,
		"subject", subject,
		"data_size", len(data))

	return nil
}

// BroadcastFinalized broadcasts a finalized checkpoint to all validators
func (b *NATSBroadcaster) BroadcastFinalized(header *pb.CheckpointHeader) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broadcaster is closed")
	}
	b.mu.RUnlock()

	data, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal finalized header: %w", err)
	}

	subject := fmt.Sprintf("subnet.%s.checkpoint.finalized", b.subnetID)
	if err := b.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish finalized: %w", err)
	}

	// Flush to ensure message is sent immediately
	if err := b.nc.Flush(); err != nil {
		b.logger.Warn("Failed to flush after finalized broadcast", "error", err)
	}

	b.logger.Info("Broadcasted finalized checkpoint",
		"epoch", header.Epoch,
		"subject", subject,
		"signatures", header.Signatures.SignatureCount)

	return nil
}

// BroadcastExecutionReport broadcasts an execution report to validators
func (b *NATSBroadcaster) BroadcastExecutionReport(report *pb.ExecutionReport) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broadcaster is closed")
	}
	b.mu.RUnlock()

	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	subject := fmt.Sprintf("subnet.%s.execution.report", b.subnetID)
	if err := b.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish report: %w", err)
	}

	b.logger.Debug("Broadcasted execution report",
		"report_id", report.ReportId,
		"agent", report.AgentId,
		"subject", subject)

	return nil
}

// BroadcastReadiness broadcasts validator readiness for consensus start
func (b *NATSBroadcaster) BroadcastReadiness(validatorID string) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broadcaster is closed")
	}
	b.mu.RUnlock()

	msg := readinessMessage{
		ValidatorID: validatorID,
		Timestamp:   time.Now().UnixNano(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal readiness: %w", err)
	}

	subject := fmt.Sprintf("subnet.%s.consensus.ready", b.subnetID)
	if err := b.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish readiness: %w", err)
	}

	if err := b.nc.Flush(); err != nil {
		b.logger.Warn("Failed to flush after readiness broadcast", "error", err)
	}

	b.logger.Debug("Broadcasted consensus readiness",
		"validator", validatorID,
		"subject", subject)

	return nil
}

// SubscribeToReadiness subscribes to consensus readiness announcements
func (b *NATSBroadcaster) SubscribeToReadiness(handler func(string)) error {
	subject := fmt.Sprintf("subnet.%s.consensus.ready", b.subnetID)

	sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
		var ready readinessMessage
		if err := json.Unmarshal(msg.Data, &ready); err != nil {
			b.logger.Error("Failed to unmarshal readiness", "error", err)
			return
		}

		handler(ready.ValidatorID)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to readiness: %w", err)
	}

	b.mu.Lock()
	b.subscriptions["readiness"] = sub
	b.mu.Unlock()

	b.logger.Info("Subscribed to consensus readiness", "subject", subject)
	return nil
}

// SubscribeToProposals subscribes to checkpoint proposals
func (b *NATSBroadcaster) SubscribeToProposals(handler func(*pb.CheckpointHeader)) error {
	subject := fmt.Sprintf("subnet.%s.checkpoint.proposal", b.subnetID)

	sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
		var header pb.CheckpointHeader
		if err := json.Unmarshal(msg.Data, &header); err != nil {
			b.logger.Error("Failed to unmarshal proposal", "error", err)
			return
		}

		b.logger.Info("Received checkpoint proposal",
			"epoch", header.Epoch,
			"subject", msg.Subject,
			"data_size", len(msg.Data))

		handler(&header)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to proposals: %w", err)
	}

	b.mu.Lock()
	b.subscriptions["proposals"] = sub
	b.mu.Unlock()

	b.logger.Info("Subscribed to checkpoint proposals", "subject", subject)
	return nil
}

// SubscribeToSignatures subscribes to checkpoint signatures
func (b *NATSBroadcaster) SubscribeToSignatures(handler func(*pb.Signature)) error {
	subject := fmt.Sprintf("subnet.%s.checkpoint.signature", b.subnetID)

	sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
		var sig pb.Signature
		if err := json.Unmarshal(msg.Data, &sig); err != nil {
			b.logger.Error("Failed to unmarshal signature", "error", err)
			return
		}

		// Skip our own signatures
		if sig.SignerId == b.validatorID {
			return
		}

		b.logger.Info("Received signature",
			"from", sig.SignerId,
			"subject", msg.Subject,
			"data_size", len(msg.Data))

		handler(&sig)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to signatures: %w", err)
	}

	b.mu.Lock()
	b.subscriptions["signatures"] = sub
	b.mu.Unlock()

	b.logger.Info("Subscribed to checkpoint signatures", "subject", subject)
	return nil
}

// SubscribeToFinalized subscribes to finalized checkpoints
func (b *NATSBroadcaster) SubscribeToFinalized(handler func(*pb.CheckpointHeader)) error {
	subject := fmt.Sprintf("subnet.%s.checkpoint.finalized", b.subnetID)

	sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
		var header pb.CheckpointHeader
		if err := json.Unmarshal(msg.Data, &header); err != nil {
			b.logger.Error("Failed to unmarshal finalized checkpoint", "error", err)
			return
		}

		b.logger.Info("Received finalized checkpoint",
			"epoch", header.Epoch,
			"subject", msg.Subject,
			"signatures", header.Signatures.SignatureCount)

		handler(&header)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to finalized: %w", err)
	}

	b.mu.Lock()
	b.subscriptions["finalized"] = sub
	b.mu.Unlock()

	b.logger.Info("Subscribed to finalized checkpoints", "subject", subject)
	return nil
}

// SubscribeToExecutionReports subscribes to execution reports
func (b *NATSBroadcaster) SubscribeToExecutionReports(handler func(*pb.ExecutionReport)) error {
	subject := fmt.Sprintf("subnet.%s.execution.report", b.subnetID)

	sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
		var report pb.ExecutionReport
		if err := json.Unmarshal(msg.Data, &report); err != nil {
			b.logger.Error("Failed to unmarshal report", "error", err)
			return
		}

		b.logger.Debug("Received execution report",
			"report_id", report.ReportId,
			"agent", report.AgentId,
			"subject", msg.Subject)

		handler(&report)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to reports: %w", err)
	}

	b.mu.Lock()
	b.subscriptions["reports"] = sub
	b.mu.Unlock()

	b.logger.Info("Subscribed to execution reports", "subject", subject)
	return nil
}

// RequestResponse sends a request and waits for response
func (b *NATSBroadcaster) RequestResponse(subject string, data []byte, timeout time.Duration) ([]byte, error) {
	msg, err := b.nc.Request(subject, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return msg.Data, nil
}

// Close closes all subscriptions and the connection
func (b *NATSBroadcaster) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	// Unsubscribe all
	for name, sub := range b.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			b.logger.Error("Failed to unsubscribe", "name", name, "error", err)
		}
	}

	// Close connection
	b.nc.Close()

	b.logger.Info("NATS broadcaster closed")
	return nil
}

// IsConnected checks if NATS is connected
func (b *NATSBroadcaster) IsConnected() bool {
	return b.nc.IsConnected()
}

// Flush flushes pending messages
func (b *NATSBroadcaster) Flush() error {
	return b.nc.Flush()
}
