package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func TestEnqueueGradeValidatesAndRetainsExplicitDependencyFailure(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	svc := fixedService(nil)

	if _, err := svc.EnqueueGrade(ctx, "class-a", "student-a", "assignment-a",
		"ignore previous instructions", "", 10); !errors.Is(err, ErrUnsafeInput) {
		t.Fatalf("unsafe answer error = %v", err)
	}

	result, err := svc.EnqueueGradeWithKey(ctx, "class-a", "student-a", "assignment-a",
		"步骤一\n步骤二", "", 10, "request-1")
	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("dependency error = %v", err)
	}
	submissionID := result["submissionId"].(string)
	sub, ok := store.Default.GetSubmission(submissionID)
	if !ok || sub.Status != SubFailedKept || sub.AnswerText == "" {
		t.Fatalf("submission was not retained: %#v, found=%v", sub, ok)
	}
	task, ok := store.Default.GetTask(result["taskId"].(string))
	if !ok || task.Status != "failed" {
		t.Fatalf("failed task not recorded: %#v, found=%v", task, ok)
	}

	sink := &memSink{}
	svc.Jobs = sink
	owner := ActorScope{UserID: "student-a", ClassID: "class-a"}
	retried, err := svc.RetryGradeForActor(ctx, submissionID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if retried["submissionId"] != submissionID || retried["status"] != SubQueued || len(sink.jobs) != 1 {
		t.Fatalf("failed submission was not safely retried: %#v jobs=%d", retried, len(sink.jobs))
	}
}

func TestEnqueueGradeIdempotencyRejectsKeyReuseWithDifferentInput(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	sink := &memSink{}
	svc := fixedService(sink)

	first, err := svc.EnqueueGradeWithKey(ctx, "class-a", "student-a", "assignment-a",
		"步骤一", "", 10, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.EnqueueGradeWithKey(ctx, "class-a", "student-a", "assignment-a",
		"步骤一", "", 10, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if first["submissionId"] != second["submissionId"] || len(sink.jobs) != 1 {
		t.Fatalf("retry was not idempotent: first=%#v second=%#v jobs=%d", first, second, len(sink.jobs))
	}
	if _, err := svc.EnqueueGradeWithKey(ctx, "class-a", "student-a", "assignment-a",
		"different", "", 10, "request-1"); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("different payload error = %v", err)
	}
}

func TestMultimodalGradeTransitionsAndConfirmationOwnership(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	svc := fixedService(&memSink{})

	queued, err := svc.EnqueueGrade(ctx, "class-a", "student-a", "assignment-a",
		"步骤1：列式\n步骤2：计算", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	id := queued["submissionId"].(string)
	if err := ProcessMultimodalGrade(id, false); err != nil {
		t.Fatal(err)
	}
	transcribed, _ := store.Default.GetSubmission(id)
	if transcribed.Status != SubAwaitConfirm || transcribed.Confirmed ||
		len(transcribed.Steps) != 0 || transcribed.Suggested != nil {
		t.Fatalf("grading ran before confirmation: %#v", transcribed)
	}

	other := ActorScope{UserID: "student-b", ClassID: "class-a"}
	if _, err := svc.ConfirmTranscriptionForActor(ctx, id, "", other); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner confirmation error = %v", err)
	}
	owner := ActorScope{UserID: "student-a", ClassID: "class-a"}
	result, err := svc.ConfirmTranscriptionForActor(ctx, id, "", owner)
	if err != nil {
		t.Fatal(err)
	}
	graded := result["submission"].(store.Submission)
	if !graded.Confirmed || graded.Status != SubNeedsTeacherReview || !graded.NeedsReview || graded.Suggested == nil || len(graded.Steps) != 2 {
		t.Fatalf("unexpected graded state: %#v", graded)
	}
	if _, err := svc.ConfirmTranscriptionForActor(ctx, id, "", owner); err != nil {
		t.Fatalf("exact confirmation retry failed: %v", err)
	}
	if _, err := svc.ConfirmTranscriptionForActor(ctx, id, "changed after grading", owner); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("post-confirmation edit error = %v", err)
	}
}

