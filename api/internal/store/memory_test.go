package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProductionStoreDoesNotInjectDemoEvidence(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	memory := New()
	if len(memory.Materials) != 0 || len(memory.Mistakes) != 0 || len(memory.Events) != 0 {
		t.Fatalf("production store contains demo data: materials=%d mistakes=%d events=%d",
			len(memory.Materials), len(memory.Mistakes), len(memory.Events))
	}
}

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

func TestChunkMaterialIsTraceableAndBounded(t *testing.T) {
	content := strings.Repeat("高斯消元需要对增广矩阵做初等行变换。", 40)
	chunks := ChunkMaterial("mat-test", "v3", "测试讲义", content)
	if len(chunks) < 2 {
		t.Fatalf("chunk count = %d, want multiple", len(chunks))
	}
	for i, chunk := range chunks {
		if chunk.MaterialID != "mat-test" || chunk.MaterialVersion != "v3" || chunk.Index != i {
			t.Fatalf("invalid trace metadata: %#v", chunk)
		}
		if chunk.CharacterEnd <= chunk.CharacterStart || len([]rune(chunk.Text)) > 320 || chunk.ContentHash != Digest(chunk.Text) {
			t.Fatalf("invalid chunk bounds/hash: %#v", chunk)
		}
	}
}

func TestSearchChunksDoesNotLeakOtherClass(t *testing.T) {
	memory := New()
	hits := memory.SearchChunks("class-other", "高斯消元", 10)
	for _, hit := range hits {
		if hit.MaterialID == "mat-1" {
			t.Fatalf("cross-class hit leaked: %#v", hit)
		}
	}
}

func TestClaimIdempotencyDetectsPayloadConflict(t *testing.T) {
	memory := New()
	first, duplicate, err := memory.ClaimIdempotency("stu", "submit", "key", "hash-a", "task-a")
	if err != nil || duplicate || first.ID != "task-a" {
		t.Fatalf("first claim = %#v duplicate=%v err=%v", first, duplicate, err)
	}
	repeated, duplicate, err := memory.ClaimIdempotency("stu", "submit", "key", "hash-a", "task-b")
	if err != nil || !duplicate || repeated.ID != "task-a" {
		t.Fatalf("repeat claim = %#v duplicate=%v err=%v", repeated, duplicate, err)
	}
	if _, _, err := memory.ClaimIdempotency("stu", "submit", "key", "hash-b", "task-c"); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting claim error = %v", err)
	}
}
