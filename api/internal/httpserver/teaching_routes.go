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

	// 刘恺骐：智能内容生产与统一编排（骨架占位，统一 501）。
	// 智能命题 / 智能备课 / 智能预习单 / 虚拟学生课堂演练
	mux.HandleFunc("POST /api/v1/skills/proposition", handler.start(teaching.SkillProposition))
	mux.HandleFunc("POST /api/v1/skills/lesson-plan", handler.start(teaching.SkillLessonPlan))
	mux.HandleFunc("POST /api/v1/skills/preview-sheet", handler.start(teaching.SkillPreviewSheet))
	mux.HandleFunc("POST /api/v1/skills/virtual-classroom", handler.start(teaching.SkillVirtualClassroom))

	// 刘沛鑫：个性化辅导与课堂互动（骨架占位，统一 501）。
	// 个性化复习路径 / 追问式辅导 / 课堂回答聚类 / 自适应练习闯关
	mux.HandleFunc("POST /api/v1/skills/review-path", handler.start(teaching.SkillReviewPath))
	mux.HandleFunc("POST /api/v1/skills/socratic-tutor", handler.start(teaching.SkillSocraticTutor))
	mux.HandleFunc("POST /api/v1/skills/classroom-cluster", handler.start(teaching.SkillClassroomCluster))
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice", handler.start(teaching.SkillAdaptivePractice))

	// 杨瑾文：作业评价与班级分析（实装路由）。
	// 多模态主观题批改 / 班级讲评 / 学习诊断 / 作答过程回放与原创性审查
	teaching.RegisterEvaluationRoutes(mux)
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
