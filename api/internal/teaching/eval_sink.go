package teaching

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

// evalJobSink 将 evaluation.JobSink 接到内存/Redis 队列。
type evalJobSink struct {
	q queue.Enqueuer
}

func (s evalJobSink) Enqueue(ctx context.Context, jobType, idempotencyKey string, payload map[string]any) error {
	return s.q.Enqueue(ctx, queue.Job{
		ID:             store.NewID("job"),
		Type:           queue.JobType(jobType),
		IdempotencyKey: idempotencyKey,
		Payload:        payload,
	})
}

var _ evaluation.JobSink = evalJobSink{}
