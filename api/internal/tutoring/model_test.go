package tutoring

import (
	"context"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
)

type recordingOrchestrator struct {
	input  map[string]any
	result *agent.StructuredResult
}

func (o *recordingOrchestrator) Plan(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (o *recordingOrchestrator) Run(_ context.Context, _ []string, input map[string]any) (*agent.StructuredResult, error) {
	o.input = input
	return o.result, nil
}

func TestOrchestratorModelBindsServerSelectedReviewTarget(t *testing.T) {
	orchestrator := &recordingOrchestrator{result: &agent.StructuredResult{Payload: map[string]any{
		"steps": []map[string]any{
			{"type": "read", "title": "阅读"},
			{"type": "example", "title": "例题"},
			{"type": "practice", "title": "练习"},
		},
	}}}
	model := OrchestratorModel{Orchestrator: orchestrator}
	steps, err := model.GenerateReviewPath(context.Background(), ReviewModelInput{Target: "高斯消元"})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.KnowledgePoint != "高斯消元" {
			t.Fatalf("step was not bound to server target: %#v", step)
		}
	}
	if _, leaked := orchestrator.input["studentId"]; leaked {
		t.Fatalf("identity leaked to orchestrator: %#v", orchestrator.input)
	}
	if _, leaked := orchestrator.input["classId"]; leaked {
		t.Fatalf("class scope leaked to orchestrator: %#v", orchestrator.input)
	}
}

func TestOrchestratorModelBindsClusterMembershipButPreservesModelClaims(t *testing.T) {
	orchestrator := &recordingOrchestrator{result: &agent.StructuredResult{Payload: map[string]any{
		"clusters": []map[string]any{
			{"label": "类型", "followUp": "为什么？"},
		},
	}}}
	model := OrchestratorModel{Orchestrator: orchestrator}
	groups := []ClusterDraft{{ID: "group-1", MemberIDs: []string{"answer-1", "answer-2"}, Texts: []string{"甲", "乙"}}}
	summaries, err := model.SummarizeClusters(context.Background(), groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].GroupID != "group-1" || len(summaries[0].MemberIDs) != 2 {
		t.Fatalf("bound summary = %#v", summaries)
	}

	orchestrator.result = &agent.StructuredResult{Payload: map[string]any{
		"clusters": []map[string]any{{
			"groupId": "invented", "memberIds": []any{"invented"}, "label": "类型",
		}},
	}}
	summaries, err = model.SummarizeClusters(context.Background(), groups)
	if err != nil {
		t.Fatal(err)
	}
	if summaries[0].GroupID != "invented" || summaries[0].MemberIDs[0] != "invented" {
		t.Fatalf("model claims were hidden before service validation: %#v", summaries)
	}
}

func TestOrchestratorModelCallsTutorWithDeidentifiedState(t *testing.T) {
	orchestrator := &recordingOrchestrator{result: &agent.StructuredResult{Payload: map[string]any{
		"prompt": "下一步是什么？",
	}}}
	model := OrchestratorModel{Orchestrator: orchestrator}
	prompt, err := model.NextTutorQuestion(context.Background(), TutorModelInput{
		QuestionStem: "题目", Round: 1, HitStreak: 0, StudentReply: "回答",
	})
	if err != nil || prompt != "下一步是什么？" {
		t.Fatalf("prompt=%q err=%v", prompt, err)
	}
	if _, leaked := orchestrator.input["studentId"]; leaked {
		t.Fatalf("identity leaked to tutor model: %#v", orchestrator.input)
	}
}
