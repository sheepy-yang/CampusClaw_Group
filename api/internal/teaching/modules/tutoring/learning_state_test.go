package tutoring

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

var fixedLearningStateNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func learningEvent(id, studentID, classID, kp string, outcome store.LearningOutcome, at time.Time) store.LearningEvent {
	return store.LearningEvent{
		ID: id, StudentID: studentID, ClassID: classID,
		Kind: "practice", KP: kp, Outcome: outcome, At: at,
	}
}

func TestSummarizeLearningStateFiltersAndAggregatesEvidence(t *testing.T) {
	now := fixedLearningStateNow
	start := now.Add(-14 * 24 * time.Hour)
	events := []store.LearningEvent{
		learningEvent("correct", "stu-1", "class-1", "高斯消元", store.LearningOutcomeCorrect, now.Add(-time.Hour)),
		learningEvent("incorrect", "stu-1", "class-1", "高斯消元", store.LearningOutcomeIncorrect, now.Add(-2*time.Hour)),
		learningEvent("understood", "stu-1", "class-1", "高斯消元", store.LearningOutcomeUnderstood, now.Add(-3*time.Hour)),
		learningEvent("completed", "stu-1", "class-1", "高斯消元", store.LearningOutcomeCompleted, now.Add(-4*time.Hour)),
		learningEvent("unknown", "stu-1", "class-1", "高斯消元", store.LearningOutcomeUnknown, now.Add(-5*time.Hour)),
		learningEvent("legacy", "stu-1", "class-1", "高斯消元", "", now.Add(-6*time.Hour)),
		learningEvent("boundary", "stu-1", "class-1", "矩阵秩", store.LearningOutcomeCorrect, start),
		learningEvent("second-kp", "stu-1", "class-1", "矩阵秩", store.LearningOutcomeCorrect, now),
		learningEvent("too-old", "stu-1", "class-1", "高斯消元", store.LearningOutcomeIncorrect, start.Add(-time.Nanosecond)),
		learningEvent("future", "stu-1", "class-1", "高斯消元", store.LearningOutcomeIncorrect, now.Add(time.Nanosecond)),
		learningEvent("other-student", "stu-2", "class-1", "高斯消元", store.LearningOutcomeIncorrect, now),
		learningEvent("other-class", "stu-1", "class-2", "高斯消元", store.LearningOutcomeIncorrect, now),
		learningEvent("empty-kp", "stu-1", "class-1", "  ", store.LearningOutcomeCorrect, now),
	}

	got, err := SummarizeLearningState(events, "stu-1", "class-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.StudentID != "stu-1" || got.ClassID != "class-1" || got.WindowDays != 14 {
		t.Fatalf("unexpected scope: %#v", got)
	}
	if len(got.KnowledgePoints) != 2 {
		t.Fatalf("knowledge points = %#v, want 2", got.KnowledgePoints)
	}

	// Results are sorted by knowledge point for deterministic output.
	rank := got.KnowledgePoints[0]
	if rank.KnowledgePoint != "矩阵秩" || rank.EventCount != 2 || rank.EvidenceCount != 2 {
		t.Fatalf("rank state = %#v", rank)
	}
	if rank.MasteryLevel != MasteryInsufficientData {
		t.Fatalf("rank mastery = %q", rank.MasteryLevel)
	}

	gauss := got.KnowledgePoints[1]
	if gauss.KnowledgePoint != "高斯消元" {
		t.Fatalf("unexpected stable order: %#v", got.KnowledgePoints)
	}
	if gauss.EventCount != 6 || gauss.EvidenceCount != 3 {
		t.Fatalf("gauss counts = %#v", gauss)
	}
	if gauss.CorrectCount != 1 || gauss.IncorrectCount != 1 || gauss.UnderstoodCount != 1 {
		t.Fatalf("gauss evidence = %#v", gauss)
	}
	if gauss.SuccessRate != 0.6667 || gauss.MasteryLevel != MasteryDeveloping {
		t.Fatalf("gauss result = %#v", gauss)
	}
}

