package queue

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisBrokerRecoversUnackedDelivery(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin := redis.NewClient(options)
	t.Cleanup(func() {
		_ = admin.FlushDB(context.Background()).Err()
		_ = admin.Close()
	})
	if err := admin.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	first := &RedisBroker{client: redis.NewClient(options), consumer: "worker-first", claimIdle: time.Millisecond}
	t.Cleanup(func() { _ = first.client.Close() })
	job := Job{ID: "job-1", Type: JobAssignmentGrade, Payload: map[string]any{"taskId": "task-1"}}
	if err := first.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	delivery, err := first.Dequeue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.ID != job.ID || delivery.Receipt == "" {
		t.Fatalf("unexpected first delivery: %#v", delivery)
	}

	time.Sleep(5 * time.Millisecond)
	second := &RedisBroker{client: redis.NewClient(options), consumer: "worker-second", claimIdle: time.Millisecond}
	t.Cleanup(func() { _ = second.client.Close() })
	recovered, err := second.Dequeue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != job.ID || recovered.Receipt != delivery.Receipt {
		t.Fatalf("unacked job was not recovered: first=%#v recovered=%#v", delivery, recovered)
	}
	if err := second.Ack(ctx, recovered); err != nil {
		t.Fatal(err)
	}
	if depth := second.Depth(ctx); depth != 0 {
		t.Fatalf("queue depth after ack = %d", depth)
	}
}

func TestRedisBrokerDeadLettersMalformedMessageAndContinues(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin := redis.NewClient(options)
	t.Cleanup(func() {
		_ = admin.FlushDB(context.Background()).Err()
		_ = admin.Close()
	})
	if err := admin.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	broker := &RedisBroker{client: redis.NewClient(options), consumer: "worker-dead-letter", claimIdle: time.Millisecond}
	t.Cleanup(func() { _ = broker.client.Close() })
	if err := broker.ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if err := admin.XAdd(ctx, &redis.XAddArgs{Stream: jobStream, Values: map[string]any{"job": "{"}}).Err(); err != nil {
		t.Fatal(err)
	}
	valid := Job{ID: "job-valid", Type: JobAssignmentGrade, Payload: map[string]any{"taskId": "task-valid"}}
	if err := broker.Enqueue(ctx, valid); err != nil {
		t.Fatal(err)
	}
	delivery, err := broker.Dequeue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.ID != valid.ID {
		t.Fatalf("delivery = %#v", delivery)
	}
	if count, err := admin.XLen(ctx, deadLetterStream).Result(); err != nil || count != 1 {
		t.Fatalf("dead-letter count=%d err=%v", count, err)
	}
}
