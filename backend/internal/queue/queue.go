// Package queue decouples requesting work from executing it. The in-memory
// implementation can be swapped for Kafka or RabbitMQ: Ack/Nack are part of
// the contract from day one. Messages carry ids only; the database is the
// source of truth, so redelivery is safe.
package queue

import "context"

type Message struct {
	Topic   string
	ID      string
	Payload []byte
}

type Delivery interface {
	Message() Message
	Ack() error
	Nack(requeue bool) error
}

type Queue interface {
	Publish(ctx context.Context, msg Message) error
	// Consume streams deliveries of a topic until ctx is cancelled.
	Consume(ctx context.Context, topic string) (<-chan Delivery, error)
}
