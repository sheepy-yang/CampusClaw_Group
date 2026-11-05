package teaching

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

type captureEvaluationJob struct{ calls int }

func (j *captureEvaluationJob) Enqueue(context.Context, string, string, map[string]any) error {
	j.calls++
	return nil
}

func TestSharedContentLifecycleDeniesCrossClassAndNonPublishableRecords(t *testing.T) {
	original := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = original })
	now := time.Now().UTC()
	store.Default.SaveContent(store.ContentRecord{
		ID: "review-other", Skill: string(SkillClassReview), ClassID: "class-other",
		Status: string(StatusPending), Payload: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	})
	store.Default.SaveContent(store.ContentRecord{
		ID: "private-plan", Skill: string(SkillReviewPath), ClassID: "class-demo",
		Status: "active", Payload: map[string]any{"studentId": "stu-1"}, CreatedAt: now, UpdatedAt: now,
	})
	store.Default.SaveContent(store.ContentRecord{
		ID: "review-ready", Skill: string(SkillClassReview), ClassID: "class-demo",
		Status: "pending", Payload: map[string]any{"review": map[string]any{"outline": []string{"共性错因"}}}, CreatedAt: now, UpdatedAt: now,
	})
	teacher := auth.Principal{UserID: "tea-1", ClassID: "class-demo", Role: auth.RoleTeacher}

	for _, id := range []string{"review-other", "private-plan"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/contents/"+id+"/confirm", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.SetPathValue("id", id)
		response := httptest.NewRecorder()
		handleConfirm(response, request, teacher)
		if id == "review-other" && response.Code != http.StatusNotFound {
			t.Fatalf("cross-class status=%d body=%s", response.Code, response.Body)
		}
		if id == "private-plan" && response.Code != http.StatusConflict {
			t.Fatalf("private lifecycle status=%d body=%s", response.Code, response.Body)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/contents/review-ready/confirm", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.SetPathValue("id", "review-ready")
	response := httptest.NewRecorder()
	handleConfirm(response, request, teacher)
	if response.Code != http.StatusOK {
		t.Fatalf("valid confirmation status=%d body=%s", response.Code, response.Body)
	}
}

func TestStudentContentViewEnforcesDiagnosisOwnerAndRemovesReviewInputs(t *testing.T) {
	original := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = original })
	now := time.Now().UTC()
	store.Default.SaveContent(store.ContentRecord{
		ID: "diagnosis-1", Skill: string(SkillDiagnosis), ClassID: "class-demo",
		Status: string(StatusPublished), Payload: map[string]any{
			"studentId": "stu-1", "diagnosis": map[string]any{"studentVisible": true},
		}, CreatedAt: now, UpdatedAt: now,
	})
	store.Default.SaveContent(store.ContentRecord{
		ID: "review-1", Skill: string(SkillClassReview), ClassID: "class-demo",
		Status: string(StatusPublished), Payload: map[string]any{
			"anonymousInputs": []string{"private evidence"}, "teacherOnly": true,
			"review": map[string]any{
				"outline": []string{"共性错因"}, "anonymousInputs": []string{"private evidence"}, "teacherOnly": true,
			},
		}, CreatedAt: now, UpdatedAt: now,
	})
	otherStudent := auth.Principal{UserID: "stu-2", ClassID: "class-demo", Role: auth.RoleStudent}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/contents/diagnosis-1", nil)
	request.SetPathValue("id", "diagnosis-1")
	response := httptest.NewRecorder()
	handleGetContent(response, request, otherStudent)
	if response.Code != http.StatusNotFound {
		t.Fatalf("other student diagnosis status=%d", response.Code)
	}

	owner := auth.Principal{UserID: "stu-1", ClassID: "class-demo", Role: auth.RoleStudent}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/contents/review-1", nil)
	request.SetPathValue("id", "review-1")
	response = httptest.NewRecorder()
	handleGetContent(response, request, owner)
	if response.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", response.Code, response.Body)
	}
	var content store.ContentRecord
	if err := json.NewDecoder(response.Body).Decode(&content); err != nil {
		t.Fatal(err)
	}
	if _, leaked := content.Payload["anonymousInputs"]; leaked {
		t.Fatalf("raw anonymous evidence leaked: %#v", content.Payload)
	}
	review, _ := content.Payload["review"].(map[string]any)
	if _, leaked := review["anonymousInputs"]; leaked {
		t.Fatalf("nested evidence leaked: %#v", review)
	}
}

func TestStudentContentListOnlyReturnsPublishedPreviewSheets(t *testing.T) {
	original := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = original })
	now := time.Now().UTC()
	for _, content := range []store.ContentRecord{
		{ID: "preview-published", Skill: string(SkillPreviewSheet), ClassID: "class-demo", Status: "published", Payload: map[string]any{"title": "已发布"}, CreatedAt: now},
		{ID: "preview-draft", Skill: string(SkillPreviewSheet), ClassID: "class-demo", Status: "pending_confirmation", Payload: map[string]any{"title": "草稿"}, CreatedAt: now},
	} {
		store.Default.SaveContent(content)
	}
	student := auth.Principal{UserID: "stu-1", ClassID: "class-demo", Role: auth.RoleStudent}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/classes/class-demo/contents?skill=preview_sheet", nil)
	request.SetPathValue("classID", "class-demo")
	response := httptest.NewRecorder()
	handleListContents(response, request, student)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "preview-published") || strings.Contains(response.Body.String(), "preview-draft") {
		t.Fatalf("preview list status=%d body=%s", response.Code, response.Body)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/classes/class-demo/contents?skill=diagnosis", nil)
	request.SetPathValue("classID", "class-demo")
	response = httptest.NewRecorder()
	handleListContents(response, request, student)
	if response.Code != http.StatusForbidden {
		t.Fatalf("private content list status=%d body=%s", response.Code, response.Body)
	}
}

func TestClassReviewHandlerUsesStoredSubmissionsAsEvidence(t *testing.T) {
	originalStore, originalDeps := store.Default, deps
	store.Default = store.New()
	jobs := &captureEvaluationJob{}
	deps = &apiDeps{
		Eval:  evaluation.New(jobs),
		Audit: security.NewAuditLogger(),
	}
	t.Cleanup(func() {
		store.Default = originalStore
		deps = originalDeps
	})
	now := time.Now().UTC()
	store.Default.SaveAssignment(store.Assignment{
		ID: "assignment-1", ClassID: "class-demo", Title: "线代作业", Status: "published",
	})
	for index := 0; index < evaluation.MinClassReviewSamples; index++ {
		store.Default.SaveSubmission(store.Submission{
			ID: "submission-" + string(rune('1'+index)), AssignmentID: "assignment-1",
			StudentID: "student-" + string(rune('1'+index)), ClassID: "class-demo",
			Status: evaluation.SubGradedSuggested, Confirmed: true,
			ErrorCause: "遗漏增广矩阵同步变换", SubmittedAt: now,
		})
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/skills/class-review", strings.NewReader(`{"assignmentId":"assignment-1"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handleClassReview(response, request, auth.Principal{
		UserID: "tea-1", ClassID: "class-demo", Role: auth.RoleTeacher,
	})
	if response.Code != http.StatusOK || jobs.calls != 1 {
		t.Fatalf("status=%d jobs=%d body=%s", response.Code, jobs.calls, response.Body)
	}
}
