package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/tutoring"
)

const maxTutoringRequestBytes = 64 << 10

type TutoringDependencies struct {
	Service *tutoring.Service
	Tools   *tutoring.TeachingTools
}

type tutoringHTTPHandler struct {
	service *tutoring.Service
	tools   *tutoring.TeachingTools
}

// RegisterTutoringRoutes mounts the four tutoring skills and their MCP tools.
func RegisterTutoringRoutes(mux *http.ServeMux, dependencies TutoringDependencies) {
	handler := &tutoringHTTPHandler{service: dependencies.Service, tools: dependencies.Tools}

	mux.HandleFunc("GET /api/v1/learning/state", handler.auth(handler.learningState))
	mux.HandleFunc("POST /api/v1/skills/review-path", handler.auth(handler.generateReviewPath))
	mux.HandleFunc("GET /api/v1/review-paths/latest", handler.auth(handler.latestReviewPath))
	mux.HandleFunc("GET /api/v1/review-paths/{planID}", handler.auth(handler.reviewPath))
	mux.HandleFunc("POST /api/v1/review-paths/{planID}/steps/{stepID}/complete", handler.auth(handler.completeReviewStep))

	mux.HandleFunc("POST /api/v1/skills/socratic-tutor/start", handler.auth(handler.startTutor))
	mux.HandleFunc("POST /api/v1/skills/socratic-tutor", handler.auth(handler.continueTutor))
	mux.HandleFunc("GET /api/v1/tutoring/sessions/{sessionID}", handler.auth(handler.tutorSession))

	mux.HandleFunc("POST /api/v1/skills/adaptive-practice", handler.auth(handler.startPractice))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice/start", handler.auth(handler.startPractice))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice/submit", handler.auth(handler.submitPractice))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice/hint", handler.auth(handler.practiceHint))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice/quit", handler.auth(handler.quitPractice))
	mux.HandleFunc("GET /api/v1/practice/sessions/{sessionID}", handler.auth(handler.practiceSession))

	mux.HandleFunc("POST /api/v1/classroom/interactions", handler.auth(handler.createInteraction))
	mux.HandleFunc("GET /api/v1/classroom/interactions/active", handler.auth(handler.activeInteraction))
	mux.HandleFunc("POST /api/v1/classroom/interactions/{sessionID}/close", handler.auth(handler.closeInteraction))
	mux.HandleFunc("GET /api/v1/classroom/interactions/{sessionID}/analysis", handler.auth(handler.classroomAnalysis))
	mux.HandleFunc("POST /api/v1/skills/classroom-cluster/answer", handler.auth(handler.submitClassroomAnswer))
	mux.HandleFunc("POST /api/v1/skills/classroom-cluster", handler.auth(handler.clusterInteraction))

	mux.HandleFunc("GET /api/v1/mcp/tools", handler.auth(handler.listTools))
	mux.HandleFunc("POST /api/v1/mcp/tools/{tool}/call", handler.auth(handler.callTool))
}

type tutoringHandlerFunc func(http.ResponseWriter, *http.Request, auth.Principal)

func (h *tutoringHTTPHandler) auth(next tutoringHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			httpapi.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
		next(w, r, principal)
	}
}

func (h *tutoringHTTPHandler) learningState(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	result, err := h.service.LearningState(r.Context(), principal)
	h.writeResult(w, r, http.StatusOK, result, err)
}

func (h *tutoringHTTPHandler) generateReviewPath(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct{}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	result, err := h.service.GenerateReviewPath(r.Context(), principal)
	h.writeResult(w, r, http.StatusOK, result, err)
}

func (h *tutoringHTTPHandler) latestReviewPath(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	plan, err := h.service.LatestReviewPath(r.Context(), principal, r.URL.Query().Get("studentId"))
	h.writeResult(w, r, http.StatusOK, map[string]any{"plan": plan}, err)
}

func (h *tutoringHTTPHandler) reviewPath(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	plan, err := h.service.ReviewPath(r.Context(), principal, r.PathValue("planID"))
	h.writeResult(w, r, http.StatusOK, plan, err)
}

func (h *tutoringHTTPHandler) completeReviewStep(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct{}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	plan, err := h.service.CompleteReviewStep(r.Context(), principal, r.PathValue("planID"), r.PathValue("stepID"))
	h.writeResult(w, r, http.StatusOK, plan, err)
}

func (h *tutoringHTTPHandler) startTutor(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		QuestionID string `json:"questionId"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	session, err := h.service.StartTutor(r.Context(), principal, body.QuestionID)
	h.writeResult(w, r, http.StatusCreated, session, err)
}

func (h *tutoringHTTPHandler) continueTutor(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		SessionID string `json:"sessionId"`
		Reply     string `json:"reply"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	session, err := h.service.ContinueTutor(r.Context(), principal, body.SessionID, body.Reply)
	h.writeResult(w, r, http.StatusOK, session, err)
}

