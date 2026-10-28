package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/config"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/health"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpserver"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/mcpclient"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/retrieval"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching"
)

var errConfigInvalid = errors.New("configuration invalid")

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "api")
	if err := run(logger); err != nil {
		code := "API_STOPPED"
		if errors.Is(err, errConfigInvalid) {
			code = "CONFIG_INVALID"
		}
		logger.Error("api stopped", "event", "startup_failed", "code", code, "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadAPI()
	if err != nil {
		return fmt.Errorf("%w: %v", errConfigInvalid, err)
	}
	logger = logger.With("environment", cfg.Environment)

	checkers := []health.Checker{
		health.NewTCPChecker("mysql", cfg.MySQLAddr),
		health.NewTCPChecker("redis", cfg.RedisAddr),
		health.NewHTTPChecker("qdrant", cfg.QdrantURL+"/readyz"),
		health.NewHTTPChecker("minio", cfg.MinIOURL+"/minio/health/live"),
		health.NewHTTPChecker("mcp", cfg.MCPURL+"/health"),
	}
	pipeline := teaching.NewPipeline(teaching.PipelineDependencies{
		Orchestrator:   agent.StubOrchestrator{},
		ContentAgent:   agent.StubContentAgent{},
		VirtualStudent: agent.StubVirtualStudentAgent{},
		Verifier:       agent.StubVerifierAgent{},
		Model:          gateway.StubClient{},
		Retriever:      retrieval.StubRetriever{},
		MCP:            mcpclient.StubHost{},
		Queue:          queue.StubEnqueuer{},
		Audit:          security.StubAuditLogger{},
	})
	handler := httpserver.NewMux(httpserver.Dependencies{
		Logger:           logger,
		HealthTimeout:    cfg.HealthTimeout,
		HealthChecks:     checkers,
		TeachingPipeline: pipeline,
	})
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.HTTPPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		logger.Info("api listening", "event", "service_started", "port", cfg.HTTPPort)
		errs <- server.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Info("api shutting down", "event", "service_stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}
