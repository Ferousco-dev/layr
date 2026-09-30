package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

func seeded(t *testing.T, expiresIn time.Duration) *harness {
	t.Helper()
	h := newHarness()
	h.figma.tokens.ExpiresAt = t0.Add(expiresIn)
	login(t, h)
	return h
}

func TestAccessTokenSkipsRefreshWhenFresh(t *testing.T) {
	h := seeded(t, time.Hour)

	tok, err := h.svc.AccessToken(context.Background(), "user-fig-1")

	if err != nil || tok != "acc" || h.figma.refreshCalls.Load() != 0 {
		t.Fatalf("tok=%q err=%v calls=%d", tok, err, h.figma.refreshCalls.Load())
	}
}

func TestAccessTokenRefreshesInsideSkewWindow(t *testing.T) {
	h := seeded(t, time.Minute)
	h.figma.refreshTokens = figma.Tokens{Access: "acc2", ExpiresAt: t0.Add(time.Hour)}

	tok, err := h.svc.AccessToken(context.Background(), "user-fig-1")

	if err != nil || tok != "acc2" || h.figma.refreshCalls.Load() != 1 {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	refresh, _ := h.svc.Sealer.Open(h.store.conn.RefreshCT, refreshPurpose)
	if string(refresh) != "ref" {
		t.Fatalf("refresh token overwritten with %q", refresh)
	}
}

func TestAccessTokenPersistsRotatedRefreshToken(t *testing.T) {
	h := seeded(t, time.Minute)
	h.figma.refreshTokens = figma.Tokens{Access: "acc2", Refresh: "ref2", ExpiresAt: t0.Add(time.Hour)}

	_, err := h.svc.AccessToken(context.Background(), "user-fig-1")

	refresh, _ := h.svc.Sealer.Open(h.store.conn.RefreshCT, refreshPurpose)
	if err != nil || string(refresh) != "ref2" {
		t.Fatalf("err=%v refresh=%q", err, refresh)
	}
}

func TestConcurrentCallersRefreshOnce(t *testing.T) {
	h := seeded(t, time.Minute)
	h.figma.refreshTokens = figma.Tokens{Access: "acc2", ExpiresAt: t0.Add(time.Hour)}
	h.figma.refreshDelay = 50 * time.Millisecond

	var wg sync.WaitGroup
	results := make([]string, 6)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok, err := h.svc.AccessToken(context.Background(), "user-fig-1")
			if err != nil {
				t.Error(err)
			}
			results[i] = tok
		}(i)
	}
	wg.Wait()

	if h.figma.refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", h.figma.refreshCalls.Load())
	}
	for _, tok := range results {
		if tok != "acc2" {
			t.Fatalf("results = %v", results)
		}
	}
}

func TestRevokedRefreshRequiresReconnect(t *testing.T) {
	h := seeded(t, time.Minute)
	h.figma.refreshErr = figma.ErrRejected

	_, err := h.svc.AccessToken(context.Background(), "user-fig-1")
	_, again := h.svc.AccessToken(context.Background(), "user-fig-1")

	if !errors.Is(err, ErrReconnectNeeded) || !errors.Is(again, ErrReconnectNeeded) {
		t.Fatalf("err=%v again=%v", err, again)
	}
	if h.figma.refreshCalls.Load() != 1 || h.store.conn.Status != StatusReconnectNeed {
		t.Fatal("connection should be marked and not retried")
	}
}

func TestTemporaryRefreshFailureIsNotReconnect(t *testing.T) {
	h := seeded(t, time.Minute)
	h.figma.refreshErr = figma.ErrUnavailable

	_, err := h.svc.AccessToken(context.Background(), "user-fig-1")

	if !errors.Is(err, ErrDependency) || h.store.conn.Status != StatusActive {
		t.Fatalf("err=%v status=%q", err, h.store.conn.Status)
	}
}

func TestMissingConnectionRequiresReconnect(t *testing.T) {
	h := newHarness()

	if _, err := h.svc.AccessToken(context.Background(), "nobody"); !errors.Is(err, ErrReconnectNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshRejectedRotatesEvenWhenNotNearExpiry(t *testing.T) {
	h := seeded(t, time.Hour)
	h.figma.refreshTokens = figma.Tokens{Access: "acc2", ExpiresAt: t0.Add(2 * time.Hour)}

	tok, err := h.svc.RefreshRejected(context.Background(), "user-fig-1", "acc")

	if err != nil || tok != "acc2" || h.figma.refreshCalls.Load() != 1 {
		t.Fatalf("tok=%q err=%v calls=%d", tok, err, h.figma.refreshCalls.Load())
	}
}

func TestRefreshRejectedReturnsNewerTokenWithoutRotating(t *testing.T) {
	h := seeded(t, time.Hour)

	tok, err := h.svc.RefreshRejected(context.Background(), "user-fig-1", "some-older-token")

	if err != nil || tok != "acc" || h.figma.refreshCalls.Load() != 0 {
		t.Fatalf("tok=%q err=%v calls=%d", tok, err, h.figma.refreshCalls.Load())
	}
}

func TestConcurrentRefreshRejectedRotatesOnce(t *testing.T) {
	h := seeded(t, time.Hour)
	h.figma.refreshTokens = figma.Tokens{Access: "acc2", ExpiresAt: t0.Add(2 * time.Hour)}
	h.figma.refreshDelay = 40 * time.Millisecond

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tok, err := h.svc.RefreshRejected(context.Background(), "user-fig-1", "acc"); err != nil || tok != "acc2" {
				t.Errorf("tok=%q err=%v", tok, err)
			}
		}()
	}
	wg.Wait()

	if h.figma.refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", h.figma.refreshCalls.Load())
	}
}

func TestFigmaTokensMapsReconnectError(t *testing.T) {
	h := newHarness()

	_, err := FigmaTokens{Service: h.svc}.AccessToken(context.Background(), "nobody")

	if !errors.Is(err, figma.ErrReconnectRequired) {
		t.Fatalf("err = %v", err)
	}
}
