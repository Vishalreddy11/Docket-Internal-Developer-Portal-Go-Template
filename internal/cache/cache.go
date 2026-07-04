// Package cache is the hot/ephemeral key-value layer (Redis in production).
// Used here for view counters and to demonstrate a rate-limiter pattern. When
// Redis is unreachable, an in-memory map is used.
package cache

import (
	"context"
	"log/slog"

	"github.com/example/docket/internal/config"
)

type Cache interface {
	IncrView(ctx context.Context, fileID string) (int64, error)
	GetViews(ctx context.Context, fileID string) (int64, error)
	Mode() string
	Close(ctx context.Context) error
}

func New(ctx context.Context, cfg config.RedisConfig, log *slog.Logger) Cache {
	c, err := newRedis(ctx, cfg, log)
	if err == nil {
		return c
	}
	log.Warn("redis unreachable, falling back to in-memory cache; counters will NOT survive restart", "err", err)
	return newMemoryCache()
}
