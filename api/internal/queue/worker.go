package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/metrics"
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
	JobAssignmentGrade  JobType = "assignment_grade"
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

type Delivery struct {
	Job
	Receipt string
}

type Broker interface {
	Enqueuer
	Dequeue(ctx context.Context) (Delivery, error)
	Ack(ctx context.Context, delivery Delivery) error
	Nack(ctx context.Context, delivery Delivery) error
	Depth(ctx context.Context) int
}

// MemoryEnqueuer is the deterministic local/test broker.
type MemoryEnqueuer struct {
	ch chan Job
}

var sharedQueue Broker = NewBrokerFromEnv()

func Shared() Broker { return sharedQueue }

func NewMemoryQueue(n int) *MemoryEnqueuer {
	return &MemoryEnqueuer{ch: make(chan Job, n)}
}

func (e *MemoryEnqueuer) Enqueue(ctx context.Context, job Job) error {
	select {
	case e.ch <- job:
		metrics.SetQueueDepth(len(e.ch))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrQueueFull
	}
}

var ErrQueueFull = errors.New("queue is full")

func (e *MemoryEnqueuer) Dequeue(ctx context.Context) (Delivery, error) {
	select {
	case job := <-e.ch:
		metrics.SetQueueDepth(len(e.ch))
		return Delivery{Job: job}, nil
	case <-ctx.Done():
		return Delivery{}, ctx.Err()
	}
}

func (e *MemoryEnqueuer) Ack(_ context.Context, _ Delivery) error { return nil }

func (e *MemoryEnqueuer) Nack(ctx context.Context, delivery Delivery) error {
	return e.Enqueue(ctx, delivery.Job)
}

func (e *MemoryEnqueuer) Depth(_ context.Context) int { return len(e.ch) }

// Worker 消费端。
type Worker struct {
	enqueuer Broker
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
	w.handlers[JobAssignmentGrade] = handleAssignmentGrade
	return w
}

