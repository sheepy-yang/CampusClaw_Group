package queue

import "context"

// JobType 异步任务类型。
type JobType string

const (
	JobVirtualClassroom JobType = "virtual_classroom"
	JobMultimodalGrade  JobType = "multimodal_grade"
	JobHandwritingOCR   JobType = "handwriting_ocr"
	JobSimilarity       JobType = "similarity"
	JobClassReview      JobType = "class_review"
	JobDiagnosis        JobType = "diagnosis"
	JobBulkProposition  JobType = "bulk_proposition"
)

// Job Redis 队列任务；IdempotencyKey 用于避免重复生成 / 评分 / 发布。
type Job struct {
	ID             string
	Type           JobType
	Payload        map[string]any
	IdempotencyKey string
}

// Enqueuer API 侧：先落库再投递。
type Enqueuer interface {
	Enqueue(ctx context.Context, job Job) error
}

// Worker 消费端骨架。
type Worker struct{}

func NewWorker() *Worker { return &Worker{} }

func (w *Worker) Run() error {
	// TODO: 连接 Redis，按 JobType 分发到对应 handler
	panic("TODO: queue.Worker.Run")
}

func (w *Worker) Handle(ctx context.Context, job Job) error {
	switch job.Type {
	case JobVirtualClassroom, JobMultimodalGrade, JobHandwritingOCR,
		JobSimilarity, JobClassReview, JobDiagnosis, JobBulkProposition:
		_ = ctx
		panic("TODO: queue.Worker.Handle " + string(job.Type))
	default:
		panic("TODO: unknown job type")
	}
}

// StubEnqueuer 骨架。
type StubEnqueuer struct{}

func (StubEnqueuer) Enqueue(_ context.Context, _ Job) error {
	panic("TODO: queue.Enqueuer.Enqueue")
}
