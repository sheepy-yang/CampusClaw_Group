package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
)

// GatewayContentAgent uses the configured course model. The deterministic
// transformer remains the explicit offline adapter; a configured remote model
// must return valid structured output and is never silently replaced on error.
type GatewayContentAgent struct {
	Client   gateway.Client
	Fallback ContentProductionAgent
}

func NewGatewayContentAgent(client gateway.Client) *GatewayContentAgent {
	return &GatewayContentAgent{Client: client, Fallback: NewDeterministicContentAgent()}
}

func (a *GatewayContentAgent) Produce(ctx context.Context, request ProductionRequest) (*ProductionOutput, error) {
	if a == nil || a.Client == nil {
		return nil, errors.New("content model is not configured")
	}
	if _, err := validateSources(request.Sources); err != nil {
		return nil, err
	}
	response, err := a.Client.Chat(ctx, gateway.ChatRequest{
		Messages: []gateway.ChatMessage{
			{
				Role:    "system",
				Content: "你是课程内容生成器。只能使用给定证据，不得创造引用。仅输出一个 JSON 对象；引用字段必须逐字复制证据的 sourceId、sourceVersion、chunkId、characterStart、characterEnd、contentHash 和 text（作为 snippet）。",
			},
			{Role: "user", Content: productionPrompt(request)},
		},
	})
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(response.Model, "offline-demo") {
		if a.Fallback == nil {
			return nil, errors.New("offline content adapter is unavailable")
		}
		return a.Fallback.Produce(ctx, request)
	}
	payload := response.RawJSON
	if nested, ok := payload["payload"].(map[string]any); ok {
		payload = nested
	}
	if payload == nil {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(response.Text)), &parsed); err != nil {
			return nil, fmt.Errorf("content model returned non-JSON output: %w", err)
		}
		payload = parsed
	}
	citations := make([]Citation, 0, len(request.Sources))
	for _, source := range request.Sources {
		citations = append(citations, sourceCitation(source))
	}
	inputCharacters := utf8.RuneCountInString(productionPrompt(request))
	return &ProductionOutput{
		Payload: payload, Citations: citations,
		ModelMeta: ModelMeta{Model: response.Model, PromptVersion: ContentPromptVersion, SkillVersion: ContentSkillVersion},
		Usage: ProductionUsage{
			Mode: "gateway", Model: response.Model, PromptVersion: ContentPromptVersion,
			SkillVersion: ContentSkillVersion, InputCharacters: inputCharacters,
			OutputItems: outputItemCount(request.Skill, payload), SourceCount: len(request.Sources),
		},
	}, nil
}

func productionPrompt(request ProductionRequest) string {
	schema := ""
	switch request.Skill {
	case SkillCourseCards:
		schema = `{"title":"...","cards":[{"id":"...","title":"...","summary":"必须是证据原文","citation":{完整引用}}]}`
	case SkillExercises:
		schema = `{"title":"...","questions":[{"id":"...","type":"choice|fill|short_answer","difficulty":"...","stem":"...","options":[],"answer":"...","explanation":"...","citations":[{完整引用}]}]}`
	case SkillLessonPlan:
		schema = `{"title":"...","durationMinutes":整数,"objectives":["..."],"keyPoints":[{"text":"必须是证据原文","citation":{完整引用}}],"phases":[{"name":"...","minutes":正整数,"activities":["..."]}]}`
	}
	encoded, _ := json.Marshal(struct {
		Skill           ProductionSkill   `json:"skill"`
		Topic           string            `json:"topic"`
		Count           int               `json:"count,omitempty"`
		ExerciseTypes   []string          `json:"exerciseTypes,omitempty"`
		Difficulty      string            `json:"difficulty,omitempty"`
		DurationMinutes int               `json:"durationMinutes,omitempty"`
		Objectives      []string          `json:"objectives,omitempty"`
		Evidence        []GroundingSource `json:"evidence"`
		Schema          string            `json:"requiredSchema"`
	}{
		request.Skill, request.Topic, request.Count, request.ExerciseTypes, request.Difficulty,
		request.DurationMinutes, request.Objectives, request.Sources, schema,
	})
	return string(encoded)
}

func outputItemCount(skill ProductionSkill, payload map[string]any) int {
	key := "questions"
	if skill == SkillCourseCards {
		key = "cards"
	} else if skill == SkillLessonPlan {
		key = "phases"
	}
	switch values := payload[key].(type) {
	case []any:
		return len(values)
	case []map[string]any:
		return len(values)
	default:
		return 0
	}
}

var _ ContentProductionAgent = (*GatewayContentAgent)(nil)
