// Package metadata stores flexible per-file metadata (tags, descriptions, EXIF-
// style key/value extras). It is backed by MongoDB; if Mongo is unreachable, an
// in-memory map is used. Mongo is the right choice here because the metadata
// schema is open-ended — different file types attach different fields.
package metadata

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
)

type Meta struct {
	ID          string            `json:"id"          bson:"_id"`
	FileName    string            `json:"file_name"   bson:"file_name"`
	ContentType string            `json:"content_type" bson:"content_type"`
	Size        int64             `json:"size"        bson:"size"`
	Owner       string            `json:"owner"       bson:"owner"`
	Description string            `json:"description" bson:"description"`
	Tags        []string          `json:"tags"        bson:"tags"`
	Extra       map[string]string `json:"extra"       bson:"extra"`
	UploadedAt  time.Time         `json:"uploaded_at" bson:"uploaded_at"`
}

type Store interface {
	Insert(ctx context.Context, m Meta) error
	Get(ctx context.Context, id string) (Meta, error)
	List(ctx context.Context, limit, offset int) ([]Meta, error)
	Delete(ctx context.Context, id string) error
	Mode() string
	Close(ctx context.Context) error
}

func New(ctx context.Context, cfg config.MongoConfig, log *slog.Logger) Store {
	s, err := newMongo(ctx, cfg, log)
	if err == nil {
		return s
	}
	log.Warn("mongo unreachable, falling back to in-memory metadata store; data will NOT survive restart", "err", err)
	return newMemoryStore()
}
