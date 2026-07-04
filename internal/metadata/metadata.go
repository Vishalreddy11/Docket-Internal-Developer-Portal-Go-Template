// Package metadata stores flexible per-file metadata (tags, descriptions, EXIF-
// style key/value extras). It is backed by Postgres with a JSONB column so the
// schema stays open-ended. If Postgres is unreachable, an in-memory map is
// used as a fallback (data does not survive restart).
package metadata

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
)

type Meta struct {
	ID          string            `json:"id"`
	FileName    string            `json:"file_name"`
	ContentType string            `json:"content_type"`
	Size        int64             `json:"size"`
	Owner       string            `json:"owner"`
	Description string            `json:"description"`
	Tags        []string          `json:"tags"`
	Extra       map[string]string `json:"extra"`
	UploadedAt  time.Time         `json:"uploaded_at"`
}

type Store interface {
	Insert(ctx context.Context, m Meta) error
	Get(ctx context.Context, id string) (Meta, error)
	List(ctx context.Context, limit, offset int) ([]Meta, error)
	Delete(ctx context.Context, id string) error
	Mode() string
	Close(ctx context.Context) error
}

func New(ctx context.Context, cfg config.PostgresConfig, log *slog.Logger) Store {
	s, err := newPostgres(ctx, cfg, log)
	if err == nil {
		return s
	}
	log.Warn("postgres unreachable, falling back to in-memory metadata store; data will NOT survive restart", "err", err)
	return newMemoryStore()
}
