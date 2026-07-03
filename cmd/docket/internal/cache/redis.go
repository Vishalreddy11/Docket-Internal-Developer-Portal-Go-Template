package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

type redisCache struct {
	client *redis.Client
}

func newRedis(ctx context.Context, cfg config.RedisConfig, log *slog.Logger) (*redisCache, error) {
	c := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := redisotel.InstrumentTracing(c); err != nil {
		log.Warn("redis otel instrumentation failed", "err", err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := c.Ping(probeCtx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	log.Info("redis cache connected", "addr", cfg.Addr, "db", cfg.DB)
	return &redisCache{client: c}, nil
}

func viewKey(id string) string { return "docket:views:" + id }

func (r *redisCache) IncrView(ctx context.Context, id string) (int64, error) {
	return r.client.Incr(ctx, viewKey(id)).Result()
}

func (r *redisCache) GetViews(ctx context.Context, id string) (int64, error) {
	n, err := r.client.Get(ctx, viewKey(id)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return n, err
}

func (r *redisCache) Mode() string                  { return "live" }
func (r *redisCache) Close(_ context.Context) error { return r.client.Close() }
