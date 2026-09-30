package figma

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	apiBaseURL         = "https://api.figma.com"
	userAgent          = "Layr/0.1"
	defaultMaxAttempts = 3
	defaultBackoff     = 500 * time.Millisecond
	defaultMaxWait     = 10 * time.Second
	defaultMaxBytes    = 128 << 20
	defaultTimeout     = 60 * time.Second
)

// TokenSource is the one credential lifecycle; auth.FigmaTokens implements it.
type TokenSource interface {
	AccessToken(ctx context.Context, userID string) (string, error)
	// RefreshRejected replaces a token Figma answered 401 to, or returns the newer one if already replaced.
	RefreshRejected(ctx context.Context, userID, rejected string) (string, error)
}

type APIConfig struct {
	Tokens TokenSource
	Log    *slog.Logger
	// HTTP is shared across calls; the default has a timeout and refuses redirects.
	HTTP *http.Client
	// BaseURL is fixed to Figma in production; only tests override it.
	BaseURL string
	// MaxAttempts caps requests per call for retryable failures (default 3).
	MaxAttempts int
	BaseBackoff time.Duration
	// MaxRetryWait is the longest Retry-After the client will sleep; longer waits return to the caller.
	MaxRetryWait time.Duration
	// MaxResponseBytes bounds one response body (default 128 MiB).
	MaxResponseBytes int64
	RequestID        func(context.Context) string
	Sleep            func(ctx context.Context, d time.Duration) error
	Jitter           func() float64
	// Limiter paces requests per person; share one between every client so the budget is shared. Nil means no pacing.
	Limiter *Limiter
	// MaxQueue is the longest a request may wait for a free slot before it fails as rate limited (default 90s).
	MaxQueue time.Duration
}

// API is the typed Figma REST client. Use NewAPI; it is safe for concurrent use.
type API struct {
	cfg APIConfig
	log *slog.Logger
}

func NewAPI(cfg APIConfig) *API {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{
			Timeout:       defaultTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	cfg.BaseURL = strings.TrimRight(orDefault(cfg.BaseURL, apiBaseURL), "/")
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = defaultBackoff
	}
	if cfg.MaxRetryWait <= 0 {
		cfg.MaxRetryWait = defaultMaxWait
	}
	if cfg.MaxQueue <= 0 {
		cfg.MaxQueue = 90 * time.Second
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = defaultMaxBytes
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleep
	}
	if cfg.Jitter == nil {
		cfg.Jitter = rand.Float64
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &API{cfg: cfg, log: log}
}

// call describes one logical operation for logging and error labelling.
type call struct {
	op        string
	userID    string
	fileKey   string
	nodeCount int
	path      string
	query     url.Values
	// snapshot, when set, receives the raw body of the successful response as it is decoded.
	snapshot io.Writer
}

func (a *API) get(ctx context.Context, c call, out any) error {
	start := time.Now()
	token, err := a.cfg.Tokens.AccessToken(ctx, c.userID)
	if err != nil {
		return a.finish(ctx, c, start, 0, 0, false, tokenError(c.op, err))
	}

	attempts, refreshed, limited := 0, false, false
	for {
		attempts++
		if a.cfg.Limiter != nil {
			if werr := a.cfg.Limiter.wait(ctx, c.op, c.userID, a.cfg.MaxQueue, a.cfg.Sleep); werr != nil {
				return a.finish(ctx, c, start, 0, attempts, true, werr)
			}
		}
		status, ferr := a.once(ctx, c, token, out)
		if ferr == nil {
			return a.finish(ctx, c, start, status, attempts, limited, nil)
		}
		limited = limited || ferr.Kind == KindRateLimited

		if ferr.Status == http.StatusUnauthorized && !refreshed {
			refreshed = true
			next, rerr := a.cfg.Tokens.RefreshRejected(ctx, c.userID, token)
			if rerr != nil {
				return a.finish(ctx, c, start, ferr.Status, attempts, limited, tokenError(c.op, rerr))
			}
			token = next
			continue
		}
		if !a.shouldRetry(ferr, attempts) {
			return a.finish(ctx, c, start, ferr.Status, attempts, limited, ferr)
		}
		if err := a.cfg.Sleep(ctx, a.delay(ferr, attempts)); err != nil {
			return a.finish(ctx, c, start, ferr.Status, attempts, limited, ctxError(c.op, err))
		}
	}
}

// once performs a single HTTP request and decodes a 200 body into out.
func (a *API) once(ctx context.Context, c call, token string, out any) (int, *Error) {
	target := a.cfg.BaseURL + c.path
	if len(c.query) > 0 {
		target += "?" + c.query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, &Error{Kind: KindBadRequest, Operation: c.op, Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := a.cfg.HTTP.Do(req)
	if err != nil {
		return 0, transportError(c.op, ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, statusError(c.op, resp)
	}
	if err := a.decode(resp.Body, out, c.snapshot); err != nil {
		return resp.StatusCode, &Error{Kind: KindBadResponse, Operation: c.op, Status: resp.StatusCode, Err: err}
	}
	return resp.StatusCode, nil
}

var errTooLarge = errors.New("response exceeds size limit")

// decode streams JSON through a size cap; unknown fields are ignored on purpose.
func (a *API) decode(body io.Reader, out any, snapshot io.Writer) error {
	limited := &cappedReader{r: body, remaining: a.cfg.MaxResponseBytes}
	var src io.Reader = &depthGuard{r: limited}
	if snapshot != nil {
		src = &depthGuard{r: io.TeeReader(limited, snapshot)}
	}
	if err := json.NewDecoder(src).Decode(out); err != nil {
		if errors.Is(limited.err, errTooLarge) {
			return errTooLarge
		}
		if errors.Is(err, errTooDeep) {
			return errTooDeep
		}
		return err
	}
	return nil
}

type cappedReader struct {
	r         io.Reader
	remaining int64
	err       error
}

func (c *cappedReader) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		c.err = errTooLarge
		return 0, errTooLarge
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.r.Read(p)
	c.remaining -= int64(n)
	return n, err
}

func statusError(op string, resp *http.Response) *Error {
	e := &Error{Operation: op, Status: resp.StatusCode}
	switch resp.StatusCode {
	case http.StatusBadRequest:
		e.Kind = KindBadRequest
	case http.StatusUnauthorized:
		e.Kind = KindTokenExpired
	case http.StatusForbidden:
		e.Kind = KindPermissionDenied
	case http.StatusNotFound:
		e.Kind = KindNotFound
	case http.StatusTooManyRequests:
		e.Kind, e.Retryable = KindRateLimited, true
		e.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		e.PlanTier = resp.Header.Get("X-Figma-Plan-Tier")
		e.RateLimitType = resp.Header.Get("X-Figma-Rate-Limit-Type")
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		e.Kind, e.Retryable = KindUnavailable, true
	default:
		e.Kind = KindBadResponse
	}
	return e
}

func transportError(op string, ctx context.Context, err error) *Error {
	if ctx.Err() != nil {
		return ctxError(op, ctx.Err())
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Kind: KindTimeout, Operation: op, Retryable: true, Err: err}
	}
	return &Error{Kind: KindUnavailable, Operation: op, Retryable: true, Err: err}
}

func ctxError(op string, err error) *Error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: KindTimeout, Operation: op, Retryable: true, Err: err}
	}
	return &Error{Kind: KindCancelled, Operation: op, Err: err}
}

