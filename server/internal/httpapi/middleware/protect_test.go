package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/observability"
)

const allowedOrigin = "https://app.layr.dev"

var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

func TestRecoverReturnsSafeError(t *testing.T) {
	var logs bytes.Buffer
	boom := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("secret-token") })
	h := Correlate(Recover(observability.New(&logs, slog.LevelInfo), boom))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "INTERNAL_ERROR" || body.Error.RequestID == "" {
		t.Fatalf("body = %s", w.Body.String())
	}
	if strings.Contains(w.Body.String()+logs.String(), "secret-token") {
		t.Fatal("panic value leaked")
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()

	SecurityHeaders(ok).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, name := range []string{"X-Content-Type-Options", "Cache-Control", "Referrer-Policy", "Content-Security-Policy"} {
		if w.Header().Get(name) == "" {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	cases := []struct {
		origin string
		want   string
	}{
		{allowedOrigin, allowedOrigin},
		{"https://evil.example", ""},
		{"", ""},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()

		CORS(allowedOrigin, ok).ServeHTTP(w, r)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != tc.want {
			t.Fatalf("origin %q: allow = %q, want %q", tc.origin, got, tc.want)
		}
		if w.Header().Get("Access-Control-Allow-Origin") == "*" {
			t.Fatal("wildcard origin must never be used")
		}
	}
}

func TestCORSAnswersPreflight(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "/health", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()

	CORS(allowedOrigin, http.NotFoundHandler()).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Methods") != "GET, POST, PUT, PATCH, DELETE" {
		t.Fatalf("status = %d, headers = %v", w.Code, w.Header())
	}
}

func TestClientIPTrustsForwardedHeaderOnlyFromTrustedProxy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct {
		name, remote, forwarded, want string
	}{
		{"direct peer ignores header", "203.0.113.9:4000", "1.2.3.4", "203.0.113.9"},
		{"trusted proxy uses client", "10.0.0.5:4000", "198.51.100.7", "198.51.100.7"},
		{"spoofed left entry ignored", "10.0.0.5:4000", "6.6.6.6, 198.51.100.7", "198.51.100.7"},
		{"chained trusted proxies", "10.0.0.5:4000", "198.51.100.7, 10.0.0.9", "198.51.100.7"},
		{"garbage header falls back", "10.0.0.5:4000", "not-an-ip", "10.0.0.5"},
		{"no header falls back", "10.0.0.5:4000", "", "10.0.0.5"},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tc.remote
		if tc.forwarded != "" {
			r.Header.Set("X-Forwarded-For", tc.forwarded)
		}

		if got := ClientIP(trusted)(r); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClientIPWithNoTrustedProxiesIgnoresHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:4000"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := ClientIP(nil)(r); got != "10.0.0.5" {
		t.Fatalf("got %q", got)
	}
}
