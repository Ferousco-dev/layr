package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/observability"
)

const frontend = "https://app.layr.dev"

type fakeAuth struct {
	beginErr, completeErr, logoutErr error
	session                          auth.Session
	user                             auth.User
	authErr                          error
	loggedOut                        []string
	consumed                         []string
}

func (f *fakeAuth) Begin(context.Context) (string, error) {
	return "https://www.figma.com/oauth?state=abc", f.beginErr
}

func (f *fakeAuth) Consume(_ context.Context, state string) error {
	f.consumed = append(f.consumed, state)
	return nil
}

func (f *fakeAuth) Complete(context.Context, string, string) (auth.Session, error) {
	return f.session, f.completeErr
}

func (f *fakeAuth) Authenticate(_ context.Context, token string) (auth.User, error) {
	if token != "good-token" {
		return auth.User{}, auth.ErrSessionInvalid
	}
	return f.user, f.authErr
}

func (f *fakeAuth) Logout(_ context.Context, token string) error {
	f.loggedOut = append(f.loggedOut, token)
	return f.logoutErr
}

// fakeLimiter counts per key, starting each key at n.
type fakeLimiter struct {
	mu     sync.Mutex
	n      int64
	counts map[string]int64
}

func (l *fakeLimiter) Count(_ context.Context, key string, _ time.Duration) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts == nil {
		l.counts = map[string]int64{}
	}
	if _, seen := l.counts[key]; !seen {
		l.counts[key] = l.n
	}
	l.counts[key]++
	return l.counts[key], nil
}

func authHandler(f *fakeAuth, l *fakeLimiter) (http.Handler, *bytes.Buffer) {
	var logs bytes.Buffer
	h := NewHandler(fakeHealth{}, observability.New(&logs, slog.LevelInfo), Options{
		FrontendOrigin: frontend,
		MaxBodyBytes:   1024,
		Auth:           &AuthOptions{Service: f, Limiter: l, CookieName: "layr_session", CookieSecure: true},
	})
	return h, &logs
}

func do(h http.Handler, method, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if mutate != nil {
		mutate(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func withCookie(v string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "layr_session", Value: v}) }
}

func TestStartRedirectsToFigma(t *testing.T) {
	h, _ := authHandler(&fakeAuth{}, &fakeLimiter{})

	w := do(h, "GET", "/auth/figma", nil)

	if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "https://www.figma.com/oauth") {
		t.Fatalf("status %d location %q", w.Code, w.Header().Get("Location"))
	}
}

func TestStartReportsDependencyFailure(t *testing.T) {
	h, _ := authHandler(&fakeAuth{beginErr: auth.ErrDependency}, &fakeLimiter{})

	w := do(h, "GET", "/auth/figma", nil)

	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestCallbackSuccessSetsSecureCookieAndRedirectsHome(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	h, logs := authHandler(&fakeAuth{session: auth.Session{Token: "raw-session-value", ExpiresAt: expires}}, &fakeLimiter{})

	w := do(h, "GET", "/auth/figma/callback?code=secret-code&state=secret-state", nil)

	cookie := w.Result().Cookies()[0]
	if w.Code != http.StatusFound || w.Header().Get("Location") != frontend+"/" {
		t.Fatalf("status %d location %q", w.Code, w.Header().Get("Location"))
	}
	if cookie.Name != "layr_session" || cookie.Value != "raw-session-value" || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("cookie = %#v", cookie)
	}
	if strings.Contains(logs.String(), "secret-code") || strings.Contains(logs.String(), "secret-state") || strings.Contains(logs.String(), "raw-session-value") {
		t.Fatalf("credentials leaked into logs: %s", logs.String())
	}
}

func TestCallbackFailuresRedirectWithStableCodeOnly(t *testing.T) {
	cases := map[string]struct {
		query string
		err   error
		code  string
	}{
		"missing code":    {"state=s", nil, "OAUTH_CALLBACK_FAILED"},
		"missing state":   {"code=c", nil, "OAUTH_CALLBACK_FAILED"},
		"invalid state":   {"code=c&state=s", auth.ErrStateInvalid, "OAUTH_STATE_INVALID"},
		"figma denial":    {"error=access_denied&state=s", nil, "FIGMA_AUTH_FAILED"},
		"exchange failed": {"code=c&state=s", auth.ErrFigmaAuthFailed, "FIGMA_AUTH_FAILED"},
		"identity failed": {"code=c&state=s", auth.ErrIdentityFailed, "FIGMA_IDENTITY_FAILED"},
		"dependency":      {"code=c&state=s", auth.ErrDependency, "DEPENDENCY_UNAVAILABLE"},
		"unexpected":      {"code=c&state=s", errors.New("boom secret"), "OAUTH_CALLBACK_FAILED"},
	}
	for name, tc := range cases {
		f := &fakeAuth{completeErr: tc.err}
		h, _ := authHandler(f, &fakeLimiter{})

		w := do(h, "GET", "/auth/figma/callback?"+tc.query, nil)

		want := frontend + "/?auth_error=" + tc.code
		if w.Code != http.StatusFound || w.Header().Get("Location") != want {
			t.Fatalf("%s: status %d location %q, want %q", name, w.Code, w.Header().Get("Location"), want)
		}
		if len(w.Result().Cookies()) != 0 {
			t.Fatalf("%s: cookie set on failure", name)
		}
	}
}

