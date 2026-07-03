// Package app wires together every backend adapter behind a single struct.
// Handlers depend on App rather than on concrete clients, so swapping a real
// backend for the in-memory fallback is invisible to the rest of the code.
package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/example/docket/internal/cache"
	"github.com/example/docket/internal/config"
	"github.com/example/docket/internal/events"
	"github.com/example/docket/internal/metadata"
	"github.com/example/docket/internal/records"
	"github.com/example/docket/internal/storage"
)

type App struct {
	Log      *slog.Logger
	Cfg      config.Config
	Storage  storage.Storage
	Metadata metadata.Store
	Records  records.Store
	Events   events.Bus
	Cache    cache.Cache
}

// New wires every adapter in parallel. Each adapter tries once with a short
// timeout; failures fall back to the in-memory implementation. Total startup
// is bounded by the slowest single adapter (~2s), not the sum.
func New(ctx context.Context, cfg config.Config, log *slog.Logger) *App {
	a := &App{Log: log, Cfg: cfg}
	var wg sync.WaitGroup
	wg.Add(5)
	go func() { defer wg.Done(); a.Storage = storage.New(ctx, cfg.MinIO, log) }()
	go func() { defer wg.Done(); a.Metadata = metadata.New(ctx, cfg.Mongo, log) }()
	go func() { defer wg.Done(); a.Records = records.New(ctx, cfg.Postgres, log) }()
	go func() { defer wg.Done(); a.Events = events.New(ctx, cfg.NATS, log) }()
	go func() { defer wg.Done(); a.Cache = cache.New(ctx, cfg.Redis, log) }()
	wg.Wait()
	return a
}

func (a *App) Close(ctx context.Context) {
	a.Storage.Close(ctx)
	a.Metadata.Close(ctx)
	a.Records.Close(ctx)
	a.Events.Close(ctx)
	a.Cache.Close(ctx)
}

// Health returns the per-backend connection mode (live | memory).
func (a *App) Health() map[string]string {
	return map[string]string{
		"storage":  a.Storage.Mode(),
		"metadata": a.Metadata.Mode(),
		"records":  a.Records.Mode(),
		"events":   a.Events.Mode(),
		"cache":    a.Cache.Mode(),
	}
}
