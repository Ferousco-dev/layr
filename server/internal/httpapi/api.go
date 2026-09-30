// Package httpapi assembles the bounded operational HTTP surface (DES-028).
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/ferousco-dev/layr/server/internal/httpapi/middleware"
	"github.com/ferousco-dev/layr/server/internal/postgres"
)

type Health interface {
	Liveness(http.ResponseWriter, *http.Request)
	Readiness(http.ResponseWriter, *http.Request)
}

// Options carries the request-boundary settings applied to every route.
type Options struct {
	FrontendOrigin string
	MaxBodyBytes   int64
	Auth           *AuthOptions
}

func NewHandler(h Health, log *slog.Logger, opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", method(http.MethodGet, h.Liveness))
	mux.HandleFunc("/ready", method(http.MethodGet, h.Readiness))
	if opts.Auth != nil {
		(&authRoutes{AuthOptions: *opts.Auth, frontend: opts.FrontendOrigin, log: log}).register(mux)
	}
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, p := mux.Handler(r); p != "" {
			mux.ServeHTTP(w, r)
			return
		}
		failure(w, r, http.StatusNotFound, "NOT_FOUND", "The requested endpoint does not exist.")
	})

	// Recover sits inside AccessLog so a panic is still logged as a 500.
	var chain http.Handler = middleware.LimitBody(opts.MaxBodyBytes, base)
	chain = middleware.CORS(opts.FrontendOrigin, middleware.SameOrigin(opts.FrontendOrigin, chain))
	chain = middleware.SecurityHeaders(chain)
	chain = middleware.Recover(log, chain)
	return middleware.Correlate(middleware.AccessLog(log, chain))
}

func method(want string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != want {
			w.Header().Set("Allow", want)
			failure(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "This endpoint accepts "+want+" requests only.")
			return
		}
		next(w, r)
	}
}

func failure(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": middleware.RequestID(r.Context())}})
}

// unexpected answers an unhandled error: 503 with Retry-After when the database is down, otherwise a logged 500.
func unexpected(w http.ResponseWriter, r *http.Request, log *slog.Logger, event string, err error) {
	id := slog.String("request_id", middleware.RequestID(r.Context()))
	if postgres.IsUnavailable(err) {
		log.ErrorContext(r.Context(), event, id, slog.String("code", "DEPENDENCY_UNAVAILABLE"))
		w.Header().Set("Retry-After", "5")
		failure(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required service is unavailable. Try again shortly.")
		return
	}
	log.ErrorContext(r.Context(), event, id, slog.String("code", "INTERNAL_ERROR"))
	failure(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.")
}

func NewServer(c config.HTTP, h http.Handler) *http.Server {
	return &http.Server{Addr: c.Address, Handler: h, ReadHeaderTimeout: c.ReadHeaderTimeout, ReadTimeout: c.ReadTimeout, WriteTimeout: c.WriteTimeout, IdleTimeout: c.IdleTimeout, MaxHeaderBytes: c.MaxHeaderBytes}
}
