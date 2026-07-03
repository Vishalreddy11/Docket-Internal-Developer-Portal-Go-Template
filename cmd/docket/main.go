package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/docket/internal/api"
	"github.com/example/docket/internal/app"
	"github.com/example/docket/internal/config"
	"github.com/example/docket/internal/logging"
	"github.com/example/docket/internal/otel"
)

func main() {
	cfg := config.Load()
	log := logging.New(cfg.App.LogLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	shutdownTracer, err := otel.Init(ctx, cfg.OTel, log)
	if err != nil {
		log.Warn("otel tracer init failed; continuing without tracing", "err", err)
	}
	defer func() {
		shutCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = shutdownTracer(shutCtx)
	}()

	a := app.New(ctx, cfg, log)
	defer a.Close(context.Background())

	srv := &http.Server{
		Addr:              ":" + cfg.App.Port,
		Handler:           api.Router(a, cfg, log),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("docket listening", "port", cfg.App.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal received, draining...")
	drainCtx, c := context.WithTimeout(context.Background(), 20*time.Second)
	defer c()
	if err := srv.Shutdown(drainCtx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
	}
	log.Info("docket stopped")
}
