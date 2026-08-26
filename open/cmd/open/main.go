package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/zzwsec/finger/open/internal/app"
	"github.com/zzwsec/finger/open/internal/automation"
	"github.com/zzwsec/finger/open/internal/cdn"
	"github.com/zzwsec/finger/open/internal/config"
	"github.com/zzwsec/finger/open/internal/metrics"
	"github.com/zzwsec/finger/open/internal/state"
	"github.com/zzwsec/finger/open/internal/topology"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("open stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	topo, err := topology.Load(cfg.GamesFile)
	if err != nil {
		return err
	}

	currentState := state.New(cfg.StateFile)
	metricStore, err := metrics.Open(ctx, cfg.LogDatabase)
	if err != nil {
		return err
	}
	defer metricStore.Close()

	service := app.New(app.Dependencies{
		Config:     cfg,
		Topology:   topo,
		State:      currentState,
		Metrics:    metricStore,
		Automation: automation.New(cfg.AutomationDir, logger),
		CDN:        cdn.New(cfg.CDNURL),
		Logger:     logger,
	})

	return service.Run(ctx)
}