func TestMasteryLevelBoundaries(t *testing.T) {
	now := fixedLearningStateNow
	tests := []struct {
		name      string
		outcomes  []store.LearningOutcome
		wantRate  float64
		wantLevel MasteryLevel
	}{
		{name: "fewer than three evidence", outcomes: []store.LearningOutcome{store.LearningOutcomeCorrect, store.LearningOutcomeCorrect}, wantRate: 1, wantLevel: MasteryInsufficientData},
		{name: "below fifty percent", outcomes: []store.LearningOutcome{store.LearningOutcomeCorrect, store.LearningOutcomeIncorrect, store.LearningOutcomeIncorrect}, wantRate: 0.3333, wantLevel: MasteryWeak},
		{name: "exactly fifty percent", outcomes: []store.LearningOutcome{store.LearningOutcomeCorrect, store.LearningOutcomeUnderstood, store.LearningOutcomeIncorrect, store.LearningOutcomeIncorrect}, wantRate: 0.5, wantLevel: MasteryDeveloping},
		{name: "below eighty percent", outcomes: []store.LearningOutcome{store.LearningOutcomeCorrect, store.LearningOutcomeCorrect, store.LearningOutcomeUnderstood, store.LearningOutcomeIncorrect}, wantRate: 0.75, wantLevel: MasteryDeveloping},
		{name: "exactly eighty percent", outcomes: []store.LearningOutcome{store.LearningOutcomeCorrect, store.LearningOutcomeCorrect, store.LearningOutcomeCorrect, store.LearningOutcomeUnderstood, store.LearningOutcomeIncorrect}, wantRate: 0.8, wantLevel: MasterySolid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make([]store.LearningEvent, 0, len(tt.outcomes))
			for i, outcome := range tt.outcomes {
				events = append(events, learningEvent(string(rune('a'+i)), "stu-1", "class-1", "kp", outcome, now))
			}
			got, err := SummarizeLearningState(events, "stu-1", "class-1", now)
			if err != nil {
				t.Fatal(err)
			}
			point := got.KnowledgePoints[0]
			if point.SuccessRate != tt.wantRate || point.MasteryLevel != tt.wantLevel {
				t.Fatalf("result = rate %v level %q, want rate %v level %q", point.SuccessRate, point.MasteryLevel, tt.wantRate, tt.wantLevel)
			}
		})
	}
}

func TestActivityOnlyOutcomesAreInsufficientData(t *testing.T) {
	now := fixedLearningStateNow
	events := []store.LearningEvent{
		learningEvent("completed", "stu-1", "class-1", "kp", store.LearningOutcomeCompleted, now),
		learningEvent("unknown", "stu-1", "class-1", "kp", store.LearningOutcomeUnknown, now),
		learningEvent("legacy", "stu-1", "class-1", "kp", "", now),
	}
	got, err := SummarizeLearningState(events, "stu-1", "class-1", now)
	if err != nil {
		t.Fatal(err)
	}
	point := got.KnowledgePoints[0]
	if point.EventCount != 3 || point.EvidenceCount != 0 || point.SuccessRate != 0 || point.MasteryLevel != MasteryInsufficientData {
		t.Fatalf("activity-only state = %#v", point)
	}
}

func TestSummarizeLearningStateEmptyAndDeterministic(t *testing.T) {
	first, err := SummarizeLearningState(nil, "stu-1", "class-1", fixedLearningStateNow)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SummarizeLearningState(nil, "stu-1", "class-1", fixedLearningStateNow)
	if err != nil {
		t.Fatal(err)
	}
	if first.KnowledgePoints == nil || len(first.KnowledgePoints) != 0 {
		t.Fatalf("empty knowledge points = %#v", first.KnowledgePoints)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same input produced different output:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestSummarizeLearningStateRejectsInvalidOutcome(t *testing.T) {
	events := []store.LearningEvent{learningEvent("invalid", "stu-1", "class-1", "kp", "passed", fixedLearningStateNow)}
	_, err := SummarizeLearningState(events, "stu-1", "class-1", fixedLearningStateNow)
	if !errors.Is(err, store.ErrInvalidLearningOutcome) {
		t.Fatalf("error = %v, want ErrInvalidLearningOutcome", err)
	}
}

func TestGetLearningStateRequiresAndEnforcesScope(t *testing.T) {
	memory := &store.Memory{Events: []store.LearningEvent{
		learningEvent("wanted", "stu-1", "class-1", "kp", store.LearningOutcomeCorrect, fixedLearningStateNow),
		learningEvent("other-student", "stu-2", "class-1", "kp", store.LearningOutcomeIncorrect, fixedLearningStateNow),
		learningEvent("other-class", "stu-1", "class-2", "kp", store.LearningOutcomeIncorrect, fixedLearningStateNow),
	}}
	got, err := GetLearningState(memory, "stu-1", "class-1", fixedLearningStateNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.KnowledgePoints) != 1 || got.KnowledgePoints[0].EventCount != 1 {
		t.Fatalf("scoped result = %#v", got)
	}

	for _, scope := range []struct{ studentID, classID string }{{"", "class-1"}, {"stu-1", ""}} {
		if _, err := GetLearningState(memory, scope.studentID, scope.classID, fixedLearningStateNow); !errors.Is(err, ErrLearningStateScopeRequired) {
			t.Fatalf("scope (%q,%q) error = %v", scope.studentID, scope.classID, err)
		}
	}
	if _, err := GetLearningState(nil, "stu-1", "class-1", fixedLearningStateNow); !errors.Is(err, ErrLearningStateStoreRequired) {
		t.Fatalf("nil store error = %v", err)
	}
}
