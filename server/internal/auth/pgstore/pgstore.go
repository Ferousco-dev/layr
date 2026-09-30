// Package pgstore persists users, Figma connections and sessions with explicit SQL.
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const upsertUser = `
INSERT INTO users (figma_user_id, email, display_name, avatar_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (figma_user_id) DO UPDATE
SET email = EXCLUDED.email, display_name = EXCLUDED.display_name,
    avatar_url = EXCLUDED.avatar_url, updated_at = EXCLUDED.updated_at
RETURNING id, figma_user_id, COALESCE(email, ''), display_name, COALESCE(avatar_url, '')`

const upsertConnection = `
INSERT INTO figma_connections
  (user_id, figma_user_id, access_token_ciphertext, refresh_token_ciphertext,
   token_expires_at, scopes, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $7)
ON CONFLICT (user_id) DO UPDATE
SET access_token_ciphertext = EXCLUDED.access_token_ciphertext,
    refresh_token_ciphertext = EXCLUDED.refresh_token_ciphertext,
    token_expires_at = EXCLUDED.token_expires_at, scopes = EXCLUDED.scopes,
    status = 'active', updated_at = EXCLUDED.updated_at`

// UpsertLogin writes the user and connection in one short transaction, after all network calls are done.
func (s *Store) UpsertLogin(ctx context.Context, l auth.Login) (auth.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return auth.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var u auth.User
	err = tx.QueryRow(ctx, upsertUser,
		l.Identity.ID, nullable(l.Identity.Email), l.Identity.DisplayName, nullable(l.Identity.AvatarURL), l.At.UTC(),
	).Scan(&u.ID, &u.FigmaUserID, &u.Email, &u.DisplayName, &u.AvatarURL)
	if err != nil {
		return auth.User{}, err
	}

	_, err = tx.Exec(ctx, upsertConnection,
		u.ID, u.FigmaUserID, l.AccessCT, l.RefreshCT, l.ExpiresAt.UTC(), l.Scopes, l.At.UTC(),
	)
	if err != nil {
		return auth.User{}, err
	}
	return u, tx.Commit(ctx)
}

func (s *Store) CreateSession(ctx context.Context, userID string, hash []byte, expires, now time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at, created_at, last_seen_at) VALUES ($1, $2, $3, $4, $4)`,
		userID, hash, expires.UTC(), now.UTC())
	return err
}

func (s *Store) SessionByHash(ctx context.Context, hash []byte) (auth.SessionRecord, error) {
	var r auth.SessionRecord
	err := s.pool.QueryRow(ctx, `
SELECT s.id, s.expires_at, s.last_seen_at, s.revoked_at,
       u.id, u.figma_user_id, COALESCE(u.email, ''), u.display_name, COALESCE(u.avatar_url, '')
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1`, hash).Scan(
		&r.ID, &r.ExpiresAt, &r.LastSeenAt, &r.RevokedAt,
		&r.User.ID, &r.User.FigmaUserID, &r.User.Email, &r.User.DisplayName, &r.User.AvatarURL)
	return r, notFound(err)
}

func (s *Store) TouchSession(ctx context.Context, id string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET last_seen_at = $2 WHERE id = $1`, id, now.UTC())
	return err
}

func (s *Store) RevokeSession(ctx context.Context, hash []byte, now time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL`, hash, now.UTC())
	return err
}

func (s *Store) Connection(ctx context.Context, userID string) (auth.Connection, error) {
	var c auth.Connection
	err := s.pool.QueryRow(ctx, `
SELECT id, user_id, access_token_ciphertext, refresh_token_ciphertext, token_expires_at, status
FROM figma_connections WHERE user_id = $1`, userID).Scan(
		&c.ID, &c.UserID, &c.AccessCT, &c.RefreshCT, &c.ExpiresAt, &c.Status)
	return c, notFound(err)
}

// SaveTokens keeps the stored refresh token when refreshCT is nil.
func (s *Store) SaveTokens(ctx context.Context, connID string, accessCT, refreshCT []byte, expires, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
UPDATE figma_connections
SET access_token_ciphertext = $2,
    refresh_token_ciphertext = COALESCE($3, refresh_token_ciphertext),
    token_expires_at = $4, updated_at = $5
WHERE id = $1`, connID, accessCT, refreshCT, expires.UTC(), now.UTC())
	return err
}

func (s *Store) MarkReconnectRequired(ctx context.Context, connID string, now time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE figma_connections SET status = 'reconnect_required', updated_at = $2 WHERE id = $1`, connID, now.UTC())
	return err
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrNotFound
	}
	return err
}
