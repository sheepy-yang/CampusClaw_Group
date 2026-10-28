package agent

import "context"

// StructuredResult 专业智能体按 JSON Schema 返回的结构化结果。
type StructuredResult struct {
	Payload    map[string]any
	Confidence float64
	Citations  []Citation
	ModelMeta  ModelMeta
}

// Citation 可回溯引用（讲义切片 / 用户提交文本等）。
type Citation struct {
	SourceID string
	Snippet  string
	Score    float64
}

// ModelMeta 用于审计追溯的模型 / 提示词 / 技能版本。
type ModelMeta struct {
	Model         string
	PromptVersion string
	SkillVersion  string
}

// Orchestrator 统一编排智能体：生成有限步骤计划并驱动专业智能体。
type Orchestrator interface {
	Plan(ctx context.Context, goal string, skill string) ([]string, error)
	Run(ctx context.Context, plan []string, input map[string]any) (*StructuredResult, error)
}

// ContentAgent 教学内容智能体（命题 / 备课 / 预习等）。
type ContentAgent interface {
	Generate(ctx context.Context, input map[string]any) (*StructuredResult, error)
}

// VirtualStudentAgent 虚拟学生智能体。
type VirtualStudentAgent interface {
	Simulate(ctx context.Context, lessonPlanID string, roles []string, rounds int) (*StructuredResult, error)
}

// VerifierAgent 评价验证智能体：独立检查字段、引用、答案、人数门槛与内容安全。
type VerifierAgent interface {
	Verify(ctx context.Context, result *StructuredResult, rules VerifyRules) (ok bool, issues []string, err error)
}

// TutorAgent 个性化辅导智能体（追问策略 / 学习状态）。
type TutorAgent interface {
	NextTurn(ctx context.Context, state map[string]any, studentReply string) (*StructuredResult, error)
}

// VerifyRules 验证规则摘要。
type VerifyRules struct {
	RequireCitations bool
	MinSampleSize    int
	SchemaName       string
}
