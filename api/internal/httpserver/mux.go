package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/platform"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching"
)

// NewMux 注册平台与教学技能路由。具体处理函数为骨架。
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// 平台服务
	platform.RegisterRoutes(mux)

	// 十二项教学技能
	teaching.RegisterRoutes(mux)

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
