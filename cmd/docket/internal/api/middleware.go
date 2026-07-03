package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/docket/internal/logging"
	"github.com/example/docket/internal/metrics"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func requestIDMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = uuid.NewString()
			}
			w.Header().Set("X-Request-Id", id)
			ctx := logging.WithRequestID(r.Context(), log, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }

func loggingMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			dur := time.Since(start)
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = r.URL.Path
			}
			logging.FromContext(r.Context(), log).Info("http",
				"method", r.Method,
				"route", route,
				"status", rec.status,
				"duration_ms", dur.Milliseconds(),
				"remote", r.RemoteAddr,
			)
		})
	}
}

func metricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unknown"
			}
			metrics.HTTPRequests.WithLabelValues(route, r.Method, statusBucket(rec.status)).Inc()
			metrics.HTTPDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
		})
	}
}

func statusBucket(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// apiKeyAuth gates write endpoints. If DOCKET_API_KEY is empty, auth is
// disabled (with a one-time WARN at startup, emitted in router.go).
func apiKeyAuth(expected string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				next.ServeHTTP(w, r)
				return
			}
			got := r.Header.Get("X-API-Key")
			if got == "" {
				if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
					got = strings.TrimPrefix(h, "Bearer ")
				}
			}
			if got != expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
