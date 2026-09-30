package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type checker struct {
	err   error
	calls atomic.Int32
	block bool
}

func (c *checker) Ping(ctx context.Context) error {
	c.calls.Add(1)
	if c.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return c.err
}
func TestLivenessDoesNotCheckDependencies(t *testing.T) {
	a, b := &checker{}, &checker{}
	s := New(a, b, time.Millisecond, time.Millisecond, nil)
	w := httptest.NewRecorder()
	s.Liveness(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 200 || a.calls.Load() != 0 || b.calls.Load() != 0 {
		t.Fatal("liveness touched dependencies")
	}
}
func TestReadinessStates(t *testing.T) {
	cases := []struct {
		name string
		a, b *checker
		code int
		want string
	}{{"up", &checker{}, &checker{}, 200, "ready"}, {"down", &checker{err: errors.New("x")}, &checker{}, 503, "not_ready"}, {"timeout", &checker{block: true}, &checker{}, 503, "timeout"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.a, tc.b, 5*time.Millisecond, 20*time.Millisecond, nil)
			w := httptest.NewRecorder()
			s.Readiness(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if w.Code != tc.code || !contains(w.Body.String(), tc.want) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
func contains(s, x string) bool {
	for i := 0; i+len(x) <= len(s); i++ {
		if s[i:i+len(x)] == x {
			return true
		}
	}
	return false
}