// tokenError maps credential failures without ever touching a token value.
func tokenError(op string, err error) *Error {
	if errors.Is(err, ErrReconnectRequired) {
		return &Error{Kind: KindAuthRequired, Operation: op, Err: err}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ctxError(op, err)
	}
	return &Error{Kind: KindUnavailable, Operation: op, Retryable: true, Err: err}
}

// shouldRetry allows 429, 502, 503, 504 and connection failures, never 400, 401, 403, 404, 500 or timeouts.
func (a *API) shouldRetry(e *Error, attempts int) bool {
	if attempts >= a.cfg.MaxAttempts {
		return false
	}
	switch e.Kind {
	case KindRateLimited:
		return e.RetryAfter <= a.cfg.MaxRetryWait
	case KindUnavailable:
		return e.Status == 0 || e.Status == http.StatusBadGateway ||
			e.Status == http.StatusServiceUnavailable || e.Status == http.StatusGatewayTimeout
	}
	return false
}

// delay honours Retry-After, otherwise exponential backoff with jitter between half and full.
func (a *API) delay(e *Error, attempts int) time.Duration {
	if e.RetryAfter > 0 {
		return e.RetryAfter
	}
	d := a.cfg.BaseBackoff << (attempts - 1)
	return d/2 + time.Duration(a.cfg.Jitter()*float64(d/2))
}

// parseRetryAfter accepts delta-seconds or an HTTP date; anything else yields zero.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// finish logs one line per logical call, with no credentials or payload.
func (a *API) finish(ctx context.Context, c call, start time.Time, status, attempts int, limited bool, err *Error) error {
	attrs := []any{
		slog.String("figma_operation", c.op),
		slog.String("user_id", c.userID),
		slog.String("file_key", c.fileKey),
		slog.Int("node_count", c.nodeCount),
		slog.Int("status", status),
		slog.Int("attempts", attempts),
		slog.Bool("rate_limited", limited),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if a.cfg.RequestID != nil {
		attrs = append(attrs, slog.String("request_id", a.cfg.RequestID(ctx)))
	}
	if err == nil {
		a.log.InfoContext(ctx, "figma.request.completed", attrs...)
		return nil
	}
	a.log.WarnContext(ctx, "figma.request.failed", append(attrs, slog.String("code", string(err.Kind)))...)
	return err
}
