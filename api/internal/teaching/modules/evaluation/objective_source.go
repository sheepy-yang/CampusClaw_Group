package evaluation

import (
	"context"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type StoreObjectiveAnswerSource struct {
	Memory *store.Memory
}

func (s StoreObjectiveAnswerSource) ReferenceAnswer(
	_ context.Context,
	classID, assignmentID, questionID string,
) (string, string, error) {
	memory := s.Memory
	if memory == nil {
		memory = store.Default
	}
	assignment, ok := memory.GetAssignment(strings.TrimSpace(assignmentID))
	if !ok || assignment.ClassID != strings.TrimSpace(classID) || assignment.Status != "published" {
		return "", "", ErrNotFound
	}
	for _, question := range assignment.Items {
		if question.ID == strings.TrimSpace(questionID) {
			if !validObjectiveType(question.Type) || strings.TrimSpace(question.Answer) == "" {
				return "", "", ErrInvalidArgument
			}
			return question.Type, question.Answer, nil
		}
	}
	return "", "", ErrNotFound
}

var _ ObjectiveAnswerSource = StoreObjectiveAnswerSource{}
