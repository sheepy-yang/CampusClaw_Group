package teaching

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

type syncEvalJobs struct {
	last map[string]any
}

func (j *syncEvalJobs) Enqueue(_ context.Context, jobType, _ string, payload map[string]any) error {
	j.last = map[string]any{"type": jobType, "payload": payload}
	switch jobType {
	case "multimodal_grade", "handwriting_ocr":
		id, _ := payload["submissionId"].(string)
		return evaluation.ProcessMultimodalGrade(id, false)
	case "class_review":
		id, _ := payload["contentId"].(string)
		anon, _ := payload["anonymousInputs"].([]string)
		if anon == nil {
			if raw, ok := payload["anonymousInputs"].([]any); ok {
				for _, item := range raw {
					anon = append(anon, fmt.Sprint(item))
				}
			}
		}
		return evaluation.ProcessClassReview(id, anon)
	case "diagnosis":
		id, _ := payload["contentId"].(string)
		studentID, _ := payload["studentId"].(string)
		classID, _ := payload["classId"].(string)
		return evaluation.ProcessDiagnosis(id, studentID, classID)
	case "similarity":
		id, _ := payload["attemptId"].(string)
		return evaluation.ProcessSimilarity(id)
	default:
		return fmt.Errorf("unknown job %s", jobType)
	}
}

func withEvaluationHTTP(t *testing.T) (http.Handler, *syncEvalJobs) {
	t.Helper()
	originalStore, originalDeps := store.Default, deps
	store.Default = store.New()
	// 去掉 seed 诊断证据，避免干扰样本门槛用例；作业仍重新写入。
	store.Default.Events = nil
	store.Default.Mistakes = nil
	store.Default.Assignments = map[string]store.Assignment{}
	store.Default.Submissions = map[string]store.Submission{}
	store.Default.Contents = map[string]store.ContentRecord{}
	store.Default.Tasks = map[string]store.TaskRecord{}
	jobs := &syncEvalJobs{}
	deps = &apiDeps{
		Eval: evaluation.NewWithDependencies(evaluation.Dependencies{
			Jobs:             jobs,
			ObjectiveAnswers: evaluation.StoreObjectiveAnswerSource{},
			Now:              func() time.Time { return time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC) },
		}),
		Audit: security.NewAuditLogger(),
	}
	t.Cleanup(func() {
		store.Default = originalStore
		deps = originalDeps
	})

	now := time.Now().UTC()
	store.Default.SaveAssignment(store.Assignment{
		ID: "asg-http-1", ClassID: "class-demo", Title: "HTTP 主观题", Status: "published",
		MaxScore: 10, CreatedBy: "tea-1", CreatedAt: now,
		Items: []store.Question{{
			ID: "q1", Type: "subjective", Stem: "说明高斯消元步骤",
			Answer: "写出增广矩阵并做初等行变换，比较系数矩阵与增广矩阵秩。",
			Explanation: "秩相等则有解。", Status: "published",
		}},
	})
	store.Default.SetSkills("class-demo", map[string]bool{
		"multimodal_grade": true, "class_review": true, "diagnosis": true, "answer_replay": true,
	})

	mux := http.NewServeMux()
	RegisterEvaluationRoutes(mux)
	return mux, jobs
}

func evalRequest(handler http.Handler, method, path, token, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		switch token {
		case "teacher-demo":
			token, _, _ = auth.LoginWithPassword("teacher", "demo123")
		case "student-demo":
			token, _, _ = auth.LoginWithPassword("student", "demo123")
		case "student-demo-2":
			token, _, _ = auth.LoginWithPassword("student2", "demo123")
		case "teacher-other":
			token, _, _ = auth.LoginWithPassword("teacher_other", "demo123")
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeEvalMap(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v body=%s", err, response.Body.String())
	}
	return out
}

func assertEvalAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	var apiErr httpapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if apiErr.Code != code {
		t.Fatalf("code=%q want=%q body=%#v", apiErr.Code, code, apiErr)
	}
}

