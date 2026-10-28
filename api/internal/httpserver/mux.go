package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/health"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching"
)

// Dependencies are process-level HTTP dependencies assembled in cmd/server.
type Dependencies struct {
	Logger           *slog.Logger
	HealthTimeout    time.Duration
	HealthChecks     []health.Checker
	TeachingPipeline teaching.Pipeline
}

// NewMux registers the operational skeleton and declared business routes.
func NewMux(deps Dependencies) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.TeachingPipeline == nil {
		deps.TeachingPipeline = teaching.NewPipeline(teaching.PipelineDependencies{})
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		report := health.Run(r.Context(), deps.HealthTimeout, deps.HealthChecks)
		status := http.StatusOK
		if report.Status == health.StatusDown {
			status = http.StatusServiceUnavailable
		}
		httpapi.WriteJSON(w, status, report)
	})

	// 平台服务
	registerPlatformRoutes(mux)

	// 十二项教学技能
	registerTeachingRoutes(mux, deps.TeachingPipeline)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpapi.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	})

	return requestLogging(deps.Logger, mux)
}