// Run consumes durable Redis Stream deliveries in Compose and in-memory
// deliveries in local tests. Redis deliveries remain pending until Ack.
func (w *Worker) Run(ctx context.Context) error {
	log.Println("queue worker listening")
	for {
		delivery, err := w.enqueuer.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("queue receive failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		job := delivery.Job
		metrics.SetQueueDepth(w.enqueuer.Depth(ctx))
		if job.IdempotencyKey != "" {
			done, checkErr := store.Default.JobDone(job.IdempotencyKey)
			if checkErr != nil {
				log.Printf("job completion check failed for %s: %v", job.ID, checkErr)
				time.Sleep(500 * time.Millisecond)
				_ = w.enqueuer.Nack(ctx, delivery)
				continue
			}
			if done {
				log.Printf("skip duplicate job %s key=%s", job.ID, job.IdempotencyKey)
				_ = w.enqueuer.Ack(ctx, delivery)
				continue
			}
		}
		if err := w.Handle(context.Background(), job); err != nil {
			log.Printf("job %s (%s) failed: %v", job.ID, job.Type, err)
			if taskID := jobTaskID(job); taskID != "" {
				if persistErr := store.Default.EnsureTaskDurable(taskID); persistErr != nil {
					log.Printf("durable failed-task write failed for %s: %v", job.ID, persistErr)
					time.Sleep(500 * time.Millisecond)
					_ = w.enqueuer.Nack(ctx, delivery)
					continue
				}
			}
			_ = w.enqueuer.Ack(ctx, delivery)
			continue
		}
		if err := ensureJobDurable(job); err != nil {
			log.Printf("durable job result write failed for %s: %v", job.ID, err)
			time.Sleep(500 * time.Millisecond)
			_ = w.enqueuer.Nack(ctx, delivery)
			continue
		}
		if job.IdempotencyKey != "" {
			if _, err := store.Default.MarkJobDone(job.IdempotencyKey); err != nil {
				log.Printf("durable job completion write failed for %s: %v", job.ID, err)
				time.Sleep(500 * time.Millisecond)
				_ = w.enqueuer.Nack(ctx, delivery)
				continue
			}
		}
		if err := w.enqueuer.Ack(ctx, delivery); err != nil {
			log.Printf("queue ack failed for %s: %v", job.ID, err)
		}
	}
}

func jobTaskID(job Job) string {
	taskID, _ := job.Payload["taskId"].(string)
	return taskID
}

// RunOnce 处理单任务（测试 / API 内嵌 worker）。
func (w *Worker) RunOnce(ctx context.Context, job Job) error {
	if job.IdempotencyKey != "" {
		done, err := store.Default.JobDone(job.IdempotencyKey)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if err := w.Handle(ctx, job); err != nil {
		return err
	}
	if err := ensureJobDurable(job); err != nil {
		return err
	}
	if job.IdempotencyKey != "" {
		if _, err := store.Default.MarkJobDone(job.IdempotencyKey); err != nil {
			return err
		}
	}
	return nil
}

func ensureJobDurable(job Job) error {
	if taskID := jobTaskID(job); taskID != "" {
		if err := store.Default.EnsureTaskDurable(taskID); err != nil {
			return err
		}
	}
	if submissionID, _ := job.Payload["submissionId"].(string); submissionID != "" {
		if err := store.Default.EnsureSubmissionDurable(submissionID); err != nil {
			return err
		}
	}
	if contentID, _ := job.Payload["contentId"].(string); contentID != "" {
		if err := store.Default.EnsureContentDurable(contentID); err != nil {
			return err
		}
	}
	if job.Type == JobSimilarity {
		attemptID, _ := job.Payload["attemptId"].(string)
		if err := store.Default.EnsureSimilarityDurable(attemptID); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) Handle(ctx context.Context, job Job) error {
	h, ok := w.handlers[job.Type]
	if !ok {
		return errUnknownJob
	}
	updateTaskStatus(job, "running", "")
	if err := h(ctx, job); err != nil {
		updateTaskStatus(job, "failed", err.Error())
		return err
	}
	status := "pending_confirmation" // AI 生成内容待教师确认
	if job.Type == JobAssignmentGrade || job.Type == JobSimilarity {
		status = "succeeded"
	}
	updateTaskStatus(job, status, "")
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
	for i := range t.Steps {
		if status == "running" && t.Steps[i].Status == "pending" {
			t.Steps[i].Status = "running"
			break
		}
		if status == "pending_confirmation" {
			if t.Steps[i].Name == "await_teacher_confirm" {
				t.Steps[i].Status = "pending"
			} else if t.Steps[i].Status == "pending" || t.Steps[i].Status == "running" {
				t.Steps[i].Status = "succeeded"
			}
		}
		if status == "succeeded" && (t.Steps[i].Status == "pending" || t.Steps[i].Status == "running") {
			t.Steps[i].Status = "succeeded"
		}
	}
	if detail != "" {
		t.Error = detail
		t.Steps = append(t.Steps, store.TaskStep{Name: "error", Status: detail})
	}
	store.Default.SaveTask(t)
}

var _ Enqueuer = (*MemoryEnqueuer)(nil)

// StartEmbedded 在 API 进程内启动后台 worker（演示部署简化）。
func StartEmbedded() {
	w := NewWorker()
	go func() {
		_ = w.Run(context.Background())
	}()
}

var once sync.Once

func EnsureEmbedded() {
	if _, ok := Shared().(*MemoryEnqueuer); ok {
		once.Do(StartEmbedded)
	}
}

func handleVirtualClassroom(ctx context.Context, job Job) error {
	contentID, _ := job.Payload["contentId"].(string)
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return errString("content not found")
	}
	lessonPlanID, _ := job.Payload["lessonPlanId"].(string)
	lesson, ok := store.Default.GetContent(lessonPlanID)
	if !ok || lesson.ClassID != c.ClassID || lesson.Skill != "lesson_plan" {
		return errString("confirmed lesson plan not found")
	}
	rounds := 3
	switch value := job.Payload["rounds"].(type) {
	case int:
		rounds = value
	case float64:
		rounds = int(value)
	}
	if rounds < 1 || rounds > 10 {
		return errString("invalid simulation rounds")
	}
	lessonSnapshot, err := json.Marshal(map[string]any{
		"id": lesson.ID, "payload": lesson.Payload, "objective": job.Payload["objective"],
	})
	if err != nil {
		return errString("lesson plan cannot be serialized")
	}
	roles := []string{"强基础", "一般", "薄弱"}
	simulator := &agent.DefaultVirtualStudentAgent{LLM: gateway.NewClient()}
	result, err := simulator.Simulate(ctx, string(lessonSnapshot), roles, rounds)
	if err != nil {
		return err
	}
	if result == nil || len(result.Payload) == 0 || objectArrayLength(result.Payload["dialogue"]) == 0 {
		return errString("simulation model returned an invalid report")
	}
	report := map[string]any{
		"label":         "教学演练（非真实学生评价）",
		"dialogue":      result.Payload["dialogue"],
		"risks":         result.Payload["risks"],
		"suggestions":   result.Payload["suggestions"],
		"model":         result.ModelMeta.Model,
		"promptVersion": result.ModelMeta.PromptVersion,
		"skillVersion":  result.ModelMeta.SkillVersion,
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
	}
	if mode, exists := result.Payload["generationMode"]; exists {
		report["generationMode"] = mode
	}
	previousID, _ := job.Payload["previousSimulationId"].(string)
	if previousID != "" {
		if previous, found := store.Default.GetContent(previousID); found && previous.ClassID == c.ClassID {
			if previousReport, valid := previous.Payload["drillReport"].(map[string]any); valid {
				report["comparison"] = map[string]any{
					"previousSimulationId": previousID,
					"previousRiskCount":    objectArrayLength(previousReport["risks"]),
					"currentRiskCount":     objectArrayLength(result.Payload["risks"]),
				}
			}
		}
	}
	c.Status = "pending"
	c.Payload["drillReport"] = report
	c.UpdatedAt = time.Now()
	store.Default.SaveContent(c)
	return nil
}

func objectArrayLength(value any) int {
	switch items := value.(type) {
	case []map[string]any:
		return len(items)
	case []any:
		return len(items)
	default:
		return 0
	}
}

func handleMultimodalGrade(ctx context.Context, job Job) error {
	subID, _ := job.Payload["submissionId"].(string)
	fail, _ := job.Payload["simulateFail"].(bool)
	var transcriber evaluation.ImageTranscriber
	if submission, ok := store.Default.GetSubmission(subID); ok && strings.TrimSpace(submission.ImageDigest) != "" {
		configured, err := evaluation.NewGatewayImageTranscriberFromEnv()
		if err != nil {
			return errString(err.Error())
		}
		transcriber = configured
	}
	if err := evaluation.ProcessMultimodalGradeWithTranscriber(ctx, subID, fail, transcriber); err != nil {
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

func handleClassReview(ctx context.Context, job Job) error {
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
	return evaluation.ProcessClassReviewWithModel(ctx, contentID, anon, gateway.NewClient())
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

func handleAssignmentGrade(_ context.Context, job Job) error {
	submissionID, _ := job.Payload["submissionId"].(string)
	submission, ok := store.Default.GetSubmission(submissionID)
	if !ok {
		return errString("submission not found")
	}
	assignment, ok := store.Default.GetAssignment(submission.AssignmentID)
	if !ok || assignment.ClassID != submission.ClassID {
		return errString("assignment not found")
	}
	maxScore := assignment.MaxScore
	if maxScore <= 0 {
		maxScore = 10
	}
	submission.MaxScore = maxScore
	if len(assignment.Items) == 0 {
		submission.Status = "failed"
		store.Default.SaveSubmission(submission)
		return errString("assignment contains no items")
	}
	perItem := maxScore / float64(len(assignment.Items))
	score := 0.0
	firstWrong := -1
	hasSubjective := false
	confidence := 1.0
	itemOutcomes := make([]store.LearningOutcome, len(assignment.Items))
	steps := make([]map[string]any, 0, len(assignment.Items))
	for i, item := range assignment.Items {
		answer := submission.Answers[item.ID]
		if answer == "" && len(assignment.Items) == 1 {
			answer = submission.AnswerText
		}
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "choice", "single", "multi", "fill", "blank":
			correct := normalizeAnswer(answer) == normalizeAnswer(item.Answer)
			if correct {
				score += perItem
				itemOutcomes[i] = store.LearningOutcomeCorrect
			} else {
				if firstWrong < 0 {
					firstWrong = i
				}
				itemOutcomes[i] = store.LearningOutcomeIncorrect
			}
			steps = append(steps, map[string]any{
				"index": i + 1, "text": answer, "questionId": item.ID, "method": "deterministic_rule",
				"verified": true, "ok": correct, "correct": correct, "maxScore": perItem,
			})
		default:
			hasSubjective = true
			rubric := strings.TrimSpace(item.Answer + "\n" + item.Explanation)
			suggested, itemConfidence, cause := evaluation.SuggestSubjectiveScoreFromRubric(answer, rubric, perItem)
			score += suggested
			if itemConfidence < confidence {
				confidence = itemConfidence
			}
			itemOutcomes[i] = store.LearningOutcomeUnknown
			steps = append(steps, map[string]any{
				"index": i + 1, "text": answer, "questionId": item.ID, "method": "rubric_similarity",
				"verified": false, "suggestedScore": suggested, "maxScore": perItem,
				"comment": "主观题建议分必须由教师复核",
			})
			if submission.ErrorCause == "" {
				submission.ErrorCause = cause
			}
		}
	}
	submission.Steps = steps
	submission.Suggested = &score
	if hasSubjective {
		submission.Score = nil
		submission.Status = evaluation.SubNeedsTeacherReview
		submission.Confidence = confidence
		submission.NeedsReview = true
		submission.Confirmed = true
	} else {
		submission.Score = &score
		submission.Status = "graded"
		submission.Confidence = 1
		submission.NeedsReview = false
	}
	store.Default.SaveSubmission(submission)
	if firstWrong >= 0 {
		item := assignment.Items[firstWrong]
		knowledgePoint := strings.TrimSpace(item.KnowledgePoint)
		if knowledgePoint == "" {
			knowledgePoint = assignment.Title
		}
		if err := store.Default.AppendMistake(store.Mistake{
			ID: store.NewID("mistake"), StudentID: submission.StudentID, ClassID: submission.ClassID,
			AssignmentID: assignment.ID, SubmissionID: submission.ID, KnowledgePoint: knowledgePoint,
			QuestionStem: item.Stem, ErrorCause: "答案与已发布评分规则不一致",
			Hint: "回到题干条件，逐步核对使用的定义和计算过程。", StandardAnswer: item.Answer,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return errString("mistake persistence failed")
		}
	}
	eventTime := submission.SubmittedAt
	if eventTime.IsZero() {
		eventTime = time.Now().UTC()
	}
	for index, item := range assignment.Items {
		knowledgePoint := strings.TrimSpace(item.KnowledgePoint)
		if knowledgePoint == "" {
			knowledgePoint = assignment.Title
		}
		if err := store.Default.AppendEvent(store.LearningEvent{
			ID:        "event-" + store.Digest(submission.ID+"|"+item.ID),
			StudentID: submission.StudentID, ClassID: submission.ClassID,
			Kind: "assignment", Summary: assignment.Title + " · " + item.Stem,
			KP: knowledgePoint, Outcome: itemOutcomes[index], At: eventTime,
		}); err != nil {
			return errString("learning event persistence failed")
		}
	}
	return nil
}

func normalizeAnswer(answer string) string {
	return strings.ToLower(strings.Join(strings.Fields(answer), ""))
}
