package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
)

func TestHTTPExerciseConfirmPublishLifecycle(t *testing.T) {
	// Scenario (normal/idempotent): legacy skill routes produce real content,
	// then the shared workflow confirms, publishes, and safely serves students.
	service, repository := newContentTestService(nil)
	handler := contentTestHTTPHandler(service)

	first := performJSON(t, handler, http.MethodPost, "/api/v1/skills/proposition", "teacher-a",
		`{"materialId":"material-a","count":2}`, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("generate status=%d body=%s", first.Code, first.Body.String())
	}
	var generated GenerationResult
	decodeResponse(t, first, &generated)
	if generated.ContentID == "" || generated.Status != StatusPendingConfirmation {
		t.Fatalf("unexpected generation response: %#v", generated)
	}

	replay := performJSON(t, handler, http.MethodPost, "/api/v1/skills/proposition", "teacher-a",
		`{"materialId":"material-a","count":2}`, nil)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	var replayed GenerationResult
	decodeResponse(t, replay, &replayed)
	if !replayed.Replayed || replayed.ContentID != generated.ContentID {
		t.Fatalf("HTTP replay created duplicate content: %#v", replayed)
	}

	confirm := performJSON(t, handler, http.MethodPost,
		"/api/v1/contents/"+generated.ContentID+"/confirm", "teacher-a", `{}`, nil)
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirm.Code, confirm.Body.String())
	}
	var confirmed ContentRecord
	decodeResponse(t, confirm, &confirmed)
	if confirmed.Status != StatusConfirmed {
		t.Fatalf("unexpected confirm response: %#v", confirmed)
	}

	publish := performJSON(t, handler, http.MethodPost,
		"/api/v1/contents/"+generated.ContentID+"/publish", "teacher-a", `{}`, nil)
	if publish.Code != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", publish.Code, publish.Body.String())
	}
	var published ContentRecord
	decodeResponse(t, publish, &published)
	if published.Status != StatusPublished || published.FrozenDigest == "" {
		t.Fatalf("unexpected publish response: %#v", published)
	}

	studentGet := performJSON(t, handler, http.MethodGet,
		"/api/v1/contents/"+generated.ContentID, "student-a", "", nil)
	if studentGet.Code != http.StatusOK {
		t.Fatalf("student get status=%d body=%s", studentGet.Code, studentGet.Body.String())
	}
	var studentView ContentRecord
	decodeResponse(t, studentGet, &studentView)
	for _, question := range studentView.Payload["questions"].([]any) {
		item := question.(map[string]any)
		if _, leaked := item["answer"]; leaked {
			t.Fatalf("answer leaked through HTTP: %#v", item)
		}
	}

	audits, err := repository.ListAudit(context.Background(), teacherA().ClassID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) < 4 {
		t.Fatalf("expected generation, replay, confirm, and publish audits: %#v", audits)
	}
	for _, event := range audits {
		if event.RequestID == "" {
			t.Fatalf("audit missing request ID: %#v", event)
		}
	}
}

func TestHTTPDedicatedProductionAndQualityReviewRoutes(t *testing.T) {
	// Scenario (normal): dedicated course-card/lesson-plan routes and the
	// quality-review endpoint are ready for parent router integration.
	service, _ := newContentTestService(nil)
	handler := contentTestHTTPHandler(service)

	cardsResponse := performJSON(t, handler, http.MethodPost,
		"/api/v1/content-production/course-cards", "teacher-a",
		`{"topic":"线性方程组","count":3}`, nil)
	if cardsResponse.Code != http.StatusCreated {
		t.Fatalf("cards status=%d body=%s", cardsResponse.Code, cardsResponse.Body.String())
	}
	var cards GenerationResult
	decodeResponse(t, cardsResponse, &cards)
	if string(cards.Kind) != "course_cards" {
		t.Fatalf("wrong cards kind: %#v", cards)
	}

	lessonResponse := performJSON(t, handler, http.MethodPost,
		"/api/v1/content-production/lesson-plans", "teacher-a",
		`{"topic":"线性方程组","durationMinutes":45,"objectives":["掌握高斯消元"]}`, nil)
	if lessonResponse.Code != http.StatusCreated {
		t.Fatalf("lesson status=%d body=%s", lessonResponse.Code, lessonResponse.Body.String())
	}
	var lesson GenerationResult
	decodeResponse(t, lessonResponse, &lesson)

	reviewResponse := performJSON(t, handler, http.MethodPost,
		"/api/v1/content-production/reviews", "teacher-a",
		`{"contentId":"`+lesson.ContentID+`"}`, nil)
	if reviewResponse.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", reviewResponse.Code, reviewResponse.Body.String())
	}
	var review ReviewRun
	decodeResponse(t, reviewResponse, &review)
	if !review.Report.Passed || review.ContentID != lesson.ContentID {
		t.Fatalf("unexpected review response: %#v", review)
	}
}