func (h *tutoringHTTPHandler) tutorSession(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	session, err := h.service.TutorSession(r.Context(), principal, r.PathValue("sessionID"))
	h.writeResult(w, r, http.StatusOK, session, err)
}

func (h *tutoringHTTPHandler) startPractice(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		KnowledgePoint string `json:"knowledgePoint"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	session, err := h.service.StartPractice(r.Context(), principal, body.KnowledgePoint)
	h.writeResult(w, r, http.StatusCreated, session, err)
}

func (h *tutoringHTTPHandler) submitPractice(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		SessionID    string `json:"sessionId"`
		SubmissionID string `json:"submissionId"`
		AnswerText   string `json:"answerText"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	result, err := h.service.SubmitPractice(r.Context(), principal, body.SessionID, body.SubmissionID, body.AnswerText)
	h.writeResult(w, r, http.StatusOK, result, err)
}

func (h *tutoringHTTPHandler) practiceHint(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		SessionID string `json:"sessionId"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	result, err := h.service.PracticeHint(r.Context(), principal, body.SessionID)
	h.writeResult(w, r, http.StatusOK, result, err)
}

func (h *tutoringHTTPHandler) quitPractice(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		SessionID    string `json:"sessionId"`
		SubmissionID string `json:"submissionId"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	result, err := h.service.QuitPractice(r.Context(), principal, body.SessionID, body.SubmissionID)
	h.writeResult(w, r, http.StatusOK, result, err)
}

func (h *tutoringHTTPHandler) practiceSession(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	session, err := h.service.PracticeSession(r.Context(), principal, r.PathValue("sessionID"))
	h.writeResult(w, r, http.StatusOK, session, err)
}

func (h *tutoringHTTPHandler) createInteraction(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	interaction, err := h.service.CreateInteraction(r.Context(), principal, body.Prompt)
	h.writeResult(w, r, http.StatusCreated, interaction, err)
}

func (h *tutoringHTTPHandler) activeInteraction(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	interaction, err := h.service.ActiveInteraction(r.Context(), principal)
	h.writeResult(w, r, http.StatusOK, map[string]any{"interaction": interaction}, err)
}

func (h *tutoringHTTPHandler) submitClassroomAnswer(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		SessionID string `json:"sessionId"`
		Text      string `json:"text"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	result, err := h.service.SubmitClassroomAnswer(r.Context(), principal, body.SessionID, body.Text)
	h.writeResult(w, r, http.StatusOK, result, err)
}

func (h *tutoringHTTPHandler) closeInteraction(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct{}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	interaction, err := h.service.CloseInteraction(r.Context(), principal, r.PathValue("sessionID"))
	h.writeResult(w, r, http.StatusOK, interaction, err)
}

func (h *tutoringHTTPHandler) clusterInteraction(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	var body struct {
		SessionID string `json:"sessionId"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	analysis, err := h.service.ClusterInteraction(r.Context(), principal, body.SessionID)
	h.writeResult(w, r, http.StatusOK, analysis, err)
}

func (h *tutoringHTTPHandler) classroomAnalysis(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !h.requireService(w, r) {
		return
	}
	analysis, err := h.service.ClassroomAnalysis(r.Context(), principal, r.PathValue("sessionID"))
	h.writeResult(w, r, http.StatusOK, analysis, err)
}

func (h *tutoringHTTPHandler) listTools(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if h.tools == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "MCP is unavailable")
		return
	}
	tools, err := h.tools.List(r.Context(), principal)
	h.writeResult(w, r, http.StatusOK, map[string]any{"tools": tools}, err)
}

func (h *tutoringHTTPHandler) callTool(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if h.tools == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "MCP is unavailable")
		return
	}
	var body struct {
		Arguments map[string]any `json:"arguments"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	requestID := httpapi.RequestID(r.Context())
	if requestID == "" {
		requestID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
	}
	if !requestIDPattern.MatchString(requestID) {
		requestID = newRequestID()
	}
	result, err := h.tools.Call(r.Context(), principal, requestID, r.PathValue("tool"), body.Arguments)
	h.writeResult(w, r, http.StatusOK, result, err)
}

func (h *tutoringHTTPHandler) requireService(w http.ResponseWriter, r *http.Request) bool {
	if h.service != nil {
		return true
	}
	httpapi.WriteError(w, r, http.StatusServiceUnavailable, "TUTORING_UNAVAILABLE", "tutoring service is unavailable")
	return false
}

func (h *tutoringHTTPHandler) writeResult(w http.ResponseWriter, r *http.Request, status int, value any, err error) {
	if err == nil {
		httpapi.WriteJSON(w, status, value)
		return
	}
	errorStatus, code, message := tutoring.ErrorInfo(err)
	httpapi.WriteError(w, r, errorStatus, code, message)
}

func decodeTutoringJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxTutoringRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "request body must be valid JSON")
		return false
	}
	var extra any
	err := decoder.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "request body must contain one JSON value")
		return false
	}
	return true
}
