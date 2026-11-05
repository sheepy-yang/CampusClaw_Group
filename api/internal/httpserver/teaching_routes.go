package httpserver

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/content"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching"
)

type teachingHTTPHandler struct {
	pipeline teaching.Pipeline
}

func registerTeachingRoutes(mux *http.ServeMux, pipeline teaching.Pipeline, contentHandler *content.Handler) {
	handler := &teachingHTTPHandler{pipeline: pipeline}

	// 刘恺骐：前三项内容生产由 content.Handler 挂载；此处挂载异步课堂演练。
	mux.HandleFunc("POST /api/v1/skills/virtual-classroom", handler.virtualClassroom)

	// 杨瑾文：作业评价与班级分析（实装路由）。
	// 多模态主观题批改 / 班级讲评 / 学习诊断 / 作答过程回放与原创性审查
	teaching.RegisterEvaluationRoutesWithProduction(mux, contentHandler)
}

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)

func (h *teachingHTTPHandler) virtualClassroom(w http.ResponseWriter, r *http.Request) {
	principal, err := auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	if err := auth.RequireTeacher(principal); err != nil {
		httpapi.WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "teacher role required")
		return
	}
	var body struct {
		LessonPlanID         string `json:"lessonPlanId"`
		Objective            string `json:"objective"`
		Rounds               int    `json:"rounds"`
		PreviousSimulationID string `json:"previousSimulationId,omitempty"`
	}
	if !decodeTutoringJSON(w, r, &body) {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !idempotencyKeyPattern.MatchString(key) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 8 to 128 safe characters")
		return
	}
	record, err := h.pipeline.Start(r.Context(), teaching.SkillVirtualClassroom, map[string]any{
		"classId": principal.ClassID, "userId": principal.UserID,
		"lessonPlanId": body.LessonPlanID, "objective": body.Objective,
		"rounds": body.Rounds, "previousSimulationId": body.PreviousSimulationID,
		"idempotencyKey": key,
	})
	if err != nil {
		switch {
		case errors.Is(err, teaching.ErrPipelineInvalid):
			httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, teaching.ErrPipelineNotFound):
			httpapi.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "lesson plan or previous simulation not found")
		case errors.Is(err, teaching.ErrPipelineConflict):
			httpapi.WriteError(w, r, http.StatusConflict, "CONFLICT", err.Error())
		default:
			httpapi.WriteError(w, r, http.StatusServiceUnavailable, "TASK_UNAVAILABLE", "simulation task could not be queued")
		}
		return
	}
	if err := store.Default.AppendAudit(store.AuditEvent{
		ID: store.NewID("audit"), ActorID: principal.UserID, ClassID: principal.ClassID,
		Action: "virtual_classroom.enqueue", Object: record.ObjectID, Result: "accepted",
		Skill: string(teaching.SkillVirtualClassroom), RequestID: httpapi.RequestID(r.Context()),
		CreatedAt: time.Now().UTC(), Detail: map[string]any{"taskId": record.ID},
	}); err != nil {
		log.Printf("virtual classroom audit persistence failed task=%s: %v", record.ID, err)
	}
	httpapi.WriteJSON(w, http.StatusAccepted, record)
}
