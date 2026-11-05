package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/tutoring"
)

type handlerModel struct{}

func (handlerModel) GenerateReviewPath(_ context.Context, input tutoring.ReviewModelInput) ([]tutoring.ReviewStepDraft, error) {
	return []tutoring.ReviewStepDraft{
		{Type: "read", Title: "阅读", KnowledgePoint: input.Target},
		{Type: "example", Title: "例题", KnowledgePoint: input.Target},
		{Type: "practice", Title: "练习", KnowledgePoint: input.Target},
	}, nil
}

func (handlerModel) NextTutorQuestion(context.Context, tutoring.TutorModelInput) (string, error) {
	return "右端常数项应当怎样处理？", nil
}

func (handlerModel) SummarizeClusters(_ context.Context, groups []tutoring.ClusterDraft) ([]tutoring.ClusterSummaryDraft, error) {
	result := make([]tutoring.ClusterSummaryDraft, len(groups))
	for index, group := range groups {
		result[index] = tutoring.ClusterSummaryDraft{
			GroupID: group.ID, MemberIDs: group.MemberIDs, Label: "类型", FollowUp: "为什么？",
		}
	}
	return result, nil
}

func handlerTestMemory() *store.Memory {
	now := time.Now().UTC()
	return &store.Memory{
		Materials:   map[string]store.Material{},
		Assignments: map[string]store.Assignment{},
		Contents:    map[string]store.ContentRecord{},
		Tasks:       map[string]store.TaskRecord{},
		Submissions: map[string]store.Submission{},
		Practice:    map[string]store.PracticeSession{},
		Tutor:       map[string]store.TutorSession{},
		Classroom:   map[string]store.ClassroomSession{},
		ClassSkills: map[string]map[string]bool{
			"class-demo": {
				"review_path": true, "socratic_tutor": true,
				"adaptive_practice": true, "classroom_cluster": true,
			},
		},
		Events: []store.LearningEvent{{
			ID: "e1", StudentID: "stu-1", ClassID: "class-demo", KP: "高斯消元",
			Outcome: store.LearningOutcomeIncorrect, At: now,
		}},
		Similarity:    map[string][]store.SimilarityHit{},
		ProcessedJobs: map[string]bool{},
	}
}

func newTutoringHandler(tools *tutoring.TeachingTools) http.Handler {
	memory := handlerTestMemory()
	model := handlerModel{}
	service := tutoring.NewService(tutoring.ServiceDependencies{
		Repository:   tutoring.NewRepository(memory),
		ReviewModel:  model,
		TutorModel:   model,
		ClusterModel: model,
	})
	mux := http.NewServeMux()
	RegisterTutoringRoutes(mux, TutoringDependencies{Service: service, Tools: tools})
	return mux
}

func tutoringRequest(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		switch token {
		case "teacher-demo":
			token, _, _ = auth.LoginWithPassword("teacher", "demo123")
		case "student-demo":
			token, _, _ = auth.LoginWithPassword("student", "demo123")
		case "student-demo-2":
			token, _, _ = auth.LoginWithPassword("student2", "demo123")
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestTutoringRoutesRequireAuthentication(t *testing.T) {
	handler := newTutoringHandler(nil)
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/learning/state", ""},
		{http.MethodPost, "/api/v1/skills/review-path", `{}`},
		{http.MethodPost, "/api/v1/skills/socratic-tutor/start", `{"questionId":"q-wrong-1"}`},
		{http.MethodPost, "/api/v1/skills/adaptive-practice/start", `{"knowledgePoint":"高斯消元"}`},
		{http.MethodPost, "/api/v1/classroom/interactions", `{"prompt":"问题"}`},
		{http.MethodGet, "/api/v1/mcp/tools", ""},
	}
	for _, test := range tests {
		response := tutoringRequest(handler, test.method, test.path, "", test.body)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status=%d", test.method, test.path, response.Code)
		}
	}
}

func TestReviewPathHTTPRoleValidationAndStrictJSON(t *testing.T) {
	handler := newTutoringHandler(nil)
	state := tutoringRequest(handler, http.MethodGet, "/api/v1/learning/state", "student-demo", "")
	if state.Code != http.StatusOK {
		t.Fatalf("learning state status=%d body=%s", state.Code, state.Body)
	}
	generated := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/review-path", "student-demo", `{}`)
	if generated.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", generated.Code, generated.Body)
	}
	var review tutoring.ReviewPathResult
	if err := json.NewDecoder(generated.Body).Decode(&review); err != nil || review.Plan == nil {
		t.Fatalf("review=%#v err=%v", review, err)
	}
	teacher := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/review-path", "teacher-demo", `{}`)
	assertAPIError(t, teacher, http.StatusForbidden, "FORBIDDEN")
	unknownField := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/review-path", "student-demo", `{"studentId":"stu-2"}`)
	assertAPIError(t, unknownField, http.StatusBadRequest, "INVALID_JSON")
}

