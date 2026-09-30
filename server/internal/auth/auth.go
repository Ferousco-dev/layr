// Package auth owns Figma login, Layr sessions and the Figma credential lifecycle.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/oauthstate"
)

const (
	accessPurpose  = "figma/access"
	refreshPurpose = "figma/refresh"
	touchInterval  = 5 * time.Minute
	tokenLength    = 43
)

const (
	StatusActive        = "active"
	StatusReconnectNeed = "reconnect_required"
)

var (
	ErrNotFound        = errors.New("record not found")
	ErrStateInvalid    = errors.New("oauth state invalid")
	ErrFigmaAuthFailed = errors.New("figma authorization failed")
	ErrIdentityFailed  = errors.New("figma identity failed")
	ErrDependency      = errors.New("dependency unavailable")
	ErrSessionInvalid  = errors.New("session invalid")
	ErrReconnectNeeded = errors.New("figma reconnect required")
)

type User struct {
	ID          string
	FigmaUserID string
	Email       string
	DisplayName string
	AvatarURL   string
}

type Connection struct {
	ID        string
	UserID    string
	AccessCT  []byte
	RefreshCT []byte
	ExpiresAt time.Time
	Status    string
}

type Login struct {
	Identity  figma.Identity
	AccessCT  []byte
	RefreshCT []byte
	ExpiresAt time.Time
	Scopes    string
	At        time.Time
}

type SessionRecord struct {
	ID         string
	User       User
	ExpiresAt  time.Time
	LastSeenAt time.Time
	RevokedAt  *time.Time
}

type Session struct {
	Token     string
	ExpiresAt time.Time
}

type Store interface {
	UpsertLogin(ctx context.Context, login Login) (User, error)
	CreateSession(ctx context.Context, userID string, hash []byte, expires, now time.Time) error
	SessionByHash(ctx context.Context, hash []byte) (SessionRecord, error)
	TouchSession(ctx context.Context, id string, now time.Time) error
	RevokeSession(ctx context.Context, hash []byte, now time.Time) error
	Connection(ctx context.Context, userID string) (Connection, error)
	SaveTokens(ctx context.Context, connID string, accessCT, refreshCT []byte, expires, now time.Time) error
	MarkReconnectRequired(ctx context.Context, connID string, now time.Time) error
}

type FigmaAPI interface {
	AuthorizeURL(state, challenge string) string
	Exchange(ctx context.Context, code, verifier string) (figma.Tokens, error)
	Refresh(ctx context.Context, refreshToken string) (figma.Tokens, error)
	Me(ctx context.Context, accessToken string) (figma.Identity, error)
}

type States interface {
	Begin(ctx context.Context) (oauthstate.Attempt, error)
	Consume(ctx context.Context, state string) (string, error)
}

type Sealer interface {
	Seal(plaintext []byte, purpose string) ([]byte, error)
	Open(sealed []byte, purpose string) ([]byte, error)
}

type Locker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error)
	Release(ctx context.Context, key, owner string) error
}

type Deps struct {
	Store      Store
	Figma      FigmaAPI
	States     States
	Sealer     Sealer
	Locker     Locker
	SessionTTL time.Duration
	Now        func() time.Time
	Log        *slog.Logger
}

type Service struct {
	Deps
	lockWait time.Duration
	lockPoll time.Duration
}

func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{Deps: d, lockWait: 5 * time.Second, lockPoll: 100 * time.Millisecond}
}

// Begin creates a single-use OAuth attempt and returns the Figma URL to visit.
func (s *Service) Begin(ctx context.Context) (string, error) {
	attempt, err := s.States.Begin(ctx)
	if err != nil {
		return "", ErrDependency
	}
	return s.Figma.AuthorizeURL(attempt.State, attempt.Challenge), nil
}

// Consume burns a state without logging in, used when Figma reports a denial.
func (s *Service) Consume(ctx context.Context, state string) error {
	_, err := s.States.Consume(ctx, state)
	return mapState(err)
}

