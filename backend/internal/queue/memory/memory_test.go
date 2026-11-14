package memory

import (
	"context"
	"testing"
	"time"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/queue"
)

func next(t *testing.T, ch <-chan queue.Delivery) queue.Delivery {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(time.Second):
		t.Fatal("no delivery")
		return nil
	}
}

func TestPublishConsumeAck(t *testing.T) {
	q := New(8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := q.Consume(ctx, "evaluations")
	_ = q.Publish(ctx, queue.Message{Topic: "other", ID: "x"})
	_ = q.Publish(ctx, queue.Message{Topic: "evaluations", ID: "1"})

	d := next(t, ch)
	if d.Message().ID != "1" {
		t.Fatalf("topics must be independent, got %+v", d.Message())
	}
	if err := d.Ack(); err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-ch:
		t.Fatalf("acked message redelivered: %+v", d.Message())
	case <-time.After(20 * time.Millisecond):
	}
}

func TestNackRequeue(t *testing.T) {
	q := New(8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := q.Consume(ctx, "t")
	_ = q.Publish(ctx, queue.Message{Topic: "t", ID: "1"})
	_ = next(t, ch).Nack(true)
	if d := next(t, ch); d.Message().ID != "1" {
		t.Fatal("nack(requeue) must redeliver")
	}
}

func TestConsumeStopsOnCancel(t *testing.T) {
	q := New(1)
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := q.Consume(ctx, "t")
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("want closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after cancel")
	}
}
