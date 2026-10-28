package config

import (
	"strings"
	"testing"
	"time"
)

func setValidAPIEnv(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"APP_ENV":          "test",
		"HTTP_PORT":        "8080",
		"SHUTDOWN_TIMEOUT": "5s",
		"HEALTH_TIMEOUT":   "2s",
		"MYSQL_ADDR":       "mysql:3306",
		"REDIS_ADDR":       "redis:6379",
		"QDRANT_URL":       "http://qdrant:6333",
		"MINIO_URL":        "http://minio:9000",
		"MCP_URL":          "http://mcp:8100",
		"SESSION_SECRET":   "test-secret-at-least-16",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
}

func TestLoadAPI(t *testing.T) {
	setValidAPIEnv(t)

	cfg, err := LoadAPI()
	if err != nil {
		t.Fatalf("LoadAPI() error = %v", err)
	}
	if cfg.HTTPPort != 8080 || cfg.HealthTimeout != 2*time.Second {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadAPIRejectsMissingValueWithoutLeakingSecret(t *testing.T) {
	setValidAPIEnv(t)
	t.Setenv("MCP_URL", "")
	t.Setenv("SESSION_SECRET", "sensitive-value-that-must-not-appear")

	_, err := LoadAPI()
	if err == nil {
		t.Fatal("LoadAPI() expected an error")
	}
	message := err.Error()
	if !strings.Contains(message, "MCP_URL") {
		t.Fatalf("error %q does not identify MCP_URL", message)
	}
	if strings.Contains(message, "sensitive-value") {
		t.Fatalf("error leaked a secret: %q", message)
	}
}

func TestLoadAPIRejectsInvalidFormats(t *testing.T) {
	setValidAPIEnv(t)
	t.Setenv("HTTP_PORT", "70000")
	t.Setenv("MYSQL_ADDR", "missing-port")
	t.Setenv("QDRANT_URL", "file:///tmp/qdrant")

	_, err := LoadAPI()
	if err == nil {
		t.Fatal("LoadAPI() expected an error")
	}
	for _, name := range []string{"HTTP_PORT", "MYSQL_ADDR", "QDRANT_URL"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not identify %s", err, name)
		}
	}
}
