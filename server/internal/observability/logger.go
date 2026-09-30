// Package observability provides safe JSON logging (DES-020).
package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

const redacted = "[REDACTED]"

var sensitive = []string{"password", "passwd", "secret", "token", "authorization", "cookie", "database_url", "postgres_url", "redis_url"}

type Handler struct {
	next slog.Handler
}

func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(&Handler{next: slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			a.Key = "timestamp"
		}
		if a.Key == slog.MessageKey {
			a.Key = "event"
		}
		return a
	}})})
}
func (h *Handler) Enabled(c context.Context, l slog.Level) bool { return h.next.Enabled(c, l) }
func (h *Handler) Handle(c context.Context, r slog.Record) error {
	n := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool { n.AddAttrs(clean(a)); return true })
	return h.next.Handle(c, n)
}
func (h *Handler) WithAttrs(a []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(a))
	for i := range a {
		out[i] = clean(a[i])
	}
	return &Handler{next: h.next.WithAttrs(out)}
}
func (h *Handler) WithGroup(n string) slog.Handler { return &Handler{next: h.next.WithGroup(n)} }
func clean(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if isSensitive(a.Key) {
		return slog.String(a.Key, redacted)
	}
	if a.Value.Kind() == slog.KindGroup {
		g := a.Value.Group()
		for i := range g {
			g[i] = clean(g[i])
		}
		return slog.Group(a.Key, attrsToAny(g)...)
	}
	return a
}
func attrsToAny(a []slog.Attr) []any {
	r := make([]any, len(a))
	for i := range a {
		r[i] = a[i]
	}
	return r
}
func isSensitive(k string) bool {
	k = strings.ToLower(k)
	for _, s := range sensitive {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}
