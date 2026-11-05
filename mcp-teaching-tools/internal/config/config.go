package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment     string
	Port            int
	Token           string
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	var cfg Config
	var errs []error

	cfg.Environment = require("APP_ENV", &errs)
	cfg.Token = require("MCP_TOKEN", &errs)
	cfg.Port = parsePort("MCP_PORT", &errs)
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
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		*errs = append(*errs, fmt.Errorf("%s must be an integer between 1 and 65535", name))
		return 0
	}
	return port
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