func TestCallbackDenialBurnsState(t *testing.T) {
	f := &fakeAuth{}
	h, _ := authHandler(f, &fakeLimiter{})

	do(h, "GET", "/auth/figma/callback?error=access_denied&state=abc", nil)

	if len(f.consumed) != 1 || f.consumed[0] != "abc" {
		t.Fatalf("consumed = %v", f.consumed)
	}
}

func TestRedirectTargetIgnoresAttackerInput(t *testing.T) {
	h, _ := authHandler(&fakeAuth{session: auth.Session{Token: "t", ExpiresAt: time.Now().Add(time.Hour)}}, &fakeLimiter{})

	w := do(h, "GET", "/auth/figma/callback?code=c&state=s&redirect=https://evil.example&next=//evil.example", nil)

	if w.Header().Get("Location") != frontend+"/" {
		t.Fatalf("location = %q", w.Header().Get("Location"))
	}
}

func TestMeRequiresValidSession(t *testing.T) {
	f := &fakeAuth{user: auth.User{ID: "u1", Email: "a@b.c", DisplayName: "Ada"}}
	h, _ := authHandler(f, &fakeLimiter{})

	missing := do(h, "GET", "/api/v1/me", nil)
	bad := do(h, "GET", "/api/v1/me", withCookie("bad"))
	good := do(h, "GET", "/api/v1/me", withCookie("good-token"))

	if missing.Code != 401 || !strings.Contains(missing.Body.String(), "AUTH_REQUIRED") {
		t.Fatalf("missing: %d %s", missing.Code, missing.Body.String())
	}
	if bad.Code != 401 || !strings.Contains(bad.Body.String(), "INVALID_SESSION") || bad.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("bad: %d %s", bad.Code, bad.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(good.Body.Bytes(), &body); err != nil || good.Code != 200 {
		t.Fatalf("good: %d %s", good.Code, good.Body.String())
	}
	if body.Data["id"] != "u1" || body.Data["name"] != "Ada" || body.Data["avatar_url"] != nil {
		t.Fatalf("data = %v", body.Data)
	}
	for _, forbidden := range []string{"token", "hash", "figma"} {
		if strings.Contains(strings.ToLower(good.Body.String()), forbidden) {
			t.Fatalf("response exposes %q: %s", forbidden, good.Body.String())
		}
	}
}

func TestMeReportsDependencyOutageAsUnavailable(t *testing.T) {
	h, _ := authHandler(&fakeAuth{authErr: auth.ErrDependency, user: auth.User{ID: "u"}}, &fakeLimiter{})

	w := do(h, "GET", "/api/v1/me", withCookie("good-token"))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestLogoutRevokesAndExpiresCookie(t *testing.T) {
	f := &fakeAuth{}
	h, _ := authHandler(f, &fakeLimiter{})

	first := do(h, "POST", "/auth/logout", withCookie("good-token"))
	second := do(h, "POST", "/auth/logout", nil)

	if first.Code != http.StatusNoContent || len(f.loggedOut) != 1 || f.loggedOut[0] != "good-token" {
		t.Fatalf("status %d loggedOut %v", first.Code, f.loggedOut)
	}
	if c := first.Result().Cookies()[0]; c.MaxAge != -1 || c.Value != "" {
		t.Fatalf("cookie not expired: %#v", c)
	}
	if second.Code != http.StatusNoContent {
		t.Fatalf("second logout status = %d", second.Code)
	}
}

func TestLogoutRejectsCrossSiteRequests(t *testing.T) {
	f := &fakeAuth{}
	h, _ := authHandler(f, &fakeLimiter{})

	evil := do(h, "POST", "/auth/logout", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	fetch := do(h, "POST", "/auth/logout", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") })
	own := do(h, "POST", "/auth/logout", func(r *http.Request) { r.Header.Set("Origin", frontend) })

	if evil.Code != 403 || fetch.Code != 403 || own.Code != 204 || len(f.loggedOut) != 0 {
		t.Fatalf("evil %d fetch %d own %d", evil.Code, fetch.Code, own.Code)
	}
}

func TestAuthEndpointsAreRateLimited(t *testing.T) {
	h, _ := authHandler(&fakeAuth{}, &fakeLimiter{n: rateLimit})

	w := do(h, "GET", "/auth/figma", nil)

	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("status %d", w.Code)
	}
}
