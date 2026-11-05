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

	"github.com/sheepy-yang/CampusClaw_Group/mcp-teaching-tools/internal/config"
	"github.com/sheepy-yang/CampusClaw_Group/mcp-teaching-tools/internal/mcp"
)

var errConfigInvalid = errors.New("configuration invalid")

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "mcp")
	if err := run(logger); err != nil {
		code := "MCP_STOPPED"
		if errors.Is(err, errConfigInvalid) {
			code = "CONFIG_INVALID"
		}
		logger.Error("mcp stopped", "event", "startup_failed", "code", code, "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("%w: %v", errConfigInvalid, err)
	}
	logger = logger.With("environment", cfg.Environment)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           mcp.NewHandler(cfg.Token, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		logger.Info("mcp listening", "event", "service_started", "port", cfg.Port)
		errs <- server.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Info("mcp shutting down", "event", "service_stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}
