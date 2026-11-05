package teaching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type TaskStatus string

const (
	TaskQueued       TaskStatus = "queued"
	TaskRunning      TaskStatus = "running"
	TaskPending      TaskStatus = "pending_confirmation"
	TaskSucceeded    TaskStatus = "succeeded"
	TaskFailed       TaskStatus = "failed"
	TaskNotStarted   TaskStatus = "not_started"
	TaskStepRunning  TaskStatus = "step_running"
	TaskStepFinished TaskStatus = "step_finished"
)

// TaskStep is one auditable step in a recoverable task.
type TaskStep struct {
	Name      string     `json:"name"`
	Status    TaskStatus `json:"status"`
	Attempt   int        `json:"attempt,omitempty"`
	ErrorCode string     `json:"errorCode,omitempty"`
}

// TaskRecord carries only framework state; skill-specific payloads use typed contracts.
type TaskRecord struct {
	ID             string     `json:"id"`
	Skill          SkillID    `json:"skill"`
	UserID         string     `json:"userId,omitempty"`
	ClassID        string     `json:"classId"`
	Status         TaskStatus `json:"status"`
	IdempotencyKey string     `json:"idempotencyKey,omitempty"`
	TraceID        string     `json:"traceId,omitempty"`
	ObjectID       string     `json:"objectId,omitempty"`
	Steps          []TaskStep `json:"steps,omitempty"`
}

// Pipeline 十二项功能共用任务链路接口。
// 流程：鉴权 → 编排计划 → 专业智能体 → 白名单工具 → Schema 结果 → 验证 → 教师确认 → 审计。
type Pipeline interface {
	Start(ctx context.Context, skill SkillID, input map[string]any) (*TaskRecord, error)
	Resume(ctx context.Context, taskID string) (*TaskRecord, error)
	Confirm(ctx context.Context, taskID string, edits map[string]any) error
	Publish(ctx context.Context, taskID string) error
}

// PipelineDependencies are ports assembled by cmd/server. Concrete adapters do
// not leak into the application package.
type PipelineDependencies struct {
	Queue queue.Enqueuer
}

type DefaultPipeline struct {
	deps PipelineDependencies
}

func NewPipeline(deps PipelineDependencies) Pipeline {
	return &DefaultPipeline{deps: deps}
}

var (
	ErrPipelineInvalid   = errors.New("invalid pipeline request")
	ErrPipelineForbidden = errors.New("pipeline request is forbidden")
	ErrPipelineNotFound  = errors.New("pipeline resource not found")
	ErrPipelineConflict  = errors.New("pipeline request conflicts with existing state")
	ErrPipelineQueue     = errors.New("pipeline queue is unavailable")
)

