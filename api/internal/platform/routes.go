package platform

import "net/http"

// RegisterRoutes 平台服务：认证、班级隔离、讲义检索、助手、作业与审计。
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", notImplemented)
	mux.HandleFunc("POST /api/v1/auth/logout", notImplemented)

	mux.HandleFunc("GET /api/v1/classes/{classID}/materials", notImplemented)
	mux.HandleFunc("POST /api/v1/classes/{classID}/materials", notImplemented)

	mux.HandleFunc("GET /api/v1/classes/{classID}/assignments", notImplemented)
	mux.HandleFunc("POST /api/v1/classes/{classID}/assignments", notImplemented)

	mux.HandleFunc("GET /api/v1/audit", notImplemented)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
}
