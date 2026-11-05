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
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/content"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/health"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpserver"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/objectstore"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/tutoring"
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

	llm := gateway.NewClient()
	orch := agent.NewOrchestrator(llm)
	pipeline := teaching.NewPipeline(teaching.PipelineDependencies{
		Queue: queue.Shared(),
	})
	contentService := content.NewService(content.ServiceOptions{
		Repository: content.NewStoreRepository(store.Default),
		Evidence:   content.NewSharedStoreEvidenceProvider(store.Default),
		Skills:     content.NewStoreSkillGate(store.Default),
		Producer:   agent.NewGatewayContentAgent(llm),
		Reviewer:   agent.NewDeterministicQualityAgent(),
		IDGenerator: func(prefix string) string {
			return store.NewID(prefix)
		},
	})
	contentHandler := content.NewHandler(contentService, nil)
	tutoringRepository := tutoring.NewRepository(store.Default)
	tutoringModel := tutoring.OrchestratorModel{Orchestrator: orch}
	tutoringService := tutoring.NewService(tutoring.ServiceDependencies{
		Repository: tutoringRepository, ReviewModel: tutoringModel,
		TutorModel: tutoringModel, ClusterModel: tutoringModel,
		Audit: security.NewAuditLogger(),
	})
	tutoringMCP, err := tutoring.NewHTTPMCPHost(cfg.MCPURL, cfg.MCPToken, nil, 5*time.Second)
	if err != nil {
		return fmt.Errorf("%w: tutoring MCP: %v", errConfigInvalid, err)
	}
	tutoringDependencies := &httpserver.TutoringDependencies{
		Service: tutoringService,
		Tools: tutoring.NewTeachingTools(
			tutoringMCP,
			tutoring.StoreToolDataSource{Memory: store.Default},
			security.NewAuditLogger(),
		),
	}
	attachmentStore, err := objectstore.NewFromEnv()
	if err != nil {
		return fmt.Errorf("%w: object store: %v", errConfigInvalid, err)
	}

	handler := httpserver.NewMux(httpserver.Dependencies{
		Logger:           logger,
		HealthTimeout:    cfg.HealthTimeout,
		HealthChecks:     checkers,
		TeachingPipeline: pipeline,
		ContentHandler:   contentHandler,
		Tutoring:         tutoringDependencies,
		ObjectStore:      attachmentStore,
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
