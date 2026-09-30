// Package health implements liveness and readiness (DES-024, DES-025).
package health

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type Checker interface{ Ping(context.Context) error }
type Service struct {
	postgres Checker
	redis    Checker
	probe    time.Duration
	overall  time.Duration
	log      *slog.Logger
}

func New(pg, rd Checker, probe, overall time.Duration, log *slog.Logger) *Service {
	return &Service{
		postgres: pg,
		redis:    rd,
		probe:    probe,
		overall:  overall,
		log:      log,
	}
}
func (s *Service) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type result struct{ name, state string }

func (s *Service) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.overall)
	defer cancel()

	ch := make(chan result, 2)
	go s.check(ctx, "postgres", s.postgres, ch)
	go s.check(ctx, "redis", s.redis, ch)
	checks := map[string]string{"postgres": "timeout", "redis": "timeout"}
	for i := 0; i < 2; i++ {
		select {
		case x := <-ch:
			checks[x.name] = x.state
		case <-ctx.Done():
			i = 2
		}
	}
	status, code := "ready", http.StatusOK
	if checks["postgres"] != "up" || checks["redis"] != "up" {
		status, code = "not_ready", http.StatusServiceUnavailable
		if s.log != nil {
			s.log.Warn("readiness.failed")
		}
	}
	writeJSON(w, code, map[string]any{"status": status, "checks": checks})
}
func (s *Service) check(parent context.Context, name string, c Checker, ch chan<- result) {
	ctx, cancel := context.WithTimeout(parent, s.probe)
	defer cancel()
	err := c.Ping(ctx)
	state := "up"
	if err != nil {
		state = "down"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			state = "timeout"
		}
	}
	select {
	case ch <- result{name, state}:
	case <-parent.Done():
	}
}
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
