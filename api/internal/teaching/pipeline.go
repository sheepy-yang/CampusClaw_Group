package teaching

import "context"

// TaskStep 可恢复任务链路中的一步。
type TaskStep struct {
	Name   string
	Status string
}

// TaskRecord 技能任务状态（排队 / 处理中 / 待确认 / 成功 / 失败）。
type TaskRecord struct {
	ID      string
	Skill   SkillID
	ClassID string
	Status  string
	Steps   []TaskStep
}

// Pipeline 十二项功能共用任务链路接口。
// 流程：鉴权 → 编排计划 → 专业智能体 → 白名单工具 → Schema 结果 → 验证 → 教师确认 → 审计。
type Pipeline interface {
	Start(ctx context.Context, skill SkillID, input map[string]any) (*TaskRecord, error)
	Resume(ctx context.Context, taskID string) (*TaskRecord, error)
	Confirm(ctx context.Context, taskID string, edits map[string]any) error
	Publish(ctx context.Context, taskID string) error
}

// DefaultPipeline 骨架实现。
type DefaultPipeline struct{}

func NewPipeline() Pipeline { return &DefaultPipeline{} }

func (p *DefaultPipeline) Start(_ context.Context, _ SkillID, _ map[string]any) (*TaskRecord, error) {
	panic("TODO: teaching.Pipeline.Start")
}

func (p *DefaultPipeline) Resume(_ context.Context, _ string) (*TaskRecord, error) {
	panic("TODO: teaching.Pipeline.Resume")
}

func (p *DefaultPipeline) Confirm(_ context.Context, _ string, _ map[string]any) error {
	panic("TODO: teaching.Pipeline.Confirm")
}

func (p *DefaultPipeline) Publish(_ context.Context, _ string) error {
	panic("TODO: teaching.Pipeline.Publish")
}
