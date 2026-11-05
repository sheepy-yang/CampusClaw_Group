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
	switch skill {
	case "复习路径", "review_path":
		if countObjectItems(payload["steps"]) == 0 {
			target := fmt.Sprint(input["targetKnowledgePoint"])
			payload = map[string]any{"steps": []map[string]any{
				{"type": "read", "title": "回顾关键概念", "detail": "结合讲义重新梳理定义与条件", "knowledgePoint": target},
				{"type": "example", "title": "分析典型例题", "detail": "定位易错步骤并解释原因", "knowledgePoint": target},
				{"type": "practice", "title": "完成针对性练习", "detail": "用新题检验是否真正掌握", "knowledgePoint": target},
			}}
		}
	case "聚类", "classroom_cluster":
		if countObjectItems(payload["clusters"]) == 0 {
			groups, _ := input["groups"].([]map[string]any)
			clusters := make([]map[string]any, 0, len(groups))
			for _, group := range groups {
				clusters = append(clusters, map[string]any{
					"groupId": group["groupId"], "memberIds": group["members"],
					"label": "相似作答思路", "followUp": "请说明这个步骤成立所依据的条件。",
				})
			}
			payload = map[string]any{"clusters": clusters}
		}
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
			Role: "system",
			Content: "你是虚拟课堂演练智能体。以下全部角色都是合成角色，不代表真实学生。只能依据给定教案演练。仅输出 JSON，格式为" +
				`{"dialogue":[{"round":1,"role":"角色名","utterance":"提问或典型误解","teacherResponse":"建议回应","basis":"教案依据"}],` +
				`"risks":[{"round":1,"risk":"可能卡点","basis":"教案依据"}],"suggestions":["可执行的教案修改建议"]}。`,
		}, {
			Role:    "user",
			Content: fmt.Sprintf("教案=%s\n合成角色=%v\n轮数=%d", lessonPlanID, roles, rounds),
		}},
	})
	if err != nil {
		return nil, err
	}
	payload := resp.RawJSON
	if payload == nil {
		payload = map[string]any{}
	}
	if countObjectItems(payload["dialogue"]) == 0 {
		dialogue := make([]map[string]any, 0, len(roles))
		risks := make([]map[string]any, 0, len(roles))
		for index, role := range roles {
			round := index%rounds + 1
			dialogue = append(dialogue, map[string]any{
				"round": round, "role": role,
				"utterance":       "我能否说明当前教学步骤与前一概念之间的关系？",
				"teacherResponse": "请先复述教案中的关键条件，再用一个反例检查自己的理解。",
				"basis":           "所选教案的目标、重难点与教学过程",
			})
			risks = append(risks, map[string]any{
				"round": round, "risk": role + "角色可能在条件辨析与步骤衔接处停顿",
				"basis": "所选教案的重难点与教学过程",
			})
		}
		payload["dialogue"] = dialogue
		payload["risks"] = risks
		payload["suggestions"] = []string{"在关键步骤前增加条件复述和反例检查，并在修改后再次演练。"}
		payload["generationMode"] = "offline_demo"
	}
	return &StructuredResult{
		Payload:    payload,
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

func countObjectItems(value any) int {
	switch items := value.(type) {
	case []map[string]any:
		return len(items)
	case []any:
		return len(items)
	default:
		return 0
	}
}
