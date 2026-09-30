package pgstore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/account"
	"github.com/jackc/pgx/v5/pgxpool"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func live(t *testing.T) (*Store, *pgxpool.Pool, string, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}
	var a, b string
	for fig, dst := range map[string]*string{"acct-a": &a, "acct-b": &b} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (figma_user_id, display_name, created_at, updated_at) VALUES ($1,'n',now(),now()) RETURNING id`, fig).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	return New(pool), pool, a, b
}

func TestKeysAreStoredPerPersonAndProvider(t *testing.T) {
	s, _, a, b := live(t)
	ctx := context.Background()

	if _, err := s.SaveKey(ctx, a, account.Anthropic, []byte("sealed-1"), "1234", t0); err != nil {
		t.Fatal(err)
	}
	replaced, err := s.SaveKey(ctx, a, account.Anthropic, []byte("sealed-2"), "5678", t0.Add(time.Hour))
	if err != nil || replaced.Hint != "5678" {
		t.Fatalf("replace = %+v, %v", replaced, err)
	}
	if _, err := s.SaveKey(ctx, b, account.OpenAI, []byte("sealed-b"), "bbbb", t0); err != nil {
		t.Fatal(err)
	}

	keys, _ := s.Keys(ctx, a)
	if len(keys) != 1 || keys[0].Provider != account.Anthropic || keys[0].Hint != "5678" {
		t.Fatalf("a's keys = %+v", keys)
	}
	if ct, _ := s.KeyCiphertext(ctx, a, account.Anthropic); string(ct) != "sealed-2" {
		t.Fatalf("ciphertext = %q", ct)
	}
	if _, err := s.KeyCiphertext(ctx, a, account.OpenAI); err != account.ErrKeyNotFound {
		t.Fatalf("a must not see b's provider: %v", err)
	}
	if _, err := s.SaveKey(ctx, a, "gemini", []byte("x"), "x", t0); err == nil {
		t.Fatal("the database must refuse an unknown provider")
	}
	if ok, _ := s.DeleteKey(ctx, a, account.Anthropic); !ok {
		t.Fatal("delete reported nothing")
	}
	if ok, _ := s.DeleteKey(ctx, a, account.Anthropic); ok {
		t.Fatal("second delete should report nothing")
	}
}

func TestFigmaStatusReportsMissingActiveAndReconnect(t *testing.T) {
	s, pool, a, _ := live(t)
	ctx := context.Background()
	if st, _ := s.FigmaStatus(ctx, a); st != "missing" {
		t.Fatalf("no connection: %q", st)
	}
	_, err := pool.Exec(ctx, `INSERT INTO figma_connections (user_id, figma_user_id, access_token_ciphertext, refresh_token_ciphertext, token_expires_at, scopes, status, created_at, updated_at)
VALUES ($1, 'acct-a', '\x01', '\x01', now(), 's', 'reconnect_required', now(), now())`, a)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := s.FigmaStatus(ctx, a); st != "reconnect_required" {
		t.Fatalf("status = %q", st)
	}
}

func TestDeletingAUserRemovesEverythingTheyOwnAndNothingElse(t *testing.T) {
	s, pool, a, b := live(t)
	ctx := context.Background()
	var projA, projB, impA string
	_ = pool.QueryRow(ctx, `INSERT INTO projects (user_id, name, created_at, updated_at) VALUES ($1,'a',now(),now()) RETURNING id`, a).Scan(&projA)
	_ = pool.QueryRow(ctx, `INSERT INTO projects (user_id, name, created_at, updated_at) VALUES ($1,'b',now(),now()) RETURNING id`, b).Scan(&projB)
	if err := pool.QueryRow(ctx, `INSERT INTO figma_imports (project_id, figma_file_key, status, created_at, updated_at) VALUES ($1,'KEY123456','completed',now(),now()) RETURNING id`, projA).Scan(&impA); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, `INSERT INTO figma_imports (project_id, figma_file_key, status, created_at, updated_at) VALUES ($1,'KEY123456','completed',now(),now())`, projB)
	_, _ = pool.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at, created_at, last_seen_at) VALUES ($1,'\x0102',now()+interval '1 day',now(),now())`, a)
	_, _ = s.SaveKey(ctx, a, account.XAI, []byte("s"), "1111", t0)
	_, _ = s.SaveKey(ctx, b, account.XAI, []byte("s"), "2222", t0)

	ids, err := s.ImportIDs(ctx, a)
	if err != nil || len(ids) != 1 || ids[0] != impA {
		t.Fatalf("import ids = %v, %v", ids, err)
	}
	if ok, err := s.DeleteUser(ctx, a); err != nil || !ok {
		t.Fatalf("delete = %v, %v", ok, err)
	}
	for table, want := range map[string]int{"users": 1, "projects": 1, "figma_imports": 1, "ai_credentials": 1, "sessions": 0} {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n)
		if n != want {
			t.Errorf("%s rows = %d, want %d", table, n, want)
		}
	}
	if ok, _ := s.DeleteUser(ctx, a); ok {
		t.Fatal("deleting twice should report nothing")
	}
}
