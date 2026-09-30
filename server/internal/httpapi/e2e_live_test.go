package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/auth/pgstore"
	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/ferousco-dev/layr/server/internal/credential"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/oauthstate"
	"github.com/ferousco-dev/layr/server/internal/observability"
	red "github.com/ferousco-dev/layr/server/internal/redis"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestLiveLoginFlow drives the full flow against real PostgreSQL and Redis with a fake Figma server.
func TestLiveLoginFlow(t *testing.T) {
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, _ = pool.Exec(ctx, `TRUNCATE users CASCADE`)
	rd, err := red.Open(ctx, config.Redis{URL: config.SecretURL(redisURL), PoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"user_id_string":"fig-e2e","access_token":"plain-access-token","refresh_token":"plain-refresh-token","expires_in":3600}`))
		case "/me":
			_, _ = w.Write([]byte(`{"id":"fig-e2e","email":"e2e@example.com","handle":"E2E","img_url":"https://img"}`))
		}
	}))
	defer fake.Close()

	sealer, _ := credential.New(bytes.Repeat([]byte{3}, 32))
	svc := auth.New(auth.Deps{
		Store:      pgstore.New(pool),
		Figma:      figma.New(figma.Config{ClientID: "c", ClientSecret: "s", RedirectURI: "http://localhost/cb", TokenURL: fake.URL + "/token", MeURL: fake.URL + "/me"}),
		States:     oauthstate.New(rd, 10*time.Minute),
		Sealer:     sealer,
		Locker:     rd,
		SessionTTL: time.Hour,
		Log:        slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	})
	h := NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 1024,
		Auth: &AuthOptions{Service: svc, Limiter: &fakeLimiter{}, CookieName: "layr_session", CookieSecure: true},
	})

	start := do(h, "GET", "/auth/figma", nil)
	loc, _ := url.Parse(start.Header().Get("Location"))
	state := loc.Query().Get("state")
	if start.Code != http.StatusFound || state == "" || loc.Query().Get("code_challenge") == "" {
		t.Fatalf("start: %d %q", start.Code, start.Header().Get("Location"))
	}

	callback := do(h, "GET", "/auth/figma/callback?code=abc&state="+state, nil)
	cookies := callback.Result().Cookies()
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != frontend+"/" || len(cookies) != 1 {
		t.Fatalf("callback: %d %q", callback.Code, callback.Header().Get("Location"))
	}
	raw := cookies[0].Value

	replay := do(h, "GET", "/auth/figma/callback?code=abc&state="+state, nil)
	if !strings.Contains(replay.Header().Get("Location"), "OAUTH_STATE_INVALID") {
		t.Fatalf("replay not rejected: %q", replay.Header().Get("Location"))
	}

	me := do(h, "GET", "/api/v1/me", withCookie(raw))
	if me.Code != 200 || !strings.Contains(me.Body.String(), "e2e@example.com") {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}

	var storedSessions, plaintextTokens int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE encode(token_hash,'escape') LIKE '%'||$1||'%'`, raw).Scan(&storedSessions)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM figma_connections WHERE position('plain-access-token'::bytea in access_token_ciphertext) > 0 OR position('plain-refresh-token'::bytea in refresh_token_ciphertext) > 0`).Scan(&plaintextTokens)
	if storedSessions != 0 || plaintextTokens != 0 {
		t.Fatalf("raw session rows %d, plaintext token rows %d", storedSessions, plaintextTokens)
	}

	if out := do(h, "POST", "/auth/logout", withCookie(raw)); out.Code != 204 {
		t.Fatalf("logout: %d", out.Code)
	}
	if after := do(h, "GET", "/api/v1/me", withCookie(raw)); after.Code != 401 {
		t.Fatalf("me after logout: %d", after.Code)
	}
}
