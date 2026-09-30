package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// Recover turns a handler panic into a generic 500 without logging the panic value.
func Recover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			id := RequestID(r.Context())
			log.ErrorContext(
				r.Context(),
				"request.panic",
				slog.String("request_id", id),
				slog.String("code", "INTERNAL_ERROR"),
			)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", id)
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders sets API-relevant headers only; HSTS belongs to the TLS layer.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// CORS admits one origin and echoes it, since credentials forbid a wildcard.
func CORS(allowed string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")

		origin := r.Header.Get("Origin")
		if origin == "" || origin != allowed {
			next.ServeHTTP(w, r)
			return
		}

		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Access-Control-Expose-Headers", HeaderRequestID)

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			h.Set("Access-Control-Allow-Headers", "Content-Type, "+HeaderRequestID)
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type Limiter interface {
	Count(ctx context.Context, key string, window time.Duration) (int64, error)
}

// RateLimit allows limit requests per window for each client address.
func RateLimit(l Limiter, name string, limit int64, window time.Duration, clientIP func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := l.Count(r.Context(), "ratelimit:"+name+":"+clientIP(r), window)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required service is unavailable. Try again shortly.", RequestID(r.Context()))
			return
		}
		if n > limit {
			w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again later.", RequestID(r.Context()))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SameOrigin rejects state-changing browser requests that come from another site.
func SameOrigin(allowed string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origin := r.Header.Get("Origin")
			crossSite := r.Header.Get("Sec-Fetch-Site") == "cross-site"
			if (origin != "" && origin != allowed) || (origin == "" && crossSite) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "This request is not allowed from this origin.", RequestID(r.Context()))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP returns the caller address, trusting X-Forwarded-For only when the peer is a trusted proxy.
func ClientIP(trusted []netip.Prefix) func(*http.Request) string {
	return func(r *http.Request) string {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !inPrefixes(trusted, host) {
			return host
		}

		// Walk right to left: the first hop that is not our own proxy is the real client.
		hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			hop := strings.TrimSpace(hops[i])
			if _, err := netip.ParseAddr(hop); err != nil {
				break
			}
			if !inPrefixes(trusted, hop) {
				return hop
			}
		}
		return host
	}
}

func inPrefixes(prefixes []netip.Prefix, host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