func TestSocraticHTTPReleasesOnlyAfterBackendRuleAndProtectsOwner(t *testing.T) {
	handler := newTutoringHandler(nil)
	start := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/socratic-tutor/start", "student-demo", `{"questionId":"q-wrong-1"}`)
	if start.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body)
	}
	var session tutoring.TutorSession
	if err := json.NewDecoder(start.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		response := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/socratic-tutor", "student-demo",
			`{"sessionId":"`+session.ID+`","reply":"右端常数项要同步交换"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("turn %d status=%d body=%s", index, response.Code, response.Body)
		}
		if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
			t.Fatal(err)
		}
	}
	if !session.Released || session.FullSolution == "" {
		t.Fatalf("session not released: %#v", session)
	}
	other := tutoringRequest(handler, http.MethodGet, "/api/v1/tutoring/sessions/"+session.ID, "student-demo-2", "")
	assertAPIError(t, other, http.StatusNotFound, "NOT_FOUND")
}

func TestAdaptiveHTTPAcceptsAnswerTextOnlyAndHidesAnswers(t *testing.T) {
	handler := newTutoringHandler(nil)
	start := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/adaptive-practice/start", "student-demo",
		`{"knowledgePoint":"高斯消元"}`)
	if start.Code != http.StatusCreated || bytes.Contains(start.Body.Bytes(), []byte(`"answer"`)) {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body)
	}
	var session tutoring.PracticeSession
	if err := json.NewDecoder(start.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	clientOutcome := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/adaptive-practice/submit", "student-demo",
		`{"sessionId":"`+session.ID+`","submissionId":"s1","outcome":"correct"}`)
	assertAPIError(t, clientOutcome, http.StatusBadRequest, "INVALID_JSON")
	submit := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/adaptive-practice/submit", "student-demo",
		`{"sessionId":"`+session.ID+`","submissionId":"s1","answerText":"一起交换"}`)
	if submit.Code != http.StatusOK {
		t.Fatalf("submit status=%d body=%s", submit.Code, submit.Body)
	}
	var result tutoring.PracticeResult
	if err := json.NewDecoder(submit.Body).Decode(&result); err != nil || result.Outcome != "correct" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestClassroomHTTPEnforcesTeacherStudentLifecycle(t *testing.T) {
	handler := newTutoringHandler(nil)
	studentCreate := tutoringRequest(handler, http.MethodPost, "/api/v1/classroom/interactions", "student-demo", `{"prompt":"问题"}`)
	assertAPIError(t, studentCreate, http.StatusForbidden, "FORBIDDEN")
	create := tutoringRequest(handler, http.MethodPost, "/api/v1/classroom/interactions", "teacher-demo", `{"prompt":"问题"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body)
	}
	var interaction tutoring.ClassroomInteraction
	_ = json.NewDecoder(create.Body).Decode(&interaction)
	answer := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/classroom-cluster/answer", "student-demo",
		`{"sessionId":"`+interaction.ID+`","text":"回答"}`)
	if answer.Code != http.StatusOK {
		t.Fatalf("answer status=%d body=%s", answer.Code, answer.Body)
	}
	closeResponse := tutoringRequest(handler, http.MethodPost, "/api/v1/classroom/interactions/"+interaction.ID+"/close", "teacher-demo", `{}`)
	if closeResponse.Code != http.StatusOK {
		t.Fatalf("close status=%d body=%s", closeResponse.Code, closeResponse.Body)
	}
	cluster := tutoringRequest(handler, http.MethodPost, "/api/v1/skills/classroom-cluster", "teacher-demo",
		`{"sessionId":"`+interaction.ID+`"}`)
	if cluster.Code != http.StatusOK || !bytes.Contains(cluster.Body.Bytes(), []byte(`"skipped":true`)) {
		t.Fatalf("cluster status=%d body=%s", cluster.Code, cluster.Body)
	}
}

type handlerMCPHost struct {
	calls int
}

func (h *handlerMCPHost) Discover(context.Context) ([]tutoring.MCPToolDescriptor, error) {
	return []tutoring.MCPToolDescriptor{
		{Name: "formula_verify", Description: "verify", InputSchema: map[string]any{}},
		{Name: "assignment_stats", Description: "stats", InputSchema: map[string]any{}},
	}, nil
}

func (h *handlerMCPHost) Bind(context.Context, ...string) (map[string]tutoring.MCPToolDescriptor, error) {
	return map[string]tutoring.MCPToolDescriptor{}, nil
}

func (h *handlerMCPHost) Call(_ context.Context, name string, arguments map[string]any) (map[string]any, error) {
	h.calls++
	return map[string]any{"tool": name, "expression": arguments["expression"]}, nil
}

func TestMCPHTTPFiltersToolsAndChecksRoleBeforeCall(t *testing.T) {
	host := &handlerMCPHost{}
	tools := tutoring.NewTeachingTools(host, nil, nil)
	handler := newTutoringHandler(tools)
	list := tutoringRequest(handler, http.MethodGet, "/api/v1/mcp/tools", "student-demo", "")
	if list.Code != http.StatusOK || bytes.Contains(list.Body.Bytes(), []byte("assignment_stats")) {
		t.Fatalf("student tool list status=%d body=%s", list.Code, list.Body)
	}
	denied := tutoringRequest(handler, http.MethodPost, "/api/v1/mcp/tools/assignment_stats/call", "student-demo",
		`{"arguments":{"assignment_id":"a1"}}`)
	assertAPIError(t, denied, http.StatusForbidden, "FORBIDDEN")
	if host.calls != 0 {
		t.Fatal("forbidden MCP call reached host")
	}
	success := tutoringRequest(handler, http.MethodPost, "/api/v1/mcp/tools/formula_verify/call", "student-demo",
		`{"arguments":{"expression":"1+1","expected":"2"}}`)
	if success.Code != http.StatusOK || host.calls != 1 {
		t.Fatalf("formula call status=%d calls=%d body=%s", success.Code, host.calls, success.Body)
	}
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body)
	}
	var apiError httpapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&apiError); err != nil {
		t.Fatal(err)
	}
	if apiError.Code != code {
		t.Fatalf("error=%#v want code=%s", apiError, code)
	}
}
