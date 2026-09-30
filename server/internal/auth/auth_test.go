package auth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

func login(t *testing.T, h *harness) Session {
	t.Helper()
	url, err := h.svc.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := url[strings.Index(url, "state=")+6 : strings.Index(url, "&")]
	s, err := h.svc.Complete(context.Background(), "code", state)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBeginReturnsFigmaURLWithFreshState(t *testing.T) {
	h := newHarness()

	a, _ := h.svc.Begin(context.Background())
	b, _ := h.svc.Begin(context.Background())

	if !strings.HasPrefix(a, "https://figma.test/oauth?state=") || a == b {
		t.Fatalf("a=%q b=%q", a, b)
	}
}

func TestCompleteCreatesUserSessionAndEncryptsCredentials(t *testing.T) {
	h := newHarness()

	s := login(t, h)

	if len(s.Token) != tokenLength || !s.ExpiresAt.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("session = %#v", s)
	}
	if bytes.Contains(h.store.conn.AccessCT, []byte("acc")) && !bytes.HasPrefix(h.store.conn.AccessCT, []byte(accessPurpose)) {
		t.Fatal("credentials not sealed")
	}
	for _, hash := range h.store.hashes {
		if bytes.Contains(hash, []byte(s.Token)) || string(hash) == s.Token {
			t.Fatal("raw session token stored")
		}
	}
	if len(h.store.hashes) != 1 || len(h.store.hashes[0]) != 32 {
		t.Fatal("expected one SHA-256 hash")
	}
}

func TestCompleteRejectsReplayedAndUnknownState(t *testing.T) {
	h := newHarness()
	url, _ := h.svc.Begin(context.Background())
	state := url[strings.Index(url, "state=")+6 : strings.Index(url, "&")]

	if _, err := h.svc.Complete(context.Background(), "code", state); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Complete(context.Background(), "code", state); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("replay err = %v", err)
	}
	if _, err := h.svc.Complete(context.Background(), "code", "nope"); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("unknown err = %v", err)
	}
}

func TestCompleteMapsFigmaFailures(t *testing.T) {
	cases := map[string]struct {
		mutate func(*fakeFigma)
		want   error
	}{
		"exchange rejected":   {func(f *fakeFigma) { f.exchangeErr = figma.ErrRejected }, ErrFigmaAuthFailed},
		"exchange outage":     {func(f *fakeFigma) { f.exchangeErr = figma.ErrUnavailable }, ErrDependency},
		"identity rejected":   {func(f *fakeFigma) { f.meErr = figma.ErrRejected }, ErrIdentityFailed},
		"identity malformed":  {func(f *fakeFigma) { f.meErr = figma.ErrMalformed }, ErrIdentityFailed},
		"identity user split": {func(f *fakeFigma) { f.identity.ID = "someone-else" }, ErrIdentityFailed},
	}
	for name, tc := range cases {
		h := newHarness()
		tc.mutate(h.figma)
		url, _ := h.svc.Begin(context.Background())
		state := url[strings.Index(url, "state=")+6 : strings.Index(url, "&")]

		_, err := h.svc.Complete(context.Background(), "code", state)

		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
		if len(h.store.hashes) != 0 {
			t.Fatalf("%s: session created on failure", name)
		}
	}
}

func TestRepeatedLoginKeepsSameUserWhenEmailChanges(t *testing.T) {
	h := newHarness()
	first := login(t, h)
	h.figma.identity.Email = "changed@b.c"
	second := login(t, h)

	u1, _ := h.svc.Authenticate(context.Background(), first.Token)
	u2, _ := h.svc.Authenticate(context.Background(), second.Token)

	if u1.ID != u2.ID || u2.Email != "changed@b.c" || len(h.store.users) != 1 {
		t.Fatalf("u1=%#v u2=%#v users=%d", u1, u2, len(h.store.users))
	}
}

func TestAuthenticate(t *testing.T) {
	h := newHarness()
	s := login(t, h)
	ctx := context.Background()

	if u, err := h.svc.Authenticate(ctx, s.Token); err != nil || u.FigmaUserID != "fig-1" {
		t.Fatalf("valid: %#v %v", u, err)
	}
	for name, token := range map[string]string{"empty": "", "short": "abc", "not base64": strings.Repeat("!", tokenLength), "unknown": strings.Repeat("A", tokenLength)} {
		if _, err := h.svc.Authenticate(ctx, token); !errors.Is(err, ErrSessionInvalid) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}

	*h.clock = t0.Add(25 * time.Hour)
	if _, err := h.svc.Authenticate(ctx, s.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expired: %v", err)
	}
}

func TestLogoutRevokesAndIsIdempotent(t *testing.T) {
	h := newHarness()
	s := login(t, h)
	ctx := context.Background()

	if err := h.svc.Logout(ctx, s.Token); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Logout(ctx, s.Token); err != nil {
		t.Fatalf("second logout: %v", err)
	}
	if err := h.svc.Logout(ctx, ""); err != nil {
		t.Fatalf("empty logout: %v", err)
	}
	if _, err := h.svc.Authenticate(ctx, s.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("revoked err = %v", err)
	}
}

func TestAuthenticateTouchesStaleSessionsOnly(t *testing.T) {
	h := newHarness()
	s := login(t, h)
	ctx := context.Background()

	*h.clock = t0.Add(time.Minute)
	_, _ = h.svc.Authenticate(ctx, s.Token)
	if rec, _ := h.store.SessionByHash(ctx, hashToken(s.Token)); !rec.LastSeenAt.Equal(t0) {
		t.Fatal("touched too eagerly")
	}

	*h.clock = t0.Add(10 * time.Minute)
	_, _ = h.svc.Authenticate(ctx, s.Token)
	if rec, _ := h.store.SessionByHash(ctx, hashToken(s.Token)); !rec.LastSeenAt.Equal(t0.Add(10 * time.Minute)) {
		t.Fatal("stale session not touched")
	}
}
