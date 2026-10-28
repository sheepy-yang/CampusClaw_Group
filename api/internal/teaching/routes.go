package teaching

import "net/http"

// RegisterRoutes 挂载十二项技能 HTTP 入口（均为骨架）。
func RegisterRoutes(mux *http.ServeMux) {
	// --- 刘恺骐：智能内容生产与统一编排 ---
	mux.HandleFunc("POST /api/v1/skills/proposition", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/lesson-plan", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/preview-sheet", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/virtual-classroom", notImplemented)

	// --- 杨瑾文：作业评价与班级分析 ---
	mux.HandleFunc("POST /api/v1/skills/multimodal-grade", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/class-review", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/diagnosis", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/answer-replay", notImplemented)

	// --- 刘沛鑫：个性化辅导与课堂互动 ---
	mux.HandleFunc("POST /api/v1/skills/review-path", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/socratic-tutor", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/classroom-cluster", notImplemented)
	mux.HandleFunc("POST /api/v1/skills/adaptive-practice", notImplemented)

	// 共用：教师确认 / 发布 / 班级技能开关
	mux.HandleFunc("POST /api/v1/contents/{id}/confirm", notImplemented)
	mux.HandleFunc("POST /api/v1/contents/{id}/publish", notImplemented)
	mux.HandleFunc("PUT /api/v1/classes/{classID}/skills", notImplemented)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
}