func (p *DefaultPipeline) Start(ctx context.Context, skill SkillID, input map[string]any) (*TaskRecord, error) {
	if skill != SkillVirtualClassroom {
		return nil, fmt.Errorf("%w: unsupported skill", ErrPipelineInvalid)
	}
	classID := strings.TrimSpace(stringInput(input, "classId"))
	userID := strings.TrimSpace(stringInput(input, "userId"))
	lessonPlanID := strings.TrimSpace(stringInput(input, "lessonPlanId"))
	idempotencyKey := strings.TrimSpace(stringInput(input, "idempotencyKey"))
	objective := strings.TrimSpace(stringInput(input, "objective"))
	previousID := strings.TrimSpace(stringInput(input, "previousSimulationId"))
	rounds := intInput(input, "rounds")
	if classID == "" || userID == "" || lessonPlanID == "" || idempotencyKey == "" || rounds < 1 || rounds > 10 {
		return nil, fmt.Errorf("%w: class, user, lessonPlanId, idempotencyKey and 1-10 rounds are required", ErrPipelineInvalid)
	}
	lesson, ok := store.Default.GetContent(lessonPlanID)
	if !ok || lesson.ClassID != classID || lesson.Skill != string(SkillLessonPlan) {
		return nil, ErrPipelineNotFound
	}
	if lesson.Status != string(StatusConfirmed) && lesson.Status != string(StatusPublished) {
		return nil, fmt.Errorf("%w: lesson plan must be confirmed first", ErrPipelineConflict)
	}
	if previousID != "" {
		previous, found := store.Default.GetContent(previousID)
		if !found || previous.ClassID != classID || previous.Skill != string(SkillVirtualClassroom) {
			return nil, ErrPipelineNotFound
		}
	}
	encoded, _ := json.Marshal(map[string]any{
		"lessonPlanId": lessonPlanID, "objective": objective,
		"rounds": rounds, "previousSimulationId": previousID,
	})
	taskID := store.NewID("task")
	task, duplicate, err := store.Default.ClaimIdempotency(
		userID, string(SkillVirtualClassroom), idempotencyKey, store.Digest(string(encoded)), taskID,
	)
	if errors.Is(err, store.ErrConflict) {
		return nil, ErrPipelineConflict
	}
	if err != nil {
		return nil, err
	}
	if duplicate {
		return teachingTask(task), nil
	}

	now := time.Now().UTC()
	contentID := store.NewID("simulation")
	report := store.ContentRecord{
		ID: contentID, Skill: string(SkillVirtualClassroom), ClassID: classID, Status: "queued",
		Payload: map[string]any{
			"label": "教学演练（非真实学生评价）", "lessonPlanId": lessonPlanID,
			"objective": objective, "rounds": rounds, "previousSimulationId": previousID,
			"roles": []map[string]any{
				{"name": "强基础", "boundary": "能迁移核心概念，可能暴露进阶追问"},
				{"name": "一般", "boundary": "掌握基本概念，但在条件辨析和步骤衔接上可能犹豫"},
				{"name": "薄弱", "boundary": "仅具备前置直觉，容易暴露概念混淆"},
			},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	store.Default.SaveContent(report)
	task.Skill = string(skill)
	task.ClassID = classID
	task.UserID = userID
	task.ObjectID = contentID
	task.Status = string(TaskQueued)
	task.Steps = []store.TaskStep{
		{Name: "validate_lesson_plan", Status: "succeeded"},
		{Name: "simulate_roles", Status: "pending"},
		{Name: "verify_report", Status: "pending"},
		{Name: "await_teacher_confirm", Status: "pending"},
	}
	store.Default.SaveTask(task)
	if err := store.Default.EnsureContentDurable(contentID); err != nil {
		task.Status = string(TaskFailed)
		task.Error = "content persistence unavailable"
		store.Default.SaveTask(task)
		return nil, fmt.Errorf("%w: persist simulation content: %v", ErrPipelineQueue, err)
	}
	if err := store.Default.EnsureTaskDurable(task.ID); err != nil {
		task.Status = string(TaskFailed)
		task.Error = "task persistence unavailable"
		store.Default.SaveTask(task)
		return nil, fmt.Errorf("%w: persist simulation task: %v", ErrPipelineQueue, err)
	}
	if p.deps.Queue == nil {
		task.Status = string(TaskFailed)
		task.Error = "queue is not configured"
		store.Default.SaveTask(task)
		return nil, ErrPipelineQueue
	}
	job := queue.Job{
		ID: store.NewID("job"), Type: queue.JobVirtualClassroom, IdempotencyKey: "virtual:" + taskID,
		Payload: map[string]any{
			"taskId": taskID, "contentId": contentID, "lessonPlanId": lessonPlanID,
			"objective": objective, "rounds": rounds, "previousSimulationId": previousID,
		},
	}
	if err := p.deps.Queue.Enqueue(ctx, job); err != nil {
		task.Status = string(TaskFailed)
		task.Error = err.Error()
		store.Default.SaveTask(task)
		return nil, fmt.Errorf("%w: %v", ErrPipelineQueue, err)
	}
	return teachingTask(task), nil
}

func (p *DefaultPipeline) Resume(_ context.Context, taskID string) (*TaskRecord, error) {
	task, ok := store.Default.GetTask(strings.TrimSpace(taskID))
	if !ok {
		return nil, ErrPipelineNotFound
	}
	return teachingTask(task), nil
}

func (p *DefaultPipeline) Confirm(_ context.Context, taskID string, edits map[string]any) error {
	task, ok := store.Default.GetTask(strings.TrimSpace(taskID))
	if !ok {
		return ErrPipelineNotFound
	}
	if err := ConfirmContent(task.ObjectID, edits); err != nil {
		return fmt.Errorf("%w: %v", ErrPipelineConflict, err)
	}
	task.Status = string(TaskPending)
	store.Default.SaveTask(task)
	return nil
}

func (p *DefaultPipeline) Publish(_ context.Context, taskID string) error {
	task, ok := store.Default.GetTask(strings.TrimSpace(taskID))
	if !ok {
		return ErrPipelineNotFound
	}
	if err := PublishContent(task.ObjectID); err != nil {
		return fmt.Errorf("%w: %v", ErrPipelineConflict, err)
	}
	task.Status = string(TaskSucceeded)
	store.Default.SaveTask(task)
	return nil
}

func stringInput(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func intInput(input map[string]any, key string) int {
	switch value := input[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func teachingTask(task store.TaskRecord) *TaskRecord {
	steps := make([]TaskStep, 0, len(task.Steps))
	for _, step := range task.Steps {
		steps = append(steps, TaskStep{Name: step.Name, Status: TaskStatus(step.Status)})
	}
	return &TaskRecord{
		ID: task.ID, Skill: SkillID(task.Skill), UserID: task.UserID, ClassID: task.ClassID,
		Status: TaskStatus(task.Status), IdempotencyKey: task.IdempotencyKey,
		ObjectID: task.ObjectID, Steps: steps,
	}
}

// ConfirmContent / PublishContent / RejectContent 供评价模块 HTTP 层复用。
func ConfirmContent(contentID string, edits map[string]any) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("not found")
	}
	if c.Status == string(StatusConfirmed) {
		return nil
	}
	if c.Status != "pending" {
		return fmt.Errorf("must be pending before confirm")
	}
	if len(edits) != 0 {
		return fmt.Errorf("shared content does not accept untyped edits")
	}
	c.Status = string(StatusConfirmed)
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return store.Default.EnsureContentDurable(c.ID)
}

func PublishContent(contentID string) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("not found")
	}
	if c.Status == string(StatusPublished) {
		return nil
	}
	if c.Status != string(StatusConfirmed) {
		return fmt.Errorf("must confirm before publish")
	}
	if c.Payload == nil {
		return fmt.Errorf("content payload is missing")
	}
	c.Status = string(StatusPublished)
	c.Payload["frozen"] = true
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return store.Default.EnsureContentDurable(c.ID)
}

func RejectContent(contentID string, reason string) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("not found")
	}
	if c.Status == string(StatusPublished) {
		return fmt.Errorf("published content is frozen")
	}
	if c.Status == string(StatusRejected) {
		return nil
	}
	if c.Status != "pending" && c.Status != string(StatusConfirmed) {
		return fmt.Errorf("content cannot be rejected from %s", c.Status)
	}
	if c.Payload == nil {
		c.Payload = map[string]any{}
	}
	c.Status = string(StatusRejected)
	c.Payload["rejectReason"] = reason
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return store.Default.EnsureContentDurable(c.ID)
}
