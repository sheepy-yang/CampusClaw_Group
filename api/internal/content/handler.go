package content

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
)

const maxRequestBodyBytes = 1 << 20

type AuthenticateFunc func(ctx context.Context, authorization string) (auth.Principal, error)

type Handler struct {
	service      *Service
	authenticate AuthenticateFunc
}

func NewHandler(service *Service, authenticate AuthenticateFunc) *Handler {
	if service == nil {
		service = NewDefaultService()
	}
	if authenticate == nil {
		authenticate = auth.Authenticate
	}
	return &Handler{service: service, authenticate: authenticate}
}

// RegisterSkillRoutes mounts the content-generation and quality-review skills.
func (h *Handler) RegisterSkillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/skills/preview-sheet", h.GenerateCourseCards)
	mux.HandleFunc("POST /api/v1/skills/proposition", h.GenerateExercises)
	mux.HandleFunc("POST /api/v1/skills/lesson-plan", h.GenerateLessonPlan)
	mux.HandleFunc("POST /api/v1/skills/content-quality-review", h.ReviewContent)
}

// RegisterWorkflowRoutes supplies the unified lifecycle handlers. Integrators
// should replace, rather than double-register, the legacy shared handlers.
func (h *Handler) RegisterWorkflowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/contents/{id}/confirm", h.Confirm)
	mux.HandleFunc("POST /api/v1/contents/{id}/reject", h.Reject)
	mux.HandleFunc("POST /api/v1/contents/{id}/publish", h.Publish)
	mux.HandleFunc("GET /api/v1/contents/{id}", h.Get)
}

// RegisterDedicatedRoutes exposes non-legacy aliases useful to API-only
// clients without changing shared router files.
func (h *Handler) RegisterDedicatedRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/content-production/course-cards", h.GenerateCourseCards)
	mux.HandleFunc("POST /api/v1/content-production/exercises", h.GenerateExercises)
	mux.HandleFunc("POST /api/v1/content-production/lesson-plans", h.GenerateLessonPlan)
	mux.HandleFunc("POST /api/v1/content-production/reviews", h.ReviewContent)
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	h.RegisterSkillRoutes(mux)
	h.RegisterWorkflowRoutes(mux)
	h.RegisterDedicatedRoutes(mux)
}

// HasContent lets the shared lifecycle router dispatch without exposing
// whether the record belongs to the caller; the selected handler still applies
// normal class and role checks.
func (h *Handler) HasContent(ctx context.Context, id string) bool {
	if h == nil || h.service == nil || strings.TrimSpace(id) == "" {
		return false
	}
	_, ok, err := h.service.repository.Get(ctx, strings.TrimSpace(id))
	return err == nil && ok
}

type courseCardHTTPBody struct {
	Topic          string        `json:"topic"`
	Count          int           `json:"count"`
	CardCount      int           `json:"cardCount"`
	MaterialID     string        `json:"materialId"`
	SourceRefs     []EvidenceRef `json:"sourceRefs"`
	IdempotencyKey string        `json:"idempotencyKey"`
}

