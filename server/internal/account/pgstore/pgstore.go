// Package pgstore persists AI keys and performs account deletion with explicit SQL.
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/ferousco-dev/layr/server/internal/account"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Keys(ctx context.Context, userID string) ([]account.KeyStatus, error) {
	rows, err := s.pool.Query(ctx, `SELECT provider, key_hint, updated_at FROM ai_credentials WHERE user_id = $1 ORDER BY provider`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []account.KeyStatus
	for rows.Next() {
		k := account.KeyStatus{Saved: true}
		if err := rows.Scan(&k.Provider, &k.Hint, &k.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) SaveKey(ctx context.Context, userID, provider string, ciphertext []byte, hint string, now time.Time) (account.KeyStatus, error) {
	k := account.KeyStatus{Provider: provider, Saved: true}
	err := s.pool.QueryRow(ctx, `
INSERT INTO ai_credentials (user_id, provider, key_ciphertext, key_hint, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (user_id, provider) DO UPDATE
SET key_ciphertext = EXCLUDED.key_ciphertext, key_hint = EXCLUDED.key_hint, updated_at = EXCLUDED.updated_at
RETURNING key_hint, updated_at`, userID, provider, ciphertext, hint, now.UTC()).Scan(&k.Hint, &k.UpdatedAt)
	return k, err
}

func (s *Store) DeleteKey(ctx context.Context, userID, provider string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ai_credentials WHERE user_id = $1 AND provider = $2`, userID, provider)
	return tag.RowsAffected() > 0, err
}

func (s *Store) KeyCiphertext(ctx context.Context, userID, provider string) ([]byte, error) {
	var sealed []byte
	err := s.pool.QueryRow(ctx, `SELECT key_ciphertext FROM ai_credentials WHERE user_id = $1 AND provider = $2`, userID, provider).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, account.ErrKeyNotFound
	}
	return sealed, err
}

// FigmaStatus reports the connection state, or "missing" when the person has no Figma connection.
func (s *Store) FigmaStatus(ctx context.Context, userID string) (string, error) {
	var status string
	err := s.pool.QueryRow(ctx, `SELECT status FROM figma_connections WHERE user_id = $1`, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "missing", nil
	}
	return status, err
}

func (s *Store) ImportIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT i.id FROM figma_imports i JOIN projects p ON p.id = i.project_id WHERE p.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteUser removes the user row; sessions, connections, projects, imports and keys go with it through foreign keys.
func (s *Store) DeleteUser(ctx context.Context, userID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return tag.RowsAffected() > 0, err
}
