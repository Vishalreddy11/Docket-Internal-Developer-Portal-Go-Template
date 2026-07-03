// Package events is the asynchronous event bus (NATS JetStream in production).
// Uploads publish "file.uploaded" events under the docket.files.* subject tree
// so downstream consumers (thumbnailer, indexer, virus scanner — not
// implemented here) can react without blocking the request. When NATS is
// unreachable, an in-process channel bus is used and there are no consumers.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
)

type Event struct {
	Type      string         `json:"type"`
	FileID    string         `json:"file_id"`
	Owner     string         `json:"owner"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

func (e Event) Marshal() ([]byte, error) { return json.Marshal(e) }

type Bus interface {
	Publish(ctx context.Context, ev Event) error
	Mode() string
	Close(ctx context.Context) error
}

func New(ctx context.Context, cfg config.NATSConfig, log *slog.Logger) Bus {
	b, err := newNATS(ctx, cfg, log)
	if err == nil {
		return b
	}
	log.Warn("nats unreachable, falling back to in-memory channel bus; events will NOT survive restart and have no consumers", "err", err)
	return newMemoryBus(log)
}
