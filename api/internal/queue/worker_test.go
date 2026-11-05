package queue

import (
	"context"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

func withFreshWorkerStore(t *testing.T) {
	t.Helper()
	previous := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = previous })
}

func TestAssignmentGradeUsesTeacherDefinedMaximum(t *testing.T) {
	withFreshWorkerStore(t)
	store.Default.SaveAssignment(store.Assignment{
		ID: "assignment-objective", ClassID: "class-a", Status: "published", MaxScore: 20,
		Items: []store.Question{{ID: "q-1", Type: "choice", Answer: "B", Stem: "选择 B"}},
	})
	store.Default.SaveSubmission(store.Submission{
		ID: "submission-objective", AssignmentID: "assignment-objective",
		ClassID: "class-a", StudentID: "student-a", Answers: map[string]string{"q-1": "B"},
	})
	if err := handleAssignmentGrade(context.Background(), Job{Payload: map[string]any{"submissionId": "submission-objective"}}); err != nil {
		t.Fatal(err)
	}
	submission, _ := store.Default.GetSubmission("submission-objective")
	if submission.Score == nil || *submission.Score != 20 || submission.MaxScore != 20 || submission.Status != "graded" {
		t.Fatalf("objective result = %#v", submission)
	}
}

func TestSubjectiveAssignmentNeverBecomesFinalWithoutTeacherReview(t *testing.T) {
	withFreshWorkerStore(t)
	store.Default.SaveAssignment(store.Assignment{
		ID: "assignment-subjective", ClassID: "class-a", Status: "published", MaxScore: 10,
		Items: []store.Question{{
			ID: "q-1", Type: "short", Stem: "解释等价变换",
			Answer: "初等行变换可逆", Explanation: "可逆变换保持解集不变",
		}},
	})
	store.Default.SaveSubmission(store.Submission{
		ID: "submission-subjective", AssignmentID: "assignment-subjective",
		ClassID: "class-a", StudentID: "student-a",
		Answers: map[string]string{"q-1": "初等行变换可逆，所以保持解集不变"},
	})
	if err := handleAssignmentGrade(context.Background(), Job{Payload: map[string]any{"submissionId": "submission-subjective"}}); err != nil {
		t.Fatal(err)
	}
	submission, _ := store.Default.GetSubmission("submission-subjective")
	if submission.Status != evaluation.SubNeedsTeacherReview || !submission.NeedsReview || submission.Score != nil || submission.Suggested == nil {
		t.Fatalf("subjective result = %#v", submission)
	}
}
