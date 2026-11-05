package teaching

import (
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
		Eval:  evaluation.New(evalJobSink{q: enq}),
		Audit: security.NewAuditLogger(),
	}
}

var deps = newDeps()

// RegisterEvaluationRoutes 挂载杨瑾文（作业评价与班级分析）实装入口，以及共用内容/任务查询。
// 刘恺骐 / 刘沛鑫技能入口由 httpserver.registerTeachingRoutes 以骨架 501 声明。
func RegisterEvaluationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade", authJSON(handleMultimodalGrade))
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade/objective", authJSON(handleObjectiveGrade))
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade/confirm-transcription", authJSON(handleConfirmTranscription))
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade/override", teacherJSON(handleOverrideScore))
	mux.HandleFunc("GET /api/v1/skills/multimodal-grade/{submissionID}", authJSON(handleGetSubmission))
	mux.HandleFunc("GET /api/v1/skills/multimodal-grade", teacherJSON(handleListSubmissions))

	mux.HandleFunc("POST /api/v1/skills/class-review", teacherJSON(handleClassReview))
	mux.HandleFunc("POST /api/v1/skills/diagnosis", teacherJSON(handleDiagnosis))
	mux.HandleFunc("POST /api/v1/skills/diagnosis/{contentID}/release", teacherJSON(handleReleaseDiagnosis))

	mux.HandleFunc("POST /api/v1/skills/answer-replay/snapshot", authJSON(handleSnapshot))
	mux.HandleFunc("POST /api/v1/skills/answer-replay/meta", authJSON(handleAttemptMeta))
	mux.HandleFunc("GET /api/v1/skills/answer-replay/{attemptID}/timeline", authJSON(handleTimeline))
	mux.HandleFunc("GET /api/v1/skills/answer-replay/{attemptID}/similarity", teacherJSON(handleSimilarity))

	mux.HandleFunc("POST /api/v1/contents/{id}/confirm", teacherJSON(handleConfirm))
	mux.HandleFunc("POST /api/v1/contents/{id}/publish", teacherJSON(handlePublish))
	mux.HandleFunc("POST /api/v1/contents/{id}/reject", teacherJSON(handleReject))
	mux.HandleFunc("GET /api/v1/contents/{id}", authJSON(handleGetContent))
	mux.HandleFunc("PUT /api/v1/classes/{classID}/skills", teacherJSON(handleSetSkills))
	mux.HandleFunc("GET /api/v1/classes/{classID}/skills", authJSON(handleGetSkills))
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", authJSON(handleGetTask))
}

type handlerFunc func(http.ResponseWriter, *http.Request, auth.Principal)

func teacherJSON(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := principalOr401(w, r)
		if err != nil {
			return
		}
		if err := auth.RequireTeacher(p); err != nil {
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
		writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return auth.Principal{}, err
	}
	return p, nil
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	httpapi.WriteError(w, r, status, code, msg)
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	_ = r
	httpapi.WriteJSON(w, status, v)
}
