package mcp

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestFormulaVerifyIsConservative(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		expected   string
		verified   bool
		status     string
	}{
		{name: "numeric equivalent", expression: "(1 + 2) * 3", expected: "9", verified: true, status: "verified"},
		{name: "standard unary precedence", expression: "-2^2", expected: "-4", verified: true, status: "verified"},
		{name: "numeric mismatch", expression: "2^3", expected: "9", verified: false, status: "not_equivalent"},
		{name: "exact symbolic normalization", expression: "x + 1", expected: "x+1", verified: true, status: "verified"},
		{name: "unsupported symbolic comparison", expression: "x+1", expected: "1+x", verified: false, status: "indeterminate"},
		{name: "division by zero", expression: "1/0", expected: "0", verified: false, status: "indeterminate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ExecuteTool(context.Background(), "formula_verify", map[string]any{
				"expression": test.expression, "expected": test.expected,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result["verified"] != test.verified || result["status"] != test.status {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestAssignmentStatsUsesOnlyProvidedAnonymousScores(t *testing.T) {
	result, err := ExecuteTool(context.Background(), "assignment_stats", map[string]any{
		"assignment_id": "assignment-1",
		"scores":        []any{float64(100), float64(80), float64(50), float64(70)},
		"passing_score": float64(60),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["count"] != 4 || result["average"] != 75.0 || result["median"] != 75.0 ||
		result["minimum"] != 50.0 || result["maximum"] != 100.0 || result["passRate"] != 0.75 {
		t.Fatalf("statistics = %#v", result)
	}
}

func TestStudentMistakesSummarizesOnlyInput(t *testing.T) {
	result, err := ExecuteTool(context.Background(), "student_mistakes", map[string]any{
		"student_id": "stu-1",
		"mistakes": []any{
			map[string]any{"knowledge_point": "高斯消元", "error_cause": "漏变换右端"},
			map[string]any{"knowledge_point": "高斯消元", "question_summary": "化简矩阵"},
			map[string]any{"knowledge_point": "矩阵秩"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["total"] != 3 {
		t.Fatalf("result = %#v", result)
	}
	want := []map[string]any{
		{"knowledgePoint": "矩阵秩", "count": 1},
		{"knowledgePoint": "高斯消元", "count": 2},
	}
	if !reflect.DeepEqual(result["byKnowledgePoint"], want) {
		t.Fatalf("byKnowledgePoint = %#v, want %#v", result["byKnowledgePoint"], want)
	}
}

func TestToolSchemaRejectsMissingExtraAndOutOfRangeArguments(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "missing", tool: "formula_verify", args: map[string]any{"expression": "1"}},
		{name: "extra", tool: "formula_verify", args: map[string]any{"expression": "1", "expected": "1", "secret": "x"}},
		{name: "score range", tool: "assignment_stats", args: map[string]any{
			"assignment_id": "a", "scores": []any{float64(101)},
		}},
		{name: "nested extra", tool: "student_mistakes", args: map[string]any{
			"student_id": "s", "mistakes": []any{map[string]any{"knowledge_point": "kp", "class_id": "other"}},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ExecuteTool(context.Background(), test.tool, test.args)
			if !errors.Is(err, ErrInvalidArgs) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
