// Package middleware correlates and observes requests (DES-026, DES-027).
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const HeaderRequestID = "X-Request-ID"

type contextKey struct{}

func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(contextKey{}).(string)
	return v
}

func Correlate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validID(id) {
			var raw [16]byte
			if _, e := rand.Read(raw[:]); e != nil {
				writeError(
					w,
					http.StatusInternalServerError,
					"INTERNAL_ERROR",
					"The server could not complete the request. Use the request ID to find the matching log.",
					"",
				)
				return
			}
			id = hex.EncodeToString(raw[:])
		}
		w.Header().Set(HeaderRequestID, id)
		ctx := context.WithValue(r.Context(), contextKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		digit := r >= '0' && r <= '9'
		separator := strings.ContainsRune("-._~", r)
		if !letter && !digit && !separator {
			return false
		}
	}
	return true
}

type recorder struct {
	http.ResponseWriter
	status, bytes int
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	n, e := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, e
}

func AccessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &recorder{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		status := rw.status
		if status == 0 {
			status = 200
		}
		log.InfoContext(
			r.Context(),
			"request.completed",
			slog.String("request_id", RequestID(r.Context())),
			slog.String("method", r.Method),
			slog.String("route", route(r.URL.Path)),
			slog.Int("status", status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

var knownRoutes = map[string]bool{
	"/health": true, "/ready": true, "/auth/figma": true,
	"/auth/figma/callback": true, "/auth/logout": true, "/api/v1/me": true,
	"/api/v1/projects": true,
}

// route logs only registered paths so attacker-chosen URLs never enter logs.
func route(path string) string {
	if knownRoutes[path] {
		return path
	}
	if rest, ok := strings.CutPrefix(path, "/api/v1/projects/"); ok && rest != "" {
		return projectRoute(strings.Split(rest, "/"))
	}
	return "unmatched"
}

// projectRoute names the registered pattern for a path under /api/v1/projects/.
func projectRoute(parts []string) string {
	const base = "/api/v1/projects/{projectID}"
	switch {
	case len(parts) == 1:
		return base
	case len(parts) == 2 && parts[1] == "restore":
		return base + "/restore"
	case len(parts) == 2 && parts[1] == "import":
		return base + "/import"
	case len(parts) == 3 && parts[1] == "imports":
		return base + "/imports/{importID}"
	case len(parts) == 4 && parts[1] == "imports" && parts[3] == "select":
		return base + "/imports/{importID}/select"
	case len(parts) == 2 && parts[1] == "design":
		return base + "/design"
	case len(parts) == 3 && parts[1] == "design" && parts[2] == "screens":
		return base + "/design/screens"
	case len(parts) == 4 && parts[1] == "design" && parts[2] == "screens":
		return base + "/design/screens/{screenID}"
	case len(parts) == 5 && parts[1] == "design" && parts[2] == "screens" && parts[4] == "preview":
		return base + "/design/screens/{screenID}/preview"
	case len(parts) == 4 && parts[1] == "design" && parts[2] == "flows":
		return base + "/design/flows/{flowID}"
	}
	return "unmatched"
}

func LimitBody(max int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, max)
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, code, message, id string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": id,
		},
	})
}

func DrainBody(r *http.Request) error {
	_, err := io.Copy(io.Discard, r.Body)
	return err
}
