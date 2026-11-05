package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
)

// DefaultOrchestrator 统一编排：生成有限步骤计划并驱动专业智能体。
type DefaultOrchestrator struct {
	Content ContentAgent
	Virtual VirtualStudentAgent
	Verify  VerifierAgent
	Tutor   TutorAgent
	LLM     gateway.Client
}

func NewOrchestrator(llm gateway.Client) *DefaultOrchestrator {
	return &DefaultOrchestrator{
		Content: &DefaultContentAgent{LLM: llm},
		Virtual: &DefaultVirtualStudentAgent{LLM: llm},
		Verify:  &DefaultVerifierAgent{},
		Tutor:   &DefaultTutorAgent{LLM: llm},
		LLM:     llm,
	}
}

func (o *DefaultOrchestrator) Plan(_ context.Context, _ string, skill string) ([]string, error) {
	return []string{
		"auth_and_scope",
		"retrieve_trusted_data",
		"generate_" + skill,
		"verify_result",
		"await_teacher_confirm",
		"publish_and_audit",
	}, nil
}

func (o *DefaultOrchestrator) Run(ctx context.Context, plan []string, input map[string]any) (*StructuredResult, error) {
	_ = plan
	skill, _ := input["skill"].(string)
	switch skill {
	case "virtual_classroom":
		lp, _ := input["lessonPlanId"].(string)
		rounds := 3
		if r, ok := input["rounds"].(float64); ok {
			rounds = int(r)
		}
		return o.Virtual.Simulate(ctx, lp, []string{"强基础", "一般", "薄弱"}, rounds)
	case "socratic_tutor":
		state, _ := input["state"].(map[string]any)
		reply, _ := input["reply"].(string)
		return o.Tutor.NextTurn(ctx, state, reply)
	default:
		return o.Content.Generate(ctx, input)
	}
}

type DefaultContentAgent struct{ LLM gateway.Client }

func (a *DefaultContentAgent) Generate(ctx context.Context, input map[string]any) (*StructuredResult, error) {
	skill, _ := input["skill"].(string)
	if skill == "" {
		skill = "content"
	}
	resp, err := a.LLM.Chat(ctx, gateway.ChatRequest{
		Messages: []gateway.ChatMessage{{Role: "user", Content: "生成教学结构化结果：" + skill + " " + fmt.Sprint(input)}},
	})
	if err != nil {
		return nil, err
	}
	payload := resp.RawJSON
	if payload == nil {
		payload = map[string]any{"text": resp.Text}
	}
	return &StructuredResult{
		Payload:    payload,
		Confidence: 0.85,
		Citations:  extractCitations(payload),
		ModelMeta:  ModelMeta{Model: resp.Model, PromptVersion: "v1", SkillVersion: "v1"},
	}, nil
}

type DefaultVirtualStudentAgent struct{ LLM gateway.Client }

func (a *DefaultVirtualStudentAgent) Simulate(ctx context.Context, lessonPlanID string, roles []string, rounds int) (*StructuredResult, error) {
	resp, err := a.LLM.Chat(ctx, gateway.ChatRequest{
		Messages: []gateway.ChatMessage{{
			Role:    "user",
			Content: fmt.Sprintf("虚拟课堂演练 lesson=%s roles=%v rounds=%d", lessonPlanID, roles, rounds),
		}},
	})
	if err != nil {
		return nil, err
	}
	return &StructuredResult{
		Payload:    resp.RawJSON,
		Confidence: 0.8,
		ModelMeta:  ModelMeta{Model: resp.Model, PromptVersion: "v1", SkillVersion: "v1"},
	}, nil
}

type DefaultVerifierAgent struct{}

func (DefaultVerifierAgent) Verify(_ context.Context, result *StructuredResult, rules VerifyRules) (bool, []string, error) {
	var issues []string
	if result == nil {
		return false, []string{"empty result"}, nil
	}
	if rules.RequireCitations && len(result.Citations) == 0 {
		if cites := extractCitations(result.Payload); len(cites) > 0 {
			result.Citations = cites
		} else if rules.SchemaName != "preview_sheet" {
			issues = append(issues, "missing citations")
		}
	}
	if rules.MinSampleSize > 0 {
		switch n := result.Payload["sampleCount"].(type) {
		case int:
			if n < rules.MinSampleSize {
				issues = append(issues, fmt.Sprintf("sample size %d < %d", n, rules.MinSampleSize))
			}
		case float64:
			if int(n) < rules.MinSampleSize {
				issues = append(issues, fmt.Sprintf("sample size %d < %d", int(n), rules.MinSampleSize))
			}
		}
	}
	if rules.SchemaName == "preview_sheet" {
		if countCards(result.Payload["cards"]) < 3 {
			issues = append(issues, "effective knowledge cards < 3")
		}
	}
	return len(issues) == 0, issues, nil
}

type DefaultTutorAgent struct{ LLM gateway.Client }

func (a *DefaultTutorAgent) NextTurn(ctx context.Context, state map[string]any, studentReply string) (*StructuredResult, error) {
	resp, err := a.LLM.Chat(ctx, gateway.ChatRequest{
		Messages: []gateway.ChatMessage{{
			Role:    "user",
			Content: "追问式辅导 reply=" + studentReply + " state=" + fmt.Sprint(state),
		}},
	})
	if err != nil {
		return nil, err
	}
	hit := strings.Contains(studentReply, "右端") || strings.Contains(studentReply, "增广") || strings.Contains(studentReply, "同步")
	payload := resp.RawJSON
	if payload == nil {
		payload = map[string]any{}
	}
	payload["hit"] = hit
	if _, ok := payload["prompt"]; !ok {
		payload["prompt"] = "你交换行之后，右端的常数项有没有一起交换？"
	}
	return &StructuredResult{
		Payload:    payload,
		Confidence: 0.8,
		ModelMeta:  ModelMeta{Model: resp.Model, PromptVersion: "v1", SkillVersion: "v1"},
	}, nil
}

func extractCitations(payload map[string]any) []Citation {
	var out []Citation
	if qs, ok := payload["questions"].([]any); ok {
		for _, q := range qs {
			if m, ok := q.(map[string]any); ok {
				out = append(out, extractCitations(m)...)
			}
		}
	}
	if qs, ok := payload["questions"].([]map[string]any); ok {
		for _, m := range qs {
			out = append(out, extractCitations(m)...)
		}
	}
	raw, ok := payload["citations"]
	if !ok {
		return out
	}
	switch v := raw.(type) {
	case []map[string]any:
		for _, c := range v {
			out = append(out, Citation{SourceID: fmt.Sprint(c["sourceId"]), Snippet: fmt.Sprint(c["snippet"])})
		}
	case []any:
		for _, item := range v {
			if c, ok := item.(map[string]any); ok {
				out = append(out, Citation{SourceID: fmt.Sprint(c["sourceId"]), Snippet: fmt.Sprint(c["snippet"])})
			}
		}
	}
	return out
}

func countCards(v any) int {
	switch x := v.(type) {
	case []map[string]any:
		return len(x)
	case []any:
		return len(x)
	default:
		return 0
	}
}

type StubOrchestrator = DefaultOrchestrator
type StubContentAgent = DefaultContentAgent
type StubVirtualStudentAgent = DefaultVirtualStudentAgent
type StubVerifierAgent = DefaultVerifierAgent
type StubTutorAgent = DefaultTutorAgent
