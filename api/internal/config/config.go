package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// API contains only process and dependency configuration for the project skeleton.
// Teaching-business configuration is added by the OpenSpec change that introduces it.
type API struct {
	Environment     string
	HTTPPort        int
	ShutdownTimeout time.Duration
	HealthTimeout   time.Duration
	MySQLAddr       string
	RedisAddr       string
	QdrantURL       string
	MinIOURL        string
	MCPURL          string
	SessionSecret   string
}

// Worker contains the minimum configuration required by the queue-process skeleton.
type Worker struct {
	Environment     string
	RedisAddr       string
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
	cfg.RedisAddr = parseAddress("REDIS_ADDR", &errs)
	cfg.QdrantURL = parseHTTPURL("QDRANT_URL", &errs)
	cfg.MinIOURL = parseHTTPURL("MINIO_URL", &errs)
	cfg.MCPURL = parseHTTPURL("MCP_URL", &errs)
	cfg.SessionSecret = require("SESSION_SECRET", &errs)
	if cfg.SessionSecret != "" && len(cfg.SessionSecret) < 16 {
		errs = append(errs, errors.New("SESSION_SECRET must contain at least 16 characters"))
	}

	return cfg, errors.Join(errs...)
}

// LoadWorker reads and validates Worker configuration.
func LoadWorker() (Worker, error) {
	var cfg Worker
	var errs []error

	cfg.Environment = require("APP_ENV", &errs)
	cfg.RedisAddr = parseAddress("REDIS_ADDR", &errs)
	cfg.ShutdownTimeout = parseDuration("SHUTDOWN_TIMEOUT", &errs)

	return cfg, errors.Join(errs...)
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
