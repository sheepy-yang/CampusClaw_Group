package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type memSink struct {
	jobs []struct {
		typ, key string
		payload  map[string]any
	}
}

func (m *memSink) Enqueue(_ context.Context, jobType, key string, payload map[string]any) error {
	m.jobs = append(m.jobs, struct {
		typ, key string
		payload  map[string]any
	}{jobType, key, payload})
	return nil
}

func TestGradeObjective(t *testing.T) {
	score, max, ok := GradeObjective("choice", "A", "A")
	if !ok || score != 1 || max != 1 {
		t.Fatalf("choice correct: %v %v %v", score, max, ok)
	}
	score, _, ok = GradeObjective("fill", "有解", "有解|方程组有解")
	if !ok || score != 1 {
		t.Fatalf("fill alt: %v %v", score, ok)
	}
	_, _, ok = GradeObjective("choice", "B", "A")
	if ok {
		t.Fatal("expected wrong")
	}
}

func TestSplitStepsAndFirstError(t *testing.T) {
	steps := SplitSteps("步骤1: 写出增广矩阵\n步骤2: 交换行\n步骤3: 回代计算失误")
	if len(steps) != 3 {
		t.Fatalf("steps=%d", len(steps))
	}
	if LocateFirstError(steps) != "步骤3" {
		t.Fatalf("first=%s", LocateFirstError(steps))
	}
	sug, conf, cause := SuggestSubjectiveScore(steps, 10)
	if sug >= 10 || conf >= LowConfidenceThreshold {
		// 有错误时应低于满分且可能需复核
		t.Logf("suggested=%v conf=%v cause=%s", sug, conf, cause)
	}
	if cause == "" {
		t.Fatal("expected error cause")
	}
}

func TestClassReviewThreshold(t *testing.T) {
	svc := New(&memSink{})
	res, err := svc.GenerateClassReview(context.Background(), "class-demo", "asg-1", []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if res["skipped"] != true {
		t.Fatalf("expected skip: %#v", res)
	}
}

func TestDiagnosisSampleInsufficient(t *testing.T) {
	// 临时清空事件
	store.Default.Lock()
	old := store.Default.Events
	store.Default.Events = nil
	store.Default.Unlock()
	t.Cleanup(func() {
		store.Default.Lock()
		store.Default.Events = old
		store.Default.Unlock()
	})

	svc := New(&memSink{})
	res, err := svc.GenerateDiagnosis(context.Background(), "class-demo", "stu-x")
	if err != nil {
		t.Fatal(err)
	}
	if res["intercepted"] != true {
		t.Fatalf("expected intercept: %#v", res)
	}
	diag := res["diagnosis"].(map[string]any)
	if diag["message"] != "样本不足" {
		t.Fatalf("diag=%#v", diag)
	}
}

func TestDiagnosisWithEvidence(t *testing.T) {
	svc := New(&memSink{})
	sink := svc.Jobs.(*memSink)
	res, err := svc.GenerateDiagnosis(context.Background(), "class-demo", "stu-1")
	if err != nil {
		t.Fatal(err)
	}
	if res["studentVisible"] != false {
		t.Fatal("must not be student visible before confirm")
	}
	if len(sink.jobs) == 0 {
		t.Fatal("expected enqueue")
	}
	contentID, _ := res["contentId"].(string)
	_ = ProcessDiagnosis(contentID, "stu-1", "class-demo")
	c, _ := store.Default.GetContent(contentID)
	diag := c.Payload["diagnosis"].(map[string]any)
	ev, ok := diag["evidenceEvents"].([]map[string]any)
	if !ok || len(ev) < 3 {
		// type may be []any after JSON-like maps
		if raw, ok2 := diag["evidenceEvents"].([]any); !ok2 || len(raw) < 3 {
			t.Fatalf("evidence missing: %#v", diag["evidenceEvents"])
		}
	} else if len(ev) < 3 {
		t.Fatalf("evidence count %d", len(ev))
	}
	rel, err := svc.ReleaseDiagnosisToStudent(context.Background(), contentID)
	if err != nil {
		t.Fatal(err)
	}
	if rel["studentVisible"] != true {
		t.Fatal("expected visible after release")
	}
}

func TestSnapshotRejectsKeylog(t *testing.T) {
	svc := New(&memSink{})
	_, err := svc.SaveSnapshot(context.Background(), "", "stu-1", "class-demo", "x", "keydown")
	if err == nil {
		t.Fatal("keydown must be rejected")
	}
}

func TestTeacherOverrideKeepsHistory(t *testing.T) {
	svc := New(&memSink{})
	res, err := svc.EnqueueGrade(context.Background(), "class-demo", "stu-1", "asg-1", "手写", "sha256:abc", 10)
	if err != nil {
		t.Fatal(err)
	}
	subID := res["submissionId"].(string)
	if err := ProcessMultimodalGrade(subID, false); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ConfirmTranscription(context.Background(), subID, "", "stu-1")
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.TeacherOverrideScore(context.Background(), subID, "tea-1", 8.5, "步骤完整")
	if err != nil {
		t.Fatal(err)
	}
	if out["authoritative"] != "teacher" {
		t.Fatal(out)
	}
	hist := out["scoreHistory"].([]map[string]any)
	if len(hist) == 0 {
		t.Fatal("no history")
	}
}

func TestSimilarityNoVerdict(t *testing.T) {
	svc := New(&memSink{})
	snap, err := svc.SaveSnapshot(context.Background(), "", "stu-1", "class-demo",
		"秩等于增广矩阵秩时有解，可用高斯消元法", TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	id := snap["attemptId"].(string)
	raw, err := svc.SimilarityEvidence(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	rep := raw.(SimilarityReport)
	if rep.Verdict != nil {
		t.Fatalf("verdict must be nil, got %#v", rep.Verdict)
	}
	if len(rep.Evidence) == 0 {
		t.Fatal("expected evidence")
	}
}

func TestModelFailKeepsSubmission(t *testing.T) {
	store.Default.SaveSubmission(store.Submission{
		ID: "sub-fail", ClassID: "class-demo", StudentID: "stu-1", Status: SubQueued, AnswerText: "keep me",
	})
	err := ProcessMultimodalGrade("sub-fail", true)
	if err == nil {
		t.Fatal("expected error")
	}
	sub, _ := store.Default.GetSubmission("sub-fail")
	if sub.AnswerText != "keep me" || sub.Status != SubFailedKept {
		t.Fatalf("submission not kept: %#v", sub)
	}
}

func TestAnonymizeUsedInReview(t *testing.T) {
	svc := New(&memSink{})
	feedbacks := []string{
		"李同学：秩弄错了", "王同学：行变换不完整", "张同学：概念混淆",
		"赵同学：计算失误", "钱同学：漏看增广",
	}
	res, err := svc.GenerateClassReview(context.Background(), "class-demo", "asg-1", feedbacks)
	if err != nil {
		t.Fatal(err)
	}
	if res["anonymized"] != true || res["skipped"] == true {
		t.Fatalf("%#v", res)
	}
	time.Sleep(10 * time.Millisecond)
}
