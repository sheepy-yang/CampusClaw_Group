package httpserver

import (
	"errors"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching"
)

type teachingHTTPHandler struct {
	pipeline teaching.Pipeline
}

func registerTeachingRoutes(mux *http.ServeMux, pipeline teaching.Pipeline) {
	handler := &teachingHTTPHandler{pipeline: pipeline}

	mux.HandleFunc("POST /api/v1/skills/proposition", handler.start(teaching.SkillProposition))
	mux.HandleFunc("POST /api/v1/skills/lesson-plan", handler.start(teaching.SkillLessonPlan))
	mux.HandleFunc("POST /api/v1/skills/preview-sheet", handler.start(teaching.SkillPreviewSheet))
	mux.HandleFunc("POST /api/v1/skills/virtual-classroom", handler.start(teaching.SkillVirtualClassroom))

	mux.HandleFunc("POST /api/v1/skills/multimodal-grade", handler.start(teaching.SkillMultimodalGrade))
	mux.HandleFunc("POST /api/v1/skills/class-review", handler.start(teaching.SkillClassReview))
	mux.HandleFunc("POST /api/v1/skills/diagnosis", handler.start(teaching.SkillDiagnosis))
	mux.HandleFunc("POST /api/v1/skills/answer-replay", handler.start(teaching.SkillAnswerReplay))

	mux.HandleFunc("POST /api/v1/skills/review-path", handler.start(teaching.SkillReviewPath))
	mux.HandleFunc("POST /api/v1/skills/socratic-tutor", handler.start(teaching.SkillSocraticTutor))
	mux.HandleFunc("POST /api/v1/skills/classroom-cluster", handler.start(teaching.SkillClassroomCluster))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice", handler.start(teaching.SkillAdaptivePractice))

	mux.HandleFunc("POST /api/v1/contents/{id}/confirm", handler.confirm)
	mux.HandleFunc("POST /api/v1/contents/{id}/publish", handler.publish)
	mux.HandleFunc("PUT /api/v1/classes/{classID}/skills", teachingNotImplemented)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", handler.task)
}

func (h *teachingHTTPHandler) start(skill teaching.SkillID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record, err := h.pipeline.Start(r.Context(), skill, nil)
		if writePipelineError(w, r, err) {
			return
		}
		httpapi.WriteJSON(w, http.StatusAccepted, record)
	}
}

func (h *teachingHTTPHandler) confirm(w http.ResponseWriter, r *http.Request) {
	err := h.pipeline.Confirm(r.Context(), r.PathValue("id"), nil)
	if writePipelineError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *teachingHTTPHandler) publish(w http.ResponseWriter, r *http.Request) {
	err := h.pipeline.Publish(r.Context(), r.PathValue("id"))
	if writePipelineError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *teachingHTTPHandler) task(w http.ResponseWriter, r *http.Request) {
	record, err := h.pipeline.Resume(r.Context(), r.PathValue("taskID"))
	if writePipelineError(w, r, err) {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, record)
}

func writePipelineError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, apperrors.ErrNotImplemented) {
		teachingNotImplemented(w, r)
		return true
	}
	httpapi.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	return true
}

func teachingNotImplemented(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteError(
		w,
		r,
		http.StatusNotImplemented,
		"NOT_IMPLEMENTED",
		"teaching capability is declared but not implemented in the skeleton",
	)
}
