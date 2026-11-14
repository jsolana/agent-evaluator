// Package memory implements queue.Queue with buffered channels, one per topic.
package memory

import (
	"context"
	"sync"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/queue"
)

type Queue struct {
	mu     sync.Mutex
	size   int
	topics map[string]chan queue.Message
}

var _ queue.Queue = (*Queue)(nil)

func New(buffer int) *Queue { return &Queue{size: buffer, topics: map[string]chan queue.Message{}} }

func (q *Queue) topic(name string) chan queue.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	ch, ok := q.topics[name]
	if !ok {
		ch = make(chan queue.Message, q.size)
		q.topics[name] = ch
	}
	return ch
}

func (q *Queue) Publish(ctx context.Context, msg queue.Message) error {
	select {
	case q.topic(msg.Topic) <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *Queue) Consume(ctx context.Context, name string) (<-chan queue.Delivery, error) {
	in := q.topic(name)
	out := make(chan queue.Delivery)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-in:
				select {
				case out <- &delivery{q: q, msg: m}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

type delivery struct {
	q   *Queue
	msg queue.Message
}

func (d *delivery) Message() queue.Message { return d.msg }
func (d *delivery) Ack() error             { return nil }
func (d *delivery) Nack(requeue bool) error {
	if !requeue {
		return nil
	}
	go func() { _ = d.q.Publish(context.Background(), d.msg) }()
	return nil
}
