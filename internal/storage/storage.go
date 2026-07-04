// Package storage is the object-storage layer (file bytes go here).
// In this template it talks to SeaweedFS via the S3 API; production forks
// swap in AWS S3 or another S3-compatible endpoint by changing the config.
// When the endpoint is unreachable at startup, an in-memory implementation
// is returned instead so the app still boots.
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

func New(ctx context.Context, cfg config.S3Config, log *slog.Logger) Storage {
	s, err := newS3(ctx, cfg, log)
	if err == nil {
		return s
	}
	log.Warn("s3 unreachable, falling back to in-memory storage; uploaded files will NOT survive restart", "err", err)
	return newMemoryStorage()
}
