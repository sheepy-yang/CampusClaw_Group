package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

var (
	ErrUnknownTool  = errors.New("unknown tool")
	ErrInvalidArgs  = errors.New("invalid tool arguments")
	toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
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
			Description: "Conservatively compare two supported numeric or normalized expressions.",
			InputSchema: objectSchema(map[string]any{
				"expression": map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
				"expected":   map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
			}, []string{"expression", "expected"}),
		},
		{
			Name:        "assignment_stats",
			Description: "Compute aggregate statistics from API-scoped anonymous scores.",
			InputSchema: objectSchema(map[string]any{
				"assignment_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
				"scores": map[string]any{
					"type": "array", "minItems": 1, "maxItems": 500,
					"items": map[string]any{"type": "number", "minimum": 0, "maximum": 100},
				},
				"passing_score": map[string]any{"type": "number", "minimum": 0, "maximum": 100},
			}, []string{"assignment_id", "scores"}),
		},
		{
			Name:        "student_mistakes",
			Description: "Summarize API-scoped mistake records without expanding their data scope.",
			InputSchema: objectSchema(map[string]any{
				"student_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
				"mistakes": map[string]any{
					"type": "array", "maxItems": 100,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"knowledge_point":  map[string]any{"type": "string", "minLength": 1, "maxLength": 100},
							"error_cause":      map[string]any{"type": "string", "maxLength": 300},
							"question_summary": map[string]any{"type": "string", "maxLength": 300},
						},
						"required":             []string{"knowledge_point"},
						"additionalProperties": false,
					},
				},
			}, []string{"student_id", "mistakes"}),
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

func toolByName(name string) (ToolDesc, bool) {
	for _, tool := range ListTools() {
		if tool.Name == name {
			return tool, true
		}
	}
	return ToolDesc{}, false
}

func ExecuteTool(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	tool, ok := toolByName(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
	if err := ValidateSchema(tool.InputSchema, arguments); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgs, err)
	}
	switch name {
	case "formula_verify":
		return FormulaVerify(ctx, arguments)
	case "assignment_stats":
		return AssignmentStats(ctx, arguments)
	case "student_mistakes":
		return StudentMistakes(ctx, arguments)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
}

func FormulaVerify(ctx context.Context, arguments map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	expression := strings.TrimSpace(arguments["expression"].(string))
	expected := strings.TrimSpace(arguments["expected"].(string))

	left, leftErr := parseNumericExpression(expression)
	right, rightErr := parseNumericExpression(expected)
	if leftErr == nil && rightErr == nil {
		scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
		verified := math.Abs(left-right) <= 1e-9*scale
		status := "not_equivalent"
		if verified {
			status = "verified"
		}
		return map[string]any{
			"verified": verified,
			"status":   status,
			"method":   "numeric_evaluation",
		}, nil
	}

	if normalizedExpression(expression) == normalizedExpression(expected) {
		return map[string]any{
			"verified": true,
			"status":   "verified",
			"method":   "exact_normalization",
		}, nil
	}
	return map[string]any{
		"verified": false,
		"status":   "indeterminate",
		"method":   "unsupported_expression",
	}, nil
}

func AssignmentStats(ctx context.Context, arguments map[string]any) (map[string]any, error) {
	rawScores := arguments["scores"].([]any)
	scores := make([]float64, len(rawScores))
	for index, raw := range rawScores {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		scores[index] = raw.(float64)
	}
	sort.Float64s(scores)
	total := 0.0
	minimum, maximum := scores[0], scores[len(scores)-1]
	passingScore := 60.0
	if value, ok := arguments["passing_score"].(float64); ok {
		passingScore = value
	}
	passed := 0
	for _, score := range scores {
		total += score
		if score >= passingScore {
			passed++
		}
	}
	median := scores[len(scores)/2]
	if len(scores)%2 == 0 {
		median = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
	}
	return map[string]any{
		"assignmentId": arguments["assignment_id"],
		"count":        len(scores),
		"average":      round4(total / float64(len(scores))),
		"median":       round4(median),
		"minimum":      minimum,
		"maximum":      maximum,
		"passingScore": passingScore,
		"passRate":     round4(float64(passed) / float64(len(scores))),
	}, nil
}

func StudentMistakes(ctx context.Context, arguments map[string]any) (map[string]any, error) {
	rawMistakes := arguments["mistakes"].([]any)
	counts := make(map[string]int)
	summaries := make([]map[string]any, 0, len(rawMistakes))
	for index, raw := range rawMistakes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		mistake := raw.(map[string]any)
		knowledgePoint := mistake["knowledge_point"].(string)
		counts[knowledgePoint]++
		summary := map[string]any{
			"index":          index + 1,
			"knowledgePoint": knowledgePoint,
		}
		if value, ok := mistake["error_cause"].(string); ok && strings.TrimSpace(value) != "" {
			summary["errorCause"] = strings.TrimSpace(value)
		}
		if value, ok := mistake["question_summary"].(string); ok && strings.TrimSpace(value) != "" {
			summary["questionSummary"] = strings.TrimSpace(value)
		}
		summaries = append(summaries, summary)
	}
	knowledgePoints := make([]string, 0, len(counts))
	for knowledgePoint := range counts {
		knowledgePoints = append(knowledgePoints, knowledgePoint)
	}
	sort.Strings(knowledgePoints)
	byKnowledgePoint := make([]map[string]any, 0, len(knowledgePoints))
	for _, knowledgePoint := range knowledgePoints {
		byKnowledgePoint = append(byKnowledgePoint, map[string]any{
			"knowledgePoint": knowledgePoint,
			"count":          counts[knowledgePoint],
		})
	}
	return map[string]any{
		"studentId":        arguments["student_id"],
		"total":            len(rawMistakes),
		"byKnowledgePoint": byKnowledgePoint,
		"mistakes":         summaries,
	}, nil
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
