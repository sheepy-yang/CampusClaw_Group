package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func TestClassReviewThresholdUsesStoredUniqueEffectiveSubmissions(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	sink := &memSink{}
	svc := fixedService(sink)

	for i := 0; i < MinClassReviewSamples; i++ {
		studentID := fmt.Sprintf("student-%d", i)
		if i == MinClassReviewSamples-1 {
			studentID = "student-0"
		}
		saveReviewSubmission("class-a", "assignment-a", fmt.Sprintf("submission-%d", i), studentID, "计算失误")
	}
	result, err := svc.GenerateClassReview(ctx, "class-a", "assignment-a",
		[]string{"伪造1", "伪造2", "伪造3", "伪造4", "伪造5"})
	if err != nil {
		t.Fatal(err)
	}
	if result["skipped"] != true || result["count"] != MinClassReviewSamples-1 || len(sink.jobs) != 0 {
		t.Fatalf("threshold bypassed: result=%#v jobs=%d", result, len(sink.jobs))
	}
}

func TestClassReviewRejectsClientEvidenceAndIdentityLeak(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	sink := &memSink{}
	svc := fixedService(sink)
	for i := 0; i < MinClassReviewSamples; i++ {
		saveReviewSubmission("class-a", "assignment-a", fmt.Sprintf("submission-%d", i),
			fmt.Sprintf("student-%d", i), "计算失误")
	}

	if _, err := svc.GenerateClassReview(ctx, "class-a", "assignment-a",
		[]string{"a", "b", "c", "d", "e"}); !errors.Is(err, ErrUntrustedEvidence) {
		t.Fatalf("client evidence error = %v", err)
	}
	if len(sink.jobs) != 0 {
		t.Fatalf("untrusted evidence enqueued %d jobs", len(sink.jobs))
	}

	saveReviewSubmission("class-b", "assignment-b", "leaky-0", "student-0", "李同学计算失误")
	for i := 1; i < MinClassReviewSamples; i++ {
		saveReviewSubmission("class-b", "assignment-b", fmt.Sprintf("leaky-%d", i),
			fmt.Sprintf("student-%d", i), "计算失误")
	}
	if _, err := svc.GenerateClassReview(ctx, "class-b", "assignment-b", nil); !errors.Is(err, ErrUnsafeInput) {
		t.Fatalf("identity leak error = %v", err)
	}
}

func TestClassReviewPersistsAnonymousBoundaryAndRejectsTamperedJob(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	sink := &memSink{}
	svc := fixedService(sink)
	for i := 0; i < MinClassReviewSamples; i++ {
		cause := "计算失误"
		if i < 2 {
			cause = "概念混淆"
		}
		saveReviewSubmission("class-a", "assignment-a", fmt.Sprintf("submission-%d", i),
			fmt.Sprintf("student-%d", i), cause)
	}

	result, err := svc.GenerateClassReview(ctx, "class-a", "assignment-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	contentID := result["contentId"].(string)
	content, ok := store.Default.GetContent(contentID)
	if !ok || content.Status != string(domain.StatusGenerating) {
		t.Fatalf("generating content missing: %#v, found=%v", content, ok)
	}
	inputs, ok := toStringSlice(content.Payload["anonymousInputs"])
	if !ok || len(inputs) != MinClassReviewSamples {
		t.Fatalf("anonymous inputs = %#v", content.Payload["anonymousInputs"])
	}
	for _, input := range inputs {
		if containsNameLeak(input) || strings.Contains(input, "student-") {
			t.Fatalf("identity reached model boundary: %q", input)
		}
	}

	if err := ProcessClassReview(contentID, []string{"tampered", "tampered", "tampered", "tampered", "tampered"}); !errors.Is(err, ErrUntrustedEvidence) {
		t.Fatalf("tampered job error = %v", err)
	}
	if err := ProcessClassReview(contentID, inputs); err != nil {
		t.Fatal(err)
	}
	if err := ProcessClassReview(contentID, inputs); err != nil {
		t.Fatalf("worker retry was not idempotent: %v", err)
	}
	content, _ = store.Default.GetContent(contentID)
	review := content.Payload["review"].(map[string]any)
	if content.Status != string(domain.StatusPending) || review["teacherOnly"] != true ||
		review["inputCount"] != MinClassReviewSamples {
		t.Fatalf("unexpected review state: %#v", content)
	}
	causes := review["commonCauses"].([]string)
	if len(causes) != 2 || causes[0] != "计算失误" || causes[1] != "概念混淆" {
		t.Fatalf("review was not derived deterministically: %#v", causes)
	}
}

func TestClassReviewDependencyFailureIsExplicitAndRetriable(t *testing.T) {
	useFreshEvaluationStore(t)
	ctx := context.Background()
	for i := 0; i < MinClassReviewSamples; i++ {
		saveReviewSubmission("class-a", "assignment-a", fmt.Sprintf("submission-%d", i),
			fmt.Sprintf("student-%d", i), "计算失误")
	}
	sink := &failingSink{err: errors.New("down")}
	svc := fixedService(sink)
	result, err := svc.GenerateClassReview(ctx, "class-a", "assignment-a", nil)
	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("dependency error = %v", err)
	}
	task, ok := store.Default.GetTask(result["taskId"].(string))
	if !ok || task.Status != "failed" {
		t.Fatalf("failed task = %#v, found=%v", task, ok)
	}

	svc.Jobs = &memSink{}
	retried, err := svc.GenerateClassReview(ctx, "class-a", "assignment-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if retried["contentId"] != result["contentId"] {
		t.Fatalf("retry created duplicate content: first=%#v retry=%#v", result, retried)
	}
}

func TestClassReviewAcceptsDeterministicallyGradedPlatformSubmissions(t *testing.T) {
	useFreshEvaluationStore(t)
	sink := &memSink{}
	svc := fixedService(sink)
	for i := 0; i < MinClassReviewSamples; i++ {
		submissionID := fmt.Sprintf("platform-%d", i)
		store.Default.SaveSubmission(store.Submission{
			ID: submissionID, ClassID: "class-a", AssignmentID: "assignment-a",
			StudentID: fmt.Sprintf("student-%d", i), Status: "graded",
		})
		store.Default.AppendMistake(store.Mistake{
			ID: "mistake-" + submissionID, ClassID: "class-a",
			StudentID: fmt.Sprintf("student-%d", i), SubmissionID: submissionID,
			ErrorCause: "答案与评分规则不一致",
		})
	}
	result, err := svc.GenerateClassReview(context.Background(), "class-a", "assignment-a", nil)
	if err != nil || result["skipped"] == true || len(sink.jobs) != 1 {
		t.Fatalf("result=%#v jobs=%d err=%v", result, len(sink.jobs), err)
	}
	content, ok := store.Default.GetContent(result["contentId"].(string))
	if !ok {
		t.Fatal("class review content was not persisted")
	}
	inputs, _ := toStringSlice(content.Payload["anonymousInputs"])
	if len(inputs) != MinClassReviewSamples || inputs[0] != "答案与评分规则不一致" {
		t.Fatalf("platform mistake evidence was not used: %#v", inputs)
	}
}
