package messaging

import (
	"io"

	"github.com/nats-io/nats.go"
)

type NATSBus struct{ nc *nats.Conn }

func NewNATSBus(url string, opts ...nats.Option) (*NATSBus, error) {
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	return &NATSBus{nc: nc}, nil
}

func (b *NATSBus) Publish(subject string, data []byte) error { return b.nc.Publish(subject, data) }

func (b *NATSBus) Subscribe(subject string, handler func([]byte)) (io.Closer, error) {
	sub, err := b.nc.Subscribe(subject, func(m *nats.Msg) { handler(m.Data) })
	if err != nil {
		return nil, err
	}
	return closerFunc(func() error { return sub.Unsubscribe() }), nil
}

type closerFunc func() error

func (c closerFunc) Close() error { return c() }
