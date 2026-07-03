// Package logging produces a slog JSON logger that automatically carries a
// per-request ID. Handlers retrieve the logger via FromContext so every line
// emitted while serving a request is correlated.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{}

const RequestIDKey = "request_id"

func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

func WithRequestID(ctx context.Context, log *slog.Logger, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, log.With(RequestIDKey, id))
}

func FromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return fallback
}
