package figma

import (
	"context"
	"sync"
	"time"
)

// DefaultRequestsPerMinute is Layr's own ceiling on requests to Figma for one person.
// It stays under what Figma allows a low seat, so one busy person cannot exhaust their own quota.
const DefaultRequestsPerMinute = 8

const window = time.Minute

// Limiter is a sliding window over the last minute. Every HTTP request to Figma, including a retry, takes one slot.
type Limiter struct {
	limit int
	now   func() time.Time

	mu   sync.Mutex
	seen map[string][]time.Time
}

// NewLimiter allows at most perMinute requests per person in any 60 seconds.
func NewLimiter(perMinute int) *Limiter {
	if perMinute < 1 {
		perMinute = DefaultRequestsPerMinute
	}
	return &Limiter{limit: perMinute, now: time.Now, seen: map[string][]time.Time{}}
}

// Reserve takes a slot when one is free and returns 0. Otherwise it takes nothing and returns how long until one frees.
func (l *Limiter) Reserve(userID string) time.Duration {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	live := l.seen[userID][:0]
	for _, t := range l.seen[userID] {
		if now.Sub(t) < window {
			live = append(live, t)
		}
	}
	if len(live) < l.limit {
		l.seen[userID] = append(live, now)
		return 0
	}
	l.seen[userID] = live
	return live[0].Add(window).Sub(now)
}

// Used is how many slots the person has taken in the last minute.
func (l *Limiter) Used(userID string) int {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, t := range l.seen[userID] {
		if now.Sub(t) < window {
			n++
		}
	}
	return n
}

// Limit is the ceiling per minute.
func (l *Limiter) Limit() int { return l.limit }

// wait blocks until a slot is free. It gives up at once, without sleeping, when the wait would pass the
// context deadline or maxQueue, and reports the wait as a rate limit so the caller can tell the person.
func (l *Limiter) wait(ctx context.Context, op, userID string, maxQueue time.Duration, sleep func(context.Context, time.Duration) error) *Error {
	for {
		d := l.Reserve(userID)
		if d <= 0 {
			return nil
		}
		if deadline, ok := ctx.Deadline(); (ok && time.Until(deadline) < d) || d > maxQueue {
			return &Error{Kind: KindRateLimited, Operation: op, Retryable: true, RetryAfter: d + time.Second}
		}
		if err := sleep(ctx, d+50*time.Millisecond); err != nil {
			return ctxError(op, err)
		}
	}
}
