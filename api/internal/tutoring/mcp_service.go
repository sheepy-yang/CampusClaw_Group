package tutoring

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

var ErrScopedDataNotFound = errors.New("scoped tool data not found")

var auditRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type ToolMistake struct {
	KnowledgePoint  string
	ErrorCause      string
	QuestionSummary string
}

type TeachingToolDataSource interface {
	AssignmentScores(context.Context, string, string) ([]float64, error)
	StudentMistakes(context.Context, string, string) ([]ToolMistake, error)
}

type StoreToolDataSource struct {
	Memory *store.Memory
}

func (d StoreToolDataSource) AssignmentScores(_ context.Context, classID, assignmentID string) ([]float64, error) {
	memory := d.Memory
	if memory == nil {
		memory = store.Default
	}
	memory.Lock()
	assignment, exists := memory.Assignments[assignmentID]
	if !exists || assignment.ClassID != classID {
		memory.Unlock()
		return nil, ErrScopedDataNotFound
	}
	var scores []float64
	for _, submission := range memory.Submissions {
		if submission.ClassID != classID || submission.AssignmentID != assignmentID || submission.Score == nil {
			continue
		}
		score := *submission.Score
		if submission.MaxScore > 0 {
			score = score / submission.MaxScore * 100
		}
		if score < 0 || score > 100 {
			memory.Unlock()
			return nil, errors.New("stored assignment score is outside 0 to 100")
		}
		scores = append(scores, score)
	}
	memory.Unlock()
	if len(scores) == 0 {
		return nil, errors.New("assignment has no graded submissions")
	}
	return scores, nil
}

func (d StoreToolDataSource) StudentMistakes(_ context.Context, classID, studentID string) ([]ToolMistake, error) {
	memory := d.Memory
	if memory == nil {
		memory = store.Default
	}
	mistakes := memory.MistakesFor(studentID, classID)
	if len(mistakes) == 0 {
		return nil, ErrScopedDataNotFound
	}
	result := make([]ToolMistake, 0, len(mistakes))
	for _, mistake := range mistakes {
		if mistake.StudentID != studentID || mistake.ClassID != classID {
			continue
		}
		result = append(result, ToolMistake{
			KnowledgePoint:  mistake.KnowledgePoint,
			ErrorCause:      mistake.ErrorCause,
			QuestionSummary: mistake.QuestionStem,
		})
	}
	if len(result) == 0 {
		return nil, ErrScopedDataNotFound
	}
	return result, nil
}

type TeachingTools struct {
	Host  MCPHost
	Data  TeachingToolDataSource
	Audit security.AuditLogger
	Now   func() time.Time
}

type TeachingToolResult struct {
	Tool      string         `json:"tool"`
	RequestID string         `json:"requestId"`
	Data      map[string]any `json:"data"`
}

func NewTeachingTools(host MCPHost, data TeachingToolDataSource, audit security.AuditLogger) *TeachingTools {
	if data == nil {
		data = StoreToolDataSource{}
	}
	if audit == nil {
		audit = discardAuditLogger{}
	}
	return &TeachingTools{Host: host, Data: data, Audit: audit, Now: time.Now}
}

func (s *TeachingTools) List(ctx context.Context, principal auth.Principal) ([]MCPToolDescriptor, error) {
	if s == nil || s.Host == nil {
		return nil, newError(503, "MCP_UNAVAILABLE", "MCP host is unavailable", nil)
	}
	tools, err := s.Host.Discover(ctx)
	if err != nil {
		return nil, mapMCPError(err)
	}
	result := make([]MCPToolDescriptor, 0, len(tools))
	for _, tool := range tools {
		switch tool.Name {
		case "formula_verify", "student_mistakes":
		case "assignment_stats":
			if principal.Role != auth.RoleTeacher {
				continue
			}
		default:
			continue
		}
		result = append(result, tool)
	}
	return result, nil
}

func (s *TeachingTools) Call(ctx context.Context, principal auth.Principal, requestID, tool string, clientArguments map[string]any) (result TeachingToolResult, err error) {
	started := time.Now()
	tool = strings.TrimSpace(tool)
	auditTool := tool
	if !mcpToolNamePattern.MatchString(auditTool) {
		auditTool = "[invalid]"
	}
	requestID = strings.TrimSpace(requestID)
	auditRequestID := requestID
	if !auditRequestIDPattern.MatchString(auditRequestID) {
		auditRequestID = "[invalid]"
	}
	if s != nil && s.Now != nil {
		started = s.Now()
	}
	status := "failed"
	errorCode := ""
	defer func() {
		if err != nil {
			_, errorCode, _ = ErrorInfo(err)
		} else {
			status = "succeeded"
		}
		if s == nil || s.Audit == nil {
			return
		}
		duration := time.Since(started)
		if s.Now != nil {
			duration = s.Now().Sub(started)
		}
		if duration < 0 {
			duration = 0
		}
		auditErr := s.Audit.Log(ctx, security.AuditEvent{
			ActorID: principal.UserID,
			ClassID: principal.ClassID,
			Action:  "mcp.tool_call",
			Detail: map[string]any{
				"role":       principal.Role,
				"tool":       auditTool,
				"requestId":  auditRequestID,
				"status":     status,
				"durationMs": duration.Milliseconds(),
				"errorCode":  errorCode,
			},
		})
		if auditErr != nil {
			log.Printf("MCP audit persistence failed tool=%s request_id=%s: %v", auditTool, auditRequestID, auditErr)
			if err == nil {
				result = TeachingToolResult{}
				err = persistenceUnavailable(auditErr)
			}
		}
	}()

	if s == nil || s.Host == nil {
		return TeachingToolResult{}, newError(503, "MCP_UNAVAILABLE", "MCP host is unavailable", nil)
	}
	if !auditRequestIDPattern.MatchString(requestID) {
		return TeachingToolResult{}, invalid("a valid request ID is required")
	}
	if clientArguments == nil {
		clientArguments = map[string]any{}
	}
	if !mcpToolNamePattern.MatchString(tool) {
		return TeachingToolResult{}, newError(400, "MCP_TOOL_NOT_ALLOWED", "MCP tool is not allowed", nil)
	}

	arguments, err := s.scopedArguments(ctx, principal, tool, clientArguments)
	if err != nil {
		return TeachingToolResult{}, err
	}
	data, err := s.Host.Call(ctx, tool, arguments)
	if err != nil {
		return TeachingToolResult{}, mapMCPError(err)
	}
	return TeachingToolResult{Tool: tool, RequestID: requestID, Data: data}, nil
}