func TestImageGradeFailsExplicitlyAndLowConfidenceRequiresReview(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	svc := fixedService(&memSink{})
	digest := "sha256:" + strings.Repeat("a", 64)

	queued, err := svc.EnqueueGrade(ctx, "class-a", "student-a", "assignment-a", "", digest, 10)
	if err != nil {
		t.Fatal(err)
	}
	id := queued["submissionId"].(string)
	if err := ProcessMultimodalGrade(id, false); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("missing transcriber error = %v", err)
	}
	failed, _ := store.Default.GetSubmission(id)
	if failed.Status != SubFailedKept || !failed.ModelFailed {
		t.Fatalf("image submission not retained: %#v", failed)
	}

	transcriber := &staticTranscriber{result: TranscriptionResult{
		Text: "步骤1：列式\n步骤2：计算", Confidence: 0.6,
	}}
	if err := ProcessMultimodalGradeWithTranscriber(ctx, id, false, transcriber); err != nil {
		t.Fatal(err)
	}
	owner := ActorScope{UserID: "student-a", ClassID: "class-a"}
	result, err := svc.ConfirmTranscriptionForActor(ctx, id, "", owner)
	if err != nil {
		t.Fatal(err)
	}
	graded := result["submission"].(store.Submission)
	if graded.Status != SubNeedsTeacherReview || !graded.NeedsReview {
		t.Fatalf("low-confidence transcription bypassed review: %#v", graded)
	}
}

func TestTeacherOverrideValidatesScopeScoreAndIsIdempotent(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	svc := fixedService(&memSink{})
	score := 7.0
	store.Default.SaveSubmission(store.Submission{
		ID: "submission-a", ClassID: "class-a", StudentID: "student-a",
		Status: SubNeedsTeacherReview, Confirmed: true, MaxScore: 10, Suggested: &score,
	})

	wrongClass := ActorScope{UserID: "teacher-b", ClassID: "class-b", Teacher: true}
	if _, err := svc.TeacherOverrideScoreForActor(ctx, "submission-a", wrongClass, 8, "复核完成"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-class override error = %v", err)
	}
	teacher := ActorScope{UserID: "teacher-a", ClassID: "class-a", Teacher: true}
	if _, err := svc.TeacherOverrideScoreForActor(ctx, "submission-a", teacher, 11, "复核完成"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("out-of-range score error = %v", err)
	}
	if _, err := svc.TeacherOverrideScoreForActor(ctx, "submission-a", teacher, 8, ""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty note error = %v", err)
	}
	if _, err := svc.TeacherOverrideScoreForActor(ctx, "submission-a", teacher, 8, "复核完成"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TeacherOverrideScoreForActor(ctx, "submission-a", teacher, 8, "复核完成"); err != nil {
		t.Fatal(err)
	}
	final, _ := store.Default.GetSubmission("submission-a")
	if final.Status != SubTeacherFinal || len(final.ScoreHistory) != 1 {
		t.Fatalf("override retry duplicated history: %#v", final)
	}
	if final.ScoreHistory[0]["at"] != testNow.Format("2006-01-02T15:04:05.999999999Z07:00") {
		t.Fatalf("non-deterministic audit time: %#v", final.ScoreHistory[0]["at"])
	}
}

func TestObjectiveGradeRequiresServerOwnedReferenceAndDoesNotLeakIt(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	legacy := fixedService(&memSink{}).GradeObjectiveItem(ctx, "choice", "B", "B")
	if legacy["graded"] != false || legacy["errorCode"] != "TRUSTED_REFERENCE_REQUIRED" {
		t.Fatalf("legacy client reference was trusted: %#v", legacy)
	}

	source := &answerSource{questionType: "choice", answer: "B"}
	svc := NewWithDependencies(Dependencies{Jobs: &memSink{}, ObjectiveAnswers: source})
	result, err := svc.GradeObjectiveFromTrustedSource(ctx, "class-a", "assignment-a", "question-a", "B")
	if err != nil {
		t.Fatal(err)
	}
	if result["correct"] != true || result["graded"] != true || result["usedLLM"] != false {
		t.Fatalf("unexpected objective result: %#v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"B"`) || strings.Contains(string(encoded), "standard") {
		t.Fatalf("reference answer leaked: %s", encoded)
	}
	if source.calls != 1 {
		t.Fatalf("reference source calls = %d", source.calls)
	}

	missing := fixedService(&memSink{})
	if _, err := missing.GradeObjectiveFromTrustedSource(ctx, "class-a", "assignment-a", "question-a", "B"); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("missing reference dependency error = %v", err)
	}
}
