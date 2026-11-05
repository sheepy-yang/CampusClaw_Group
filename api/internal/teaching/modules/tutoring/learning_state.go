package tutoring

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

const learningStateWindowDays = 14

var (
	ErrLearningStateScopeRequired = errors.New("student and class scope are required")
	ErrLearningStateStoreRequired = errors.New("learning state store is required")
)

type MasteryLevel string

const (
	MasteryWeak             MasteryLevel = "weak"
	MasteryDeveloping       MasteryLevel = "developing"
	MasterySolid            MasteryLevel = "solid"
	MasteryInsufficientData MasteryLevel = "insufficient_data"
)

// KnowledgePointState is an explainable summary of explicit event evidence.
// The mastery level is an MVP heuristic, not a scientifically validated model.
type KnowledgePointState struct {
	KnowledgePoint  string       `json:"knowledgePoint"`
	EventCount      int          `json:"eventCount"`
	EvidenceCount   int          `json:"evidenceCount"`
	CorrectCount    int          `json:"correctCount"`
	IncorrectCount  int          `json:"incorrectCount"`
	UnderstoodCount int          `json:"understoodCount"`
	SuccessRate     float64      `json:"successRate"`
	MasteryLevel    MasteryLevel `json:"masteryLevel"`
}

type LearningState struct {
	StudentID       string                `json:"studentId"`
	ClassID         string                `json:"classId"`
	WindowDays      int                   `json:"windowDays"`
	KnowledgePoints []KnowledgePointState `json:"knowledgePoints"`
}

// GetLearningState reads already-scoped events from the existing Memory store.
// Empty scope is rejected so existing permissive store queries cannot degrade
// into an accidental all-student or all-class query.
func GetLearningState(memory *store.Memory, studentID, classID string, now time.Time) (LearningState, error) {
	if memory == nil {
		return LearningState{}, ErrLearningStateStoreRequired
	}
	if strings.TrimSpace(studentID) == "" || strings.TrimSpace(classID) == "" {
		return LearningState{}, ErrLearningStateScopeRequired
	}
	return SummarizeLearningState(memory.EventsFor(studentID, classID), studentID, classID, now)
}

// SummarizeLearningState deterministically aggregates events in the inclusive
// rolling window [now-14d, now]. Invalid outcomes are returned as errors.
func SummarizeLearningState(events []store.LearningEvent, studentID, classID string, now time.Time) (LearningState, error) {
	if strings.TrimSpace(studentID) == "" || strings.TrimSpace(classID) == "" {
		return LearningState{}, ErrLearningStateScopeRequired
	}

	state := LearningState{
		StudentID:       studentID,
		ClassID:         classID,
		WindowDays:      learningStateWindowDays,
		KnowledgePoints: make([]KnowledgePointState, 0),
	}
	windowStart := now.Add(-learningStateWindowDays * 24 * time.Hour)
	byKnowledgePoint := make(map[string]*KnowledgePointState)

	for _, event := range events {
		if event.StudentID != studentID || event.ClassID != classID {
			continue
		}
		knowledgePoint := strings.TrimSpace(event.KP)
		if knowledgePoint == "" || event.At.Before(windowStart) || event.At.After(now) {
			continue
		}

		outcome, err := store.NormalizeLearningOutcome(event.Outcome)
		if err != nil {
			return LearningState{}, err
		}
		point := byKnowledgePoint[knowledgePoint]
		if point == nil {
			point = &KnowledgePointState{KnowledgePoint: knowledgePoint}
			byKnowledgePoint[knowledgePoint] = point
		}
		point.EventCount++
		switch outcome {
		case store.LearningOutcomeCorrect:
			point.CorrectCount++
		case store.LearningOutcomeIncorrect:
			point.IncorrectCount++
		case store.LearningOutcomeUnderstood:
			point.UnderstoodCount++
		case store.LearningOutcomeCompleted, store.LearningOutcomeUnknown:
			// Activity-only outcomes do not contribute mastery evidence.
		}
	}

	knowledgePoints := make([]string, 0, len(byKnowledgePoint))
	for knowledgePoint := range byKnowledgePoint {
		knowledgePoints = append(knowledgePoints, knowledgePoint)
	}
	sort.Strings(knowledgePoints)

	for _, knowledgePoint := range knowledgePoints {
		point := byKnowledgePoint[knowledgePoint]
		point.EvidenceCount = point.CorrectCount + point.IncorrectCount + point.UnderstoodCount
		if point.EvidenceCount > 0 {
			rawRate := float64(point.CorrectCount+point.UnderstoodCount) / float64(point.EvidenceCount)
			point.SuccessRate = math.Round(rawRate*10000) / 10000
		}
		point.MasteryLevel = masteryLevel(point.EvidenceCount, point.SuccessRate)
		state.KnowledgePoints = append(state.KnowledgePoints, *point)
	}

	return state, nil
}

func masteryLevel(evidenceCount int, successRate float64) MasteryLevel {
	if evidenceCount < 3 {
		return MasteryInsufficientData
	}
	if successRate < 0.5 {
		return MasteryWeak
	}
	if successRate < 0.8 {
		return MasteryDeveloping
	}
	return MasterySolid
}
