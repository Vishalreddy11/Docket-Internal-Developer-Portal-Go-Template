// Package storage is the object-storage layer (file bytes go here).
// In production it talks to MinIO (S3-compatible). When MinIO is unreachable
// at startup, an in-memory implementation is returned instead so the app
// still boots — useful for local dev with zero dependencies.
package storage

import (
	"context"
	"io"
	"log/slog"

	"github.com/example/docket/internal/config"
)

type Storage interface {
	Put(ctx context.Context, id string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, id string) (data io.ReadCloser, size int64, contentType string, err error)
	Delete(ctx context.Context, id string) error
	Mode() string
	Close(ctx context.Context) error
}

func New(ctx context.Context, cfg config.MinIOConfig, log *slog.Logger) Storage {
	s, err := newMinIO(ctx, cfg, log)
	if err == nil {
		return s
	}
	log.Warn("minio unreachable, falling back to in-memory storage; uploaded files will NOT survive restart", "err", err)
	return newMemoryStorage()
}