func TestEvaluationHTTPMultimodalGradeHappyPathAndIsolation(t *testing.T) {
	handler, _ := withEvaluationHTTP(t)

	unauthorized := evalRequest(handler, http.MethodPost, "/api/v1/skills/multimodal-grade", "", `{}`, nil)
	assertEvalAPIError(t, unauthorized, http.StatusUnauthorized, "UNAUTHORIZED")

	teacherBlocked := evalRequest(handler, http.MethodPost, "/api/v1/skills/multimodal-grade", "teacher-demo",
		`{"assignmentId":"asg-http-1","answerText":"步骤1：列增广矩阵\n步骤2：行变换"}`,
		map[string]string{"Idempotency-Key": "grade-key-teacher-1"},
	)
	assertEvalAPIError(t, teacherBlocked, http.StatusForbidden, "FORBIDDEN")

	missingKey := evalRequest(handler, http.MethodPost, "/api/v1/skills/multimodal-grade", "student-demo",
		`{"assignmentId":"asg-http-1","answerText":"步骤1：列增广矩阵"}`, nil)
	assertEvalAPIError(t, missingKey, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY")

	accepted := evalRequest(handler, http.MethodPost, "/api/v1/skills/multimodal-grade", "student-demo",
		`{"assignmentId":"asg-http-1","answerText":"步骤1：写出增广矩阵\n步骤2：初等行变换求阶梯型\n步骤3：比较秩判断有解"}`,
		map[string]string{"Idempotency-Key": "grade-key-student-1"},
	)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("enqueue status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	payload := decodeEvalMap(t, accepted)
	submissionID, _ := payload["submissionId"].(string)
	if submissionID == "" {
		t.Fatalf("unexpected enqueue payload %#v", payload)
	}

	// 异步投递响应可能仍是 queued；同步 worker 处理后以查询结果为准。
	got := evalRequest(handler, http.MethodGet, "/api/v1/skills/multimodal-grade/"+submissionID, "student-demo", "", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get after enqueue status=%d body=%s", got.Code, got.Body.String())
	}
	gotPayload := decodeEvalMap(t, got)
	if gotPayload["status"] != evaluation.SubAwaitConfirm {
		t.Fatalf("expected awaiting confirm after worker, got %#v", gotPayload)
	}

	foreign := evalRequest(handler, http.MethodGet, "/api/v1/skills/multimodal-grade/"+submissionID, "student-demo-2", "", nil)
	assertEvalAPIError(t, foreign, http.StatusNotFound, "NOT_FOUND")

	confirmed := evalRequest(handler, http.MethodPost, "/api/v1/skills/multimodal-grade/confirm-transcription", "student-demo",
		fmt.Sprintf(`{"submissionId":%q,"text":""}`, submissionID), nil)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	confirmPayload := decodeEvalMap(t, confirmed)
	submission, _ := confirmPayload["submission"].(map[string]any)
	if submission["status"] != evaluation.SubNeedsTeacherReview {
		t.Fatalf("expected teacher review after confirm: %#v", submission)
	}

	override := evalRequest(handler, http.MethodPost, "/api/v1/skills/multimodal-grade/override", "teacher-demo",
		fmt.Sprintf(`{"resultId":%q,"score":8.5,"note":"复核后定分"}`, submissionID), nil)
	if override.Code != http.StatusOK {
		t.Fatalf("override status=%d body=%s", override.Code, override.Body.String())
	}
	overridePayload := decodeEvalMap(t, override)
	if overridePayload["authoritative"] != "teacher" || overridePayload["after"] != 8.5 {
		t.Fatalf("override payload %#v", overridePayload)
	}

	listed := evalRequest(handler, http.MethodGet, "/api/v1/skills/multimodal-grade?assignmentId=asg-http-1", "teacher-demo", "", nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), submissionID) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
}

func TestEvaluationHTTPClassReviewThresholdAnonymizeAndStudentForbidden(t *testing.T) {
	handler, jobs := withEvaluationHTTP(t)

	studentDenied := evalRequest(handler, http.MethodPost, "/api/v1/skills/class-review", "student-demo",
		`{"assignmentId":"asg-http-1"}`, nil)
	assertEvalAPIError(t, studentDenied, http.StatusForbidden, "FORBIDDEN")

	skipped := evalRequest(handler, http.MethodPost, "/api/v1/skills/class-review", "teacher-demo",
		`{"assignmentId":"asg-http-1"}`, nil)
	if skipped.Code != http.StatusOK {
		t.Fatalf("skip status=%d body=%s", skipped.Code, skipped.Body.String())
	}
	skipPayload := decodeEvalMap(t, skipped)
	if skipPayload["skipped"] != true || jobs.last != nil {
		t.Fatalf("expected skip without job: %#v jobs=%#v", skipPayload, jobs.last)
	}

	for i := 0; i < evaluation.MinClassReviewSamples; i++ {
		score := 7.0
		store.Default.SaveSubmission(store.Submission{
			ID: fmt.Sprintf("review-sub-%d", i), AssignmentID: "asg-http-1",
			StudentID: fmt.Sprintf("stu-%d", i+1), ClassID: "class-demo",
			Status: evaluation.SubGradedSuggested, Confirmed: true,
			ErrorCause: "行变换不完整", Suggested: &score, Score: &score, MaxScore: 10,
			SubmittedAt: time.Now().UTC(),
		})
	}

	started := evalRequest(handler, http.MethodPost, "/api/v1/skills/class-review", "teacher-demo",
		`{"assignmentId":"asg-http-1"}`, nil)
	if started.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", started.Code, started.Body.String())
	}
	startedPayload := decodeEvalMap(t, started)
	contentID, _ := startedPayload["contentId"].(string)
	if contentID == "" || startedPayload["anonymized"] != true {
		t.Fatalf("unexpected review start %#v", startedPayload)
	}
	content, ok := store.Default.GetContent(contentID)
	if !ok || content.Status != string(StatusPending) {
		t.Fatalf("review content not ready: %#v ok=%v", content, ok)
	}
	raw := fmt.Sprint(content.Payload["anonymousInputs"])
	if strings.Contains(raw, "stu-") || strings.Contains(raw, "李同学") {
		t.Fatalf("identity leaked into model boundary: %s", raw)
	}
	if review, _ := content.Payload["review"].(map[string]any); review == nil || review["teacherOnly"] != true {
		t.Fatalf("missing teacherOnly review payload: %#v", content.Payload)
	}
}

