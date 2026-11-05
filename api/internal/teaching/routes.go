package teaching

import (
	"encoding/json"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/content"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/tutoring"
)

type apiDeps struct {
	Content *content.Services
	Eval    *evaluation.Services
	Tutor   *tutoring.Services
	Pipe    Pipeline
	Audit   security.AuditLogger
}

func newDeps() *apiDeps {
	llm := gateway.NewClient()
	orch := agent.NewOrchestrator(llm)
	enq := queue.Shared()
	queue.EnsureEmbedded()
	return &apiDeps{
		Content: content.New(orch, &agent.DefaultVerifierAgent{}, enq),
		Eval:    evaluation.New(evalJobSink{q: enq}),
		Tutor:   tutoring.New(orch, &agent.DefaultVerifierAgent{}),
		Pipe:    NewPipeline(),
		Audit:   security.NewAuditLogger(),
	}
}

var deps = newDeps()

// RegisterRoutes 挂载十二项技能 HTTP 入口。
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/skills/proposition", teacherJSON(handleProposition))
	mux.HandleFunc("POST /api/v1/skills/lesson-plan", teacherJSON(handleLessonPlan))
	mux.HandleFunc("POST /api/v1/skills/preview-sheet", teacherJSON(handlePreviewSheet))
	mux.HandleFunc("POST /api/v1/skills/virtual-classroom", teacherJSON(handleVirtualClassroom))

	// --- 杨瑾文：作业评价与班级分析 ---
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

	mux.HandleFunc("POST /api/v1/skills/review-path", authJSON(handleReviewPath))
	mux.HandleFunc("POST /api/v1/skills/socratic-tutor/start", authJSON(handleSocraticStart))
	mux.HandleFunc("POST /api/v1/skills/socratic-tutor", authJSON(handleSocraticNext))
	mux.HandleFunc("POST /api/v1/skills/classroom-cluster", teacherJSON(handleCluster))
	mux.HandleFunc("POST /api/v1/skills/classroom-cluster/answer", authJSON(handleClassroomAnswer))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice/start", authJSON(handleAdaptiveStart))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice/submit", authJSON(handleAdaptiveSubmit))

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
			writeErr(w, http.StatusForbidden, "forbidden")
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
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return auth.Principal{}, err
	}
	return p, nil
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
