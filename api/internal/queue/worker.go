package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

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
	ID             string         `json:"id"`
	Type           JobType        `json:"type"`
	Payload        map[string]any `json:"payload"`
	IdempotencyKey string         `json:"idempotencyKey"`
}

// Enqueuer API 侧：先落库再投递。
type Enqueuer interface {
	Enqueue(ctx context.Context, job Job) error
}

// MemoryEnqueuer 进程内队列；后续可替换为 Redis LIST。
type MemoryEnqueuer struct {
	ch chan Job
}

var sharedQueue = NewMemoryQueue(64)

func Shared() *MemoryEnqueuer { return sharedQueue }

func NewMemoryQueue(n int) *MemoryEnqueuer {
	return &MemoryEnqueuer{ch: make(chan Job, n)}
}

func (e *MemoryEnqueuer) Enqueue(_ context.Context, job Job) error {
	select {
	case e.ch <- job:
		return nil
	default:
		go func() { e.ch <- job }()
		return nil
	}
}

// Worker 消费端。
type Worker struct {
	enqueuer *MemoryEnqueuer
	handlers map[JobType]func(context.Context, Job) error
}

func NewWorker() *Worker {
	w := &Worker{
		enqueuer: Shared(),
		handlers: map[JobType]func(context.Context, Job) error{},
	}
	w.handlers[JobVirtualClassroom] = handleVirtualClassroom
	w.handlers[JobMultimodalGrade] = handleMultimodalGrade
	w.handlers[JobHandwritingOCR] = handleHandwritingOCR
	w.handlers[JobSimilarity] = handleSimilarity
	w.handlers[JobClassReview] = handleClassReview
	w.handlers[JobDiagnosis] = handleDiagnosis
	w.handlers[JobBulkProposition] = handleBulkProposition
	return w
}

func (w *Worker) Run() error {
	log.Println("queue worker listening (in-memory)")
	for job := range w.enqueuer.ch {
		if job.IdempotencyKey != "" && !store.Default.MarkJobDone(job.IdempotencyKey) {
			log.Printf("skip duplicate job %s key=%s", job.ID, job.IdempotencyKey)
			continue
		}
		if err := w.Handle(context.Background(), job); err != nil {
			log.Printf("job %s (%s) failed: %v", job.ID, job.Type, err)
			updateTaskStatus(job, "failed", err.Error())
		}
	}
	return nil
}

// RunOnce 处理单任务（测试 / API 内嵌 worker）。
func (w *Worker) RunOnce(ctx context.Context, job Job) error {
	if job.IdempotencyKey != "" && !store.Default.MarkJobDone(job.IdempotencyKey) {
		return nil
	}
	return w.Handle(ctx, job)
}

func (w *Worker) Handle(ctx context.Context, job Job) error {
	h, ok := w.handlers[job.Type]
	if !ok {
		return errUnknownJob
	}
	updateTaskStatus(job, "processing", "")
	if err := h(ctx, job); err != nil {
		return err
	}
	updateTaskStatus(job, "pending", "") // 待教师确认
	return nil
}

var errUnknownJob = errString("unknown job type")

type errString string

func (e errString) Error() string { return string(e) }

func updateTaskStatus(job Job, status, detail string) {
	taskID, _ := job.Payload["taskId"].(string)
	if taskID == "" {
		return
	}
	t, ok := store.Default.GetTask(taskID)
	if !ok {
		return
	}
	t.Status = status
	if detail != "" {
		t.Steps = append(t.Steps, store.TaskStep{Name: "error", Status: detail})
	}
	store.Default.SaveTask(t)
}

// StubEnqueuer 兼容。
type StubEnqueuer = MemoryEnqueuer

var _ Enqueuer = (*MemoryEnqueuer)(nil)

// StartEmbedded 在 API 进程内启动后台 worker（演示部署简化）。
func StartEmbedded() {
	w := NewWorker()
	go func() {
		_ = w.Run()
	}()
}

var once sync.Once

func EnsureEmbedded() {
	once.Do(StartEmbedded)
}

func handleVirtualClassroom(_ context.Context, job Job) error {
	contentID, _ := job.Payload["contentId"].(string)
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return errString("content not found")
	}
	c.Status = "pending"
	c.Payload["drillReport"] = map[string]any{
		"label": "教学演练（非真实学生评价）",
		"roles": []map[string]any{
			{"name": "强基础", "boundary": "掌握消元，偶发计算失误"},
			{"name": "一般", "boundary": "理解概念但易混淆秩条件"},
			{"name": "薄弱", "boundary": "不清楚为何要变换增广矩阵"},
		},
		"dialogue": job.Payload["dialogue"],
		"suggestions": []string{"增加「为何同时变换」板书环节"},
		"generatedAt": time.Now().Format(time.RFC3339),
	}
	c.UpdatedAt = time.Now()
	store.Default.SaveContent(c)
	return nil
}

func handleMultimodalGrade(_ context.Context, job Job) error {
	subID, _ := job.Payload["submissionId"].(string)
	fail, _ := job.Payload["simulateFail"].(bool)
	if err := evaluation.ProcessMultimodalGrade(subID, fail); err != nil {
		// 模型失败已保留提交；仍标记任务失败以便重试
		return errString(err.Error())
	}
	return nil
}

func handleHandwritingOCR(ctx context.Context, job Job) error {
	return handleMultimodalGrade(ctx, job)
}

func handleSimilarity(_ context.Context, job Job) error {
	attemptID, _ := job.Payload["attemptId"].(string)
	return evaluation.ProcessSimilarity(attemptID)
}

func handleClassReview(_ context.Context, job Job) error {
	contentID, _ := job.Payload["contentId"].(string)
	var anon []string
	if raw, ok := job.Payload["anonymousInputs"].([]any); ok {
		for _, x := range raw {
			anon = append(anon, fmt.Sprint(x))
		}
	}
	if raw, ok := job.Payload["anonymousInputs"].([]string); ok {
		anon = raw
	}
	return evaluation.ProcessClassReview(contentID, anon)
}

func handleDiagnosis(_ context.Context, job Job) error {
	contentID, _ := job.Payload["contentId"].(string)
	studentID, _ := job.Payload["studentId"].(string)
	classID, _ := job.Payload["classId"].(string)
	sampleCount, _ := job.Payload["sampleCount"].(float64)
	if int(sampleCount) < evaluation.MinDiagnosisSamples {
		c, ok := store.Default.GetContent(contentID)
		if !ok {
			return errString("content not found")
		}
		c.Status = "pending"
		c.Payload["diagnosis"] = map[string]any{"message": "样本不足", "sampleCount": sampleCount}
		c.UpdatedAt = time.Now()
		store.Default.SaveContent(c)
		return nil
	}
	return evaluation.ProcessDiagnosis(contentID, studentID, classID)
}

func handleBulkProposition(_ context.Context, job Job) error {
	contentID, _ := job.Payload["contentId"].(string)
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return errString("content not found")
	}
	c.Status = "pending"
	c.UpdatedAt = time.Now()
	store.Default.SaveContent(c)
	return nil
}