func (h *Handler) GenerateCourseCards(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var body courseCardHTTPBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if body.Count == 0 {
		body.Count = body.CardCount
	}
	if body.Count == 0 {
		body.Count = 3
	}
	if strings.TrimSpace(body.Topic) == "" {
		body.Topic = "课程预习"
	}
	body.SourceRefs = withMaterialRef(body.SourceRefs, body.MaterialID)
	key := requestIdempotencyKey(r, body.IdempotencyKey, body)
	result, err := h.service.GenerateCourseCards(r.Context(), principal, CourseCardRequest{
		Topic:      body.Topic,
		Count:      body.Count,
		SourceRefs: body.SourceRefs,
		Meta: RequestMeta{
			IdempotencyKey: key,
			RequestID:      httpapi.RequestID(r.Context()),
		},
	})
	if h.writeServiceError(w, r, err) {
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	httpapi.WriteJSON(w, status, result)
}

type exerciseHTTPBody struct {
	Topic          string        `json:"topic"`
	Count          int           `json:"count"`
	QuestionCount  int           `json:"questionCount"`
	ExerciseTypes  []string      `json:"exerciseTypes"`
	QuestionTypes  []string      `json:"questionTypes"`
	Difficulty     string        `json:"difficulty"`
	MaterialID     string        `json:"materialId"`
	SourceRefs     []EvidenceRef `json:"sourceRefs"`
	IdempotencyKey string        `json:"idempotencyKey"`
}

func (h *Handler) GenerateExercises(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var body exerciseHTTPBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if body.Count == 0 {
		body.Count = body.QuestionCount
	}
	if body.Count == 0 {
		body.Count = 1
	}
	if len(body.ExerciseTypes) == 0 {
		body.ExerciseTypes = body.QuestionTypes
	}
	if len(body.ExerciseTypes) == 0 {
		body.ExerciseTypes = []string{"short_answer"}
	}
	if strings.TrimSpace(body.Difficulty) == "" {
		body.Difficulty = "medium"
	}
	if strings.TrimSpace(body.Topic) == "" {
		body.Topic = "课程练习"
	}
	body.SourceRefs = withMaterialRef(body.SourceRefs, body.MaterialID)
	key := requestIdempotencyKey(r, body.IdempotencyKey, body)
	result, err := h.service.GenerateExercises(r.Context(), principal, ExerciseRequest{
		Topic:         body.Topic,
		Count:         body.Count,
		ExerciseTypes: body.ExerciseTypes,
		Difficulty:    body.Difficulty,
		SourceRefs:    body.SourceRefs,
		Meta: RequestMeta{
			IdempotencyKey: key,
			RequestID:      httpapi.RequestID(r.Context()),
		},
	})
	if h.writeServiceError(w, r, err) {
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	httpapi.WriteJSON(w, status, result)
}

type lessonPlanHTTPBody struct {
	Topic           string        `json:"topic"`
	DurationMinutes int           `json:"durationMinutes"`
	Objectives      []string      `json:"objectives"`
	MaterialID      string        `json:"materialId"`
	SourceRefs      []EvidenceRef `json:"sourceRefs"`
	IdempotencyKey  string        `json:"idempotencyKey"`
}

func (h *Handler) GenerateLessonPlan(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var body lessonPlanHTTPBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	if body.DurationMinutes == 0 {
		body.DurationMinutes = 45
	}
	if strings.TrimSpace(body.Topic) == "" && len(body.Objectives) > 0 {
		body.Topic = body.Objectives[0]
	}
	if strings.TrimSpace(body.Topic) == "" {
		body.Topic = "课程教学"
	}
	if len(body.Objectives) == 0 {
		body.Objectives = []string{"理解" + body.Topic + "的核心概念"}
	}
	body.SourceRefs = withMaterialRef(body.SourceRefs, body.MaterialID)
	key := requestIdempotencyKey(r, body.IdempotencyKey, body)
	result, err := h.service.GenerateLessonPlan(r.Context(), principal, LessonPlanRequest{
		Topic:           body.Topic,
		DurationMinutes: body.DurationMinutes,
		Objectives:      body.Objectives,
		SourceRefs:      body.SourceRefs,
		Meta: RequestMeta{
			IdempotencyKey: key,
			RequestID:      httpapi.RequestID(r.Context()),
		},
	})
	if h.writeServiceError(w, r, err) {
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	httpapi.WriteJSON(w, status, result)
}

type qualityReviewHTTPBody struct {
	ContentID  string                `json:"contentId"`
	Kind       agent.ProductionSkill `json:"kind"`
	Payload    map[string]any        `json:"payload"`
	MaterialID string                `json:"materialId"`
	SourceRefs []EvidenceRef         `json:"sourceRefs"`
}

func (h *Handler) ReviewContent(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var body qualityReviewHTTPBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	body.SourceRefs = withMaterialRef(body.SourceRefs, body.MaterialID)
	result, err := h.service.ReviewContent(r.Context(), principal, QualityReviewRequest{
		ContentID:  body.ContentID,
		Kind:       body.Kind,
		Payload:    body.Payload,
		SourceRefs: body.SourceRefs,
		RequestID:  httpapi.RequestID(r.Context()),
	})
	if h.writeServiceError(w, r, err) {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var body ConfirmRequest
	if !decodeJSON(w, r, &body, true) {
		return
	}
	body.RequestID = httpapi.RequestID(r.Context())
	result, err := h.service.Confirm(r.Context(), principal, r.PathValue("id"), body)
	if h.writeServiceError(w, r, err) {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var body RejectRequest
	if !decodeJSON(w, r, &body, false) {
		return
	}
	body.RequestID = httpapi.RequestID(r.Context())
	result, err := h.service.Reject(r.Context(), principal, r.PathValue("id"), body)
	if h.writeServiceError(w, r, err) {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

type publishHTTPBody struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var body publishHTTPBody
	if !decodeJSON(w, r, &body, true) {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(body.IdempotencyKey)
	}
	if key == "" {
		digest, _ := hashJSON(r.PathValue("id"))
		key = "auto-publish-" + digest[:24]
	}
	result, err := h.service.Publish(r.Context(), principal, r.PathValue("id"), PublishRequest{
		IdempotencyKey: key,
		RequestID:      httpapi.RequestID(r.Context()),
	})
	if h.writeServiceError(w, r, err) {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), principal, r.PathValue("id"))
	if h.writeServiceError(w, r, err) {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) principal(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	principal, err := h.authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, false
	}
	return principal, true
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	var validation *ValidationError
	switch {
	case errors.As(err, &validation):
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_ARGUMENT", validation.Error())
	case errors.Is(err, auth.ErrUnauthorized):
		httpapi.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	case errors.Is(err, auth.ErrForbidden), errors.Is(err, ErrSkillDisabled):
		httpapi.WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "operation is not allowed")
	case errors.Is(err, auth.ErrNotFound), errors.Is(err, ErrEvidenceNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, ErrIdempotencyConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, ErrInvalidTransition):
		httpapi.WriteError(w, r, http.StatusConflict, "INVALID_STATE", "content must be confirmed before publication")
	case errors.Is(err, ErrImmutableContent):
		httpapi.WriteError(w, r, http.StatusConflict, "CONTENT_IMMUTABLE", err.Error())
	case errors.Is(err, ErrQualityReviewFailed):
		httpapi.WriteError(w, r, http.StatusUnprocessableEntity, "QUALITY_REVIEW_FAILED", err.Error())
	case errors.Is(err, ErrEvidenceUnavailable),
		errors.Is(err, ErrSkillGateUnavailable),
		errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled):
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "required dependency is unavailable")
	case errors.Is(err, ErrGenerationFailed):
		httpapi.WriteError(w, r, http.StatusBadGateway, "GENERATION_FAILED", "content generation returned an invalid result")
	case errors.Is(err, ErrReviewFailed):
		httpapi.WriteError(w, r, http.StatusBadGateway, "REVIEW_FAILED", "content quality review failed")
	case errors.Is(err, ErrConcurrentUpdate):
		httpapi.WriteError(w, r, http.StatusConflict, "CONCURRENT_UPDATE", "content changed concurrently; retry the request")
	default:
		httpapi.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any, allowEmpty bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(destination)
	if allowEmpty && errors.Is(err, io.EOF) {
		return true
	}
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "request body must be valid JSON")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "request body must contain one JSON object")
		return false
	}
	return true
}

func withMaterialRef(refs []EvidenceRef, materialID string) []EvidenceRef {
	materialID = strings.TrimSpace(materialID)
	if materialID == "" {
		return refs
	}
	for _, ref := range refs {
		if strings.TrimSpace(ref.MaterialID) == materialID && strings.TrimSpace(ref.ChunkID) == "" {
			return refs
		}
	}
	return append(refs, EvidenceRef{MaterialID: materialID})
}

func requestIdempotencyKey(r *http.Request, bodyKey string, normalizedBody any) string {
	if key := strings.TrimSpace(r.Header.Get("Idempotency-Key")); key != "" {
		return key
	}
	if key := strings.TrimSpace(bodyKey); key != "" {
		return key
	}
	hash, err := hashJSON(normalizedBody)
	if err != nil {
		return "automatic-idempotency-error"
	}
	return "auto-" + hash[:24]
}
