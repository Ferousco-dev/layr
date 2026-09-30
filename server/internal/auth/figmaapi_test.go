package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

func apiFor(t *testing.T, h *harness, handler http.HandlerFunc) *figma.API {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return figma.NewAPI(figma.APIConfig{Tokens: FigmaTokens{Service: h.svc}, BaseURL: srv.URL})
}

func TestAPIUsesStoredTokenThenRecoversFromRejection(t *testing.T) {
	h := seeded(t, time.Hour)
	h.figma.refreshTokens = figma.Tokens{Access: "acc2", ExpiresAt: t0.Add(2 * time.Hour)}
	var seen []string
	api := apiFor(t, h, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer acc2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"name":"Design","document":{"id":"0:0","type":"DOCUMENT","name":"d"}}`))
	})

	file, err := api.GetFile(context.Background(), "user-fig-1", "KEY", figma.FileOptions{})

	if err != nil || file.Name != "Design" {
		t.Fatalf("err = %v", err)
	}
	if len(seen) != 2 || seen[0] != "Bearer acc" || seen[1] != "Bearer acc2" || h.figma.refreshCalls.Load() != 1 {
		t.Fatalf("requests = %v, refreshes = %d", seen, h.figma.refreshCalls.Load())
	}
}

func TestAPIReportsReconnectWhenRefreshIsRevoked(t *testing.T) {
	h := seeded(t, time.Hour)
	h.figma.refreshErr = figma.ErrRejected
	var hits atomic.Int32
	api := apiFor(t, h, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := api.GetFile(context.Background(), "user-fig-1", "KEY", figma.FileOptions{})

	if figma.KindOf(err) != figma.KindAuthRequired || !errors.Is(err, figma.ErrReconnectRequired) || hits.Load() != 1 {
		t.Fatalf("err = %v hits = %d", err, hits.Load())
	}
	if h.store.conn.Status != StatusReconnectNeed {
		t.Fatal("connection not marked for reconnect")
	}
}