// Complete finishes the callback: exchange, identity, atomic persistence, then a new session.
func (s *Service) Complete(ctx context.Context, code, state string) (Session, error) {
	verifier, err := s.States.Consume(ctx, state)
	if err != nil {
		return Session{}, mapState(err)
	}

	tokens, err := s.Figma.Exchange(ctx, code, verifier)
	if err != nil {
		s.Log.WarnContext(ctx, "auth.figma.exchange_failed", slog.String("reason", err.Error()))
		return Session{}, mapFigma(err, ErrFigmaAuthFailed)
	}
	ident, err := s.Figma.Me(ctx, tokens.Access)
	if err != nil {
		s.Log.WarnContext(ctx, "auth.figma.identity_failed", slog.String("reason", err.Error()))
		return Session{}, mapFigma(err, ErrIdentityFailed)
	}
	if ident.ID != tokens.UserID {
		return Session{}, ErrIdentityFailed
	}

	now := s.Now()
	login, err := s.sealLogin(ident, tokens, now)
	if err != nil {
		return Session{}, err
	}
	user, err := s.Store.UpsertLogin(ctx, login)
	if err != nil {
		return Session{}, ErrDependency
	}

	session, err := s.newSession(ctx, user.ID, now)
	if err != nil {
		return Session{}, err
	}
	s.Log.InfoContext(ctx, "auth.login.completed",
		slog.String("user_id", user.ID), slog.String("figma_user_id", user.FigmaUserID))
	return session, nil
}

func (s *Service) sealLogin(ident figma.Identity, t figma.Tokens, now time.Time) (Login, error) {
	access, err := s.Sealer.Seal([]byte(t.Access), accessPurpose)
	if err != nil {
		return Login{}, ErrDependency
	}
	refresh, err := s.Sealer.Seal([]byte(t.Refresh), refreshPurpose)
	if err != nil {
		return Login{}, ErrDependency
	}
	return Login{
		Identity: ident, AccessCT: access, RefreshCT: refresh,
		ExpiresAt: t.ExpiresAt, Scopes: scopeString(), At: now,
	}, nil
}

func (s *Service) newSession(ctx context.Context, userID string, now time.Time) (Session, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Session{}, ErrDependency
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	expires := now.Add(s.SessionTTL)
	if err := s.Store.CreateSession(ctx, userID, hashToken(token), expires, now); err != nil {
		return Session{}, ErrDependency
	}
	return Session{Token: token, ExpiresAt: expires}, nil
}

// Authenticate resolves an opaque cookie value to a user, rejecting revoked or expired sessions.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if len(token) != tokenLength {
		return User{}, ErrSessionInvalid
	}
	if _, err := base64.RawURLEncoding.DecodeString(token); err != nil {
		return User{}, ErrSessionInvalid
	}

	rec, err := s.Store.SessionByHash(ctx, hashToken(token))
	if errors.Is(err, ErrNotFound) {
		return User{}, ErrSessionInvalid
	}
	if err != nil {
		return User{}, ErrDependency
	}

	now := s.Now()
	if rec.RevokedAt != nil || !now.Before(rec.ExpiresAt) {
		return User{}, ErrSessionInvalid
	}
	if now.Sub(rec.LastSeenAt) > touchInterval {
		_ = s.Store.TouchSession(ctx, rec.ID, now)
	}
	return rec.User, nil
}

// Logout revokes the session and is safe to repeat.
func (s *Service) Logout(ctx context.Context, token string) error {
	if len(token) != tokenLength {
		return nil
	}
	if err := s.Store.RevokeSession(ctx, hashToken(token), s.Now()); err != nil {
		return ErrDependency
	}
	return nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func scopeString() string {
	out := ""
	for i, sc := range figma.Scopes {
		if i > 0 {
			out += " "
		}
		out += sc
	}
	return out
}

func mapState(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, oauthstate.ErrInvalid):
		return ErrStateInvalid
	default:
		return ErrDependency
	}
}

func mapFigma(err, rejected error) error {
	if errors.Is(err, figma.ErrUnavailable) {
		return ErrDependency
	}
	return rejected
}
