package teaching

import (
	"context"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

type apiDeps struct {
	Eval  *evaluation.Services
	Audit security.AuditLogger
}

func newDeps() *apiDeps {
	enq := queue.Shared()
	queue.EnsureEmbedded()
	return &apiDeps{
		Eval: evaluation.NewWithDependencies(evaluation.Dependencies{
			Jobs:             evalJobSink{q: enq},
			ObjectiveAnswers: evaluation.StoreObjectiveAnswerSource{},
		}),
		Audit: security.NewAuditLogger(),
	}
}

var deps = newDeps()

// RegisterEvaluationRoutes 挂载作业评价、班级分析和共用内容/任务查询。
type ProductionLifecycle interface {
	HasContent(context.Context, string) bool
	Confirm(http.ResponseWriter, *http.Request)
	Reject(http.ResponseWriter, *http.Request)
	Publish(http.ResponseWriter, *http.Request)
	Get(http.ResponseWriter, *http.Request)
}

func RegisterEvaluationRoutes(mux *http.ServeMux) {
	RegisterEvaluationRoutesWithProduction(mux, nil)
}

func RegisterEvaluationRoutesWithProduction(mux *http.ServeMux, production ProductionLifecycle) {
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade", authJSON(handleMultimodalGrade))
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade/objective", authJSON(handleObjectiveGrade))
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade/confirm-transcription", authJSON(handleConfirmTranscription))
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade/override", teacherJSON(handleOverrideScore))
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade/{submissionID}/retry", authJSON(handleRetryMultimodalGrade))
	mux.HandleFunc("GET /api/v1/skills/multimodal-grade/{submissionID}", authJSON(handleGetSubmission))
	mux.HandleFunc("GET /api/v1/skills/multimodal-grade", teacherJSON(handleListSubmissions))

	mux.HandleFunc("POST /api/v1/skills/class-review", teacherJSON(handleClassReview))
	mux.HandleFunc("POST /api/v1/skills/diagnosis", teacherJSON(handleDiagnosis))
	mux.HandleFunc("POST /api/v1/skills/diagnosis/{contentID}/release", teacherJSON(handleReleaseDiagnosis))

	mux.HandleFunc("POST /api/v1/skills/answer-replay/snapshot", authJSON(handleSnapshot))
	mux.HandleFunc("POST /api/v1/skills/answer-replay/meta", authJSON(handleAttemptMeta))
	mux.HandleFunc("GET /api/v1/skills/answer-replay/{attemptID}/timeline", authJSON(handleTimeline))
	mux.HandleFunc("GET /api/v1/skills/answer-replay/{attemptID}/similarity", teacherJSON(handleSimilarity))

	mux.HandleFunc("POST /api/v1/contents/{id}/confirm", dispatchProduction(production, (*productionHandlers).confirm, teacherJSON(handleConfirm)))
	mux.HandleFunc("POST /api/v1/contents/{id}/publish", dispatchProduction(production, (*productionHandlers).publish, teacherJSON(handlePublish)))
	mux.HandleFunc("POST /api/v1/contents/{id}/reject", dispatchProduction(production, (*productionHandlers).reject, teacherJSON(handleReject)))
	mux.HandleFunc("GET /api/v1/contents/{id}", dispatchProduction(production, (*productionHandlers).get, authJSON(handleGetContent)))
	mux.HandleFunc("GET /api/v1/classes/{classID}/contents", authJSON(handleListContents))
	mux.HandleFunc("PUT /api/v1/classes/{classID}/skills", teacherJSON(handleSetSkills))
	mux.HandleFunc("GET /api/v1/classes/{classID}/skills", authJSON(handleGetSkills))
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", authJSON(handleGetTask))
}

type productionHandlers struct {
	lifecycle ProductionLifecycle
}

func (h *productionHandlers) confirm(w http.ResponseWriter, r *http.Request) {
	h.lifecycle.Confirm(w, r)
}
func (h *productionHandlers) reject(w http.ResponseWriter, r *http.Request) {
	h.lifecycle.Reject(w, r)
}
func (h *productionHandlers) publish(w http.ResponseWriter, r *http.Request) {
	h.lifecycle.Publish(w, r)
}
func (h *productionHandlers) get(w http.ResponseWriter, r *http.Request) { h.lifecycle.Get(w, r) }

func dispatchProduction(
	lifecycle ProductionLifecycle,
	action func(*productionHandlers, http.ResponseWriter, *http.Request),
	fallback http.HandlerFunc,
) http.HandlerFunc {
	handlers := &productionHandlers{lifecycle: lifecycle}
	return func(w http.ResponseWriter, r *http.Request) {
		if lifecycle != nil && lifecycle.HasContent(r.Context(), r.PathValue("id")) {
			action(handlers, w, r)
			return
		}
		fallback(w, r)
	}
}

type handlerFunc func(http.ResponseWriter, *http.Request, auth.Principal)

func teacherJSON(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := principalOr401(w, r)
		if err != nil {
			return
		}
		if err := auth.RequireTeacher(p); err != nil {
			auditRejected(r, p, "authorization", "teacher_route")
			writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "forbidden")
			return
		}
		h(w, r, p)
	}
}

func authJSON(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := principalOr401(w, r)
		if err != nil {
			return
		}
		h(w, r, p)
	}
}

func principalOr401(w http.ResponseWriter, r *http.Request) (auth.Principal, error) {
	p, err := auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		auditRejected(r, auth.Principal{}, "authorization", "protected_route")
		writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return auth.Principal{}, err
	}
	return p, nil
}

func auditRejected(r *http.Request, p auth.Principal, action, object string) {
	_ = deps.Audit.Log(r.Context(), security.AuditEvent{
		ActorID: p.UserID, ClassID: p.ClassID, Action: action, Object: object,
		Result: "denied", RequestID: httpapi.RequestID(r.Context()),
		Detail: map[string]any{"path": r.URL.Path},
	})
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	httpapi.WriteError(w, r, status, code, msg)
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	_ = r
	httpapi.WriteJSON(w, status, v)
}
