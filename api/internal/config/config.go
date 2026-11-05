package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// API contains process and dependency configuration. Model-gateway settings are
// consumed by the gateway adapter directly and are never logged.
type API struct {
	Environment      string
	HTTPPort         int
	ShutdownTimeout  time.Duration
	HealthTimeout    time.Duration
	MySQLAddr        string
	MySQLDSN         string
	RedisAddr        string
	RedisURL         string
	QdrantURL        string
	QdrantCollection string
	MinIOURL         string
	MCPURL           string
	SessionSecret    string
	MCPToken         string
}

// Worker contains the configuration required by the durable queue process.
type Worker struct {
	Environment     string
	RedisAddr       string
	RedisURL        string
	MySQLDSN        string
	ShutdownTimeout time.Duration
}

// LoadAPI reads and validates API configuration without logging any values.
func LoadAPI() (API, error) {
	var cfg API
	var errs []error

	cfg.Environment = require("APP_ENV", &errs)
	cfg.HTTPPort = parsePort("HTTP_PORT", &errs)
	cfg.ShutdownTimeout = parseDuration("SHUTDOWN_TIMEOUT", &errs)
	cfg.HealthTimeout = parseDuration("HEALTH_TIMEOUT", &errs)
	cfg.MySQLAddr = parseAddress("MYSQL_ADDR", &errs)
	cfg.MySQLDSN = require("MYSQL_DSN", &errs)
	cfg.RedisAddr = parseAddress("REDIS_ADDR", &errs)
	cfg.RedisURL = parseRedisURL("REDIS_URL", &errs)
	cfg.QdrantURL = parseHTTPURL("QDRANT_URL", &errs)
	cfg.QdrantCollection = require("QDRANT_COLLECTION", &errs)
	if cfg.QdrantCollection != "" && !regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`).MatchString(cfg.QdrantCollection) {
		errs = append(errs, errors.New("QDRANT_COLLECTION must contain only letters, digits, underscores, or hyphens"))
	}
	cfg.MinIOURL = parseHTTPURL("MINIO_URL", &errs)
	cfg.MCPURL = parseHTTPURL("MCP_URL", &errs)
	cfg.SessionSecret = require("SESSION_SECRET", &errs)
	cfg.MCPToken = require("MCP_TOKEN", &errs)
	if cfg.SessionSecret != "" && len(cfg.SessionSecret) < 16 {
		errs = append(errs, errors.New("SESSION_SECRET must contain at least 16 characters"))
	}
	validateModelGateway(cfg.Environment, &errs)
	if !isDemoEnvironment(cfg.Environment) && strings.TrimSpace(os.Getenv("AUTH_USERS_JSON")) == "" {
		errs = append(errs, errors.New("AUTH_USERS_JSON is required outside demo environments"))
	}

	return cfg, errors.Join(errs...)
}

// LoadWorker reads and validates Worker configuration.
func LoadWorker() (Worker, error) {
	var cfg Worker
	var errs []error

	cfg.Environment = require("APP_ENV", &errs)
	cfg.RedisAddr = parseAddress("REDIS_ADDR", &errs)
	cfg.RedisURL = parseRedisURL("REDIS_URL", &errs)
	cfg.MySQLDSN = require("MYSQL_DSN", &errs)
	cfg.ShutdownTimeout = parseDuration("SHUTDOWN_TIMEOUT", &errs)
	validateModelGateway(cfg.Environment, &errs)

	return cfg, errors.Join(errs...)
}

func validateModelGateway(environment string, errs *[]error) {
	if isDemoEnvironment(environment) {
		return
	}
	if strings.TrimSpace(os.Getenv("LLM_API_KEY")) == "" && strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY")) == "" {
		*errs = append(*errs, errors.New("a model gateway API key is required outside demo environments"))
	}
	if strings.TrimSpace(os.Getenv("LLM_BASE_URL")) == "" && strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL")) == "" {
		*errs = append(*errs, errors.New("a model gateway base URL is required outside demo environments"))
	}
}

func isDemoEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "", "local", "development", "dev", "test", "demo":
		return true
	default:
		return false
	}
}

func require(name string, errs *[]error) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", name))
	}
	return value
}

func parsePort(name string, errs *[]error) int {
	raw := require(name, errs)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 65535 {
		*errs = append(*errs, fmt.Errorf("%s must be an integer between 1 and 65535", name))
		return 0
	}
	return value
}

func parseDuration(name string, errs *[]error) time.Duration {
	raw := require(name, errs)
	if raw == "" {
		return 0
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		*errs = append(*errs, fmt.Errorf("%s must be a positive duration", name))
		return 0
	}
	return value
}

func parseAddress(name string, errs *[]error) string {
	raw := require(name, errs)
	if raw == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		*errs = append(*errs, fmt.Errorf("%s must use host:port format", name))
		return ""
	}
	return raw
}

func parseHTTPURL(name string, errs *[]error) string {
	raw := require(name, errs)
	if raw == "" {
		return ""
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		*errs = append(*errs, fmt.Errorf("%s must be an absolute http or https URL", name))
		return ""
	}
	return strings.TrimRight(raw, "/")
}

func parseRedisURL(name string, errs *[]error) string {
	raw := require(name, errs)
	if raw == "" {
		return ""
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") {
		*errs = append(*errs, fmt.Errorf("%s must be an absolute redis or rediss URL", name))
		return ""
	}
	return raw
}
