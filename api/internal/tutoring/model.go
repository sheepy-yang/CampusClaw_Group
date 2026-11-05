package tutoring

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
)

// OrchestratorModel binds the existing project orchestrator to the narrow,
// typed contracts accepted by the tutoring services. Identity and class data
// are intentionally absent from every model input.
type OrchestratorModel struct {
	Orchestrator agent.Orchestrator
}

func (m OrchestratorModel) GenerateReviewPath(ctx context.Context, input ReviewModelInput) ([]ReviewStepDraft, error) {
	if m.Orchestrator == nil {
		return nil, errors.New("orchestrator is not configured")
	}
	result, err := m.Orchestrator.Run(ctx, nil, map[string]any{
		"skill":                "复习路径",
		"knowledgePoints":      input.KnowledgePoints,
		"mistakes":             input.Mistakes,
		"targetKnowledgePoint": input.Target,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("empty model result")
	}
	items, err := objectList(result.Payload["steps"])
	if err != nil {
		return nil, fmt.Errorf("decode review steps: %w", err)
	}
	drafts := make([]ReviewStepDraft, 0, len(items))
	for _, item := range items {
		knowledgePoint := stringValue(item["knowledgePoint"])
		if knowledgePoint == "" {
			// The target is server-selected from real evidence. Binding an
			// omitted model field to it does not allow a new knowledge point.
			knowledgePoint = input.Target
		}
		drafts = append(drafts, ReviewStepDraft{
			Type:           stringValue(item["type"]),
			Title:          stringValue(item["title"]),
			Detail:         stringValue(item["detail"]),
			KnowledgePoint: knowledgePoint,
		})
	}
	return drafts, nil
}

func (m OrchestratorModel) NextTutorQuestion(ctx context.Context, input TutorModelInput) (string, error) {
	if m.Orchestrator == nil {
		return "", errors.New("orchestrator is not configured")
	}
	result, err := m.Orchestrator.Run(ctx, nil, map[string]any{
		"skill": "socratic_tutor",
		"state": map[string]any{
			"question":  input.QuestionStem,
			"round":     input.Round,
			"hitStreak": input.HitStreak,
		},
		"reply": input.StudentReply,
	})
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", errors.New("empty model result")
	}
	return stringValue(result.Payload["prompt"]), nil
}

func (m OrchestratorModel) SummarizeClusters(ctx context.Context, groups []ClusterDraft) ([]ClusterSummaryDraft, error) {
	if m.Orchestrator == nil {
		return nil, errors.New("orchestrator is not configured")
	}
	anonymousGroups := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		anonymousGroups = append(anonymousGroups, map[string]any{
			"groupId": group.ID,
			"members": group.MemberIDs,
			"texts":   group.Texts,
		})
	}
	result, err := m.Orchestrator.Run(ctx, nil, map[string]any{
		"skill":  "聚类",
		"groups": anonymousGroups,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("empty model result")
	}
	items, err := objectList(result.Payload["clusters"])
	if err != nil {
		return nil, fmt.Errorf("decode cluster summaries: %w", err)
	}
	summaries := make([]ClusterSummaryDraft, 0, len(items))
	for index, item := range items {
		if index >= len(groups) {
			// Preserve extras so the service validator rejects invented groups.
			summaries = append(summaries, ClusterSummaryDraft{
				GroupID:   stringValue(item["groupId"]),
				MemberIDs: stringList(item["memberIds"]),
				Label:     stringValue(item["label"]),
				FollowUp:  stringValue(item["followUp"]),
			})
			continue
		}
		groupID := stringValue(item["groupId"])
		if groupID == "" {
			groupID = groups[index].ID
		}
		members := stringList(item["memberIds"])
		if len(members) == 0 {
			members = append([]string(nil), groups[index].MemberIDs...)
		}
		summaries = append(summaries, ClusterSummaryDraft{
			GroupID:   groupID,
			MemberIDs: members,
			Label:     stringValue(item["label"]),
			FollowUp:  stringValue(item["followUp"]),
		})
	}
	return summaries, nil
}

func objectList(value any) ([]map[string]any, error) {
	switch items := value.(type) {
	case []map[string]any:
		return items, nil
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("array item is not an object")
			}
			result = append(result, object)
		}
		return result, nil
	default:
		return nil, errors.New("value is not an object array")
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func stringList(value any) []string {
	switch values := value.(type) {
	case []string:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				result = append(result, value)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text := stringValue(value); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}
