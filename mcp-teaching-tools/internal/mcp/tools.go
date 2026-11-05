package mcp

import (
	"context"
	"errors"
)

var ErrNotImplemented = errors.New("tool execution is not implemented in the skeleton")

// ToolDesc MCP 工具描述。
type ToolDesc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ListTools 至少提供选题要求的三个工具。
func ListTools() []ToolDesc {
	return []ToolDesc{
		{
			Name:        "formula_verify",
			Description: "验证数学表达式、方程答案和步骤等价性",
			InputSchema: objectSchema(map[string]any{
				"expression": map[string]string{"type": "string"},
				"expected":   map[string]string{"type": "string"},
			}, []string{"expression", "expected"}),
		},
		{
			Name:        "assignment_stats",
			Description: "按权限返回本班作业的匿名聚合统计（仅教师）",
			InputSchema: objectSchema(map[string]any{
				"assignment_id": map[string]string{"type": "string"},
			}, []string{"assignment_id"}),
		},
		{
			Name:        "student_mistakes",
			Description: "返回当前学生本人的错题摘要；教师调用须限定本班",
			InputSchema: objectSchema(map[string]any{
				"student_id": map[string]string{"type": "string"},
			}, []string{"student_id"}),
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

// FormulaVerify is a declared tool boundary; business execution is intentionally absent.
func FormulaVerify(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, ErrNotImplemented
}

// AssignmentStats is a declared teacher-only boundary.
func AssignmentStats(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, ErrNotImplemented
}

// StudentMistakes is a declared current-user/current-class boundary.
func StudentMistakes(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, ErrNotImplemented
}