func (s *TeachingTools) scopedArguments(ctx context.Context, principal auth.Principal, tool string, client map[string]any) (map[string]any, error) {
	switch tool {
	case "formula_verify":
		if err := onlyKeys(client, "expression", "expected"); err != nil {
			return nil, err
		}
		expression, okExpression := client["expression"].(string)
		expected, okExpected := client["expected"].(string)
		if !okExpression || !okExpected {
			return nil, invalid("expression and expected must be strings")
		}
		if err := security.CheckInput(ctx, expression+"\n"+expected); err != nil {
			return nil, unsafeInput(err)
		}
		return map[string]any{"expression": expression, "expected": expected}, nil
	case "assignment_stats":
		if principal.Role != auth.RoleTeacher {
			return nil, forbidden("assignment statistics are teacher-only")
		}
		if err := onlyKeys(client, "assignment_id", "passing_score"); err != nil {
			return nil, err
		}
		assignmentID, ok := client["assignment_id"].(string)
		if !ok || strings.TrimSpace(assignmentID) == "" {
			return nil, invalid("assignment_id is required")
		}
		scores, err := s.Data.AssignmentScores(ctx, principal.ClassID, strings.TrimSpace(assignmentID))
		if errors.Is(err, ErrScopedDataNotFound) {
			return nil, notFound()
		}
		if err != nil {
			return nil, newError(500, "SCOPED_DATA_INVALID", "scoped assignment data is invalid", err)
		}
		rawScores := make([]any, len(scores))
		for index, score := range scores {
			rawScores[index] = score
		}
		arguments := map[string]any{
			"assignment_id": strings.TrimSpace(assignmentID),
			"scores":        rawScores,
		}
		if passingScore, exists := client["passing_score"]; exists {
			if _, ok := JSONNumber(passingScore); !ok {
				return nil, invalid("passing_score must be numeric")
			}
			arguments["passing_score"] = passingScore
		}
		return arguments, nil
	case "student_mistakes":
		if err := onlyKeys(client, "student_id"); err != nil {
			return nil, err
		}
		requested, _ := client["student_id"].(string)
		requested = strings.TrimSpace(requested)
		studentID, err := scopedStudent(principal, requested)
		if err != nil {
			return nil, err
		}
		mistakes, err := s.Data.StudentMistakes(ctx, principal.ClassID, studentID)
		if errors.Is(err, ErrScopedDataNotFound) {
			if principal.Role == auth.RoleStudent && studentID == principal.UserID {
				mistakes = []ToolMistake{}
				err = nil
			} else {
				return nil, notFound()
			}
		}
		if err != nil {
			return nil, newError(500, "SCOPED_DATA_INVALID", "scoped mistake data is invalid", err)
		}
		rawMistakes := make([]any, 0, len(mistakes))
		for _, mistake := range mistakes {
			if err := security.CheckInput(ctx, mistake.KnowledgePoint+"\n"+mistake.ErrorCause+"\n"+mistake.QuestionSummary); err != nil {
				return nil, unsafeInput(err)
			}
			rawMistakes = append(rawMistakes, map[string]any{
				"knowledge_point":  strings.TrimSpace(mistake.KnowledgePoint),
				"error_cause":      strings.TrimSpace(mistake.ErrorCause),
				"question_summary": strings.TrimSpace(mistake.QuestionSummary),
			})
		}
		return map[string]any{"student_id": studentID, "mistakes": rawMistakes}, nil
	default:
		return nil, newError(400, "MCP_TOOL_NOT_ALLOWED", "MCP tool is not allowed", nil)
	}
}

func onlyKeys(values map[string]any, allowed ...string) error {
	allowedSet := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = true
	}
	for key := range values {
		if !allowedSet[key] {
			return invalid(fmt.Sprintf("argument %q is not allowed", key))
		}
	}
	return nil
}

func mapMCPError(err error) error {
	switch {
	case errors.Is(err, ErrMCPArguments):
		return newError(400, "MCP_ARGUMENTS_INVALID", "MCP tool arguments are invalid", err)
	case errors.Is(err, ErrMCPToolNotAllowed):
		return newError(400, "MCP_TOOL_NOT_ALLOWED", "MCP tool is not allowed", err)
	case errors.Is(err, ErrMCPOutput):
		return newError(502, "MCP_OUTPUT_INVALID", "MCP returned an invalid result", err)
	case errors.Is(err, ErrMCPTimeout):
		return newError(503, "MCP_TIMEOUT", "MCP request timed out", err)
	case errors.Is(err, ErrMCPUnavailable), errors.Is(err, ErrMCPProtocol):
		return newError(503, "MCP_UNAVAILABLE", "MCP is unavailable", err)
	default:
		return newError(503, "MCP_UNAVAILABLE", "MCP is unavailable", err)
	}
}
