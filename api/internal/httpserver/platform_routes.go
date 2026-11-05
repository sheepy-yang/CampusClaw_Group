package httpserver

import (
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/platform"
)

// registerPlatformRoutes mounts platform capabilities.
// Auth / class-scoped materials / assignments / audit are implemented for local demo;
// additional platform surfaces stay behind the OpenSpec skeleton until their change lands.
func registerPlatformRoutes(mux *http.ServeMux) {
	platform.RegisterRoutes(mux)
}
