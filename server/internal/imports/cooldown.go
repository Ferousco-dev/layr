package imports

import (
	"errors"
	"fmt"
	"time"
)

// ErrRateLimited means Figma told this person to wait; no request is sent to Figma until the wait is over.
var ErrRateLimited = errors.New("figma is limiting requests")

// RateLimitedError carries how long Figma asked the person to wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("%s: retry in %s", ErrRateLimited, e.RetryAfter.Round(time.Second))
}

func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }

const (
	defaultCooldown = time.Minute
	maxCooldown     = 30 * 24 * time.Hour
)

// noteRateLimit remembers Figma's delay for a person, so later imports do not spend more of their quota.
func (s *Service) noteRateLimit(userID string, wait time.Duration) {
	if userID == "" {
		return
	}
	if wait <= 0 {
		wait = defaultCooldown
	}
	wait = min(wait, maxCooldown)
	until := s.now().Add(wait)
	s.coolMu.Lock()
	defer s.coolMu.Unlock()
	if s.cooldown == nil {
		s.cooldown = map[string]time.Time{}
	}
	if until.After(s.cooldown[userID]) {
		s.cooldown[userID] = until
	}
}

// RetryAfter is how long the person must still wait before Figma will answer again; zero when free to go.
func (s *Service) RetryAfter(userID string) time.Duration {
	s.coolMu.Lock()
	defer s.coolMu.Unlock()
	until, ok := s.cooldown[userID]
	if !ok {
		return 0
	}
	if left := until.Sub(s.now()); left > 0 {
		return left
	}
	delete(s.cooldown, userID)
	return 0
}

func (s *Service) guard(userID string) error {
	if wait := s.RetryAfter(userID); wait > 0 {
		return &RateLimitedError{RetryAfter: wait}
	}
	return nil
}

// WaitPhrase says a delay the way a person would, rounded up.
func WaitPhrase(d time.Duration) string {
	plural := func(n int, unit string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s", unit)
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	switch {
	case d <= 90*time.Second:
		return "a minute"
	case d < 90*time.Minute:
		return plural(int((d+30*time.Second)/time.Minute), "minute")
	case d < 36*time.Hour:
		return plural(int((d+30*time.Minute)/time.Hour), "hour")
	default:
		return plural(int((d+12*time.Hour)/(24*time.Hour)), "day")
	}
}

// RateLimitMessage is what a person sees while Figma is limiting their requests.
func RateLimitMessage(wait time.Duration) string {
	if wait <= 0 {
		return "Figma is limiting requests for your account. Wait a while before trying again."
	}
	return "Figma is limiting requests for your account. Try again in about " + WaitPhrase(wait) + "."
}
