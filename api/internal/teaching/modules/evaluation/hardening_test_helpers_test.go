package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

var testNow = time.Date(2026, time.November, 6, 1, 2, 3, 4, time.UTC)

func useFreshEvaluationStore(t *testing.T) {
	t.Helper()
	previous := store.Default
	store.Default = store.New()
	store.Default.SaveAssignment(store.Assignment{
		ID: "assignment-a", ClassID: "class-a", Status: "published", MaxScore: 10,
		Items: []store.Question{{ID: "q-1", Stem: "说明计算步骤", Answer: "列式并计算", Explanation: "步骤和结果均需正确"}},
	})
	t.Cleanup(func() {
		store.Default = previous
	})
}

func fixedService(jobs JobSink) *Services {
	return NewWithDependencies(Dependencies{
		Jobs: jobs,
		Now:  func() time.Time { return testNow },
	})
}

type failingSink struct {
	err   error
	calls int
}

func (s *failingSink) Enqueue(context.Context, string, string, map[string]any) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	return errors.New("queue unavailable")
}

type answerSource struct {
	questionType string
	answer       string
	err          error
	calls        int
}

func (s *answerSource) ReferenceAnswer(context.Context, string, string, string) (string, string, error) {
	s.calls++
	return s.questionType, s.answer, s.err
}

type staticTranscriber struct {
	result TranscriptionResult
	err    error
	calls  int
}

func (t *staticTranscriber) Transcribe(context.Context, string, string, string) (TranscriptionResult, error) {
	t.calls++
	return t.result, t.err
}

func saveReviewSubmission(classID, assignmentID, id, studentID, cause string) {
	score := 8.0
	store.Default.SaveSubmission(store.Submission{
		ID: id, ClassID: classID, AssignmentID: assignmentID, StudentID: studentID,
		AnswerText: "步骤完整", Transcription: "步骤完整", Confirmed: true,
		Suggested: &score, Score: &score, MaxScore: 10,
		ErrorCause: cause, Status: SubGradedSuggested,
	})
}
