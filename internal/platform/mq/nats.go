package mq

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
)

type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
	Close()
}

type natsPublisher struct {
	conn *nats.Conn
}

func NewPublisher(url string) (Publisher, error) {
	conn, err := nats.Connect(url, nats.Name("mmorp-server"))
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	return &natsPublisher{conn: conn}, nil
}

func (n *natsPublisher) Publish(_ context.Context, subject string, data []byte) error {
	return n.conn.Publish(subject, data)
}

func (n *natsPublisher) Close() {
	if n.conn != nil {
		n.conn.Drain()
		n.conn.Close()
	}
}

type noopPublisher struct{}

func NewNoopPublisher() Publisher {
	return noopPublisher{}
}

func (noopPublisher) Publish(context.Context, string, []byte) error { return nil }
func (noopPublisher) Close()                                        {}
