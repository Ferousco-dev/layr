package figma

import (
	"errors"
	"fmt"
	"time"
)

// Kind is a stable, script-safe category for a Figma API failure.
type Kind string

const (
	KindAuthRequired     Kind = "FIGMA_AUTH_REQUIRED"
	KindTokenExpired     Kind = "FIGMA_TOKEN_EXPIRED"
	KindPermissionDenied Kind = "FIGMA_PERMISSION_DENIED"
	KindNotFound         Kind = "FIGMA_FILE_NOT_FOUND"
	KindRateLimited      Kind = "FIGMA_RATE_LIMITED"
	KindBadRequest       Kind = "FIGMA_BAD_REQUEST"
	KindBadResponse      Kind = "FIGMA_BAD_RESPONSE"
	KindUnavailable      Kind = "FIGMA_UNAVAILABLE"
	KindTimeout          Kind = "FIGMA_REQUEST_TIMEOUT"
	KindCancelled        Kind = "FIGMA_REQUEST_CANCELLED"
)

var (
	// ErrInvalidInput marks caller mistakes rejected before any network call.
	ErrInvalidInput = errors.New("figma: invalid input")
	// ErrReconnectRequired means the user must sign in with Figma again.
	ErrReconnectRequired = errors.New("figma: reconnect required")
)

// Error carries only safe metadata: never the token, URL query, or provider body.
type Error struct {
	Kind      Kind
	Operation string
	Status    int
	// Retryable tells a job scheduler the same call may succeed later.
	Retryable bool
	// RetryAfter is Figma's requested delay for rate limits, zero when absent.
	RetryAfter    time.Duration
	PlanTier      string
	RateLimitType string
	Err           error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("figma %s: %s", e.Operation, e.Kind)
	if e.Status != 0 {
		msg += fmt.Sprintf(" (status %d)", e.Status)
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf returns the kind of a Figma error, or empty for other errors.
func KindOf(err error) Kind {
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Kind
	}
	return ""
}
