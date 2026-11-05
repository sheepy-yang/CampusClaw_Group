package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/metrics"
)

const (
	jobStream        = "campusclaw:v1:jobs"
	deadLetterStream = "campusclaw:v1:jobs:dead"
	jobGroup         = "campusclaw-workers"
)

type RedisBroker struct {
	client    *redis.Client
	consumer  string
	once      sync.Once
	initError error
	claimIdle time.Duration
}

func NewBrokerFromEnv() Broker {
	rawURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if rawURL == "" {
		if address := strings.TrimSpace(os.Getenv("REDIS_ADDR")); address != "" {
			rawURL = "redis://" + address + "/0"
		}
	}
	if rawURL == "" {
		return NewMemoryQueue(64)
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return NewMemoryQueue(64)
	}
	hostname, _ := os.Hostname()
	return &RedisBroker{
		client:    redis.NewClient(options),
		consumer:  fmt.Sprintf("%s-%d", hostname, os.Getpid()),
		claimIdle: 30 * time.Second,
	}
}

func (b *RedisBroker) ensure(ctx context.Context) error {
	b.once.Do(func() {
		err := b.client.XGroupCreateMkStream(ctx, jobStream, jobGroup, "0").Err()
		if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
			b.initError = err
		}
	})
	return b.initError
}

func (b *RedisBroker) Enqueue(ctx context.Context, job Job) error {
	if err := b.ensure(ctx); err != nil {
		return err
	}
	encoded, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if _, err := b.client.XAdd(ctx, &redis.XAddArgs{
		Stream: jobStream,
		Values: map[string]any{"job": string(encoded)},
	}).Result(); err != nil {
		return err
	}
	metrics.SetQueueDepth(b.Depth(ctx))
	return nil
}

func (b *RedisBroker) Dequeue(ctx context.Context) (Delivery, error) {
	if err := b.ensure(ctx); err != nil {
		return Delivery{}, err
	}
	for {
		claimIdle := b.claimIdle
		if claimIdle <= 0 {
			claimIdle = 30 * time.Second
		}
		messages, _, err := b.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: jobStream, Group: jobGroup, Consumer: b.consumer,
			MinIdle: claimIdle, Start: "0-0", Count: 1,
		}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return Delivery{}, err
		}
		if len(messages) > 0 {
			delivery, decodeErr := decodeDelivery(messages[0])
			if decodeErr == nil {
				return delivery, nil
			}
			if err := b.deadLetter(ctx, messages[0], decodeErr); err != nil {
				return Delivery{}, err
			}
			continue
		}

		streams, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: jobGroup, Consumer: b.consumer,
			Streams: []string{jobStream, ">"}, Count: 1, Block: 5 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			if ctx.Err() != nil {
				return Delivery{}, ctx.Err()
			}
			continue
		}
		if err != nil {
			return Delivery{}, err
		}
		if len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}
		message := streams[0].Messages[0]
		delivery, decodeErr := decodeDelivery(message)
		if decodeErr == nil {
			return delivery, nil
		}
		if err := b.deadLetter(ctx, message, decodeErr); err != nil {
			return Delivery{}, err
		}
	}
}

func (b *RedisBroker) deadLetter(ctx context.Context, message redis.XMessage, cause error) error {
	raw := fmt.Sprint(message.Values["job"])
	pipe := b.client.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: deadLetterStream,
		Values: map[string]any{
			"sourceStream": jobStream,
			"sourceId":     message.ID,
			"payload":      raw,
			"error":        cause.Error(),
			"failedAt":     time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	pipe.XAck(ctx, jobStream, jobGroup, message.ID)
	pipe.XDel(ctx, jobStream, message.ID)
	_, err := pipe.Exec(ctx)
	return err
}

func decodeDelivery(message redis.XMessage) (Delivery, error) {
	raw, ok := message.Values["job"]
	if !ok {
		return Delivery{}, errors.New("redis job message has no payload")
	}
	var job Job
	if err := json.Unmarshal([]byte(fmt.Sprint(raw)), &job); err != nil {
		return Delivery{}, fmt.Errorf("decode redis job: %w", err)
	}
	if job.ID == "" || job.Type == "" {
		return Delivery{}, errors.New("redis job message is invalid")
	}
	return Delivery{Job: job, Receipt: message.ID}, nil
}

func (b *RedisBroker) Ack(ctx context.Context, delivery Delivery) error {
	if delivery.Receipt == "" {
		return nil
	}
	pipe := b.client.TxPipeline()
	pipe.XAck(ctx, jobStream, jobGroup, delivery.Receipt)
	pipe.XDel(ctx, jobStream, delivery.Receipt)
	_, err := pipe.Exec(ctx)
	metrics.SetQueueDepth(b.Depth(ctx))
	return err
}

func (b *RedisBroker) Nack(ctx context.Context, delivery Delivery) error {
	if delivery.Receipt == "" {
		return b.Enqueue(ctx, delivery.Job)
	}
	encoded, err := json.Marshal(delivery.Job)
	if err != nil {
		return err
	}
	pipe := b.client.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{Stream: jobStream, Values: map[string]any{"job": string(encoded)}})
	pipe.XAck(ctx, jobStream, jobGroup, delivery.Receipt)
	pipe.XDel(ctx, jobStream, delivery.Receipt)
	_, err = pipe.Exec(ctx)
	metrics.SetQueueDepth(b.Depth(ctx))
	return err
}

func (b *RedisBroker) Depth(ctx context.Context) int {
	count, err := b.client.XLen(ctx, jobStream).Result()
	if err != nil {
		return 0
	}
	if count > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	value, _ := strconv.Atoi(strconv.FormatInt(count, 10))
	return value
}
