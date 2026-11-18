package evaluation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type invalidReviewClient struct{}

func (invalidReviewClient) Chat(context.Context, gateway.ChatRequest) (*gateway.ChatResponse, error) {
	return &gateway.ChatResponse{
		Text: `{"outline":[{"title":"高频错因"}],"examples":[{"text":"对比例题"}],"remedial":"补救题不是数组","commonCauses":["改写后的错因"]}`,
		RawJSON: map[string]any{
			"outline":      []any{map[string]any{"title": "高频错因"}},
			"examples":     []any{map[string]any{"text": "对比例题"}},
			"remedial":     "补救题不是数组",
			"commonCauses": []any{"改写后的错因"},
		},
		Model: "course-chat-test",
	}, nil
}

func (invalidReviewClient) Embed(context.Context, gateway.EmbeddingRequest) (*gateway.EmbeddingResponse, error) {
	return nil, nil
}

func TestNormalizeClassReviewJSONCoercesObjectArrays(t *testing.T) {
	inputs := []string{"行变换不完整", "秩概念混淆"}
	raw := map[string]any{
		"outline":      []any{map[string]any{"title": "围绕行变换讲解"}},
		"examples":     []any{map[string]any{"text": "对比例题"}},
		"remedial":     []any{map[string]any{"content": "补救练习"}},
		"commonCauses": []any{"行变换不完整", "杜撰错因"},
	}
	got := normalizeClassReviewJSON(raw, inputs)
	outline, _ := toStringSlice(got["outline"])
	causes, _ := toStringSlice(got["commonCauses"])
	if len(outline) != 1 || outline[0] != "围绕行变换讲解" {
		t.Fatalf("outline = %#v", outline)
	}
	if len(causes) != 1 || causes[0] != "行变换不完整" {
		t.Fatalf("commonCauses filtered = %#v", causes)
	}
}

func TestClassReviewFallsBackWhenModelJSONInvalid(t *testing.T) {
	useFreshEvaluationStore(t)
	inputs := []string{"行变换不完整", "秩概念混淆", "行变换不完整", "增广矩阵漏变右端", "秩概念混淆"}
	contentID := "review-fallback-1"
	now := time.Now().UTC()
	store.Default.SaveContent(store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillClassReview), ClassID: "class-a",
		Status: string(domain.StatusGenerating),
		Payload: map[string]any{
			"assignmentId": "asg-a", "anonymousInputs": inputs, "teacherOnly": true,
		},
		CreatedAt: now, UpdatedAt: now,
	})
	if err := ProcessClassReviewWithModel(context.Background(), contentID, inputs, invalidReviewClient{}); err != nil {
		t.Fatal(err)
	}
	content, ok := store.Default.GetContent(contentID)
	if !ok {
		t.Fatal("missing content")
	}
	review, _ := content.Payload["review"].(map[string]any)
	if content.Status != string(domain.StatusPending) || review == nil {
		t.Fatalf("expected pending review, got status=%s review=%#v", content.Status, review)
	}
	if review["generationMode"] != "evidence_fallback" {
		t.Fatalf("expected evidence_fallback, got %#v", review["generationMode"])
	}
	if review["teacherOnly"] != true {
		t.Fatalf("teacherOnly missing: %#v", review)
	}
	causes, _ := toStringSlice(review["commonCauses"])
	if len(causes) == 0 {
		t.Fatalf("fallback causes empty: %#v", review)
	}
}

func TestMarkClassReviewFailedPersistsGenerationError(t *testing.T) {
	useFreshEvaluationStore(t)
	contentID := "review-fail-1"
	store.Default.SaveContent(store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillClassReview), ClassID: "class-a",
		Status: string(domain.StatusGenerating), Payload: map[string]any{"teacherOnly": true},
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	MarkClassReviewFailed(contentID, "model gateway returned HTTP 500 "+strings.Repeat("x", 300))
	content, _ := store.Default.GetContent(contentID)
	errText, _ := content.Payload["generationError"].(string)
	if errText == "" || len([]rune(errText)) > 250 {
		t.Fatalf("generationError = %q", errText)
	}
	if content.Status != string(domain.StatusGenerating) {
		t.Fatalf("status should remain generating until retry, got %s", content.Status)
	}
}
