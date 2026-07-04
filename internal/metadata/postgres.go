package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS file_metadata (
    id           TEXT PRIMARY KEY,
    uploaded_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    doc          JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_file_metadata_uploaded_at ON file_metadata(uploaded_at DESC);
CREATE INDEX IF NOT EXISTS idx_file_metadata_doc_gin     ON file_metadata USING GIN (doc);
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
	log.Info("postgres metadata store connected", "host", cfg.Host, "db", cfg.DB)
	return &pgStore{pool: pool}, nil
}

func (s *pgStore) Insert(ctx context.Context, m Meta) error {
	doc, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO file_metadata (id, uploaded_at, doc) VALUES ($1, $2, $3)`,
		m.ID, m.UploadedAt, doc,
	)
	return err
}

func (s *pgStore) Get(ctx context.Context, id string) (Meta, error) {
	var doc []byte
	err := s.pool.QueryRow(ctx, `SELECT doc FROM file_metadata WHERE id = $1`, id).Scan(&doc)
	if errors.Is(err, pgx.ErrNoRows) {
		return Meta{}, ErrNotFound
	}
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(doc, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func (s *pgStore) List(ctx context.Context, limit, offset int) ([]Meta, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT doc FROM file_metadata ORDER BY uploaded_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Meta
	for rows.Next() {
		var doc []byte
		if err := rows.Scan(&doc); err != nil {
			return nil, err
		}
		var m Meta
		if err := json.Unmarshal(doc, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *pgStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM file_metadata WHERE id = $1`, id)
	return err
}

func (s *pgStore) Mode() string { return "live" }
func (s *pgStore) Close(_ context.Context) error {
	s.pool.Close()
	return nil
}

var ErrNotFound = errors.New("metadata: not found")
