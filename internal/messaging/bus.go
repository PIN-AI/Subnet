package messaging

import (
	"io"
)

// Bus is a pluggable messaging interface for broadcast and subscription.
// Implementations may adapt NATS, libp2p, Kafka, etc.
type Bus interface {
	Publish(subject string, data []byte) error
	Subscribe(subject string, handler func([]byte)) (io.Closer, error)
}
