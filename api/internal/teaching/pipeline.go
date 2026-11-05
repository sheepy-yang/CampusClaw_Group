package teaching

import (
	"context"
	"fmt"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/mcpclient"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/retrieval"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type TaskStatus string

const (
	TaskQueued      TaskStatus = "queued"
	TaskRunning     TaskStatus = "running"
	TaskPending     TaskStatus = "pending_confirmation"
	TaskSucceeded   TaskStatus = "succeeded"
	TaskFailed      TaskStatus = "failed"
	TaskNotStarted  TaskStatus = "not_started"
	TaskStepRunning TaskStatus = "step_running"
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

// 骨架阶段：通用 Pipeline 入口保持未实现，业务由杨瑾文评价路由直接调用模块服务。
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

// ConfirmContent / PublishContent / RejectContent 供评价模块 HTTP 层复用。
func ConfirmContent(contentID string, edits map[string]any) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("not found")
	}
	if edits != nil {
		for k, v := range edits {
			c.Payload[k] = v
		}
	}
	c.Status = string(StatusConfirmed)
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return nil
}

func PublishContent(contentID string) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("not found")
	}
	if c.Status != string(StatusConfirmed) {
		return fmt.Errorf("must confirm before publish")
	}
	c.Status = string(StatusPublished)
	c.Payload["frozen"] = true
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return nil
}

func RejectContent(contentID string, reason string) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("not found")
	}
	c.Status = string(StatusRejected)
	c.Payload["rejectReason"] = reason
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return nil
}