func TestHTTPValidationAuthorizationAndConflictErrors(t *testing.T) {
	// Scenario (failure): transport errors use stable status/code pairs and do
	// not leak cross-class resource existence.
	service, _ := newContentTestService(nil)
	handler := contentTestHTTPHandler(service)
	cases := []struct {
		name       string
		method     string
		path       string
		token      string
		body       string
		headers    map[string]string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing authentication",
			method:     http.MethodPost,
			path:       "/api/v1/skills/preview-sheet",
			body:       `{}`,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "student teacher-only skill",
			method:     http.MethodPost,
			path:       "/api/v1/skills/preview-sheet",
			token:      "student-a",
			body:       `{}`,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "malformed JSON",
			method:     http.MethodPost,
			path:       "/api/v1/skills/preview-sheet",
			token:      "teacher-a",
			body:       `{"count":`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
		{
			name:       "unknown JSON field",
			method:     http.MethodPost,
			path:       "/api/v1/skills/preview-sheet",
			token:      "teacher-a",
			body:       `{"unknown":true}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
		{
			name:       "invalid count",
			method:     http.MethodPost,
			path:       "/api/v1/skills/preview-sheet",
			token:      "teacher-a",
			body:       `{"topic":"线性方程组","count":2}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ARGUMENT",
		},
		{
			name:       "cross class source",
			method:     http.MethodPost,
			path:       "/api/v1/skills/preview-sheet",
			token:      "teacher-b",
			body:       `{"topic":"线性方程组","count":3,"materialId":"material-a"}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := performJSON(
				t,
				handler,
				testCase.method,
				testCase.path,
				testCase.token,
				testCase.body,
				testCase.headers,
			)
			assertHTTPError(t, response, testCase.wantStatus, testCase.wantCode)
		})
	}

	headers := map[string]string{"Idempotency-Key": "explicit-conflict-key"}
	first := performJSON(t, handler, http.MethodPost, "/api/v1/skills/proposition", "teacher-a",
		`{"topic":"线性方程组","count":1}`, headers)
	if first.Code != http.StatusCreated {
		t.Fatalf("first idempotent request failed: %d %s", first.Code, first.Body.String())
	}
	conflict := performJSON(t, handler, http.MethodPost, "/api/v1/skills/proposition", "teacher-a",
		`{"topic":"矩阵秩","count":1}`, headers)
	assertHTTPError(t, conflict, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
}

func TestHTTPWorkflowFailureAndCrossClassErrors(t *testing.T) {
	// Scenario (failure): publication state, failed quality edits, and
	// cross-class reads are enforced by the application service.
	service, _ := newContentTestService(nil)
	handler := contentTestHTTPHandler(service)
	generate := performJSON(t, handler, http.MethodPost,
		"/api/v1/content-production/course-cards", "teacher-a",
		`{"topic":"线性方程组","count":3}`, nil)
	var generated GenerationResult
	decodeResponse(t, generate, &generated)

	publishEarly := performJSON(t, handler, http.MethodPost,
		"/api/v1/contents/"+generated.ContentID+"/publish", "teacher-a", `{}`, nil)
	assertHTTPError(t, publishEarly, http.StatusConflict, "INVALID_STATE")

	badPayload := cloneMap(generated.Content.Payload)
	cards := badPayload["cards"].([]any)
	cards[0].(map[string]any)["citation"].(map[string]any)["sourceId"] = "fabricated-source"
	confirmBody, err := json.Marshal(map[string]any{"payload": badPayload})
	if err != nil {
		t.Fatal(err)
	}
	failedConfirm := performJSON(t, handler, http.MethodPost,
		"/api/v1/contents/"+generated.ContentID+"/confirm", "teacher-a",
		string(confirmBody), nil)
	assertHTTPError(t, failedConfirm, http.StatusUnprocessableEntity, "QUALITY_REVIEW_FAILED")

	teacherGet := performJSON(t, handler, http.MethodGet,
		"/api/v1/contents/"+generated.ContentID, "teacher-a", "", nil)
	if teacherGet.Code != http.StatusOK {
		t.Fatalf("teacher get status=%d body=%s", teacherGet.Code, teacherGet.Body.String())
	}
	var needsRevision ContentRecord
	decodeResponse(t, teacherGet, &needsRevision)
	if needsRevision.Status != StatusNeedsRevision {
		t.Fatalf("failed review state not retained: %#v", needsRevision)
	}

	crossClass := performJSON(t, handler, http.MethodGet,
		"/api/v1/contents/"+generated.ContentID, "teacher-b", "", nil)
	assertHTTPError(t, crossClass, http.StatusNotFound, "NOT_FOUND")
}

func TestHTTPDependencyFailures(t *testing.T) {
	// Scenario (failure): generation and review dependency failures are
	// explicit 502 responses, not successful placeholders.
	producerService, _ := newContentTestService(&ServiceOptions{Producer: failingContentProducer{}})
	producerHandler := contentTestHTTPHandler(producerService)
	producerFailure := performJSON(t, producerHandler, http.MethodPost,
		"/api/v1/content-production/course-cards", "teacher-a",
		`{"topic":"线性方程组","count":3}`, nil)
	assertHTTPError(t, producerFailure, http.StatusBadGateway, "GENERATION_FAILED")

	reviewerService, _ := newContentTestService(&ServiceOptions{Reviewer: failingQualityReviewer{}})
	reviewerHandler := contentTestHTTPHandler(reviewerService)
	reviewerFailure := performJSON(t, reviewerHandler, http.MethodPost,
		"/api/v1/content-production/course-cards", "teacher-a",
		`{"topic":"线性方程组","count":3}`, nil)
	assertHTTPError(t, reviewerFailure, http.StatusBadGateway, "REVIEW_FAILED")
}

func contentTestHTTPHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	NewHandler(service, testAuthenticate).RegisterRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "http-test-request"
		}
		mux.ServeHTTP(w, r.WithContext(httpapi.WithRequestID(r.Context(), requestID)))
	})
}

func testAuthenticate(_ context.Context, authorization string) (auth.Principal, error) {
	switch authorization {
	case "Bearer teacher-a":
		return teacherA(), nil
	case "Bearer teacher-b":
		return teacherB(), nil
	case "Bearer student-a":
		return studentA(), nil
	default:
		return auth.Principal{}, auth.ErrUnauthorized
	}
}

func performJSON(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	token string,
	body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Request-ID", "request-"+token)
	request = request.WithContext(httpapi.WithRequestID(request.Context(), "request-"+token))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode response: %v body=%s", err, response.Body.String())
	}
}

func assertHTTPError(
	t *testing.T,
	response *httptest.ResponseRecorder,
	status int,
	code string,
) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	var apiError httpapi.ErrorResponse
	decodeResponse(t, response, &apiError)
	if apiError.Code != code || apiError.RequestID == "" {
		t.Fatalf("unexpected error envelope: %#v", apiError)
	}
}

func TestHandlerAuthenticateFailureDoesNotDecodeBody(t *testing.T) {
	// Scenario (security): authentication happens before parsing an untrusted
	// request body.
	service, _ := newContentTestService(nil)
	called := false
	authenticate := func(context.Context, string) (auth.Principal, error) {
		called = true
		return auth.Principal{}, errors.New("invalid token")
	}
	mux := http.NewServeMux()
	NewHandler(service, authenticate).RegisterDedicatedRoutes(mux)
	response := performJSON(t, mux, http.MethodPost,
		"/api/v1/content-production/course-cards", "", `not-json`, nil)
	if !called {
		t.Fatal("authenticator was not called")
	}
	assertHTTPError(t, response, http.StatusUnauthorized, "UNAUTHORIZED")
}