func TestEvaluationHTTPDiagnosisEvidenceReleaseAndCrossClass(t *testing.T) {
	handler, _ := withEvaluationHTTP(t)
	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		if err := store.Default.AppendEvent(store.LearningEvent{
			ID: fmt.Sprintf("diag-ev-%d", i), StudentID: "stu-1", ClassID: "class-demo",
			Kind: "assignment", Summary: fmt.Sprintf("证据不足样本 %d", i), KP: "高斯消元",
			Outcome: store.LearningOutcomeIncorrect, At: now.Add(-time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	intercepted := evalRequest(handler, http.MethodPost, "/api/v1/skills/diagnosis", "teacher-demo",
		`{"studentId":"stu-1"}`, nil)
	if intercepted.Code != http.StatusOK {
		t.Fatalf("intercept status=%d body=%s", intercepted.Code, intercepted.Body.String())
	}
	interceptPayload := decodeEvalMap(t, intercepted)
	if interceptPayload["intercepted"] != true {
		t.Fatalf("expected intercept %#v", interceptPayload)
	}

	if err := store.Default.AppendEvent(store.LearningEvent{
		ID: "diag-ev-2", StudentID: "stu-1", ClassID: "class-demo", Kind: "practice",
		Summary: "第三次有效事件", KP: "矩阵秩", Outcome: store.LearningOutcomeIncorrect, At: now,
	}); err != nil {
		t.Fatal(err)
	}
	generated := evalRequest(handler, http.MethodPost, "/api/v1/skills/diagnosis", "teacher-demo",
		`{"studentId":"stu-1"}`, nil)
	if generated.Code != http.StatusOK {
		t.Fatalf("generate status=%d body=%s", generated.Code, generated.Body.String())
	}
	generatedPayload := decodeEvalMap(t, generated)
	contentID, _ := generatedPayload["contentId"].(string)
	if contentID == "" || generatedPayload["intercepted"] == true {
		t.Fatalf("expected generated diagnosis %#v", generatedPayload)
	}

	cross := evalRequest(handler, http.MethodPost, "/api/v1/skills/diagnosis/"+contentID+"/release", "teacher-other", "", nil)
	assertEvalAPIError(t, cross, http.StatusNotFound, "NOT_FOUND")

	released := evalRequest(handler, http.MethodPost, "/api/v1/skills/diagnosis/"+contentID+"/release", "teacher-demo", "", nil)
	if released.Code != http.StatusOK {
		t.Fatalf("release status=%d body=%s", released.Code, released.Body.String())
	}
	releasePayload := decodeEvalMap(t, released)
	if releasePayload["studentVisible"] != true || releasePayload["status"] != string(StatusPublished) {
		t.Fatalf("release payload %#v", releasePayload)
	}
}

func TestEvaluationHTTPAnswerReplayPrivacyAndSimilarity(t *testing.T) {
	handler, _ := withEvaluationHTTP(t)

	keydown := evalRequest(handler, http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "student-demo",
		`{"content":"逐键不应保存","trigger":"keydown"}`, nil)
	assertEvalAPIError(t, keydown, http.StatusBadRequest, "BAD_REQUEST")

	teacherWrite := evalRequest(handler, http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "teacher-demo",
		`{"content":"教师不能写快照","trigger":"manual"}`, nil)
	assertEvalAPIError(t, teacherWrite, http.StatusForbidden, "FORBIDDEN")

	first := evalRequest(handler, http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "student-demo",
		`{"content":"高斯消元通过初等行变换求阶梯型","trigger":"manual"}`, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", first.Code, first.Body.String())
	}
	firstPayload := decodeEvalMap(t, first)
	attemptID, _ := firstPayload["attemptId"].(string)
	if attemptID == "" {
		t.Fatal("missing attemptId")
	}

	interval := evalRequest(handler, http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "student-demo",
		fmt.Sprintf(`{"attemptId":%q,"content":"高斯消元通过初等行变换求阶梯型。秩相等则有解。","trigger":"interval"}`, attemptID), nil)
	if interval.Code != http.StatusOK {
		t.Fatalf("interval status=%d body=%s", interval.Code, interval.Body.String())
	}

	foreignTimeline := evalRequest(handler, http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/timeline", "student-demo-2", "", nil)
	assertEvalAPIError(t, foreignTimeline, http.StatusNotFound, "NOT_FOUND")

	ownerTimeline := evalRequest(handler, http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/timeline", "student-demo", "", nil)
	if ownerTimeline.Code != http.StatusOK || !strings.Contains(ownerTimeline.Body.String(), "interval") {
		t.Fatalf("owner timeline status=%d body=%s", ownerTimeline.Code, ownerTimeline.Body.String())
	}

	teacherTimeline := evalRequest(handler, http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/timeline", "teacher-demo", "", nil)
	if teacherTimeline.Code != http.StatusOK {
		t.Fatalf("teacher timeline status=%d body=%s", teacherTimeline.Code, teacherTimeline.Body.String())
	}

	studentSim := evalRequest(handler, http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/similarity", "student-demo", "", nil)
	assertEvalAPIError(t, studentSim, http.StatusForbidden, "FORBIDDEN")

	teacherSim := evalRequest(handler, http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/similarity", "teacher-demo", "", nil)
	if teacherSim.Code != http.StatusOK {
		t.Fatalf("similarity status=%d body=%s", teacherSim.Code, teacherSim.Body.String())
	}
	simPayload := decodeEvalMap(t, teacherSim)
	if _, hasVerdict := simPayload["verdict"]; !hasVerdict || simPayload["verdict"] != nil {
		t.Fatalf("verdict must be present and null: %#v", simPayload)
	}
}
