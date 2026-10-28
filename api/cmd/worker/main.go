package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/config"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
)

var errConfigInvalid = errors.New("configuration invalid")

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "worker")
	if err := run(logger); err != nil {
		code := "WORKER_STOPPED"
		if errors.Is(err, errConfigInvalid) {
			code = "CONFIG_INVALID"
		}
		logger.Error("worker stopped", "event", "startup_failed", "code", code, "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadWorker()
	if err != nil {
		return fmt.Errorf("%w: %v", errConfigInvalid, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info(
		"worker skeleton ready",
		"event", "service_started",
		"environment", cfg.Environment,
		"mode", "lifecycle_only",
	)
	worker := queue.NewWorker()
	return worker.Run(ctx)
}
