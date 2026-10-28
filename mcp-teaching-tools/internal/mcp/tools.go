package mcp

import "context"

// ToolDesc MCP 工具描述。
type ToolDesc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListTools 至少提供选题要求的三个工具。
func ListTools() []ToolDesc {
	return []ToolDesc{
		{Name: "formula_verify", Description: "验证数学表达式、方程答案和步骤等价性"},
		{Name: "assignment_stats", Description: "按权限返回本班作业的匿名聚合统计（仅教师）"},
		{Name: "student_mistakes", Description: "返回当前学生本人的错题摘要；教师调用须限定本班"},
	}
}

// FormulaVerify TODO
func FormulaVerify(_ context.Context, _ map[string]any) (map[string]any, error) {
	panic("TODO: formula_verify")
}

// AssignmentStats TODO：仅教师；返回匿名聚合。
func AssignmentStats(_ context.Context, _ map[string]any) (map[string]any, error) {
	panic("TODO: assignment_stats")
}

// StudentMistakes TODO：学生仅本人；教师须限定本班。
func StudentMistakes(_ context.Context, _ map[string]any) (map[string]any, error) {
	panic("TODO: student_mistakes")
}
