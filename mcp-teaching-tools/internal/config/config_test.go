package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("MCP_PORT", "8100")
	t.Setenv("MCP_TOKEN", "test-token")
	t.Setenv("SHUTDOWN_TIMEOUT", "5s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8100 || cfg.ShutdownTimeout != 5*time.Second {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsMissingToken(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("MCP_PORT", "8100")
	t.Setenv("MCP_TOKEN", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "5s")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MCP_TOKEN") {
		t.Fatalf("Load() error = %v, want MCP_TOKEN error", err)
	}
}
