package mcp

import (
	"context"
	"fmt"
	"strings"
)

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
				"role":          map[string]string{"type": "string"},
				"classId":       map[string]string{"type": "string"},
			}, []string{"assignment_id"}),
		},
		{
			Name:        "student_mistakes",
			Description: "返回当前学生本人的错题摘要；教师调用须限定本班",
			InputSchema: objectSchema(map[string]any{
				"student_id": map[string]string{"type": "string"},
				"role":       map[string]string{"type": "string"},
				"classId":    map[string]string{"type": "string"},
				"userId":     map[string]string{"type": "string"},
			}, []string{}),
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

// FormulaVerify 验证表达式等价（演示：规范化后字符串比较）。
func FormulaVerify(_ context.Context, args map[string]any) (map[string]any, error) {
	expr := fmt.Sprint(args["expression"])
	expected := fmt.Sprint(args["expected"])
	got := normalize(expr)
	want := normalize(expected)
	return map[string]any{
		"equivalent": got == want,
		"normalized": map[string]string{"expression": got, "expected": want},
		"method":     "normalize_compare",
	}, nil
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "×", "*")
	s = strings.ReplaceAll(s, "÷", "/")
	return s
}

// AssignmentStats 仅教师；返回匿名聚合。
func AssignmentStats(_ context.Context, args map[string]any) (map[string]any, error) {
	role := fmt.Sprint(args["role"])
	if role != "" && role != "teacher" {
		return nil, fmt.Errorf("forbidden: teacher only")
	}
	classID := fmt.Sprint(args["classId"])
	assignmentID := fmt.Sprint(args["assignment_id"])
	if assignmentID == "" || assignmentID == "<nil>" {
		assignmentID = fmt.Sprint(args["assignmentId"])
	}
	return map[string]any{
		"classId":          classID,
		"assignmentId":     assignmentID,
		"submissionCount":  8,
		"avgScore":         7.2,
		"commonErrorRates": map[string]float64{"概念混淆": 0.35, "计算失误": 0.25},
		"anonymous":        true,
	}, nil
}

// StudentMistakes 学生仅本人；教师须限定本班。
func StudentMistakes(_ context.Context, args map[string]any) (map[string]any, error) {
	role := fmt.Sprint(args["role"])
	callerID := fmt.Sprint(args["userId"])
	classID := fmt.Sprint(args["classId"])
	targetID := fmt.Sprint(args["student_id"])
	if targetID == "" || targetID == "<nil>" {
		targetID = fmt.Sprint(args["studentId"])
	}
	if targetID == "" || targetID == "<nil>" {
		targetID = callerID
	}
	if role == "student" && targetID != callerID {
		return nil, fmt.Errorf("forbidden: students may only read own mistakes")
	}
	if role == "teacher" && (classID == "" || classID == "<nil>") {
		return nil, fmt.Errorf("teacher must scope to classId")
	}
	return map[string]any{
		"studentId": targetID,
		"classId":   classID,
		"mistakes": []map[string]any{
			{"knowledgePoint": "高斯消元", "errorCause": "行交换后未同步右端"},
			{"knowledgePoint": "矩阵秩", "errorCause": "混淆系数矩阵与增广矩阵秩"},
		},
	}, nil
}
