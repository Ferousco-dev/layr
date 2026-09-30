package figma

import (
	"context"
	"testing"
	"time"
)

func clockedLimiter(perMinute int) (*Limiter, *time.Time) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	l := NewLimiter(perMinute)
	l.now = func() time.Time { return now }
	return l, &now
}

func TestLimiterAllowsEightPerMinuteThenMakesTheNinthWait(t *testing.T) {
	l, now := clockedLimiter(8)
	for i := 0; i < 8; i++ {
		if d := l.Reserve("u1"); d != 0 {
			t.Fatalf("request %d waited %s", i+1, d)
		}
		*now = now.Add(2 * time.Second)
	}
	d := l.Reserve("u1")
	if d <= 0 || d > time.Minute {
		t.Fatalf("ninth request wait = %s", d)
	}
	if l.Used("u1") != 8 {
		t.Fatalf("a refused request must not take a slot: used %d", l.Used("u1"))
	}
	*now = now.Add(d)
	if l.Reserve("u1") != 0 {
		t.Fatal("a slot must free when the oldest request leaves the window")
	}
}

func TestLimiterIsPerPersonAndSlides(t *testing.T) {
	l, now := clockedLimiter(2)
	l.Reserve("a")
	l.Reserve("a")
	if l.Reserve("a") == 0 {
		t.Fatal("third request should wait")
	}
	if l.Reserve("b") != 0 {
		t.Fatal("another person must have their own budget")
	}
	*now = now.Add(61 * time.Second)
	if l.Reserve("a") != 0 || l.Used("a") != 1 {
		t.Fatalf("the window must slide: used %d", l.Used("a"))
	}
	if NewLimiter(0).Limit() != DefaultRequestsPerMinute {
		t.Fatal("default limit")
	}
}

func TestPacedRequestsWaitForASlotThenGoThrough(t *testing.T) {
	r := newRig(t, serve([]byte(`{"name":"F","version":"1","document":{"id":"0:0","type":"DOCUMENT"}}`)))
	l, now := clockedLimiter(2)
	r.api.cfg.Limiter = l
	r.api.cfg.Sleep = func(_ context.Context, d time.Duration) error {
		r.sleeps = append(r.sleeps, d)
		*now = now.Add(d)
		return nil
	}

	for i := 0; i < 3; i++ {
		if _, err := r.api.GetFile(context.Background(), "u1", "KEY123456", FileOptions{}); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if r.hits.Load() != 3 || len(r.sleeps) != 1 || r.sleeps[0] < 50*time.Second {
		t.Fatalf("hits %d, sleeps %v", r.hits.Load(), r.sleeps)
	}
}

func TestPacingFailsFastWhenTheWaitIsTooLong(t *testing.T) {
	r := newRig(t, serve([]byte(`{"name":"F","document":{"id":"0:0","type":"DOCUMENT"}}`)))
	l, _ := clockedLimiter(1)
	r.api.cfg.Limiter = l
	r.api.cfg.MaxQueue = 5 * time.Second
	if _, err := r.api.GetFile(context.Background(), "u1", "KEY123456", FileOptions{}); err != nil {
		t.Fatal(err)
	}

	_, err := r.api.GetFile(context.Background(), "u1", "KEY123456", FileOptions{})

	var fe *Error
	if !asError(err, &fe) || fe.Kind != KindRateLimited || fe.RetryAfter <= 0 || fe.RetryAfter > 62*time.Second {
		t.Fatalf("err = %v", err)
	}
	if r.hits.Load() != 1 || len(r.sleeps) != 0 {
		t.Fatalf("a refused call must not reach Figma or sleep: hits %d sleeps %v", r.hits.Load(), r.sleeps)
	}
}

func TestPacingGivesUpWhenTheContextWouldExpireFirst(t *testing.T) {
	r := newRig(t, serve([]byte(`{"name":"F","document":{"id":"0:0","type":"DOCUMENT"}}`)))
	l, _ := clockedLimiter(1)
	r.api.cfg.Limiter = l
	_, _ = r.api.GetFile(context.Background(), "u1", "KEY123456", FileOptions{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.api.GetFile(ctx, "u1", "KEY123456", FileOptions{})

	if KindOf(err) != KindRateLimited || r.hits.Load() != 1 {
		t.Fatalf("err = %v, hits %d", err, r.hits.Load())
	}
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}
