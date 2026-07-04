package records

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS file_records (
    id          TEXT PRIMARY KEY,
    file_id     TEXT NOT NULL,
    owner       TEXT NOT NULL,
    file_name   TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    action      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_file_records_file_id ON file_records(file_id);
CREATE INDEX IF NOT EXISTS idx_file_records_created_at ON file_records(created_at DESC);
`

type pgStore struct {
	pool *pgxpool.Pool
}

func newPostgres(ctx context.Context, cfg config.PostgresConfig, log *slog.Logger) (*pgStore, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, err
	}
	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(dialCtx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(dialCtx); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, err
	}
	log.Info("postgres record store connected", "host", cfg.Host, "db", cfg.DB)
	return &pgStore{pool: pool}, nil
}

func (s *pgStore) Insert(ctx context.Context, r Record) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO file_records (id, file_id, owner, file_name, size_bytes, action, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.ID, r.FileID, r.Owner, r.FileName, r.Size, r.Action, r.CreatedAt,
	)
	return err
}

func (s *pgStore) ListByFile(ctx context.Context, fileID string) ([]Record, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, file_id, owner, file_name, size_bytes, action, created_at
		 FROM file_records WHERE file_id = $1 ORDER BY created_at DESC`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (s *pgStore) Recent(ctx context.Context, limit int) ([]Record, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, file_id, owner, file_name, size_bytes, action, created_at
		 FROM file_records ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

type pgRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanRecords(rows pgRows) ([]Record, error) {
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.FileID, &r.Owner, &r.FileName, &r.Size, &r.Action, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) Mode() string { return "live" }
func (s *pgStore) Close(_ context.Context) error {
	s.pool.Close()
	return nil
}
