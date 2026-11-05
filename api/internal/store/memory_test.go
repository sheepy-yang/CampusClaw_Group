package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeLearningOutcome(t *testing.T) {
	tests := []struct {
		name string
		in   LearningOutcome
		want LearningOutcome
	}{
		{name: "legacy empty", in: "", want: LearningOutcomeUnknown},
		{name: "correct", in: LearningOutcomeCorrect, want: LearningOutcomeCorrect},
		{name: "incorrect", in: LearningOutcomeIncorrect, want: LearningOutcomeIncorrect},
		{name: "understood", in: LearningOutcomeUnderstood, want: LearningOutcomeUnderstood},
		{name: "completed", in: LearningOutcomeCompleted, want: LearningOutcomeCompleted},
		{name: "unknown", in: LearningOutcomeUnknown, want: LearningOutcomeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLearningOutcome(tt.in)
			if err != nil {
				t.Fatalf("NormalizeLearningOutcome() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeLearningOutcome() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeLearningOutcomeRejectsInvalidValue(t *testing.T) {
	got, err := NormalizeLearningOutcome("passed")
	if !errors.Is(err, ErrInvalidLearningOutcome) {
		t.Fatalf("error = %v, want ErrInvalidLearningOutcome", err)
	}
	if got != "" {
		t.Fatalf("outcome = %q, want empty", got)
	}
}

func TestLearningEventOutcomeJSONCompatibility(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	legacy, err := json.Marshal(LearningEvent{ID: "legacy", At: now})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacy), "outcome") {
		t.Fatalf("legacy JSON unexpectedly contains outcome: %s", legacy)
	}

	current, err := json.Marshal(LearningEvent{ID: "current", Outcome: LearningOutcomeCorrect, At: now})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), `"outcome":"correct"`) {
		t.Fatalf("current JSON does not contain outcome: %s", current)
	}
}
