package teaching

import (
	"context"
	"fmt"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/mcpclient"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/retrieval"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

// TaskStep 可恢复任务链路中的一步。
type TaskStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TaskRecord 技能任务状态（排队 / 处理中 / 待确认 / 成功 / 失败）。
type TaskRecord struct {
	ID      string     `json:"id"`
	Skill   SkillID    `json:"skill"`
	ClassID string     `json:"classId"`
	Status  string     `json:"status"`
	Steps   []TaskStep `json:"steps"`
}

// Pipeline 十二项功能共用任务链路接口。
type Pipeline interface {
	Start(ctx context.Context, skill SkillID, input map[string]any) (*TaskRecord, error)
	Resume(ctx context.Context, taskID string) (*TaskRecord, error)
	Confirm(ctx context.Context, taskID string, edits map[string]any) error
	Publish(ctx context.Context, taskID string) error
}

// PipelineDependencies are ports assembled by cmd/server（与远程工程框架对齐）。
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

// DefaultPipeline 内存实现。
type DefaultPipeline struct {
	deps PipelineDependencies
}

// NewPipeline 兼容远程装配签名；演示主路径仍使用内存实现。
func NewPipeline(deps PipelineDependencies) Pipeline {
	return &DefaultPipeline{deps: deps}
}

func toStoreTask(t TaskRecord) store.TaskRecord {
	steps := make([]store.TaskStep, len(t.Steps))
	for i, s := range t.Steps {
		steps[i] = store.TaskStep{Name: s.Name, Status: s.Status}
	}
	return store.TaskRecord{ID: t.ID, Skill: string(t.Skill), ClassID: t.ClassID, Status: t.Status, Steps: steps}
}

func fromStoreTask(t store.TaskRecord) TaskRecord {
	steps := make([]TaskStep, len(t.Steps))
	for i, s := range t.Steps {
		steps[i] = TaskStep{Name: s.Name, Status: s.Status}
	}
	return TaskRecord{ID: t.ID, Skill: SkillID(t.Skill), ClassID: t.ClassID, Status: t.Status, Steps: steps}
}

func (p *DefaultPipeline) Start(_ context.Context, skill SkillID, input map[string]any) (*TaskRecord, error) {
	classID, _ := input["classId"].(string)
	if classID == "" {
		classID = "class-demo"
	}
	id := store.NewID("task")
	t := TaskRecord{
		ID: id, Skill: skill, ClassID: classID, Status: "processing",
		Steps: []TaskStep{
			{Name: "auth_and_scope", Status: "done"},
			{Name: "retrieve_trusted_data", Status: "done"},
			{Name: "generate", Status: "done"},
			{Name: "verify", Status: "done"},
			{Name: "await_teacher_confirm", Status: "pending"},
		},
	}
	store.Default.SaveTask(toStoreTask(t))
	return &t, nil
}

func (p *DefaultPipeline) Resume(_ context.Context, taskID string) (*TaskRecord, error) {
	t, ok := store.Default.GetTask(taskID)
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	out := fromStoreTask(t)
	return &out, nil
}

func (p *DefaultPipeline) Confirm(_ context.Context, taskID string, edits map[string]any) error {
	t, ok := store.Default.GetTask(taskID)
	if !ok {
		return fmt.Errorf("not found")
	}
	contentID, _ := edits["contentId"].(string)
	if contentID != "" {
		c, ok := store.Default.GetContent(contentID)
		if !ok {
			return fmt.Errorf("not found")
		}
		if payload, ok := edits["payload"].(map[string]any); ok {
			for k, v := range payload {
				c.Payload[k] = v
			}
		}
		c.Status = string(StatusConfirmed)
		c.UpdatedAt = time.Now()
		store.Default.SaveContent(c)
	}
	rec := fromStoreTask(t)
	rec.Status = "success"
	for i := range rec.Steps {
		if rec.Steps[i].Name == "await_teacher_confirm" {
			rec.Steps[i].Status = "done"
		}
	}
	store.Default.SaveTask(toStoreTask(rec))
	return nil
}

func (p *DefaultPipeline) Publish(_ context.Context, taskID string) error {
	t, ok := store.Default.GetTask(taskID)
	if !ok {
		return fmt.Errorf("not found")
	}
	for _, c := range store.Default.ListContents(t.ClassID, t.Skill) {
		if c.Status == string(StatusConfirmed) || c.Status == string(StatusPending) {
			c.Status = string(StatusPublished)
			c.Payload["frozen"] = true
			c.UpdatedAt = time.Now()
			store.Default.SaveContent(c)
		}
	}
	rec := fromStoreTask(t)
	rec.Status = "success"
	rec.Steps = append(rec.Steps, TaskStep{Name: "publish_and_audit", Status: "done"})
	store.Default.SaveTask(toStoreTask(rec))
	return nil
}

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
	c.UpdatedAt = time.Now()
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
	c.UpdatedAt = time.Now()
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
	c.UpdatedAt = time.Now()
	store.Default.SaveContent(c)
	return nil
}
