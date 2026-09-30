// Package pgstore persists generation jobs with explicit SQL.
package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ferousco-dev/layr/server/internal/generation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `g.id, g.project_id, g.provider, g.screen_ids, g.status, g.steps, COALESCE(g.error_code, ''), g.created_at, g.updated_at, g.completed_at`

func scan(row pgx.Row) (generation.Generation, error) {
	var g generation.Generation
	var raw, screens []byte
	err := row.Scan(&g.ID, &g.ProjectID, &g.Provider, &screens, &g.Status, &raw, &g.ErrorCode, &g.CreatedAt, &g.UpdatedAt, &g.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return generation.Generation{}, generation.ErrNotFound
	}
	if err != nil {
		return generation.Generation{}, err
	}
	if err := json.Unmarshal(raw, &g.Steps); err != nil {
		return generation.Generation{}, err
	}
	if len(screens) > 0 {
		if err := json.Unmarshal(screens, &g.ScreenIDs); err != nil {
			return generation.Generation{}, err
		}
	}
	return g, nil
}

// Create checks ownership and inserts a running job; the partial unique index decides races.
func (s *Store) Create(ctx context.Context, ownerID, projectID, provider string, screenIDs []string, steps []generation.Step, now time.Time) (generation.Generation, error) {
	raw, err := json.Marshal(steps)
	if err != nil {
		return generation.Generation{}, err
	}
	var screens []byte
	if screenIDs != nil {
		if screens, err = json.Marshal(screenIDs); err != nil {
			return generation.Generation{}, err
		}
	}
	g, err := scan(s.pool.QueryRow(ctx, `
INSERT INTO generations (project_id, provider, screen_ids, status, steps, created_at, updated_at)
SELECT p.id, $3, $6, 'running', $4, $5, $5 FROM projects p
WHERE p.id = $1 AND p.user_id = $2 AND p.deleted_at IS NULL
RETURNING `+returningColumns, projectID, ownerID, provider, raw, now.UTC(), screens))
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return generation.Generation{}, generation.ErrConflict
	case errors.Is(err, generation.ErrNotFound):
		return generation.Generation{}, generation.ErrNotFound
	}
	return g, err
}

const returningColumns = `id, project_id, provider, screen_ids, status, steps, COALESCE(error_code, ''), created_at, updated_at, completed_at`

func (s *Store) Get(ctx context.Context, ownerID, projectID, id string) (generation.Generation, error) {
	return scan(s.pool.QueryRow(ctx, `
SELECT `+columns+` FROM generations g JOIN projects p ON p.id = g.project_id
WHERE g.id = $3 AND g.project_id = $2 AND p.user_id = $1 AND p.deleted_at IS NULL`, ownerID, projectID, id))
}

func (s *Store) SaveProgress(ctx context.Context, id string, steps []generation.Step, now time.Time) error {
	raw, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE generations SET steps = $2, updated_at = $3 WHERE id = $1 AND status = 'running'`, id, raw, now.UTC())
	return err
}

func (s *Store) Finish(ctx context.Context, id, status, errorCode string, steps []generation.Step, now time.Time) error {
	raw, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
UPDATE generations SET status = $2, error_code = NULLIF($3, ''), steps = $4, updated_at = $5, completed_at = $5
WHERE id = $1 AND status = 'running'`, id, status, errorCode, raw, now.UTC())
	return err
}

func (s *Store) FailAbandoned(ctx context.Context, before time.Time, code string, now time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
UPDATE generations SET status = 'failed', error_code = $1, updated_at = $2, completed_at = $2
WHERE status = 'running' AND updated_at < $3`, code, now.UTC(), before.UTC())
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
