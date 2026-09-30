// Package pgstore holds the ownership-scoped SQL for projects.
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/ferousco-dev/layr/server/internal/project"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `id, user_id, name, created_at, updated_at`

func (s *Store) Create(ctx context.Context, ownerID, name string, now time.Time) (project.Project, error) {
	return scan(s.pool.QueryRow(ctx,
		`INSERT INTO projects (user_id, name, created_at, updated_at) VALUES ($1, $2, $3, $3) RETURNING `+columns,
		ownerID, name, now.UTC()))
}

func (s *Store) Get(ctx context.Context, ownerID, id string) (project.Project, error) {
	return scan(s.pool.QueryRow(ctx,
		`SELECT `+columns+` FROM projects WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, ownerID))
}

// List pages by (updated_at, id) descending; a nil cursor starts from the newest project.
func (s *Store) List(ctx context.Context, ownerID string, after *project.Cursor, limit int) ([]project.Project, error) {
	var ts, id any
	if after != nil {
		ts, id = after.UpdatedAt.UTC(), after.ID
	}
	rows, err := s.pool.Query(ctx, `
SELECT `+columns+` FROM projects
WHERE user_id = $1 AND deleted_at IS NULL AND ($2::timestamptz IS NULL OR (updated_at, id) < ($2::timestamptz, $3::uuid))
ORDER BY updated_at DESC, id DESC
LIMIT $4`, ownerID, ts, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []project.Project
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Rename(ctx context.Context, ownerID, id, name string, now time.Time) (project.Project, error) {
	return scan(s.pool.QueryRow(ctx,
		`UPDATE projects SET name = $3, updated_at = $4 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL RETURNING `+columns,
		id, ownerID, name, now.UTC()))
}

func (s *Store) Delete(ctx context.Context, ownerID, id string, now time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE projects SET deleted_at = $3 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, ownerID, now.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return project.ErrNotFound
	}
	return nil
}

// Restore leaves updated_at alone: undoing a delete is not an edit, so the project returns to its old place in the list.
func (s *Store) Restore(ctx context.Context, ownerID, id string) (project.Project, error) {
	return scan(s.pool.QueryRow(ctx,
		`UPDATE projects SET deleted_at = NULL WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL RETURNING `+columns,
		id, ownerID))
}

// PurgeExpired permanently removes projects deleted before the cutoff.
func (s *Store) PurgeExpired(ctx context.Context, before time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE deleted_at IS NOT NULL AND deleted_at < $1`, before.UTC())
	return err
}

func scan(row pgx.Row) (project.Project, error) {
	var p project.Project
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return project.Project{}, project.ErrNotFound
	}
	p.CreatedAt, p.UpdatedAt = p.CreatedAt.UTC(), p.UpdatedAt.UTC()
	return p, err
}
