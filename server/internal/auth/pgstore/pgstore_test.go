package pgstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/jackc/pgx/v5/pgxpool"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func liveStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}
	return New(pool), pool
}

func login(figmaID, email string) auth.Login {
	return auth.Login{
		Identity:  figma.Identity{ID: figmaID, Email: email, DisplayName: "Ada", AvatarURL: "https://img"},
		AccessCT:  []byte("access-ct"),
		RefreshCT: []byte("refresh-ct"),
		ExpiresAt: now.Add(time.Hour),
		Scopes:    "current_user:read file_content:read",
		At:        now,
	}
}

func TestRepeatedLoginReusesUserAndUpdatesProfile(t *testing.T) {
	s, pool := liveStore(t)
	ctx := context.Background()

	first, err := s.UpsertLogin(ctx, login("fig-1", "old@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.UpsertLogin(ctx, login("fig-1", "new@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	var users, conns int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM figma_connections`).Scan(&conns)
	if first.ID != second.ID || second.Email != "new@example.com" || users != 1 || conns != 1 {
		t.Fatalf("ids %s/%s email %q users %d conns %d", first.ID, second.ID, second.Email, users, conns)
	}
}

func TestConcurrentFirstLoginCreatesOneUser(t *testing.T) {
	s, pool := liveStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.UpsertLogin(ctx, login("fig-race", "a@example.com")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	var users int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users)
	if users != 1 {
		t.Fatalf("users = %d, want 1", users)
	}
}

func TestSessionLifecycleStoresOnlyHash(t *testing.T) {
	s, pool := liveStore(t)
	ctx := context.Background()
	u, _ := s.UpsertLogin(ctx, login("fig-2", ""))
	hash := bytes.Repeat([]byte{7}, 32)

	if err := s.CreateSession(ctx, u.ID, hash, now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	rec, err := s.SessionByHash(ctx, hash)
	if err != nil || rec.User.ID != u.ID || rec.RevokedAt != nil {
		t.Fatalf("rec = %#v, err = %v", rec, err)
	}

	if err := s.RevokeSession(ctx, hash, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeSession(ctx, hash, now.Add(time.Minute)); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
	rec, _ = s.SessionByHash(ctx, hash)
	if rec.RevokedAt == nil || !rec.RevokedAt.Equal(now) {
		t.Fatalf("revoked_at = %v", rec.RevokedAt)
	}

	if _, err := s.SessionByHash(ctx, []byte("unknown")); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("unknown err = %v", err)
	}
	var columns int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='sessions' AND column_name IN ('token','raw_token')`).Scan(&columns)
	if columns != 0 {
		t.Fatal("sessions table must not hold a raw token column")
	}
}

func TestSaveTokensKeepsRefreshWhenOmitted(t *testing.T) {
	s, _ := liveStore(t)
	ctx := context.Background()
	u, _ := s.UpsertLogin(ctx, login("fig-3", ""))
	conn, _ := s.Connection(ctx, u.ID)

	if err := s.SaveTokens(ctx, conn.ID, []byte("new-access"), nil, now.Add(2*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Connection(ctx, u.ID)
	if string(got.AccessCT) != "new-access" || string(got.RefreshCT) != "refresh-ct" || !got.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("got = %#v", got)
	}

	if err := s.MarkReconnectRequired(ctx, conn.ID, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Connection(ctx, u.ID)
	if got.Status != auth.StatusReconnectNeed {
		t.Fatalf("status = %q", got.Status)
	}
}
