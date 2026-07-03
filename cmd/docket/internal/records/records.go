// Package records is the relational audit log (who did what, when). It is
// backed by Postgres because we want ACID guarantees and SQL queries — this is
// the right home for anything you'd put in a "transactions" or "audit" table.
// When Postgres is unreachable an in-memory store is used instead.
package records

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
)

type Record struct {
	ID        string    `json:"id"`
	FileID    string    `json:"file_id"`
	Owner     string    `json:"owner"`
	FileName  string    `json:"file_name"`
	Size      int64     `json:"size"`
	Action    string    `json:"action"` // upload | view | delete
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	Insert(ctx context.Context, r Record) error
	ListByFile(ctx context.Context, fileID string) ([]Record, error)
	Recent(ctx context.Context, limit int) ([]Record, error)
	Mode() string
	Close(ctx context.Context) error
}

func New(ctx context.Context, cfg config.PostgresConfig, log *slog.Logger) Store {
	s, err := newPostgres(ctx, cfg, log)
	if err == nil {
		return s
	}
	log.Warn("postgres unreachable, falling back to in-memory record store; audit log will NOT survive restart", "err", err)
	return newMemoryStore()
}
