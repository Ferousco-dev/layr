package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/observability"
)

type fakeHealth struct{}

func (fakeHealth) Liveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (fakeHealth) Readiness(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }

func TestRoutesAndRequestIDs(t *testing.T) {
	var logs bytes.Buffer
	h := NewHandler(fakeHealth{}, observability.New(&logs, slog.LevelInfo), Options{FrontendOrigin: "https://app.layr.dev", MaxBodyBytes: 1024})
	for _, tc := range []struct {
		method, path string
		want         int
		id           string
	}{{"GET", "/health", 200, "given-1"}, {"POST", "/health", 405, ""}, {"GET", "/missing", 404, ""}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.id != "" {
			r.Header.Set("X-Request-ID", tc.id)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s %s: %d id=%q", tc.method, tc.path, w.Code, w.Header().Get("X-Request-ID"))
		}
	}
}

func TestHandlerAppliesSecurityAndCORSHeaders(t *testing.T) {
	var logs bytes.Buffer
	h := NewHandler(fakeHealth{}, observability.New(&logs, slog.LevelInfo), Options{FrontendOrigin: "https://app.layr.dev", MaxBodyBytes: 1024})
	r := httptest.NewRequest("GET", "/health", nil)
	r.Header.Set("Origin", "https://app.layr.dev")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.layr.dev" {
		t.Fatal("CORS header missing for allowed origin")
	}
}
