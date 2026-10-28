package httpserver

import (
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
)

func registerPlatformRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", platformNotImplemented)
	mux.HandleFunc("POST /api/v1/auth/logout", platformNotImplemented)

	mux.HandleFunc("GET /api/v1/classes/{classID}/materials", platformNotImplemented)
	mux.HandleFunc("POST /api/v1/classes/{classID}/materials", platformNotImplemented)

	mux.HandleFunc("GET /api/v1/classes/{classID}/assignments", platformNotImplemented)
	mux.HandleFunc("POST /api/v1/classes/{classID}/assignments", platformNotImplemented)

	mux.HandleFunc("GET /api/v1/audit", platformNotImplemented)
}

func platformNotImplemented(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteError(
		w,
		r,
		http.StatusNotImplemented,
		"NOT_IMPLEMENTED",
		"platform capability is declared but not implemented in the skeleton",
	)
}
