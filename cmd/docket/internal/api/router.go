package api

import (
	"log/slog"
	"net/http"

	"github.com/example/docket/internal/app"
	"github.com/example/docket/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func Router(a *app.App, cfg config.Config, log *slog.Logger) http.Handler {
	if cfg.App.APIKey == "" {
		log.Warn("DOCKET_API_KEY is empty — write endpoints are UNAUTHENTICATED. Set it for any non-local use.")
	}

	h := newHandlers(a)
	r := chi.NewRouter()

	r.Use(requestIDMiddleware(log))
	r.Use(loggingMiddleware(log))
	r.Use(metricsMiddleware())

	r.Get("/healthz", h.Health)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)
	r.Get("/openapi.json", h.OpenAPI)
	r.Get("/swagger", h.SwaggerUI)

	r.Get("/files", h.ListFiles)
	r.Get("/files/{id}", h.GetFile)
	r.Get("/files/{id}/download", h.DownloadFile)
	r.Get("/files/{id}/audit", h.AuditTrail)

	// Write endpoints require the API key.
	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuth(cfg.App.APIKey, log))
		r.Post("/files", h.UploadFile)
		r.Delete("/files/{id}", h.DeleteFile)
		r.Post("/seed", h.Seed)
		r.Post("/loadtest", h.LoadTest)
	})

	return otelhttp.NewHandler(r, "docket.http")
}
