package teaching

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/mcpclient"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/retrieval"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
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
	Name      string
	Status    TaskStatus
	Attempt   int
	ErrorCode string
}

// TaskRecord carries only framework state; skill-specific payloads use typed contracts.
type TaskRecord struct {
	ID             string
	Skill          SkillID
	UserID         string
	ClassID        string
	Status         TaskStatus
	IdempotencyKey string
	TraceID        string
	Steps          []TaskStep
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
	Orchestrator   agent.Orchestrator
	ContentAgent   agent.ContentAgent
	VirtualStudent agent.VirtualStudentAgent
	Verifier       agent.VerifierAgent
	Model          gateway.Client
	Retriever      retrieval.Retriever
	MCP            mcpclient.Host
	Queue          queue.Enqueuer
	Audit          security.AuditLogger
}

type DefaultPipeline struct {
	deps PipelineDependencies
}

func NewPipeline(deps PipelineDependencies) Pipeline {
	return &DefaultPipeline{deps: deps}
}

func (p *DefaultPipeline) Start(_ context.Context, _ SkillID, _ map[string]any) (*TaskRecord, error) {
	return nil, apperrors.ErrNotImplemented
}

func (p *DefaultPipeline) Resume(_ context.Context, _ string) (*TaskRecord, error) {
	return nil, apperrors.ErrNotImplemented
}

func (p *DefaultPipeline) Confirm(_ context.Context, _ string, _ map[string]any) error {
	return apperrors.ErrNotImplemented
}

func (p *DefaultPipeline) Publish(_ context.Context, _ string) error {
	return apperrors.ErrNotImplemented
}
