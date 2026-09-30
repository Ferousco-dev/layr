package auth

import (
	"context"
	"errors"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

const (
	refreshSkew = 2 * time.Minute
	lockTTL     = 30 * time.Second
	lockPrefix  = "figma:token-refresh:"
)

// AccessToken returns a Figma access token valid beyond the skew window, refreshing under a lock when needed.
func (s *Service) AccessToken(ctx context.Context, userID string) (string, error) {
	conn, err := s.connection(ctx, userID)
	if err != nil {
		return "", err
	}
	if s.fresh(conn) {
		return s.open(conn.AccessCT, accessPurpose)
	}
	return s.refresh(ctx, conn, "")
}

// RefreshRejected replaces a token Figma rejected, or returns the newer one if already replaced.
func (s *Service) RefreshRejected(ctx context.Context, userID, rejected string) (string, error) {
	conn, err := s.connection(ctx, userID)
	if err != nil {
		return "", err
	}
	if s.replaced(conn, rejected) {
		return s.open(conn.AccessCT, accessPurpose)
	}
	return s.refresh(ctx, conn, rejected)
}

func (s *Service) connection(ctx context.Context, userID string) (Connection, error) {
	conn, err := s.Store.Connection(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return Connection{}, ErrReconnectNeeded
	}
	if err != nil {
		return Connection{}, ErrDependency
	}
	if conn.Status != StatusActive {
		return Connection{}, ErrReconnectNeeded
	}
	return conn, nil
}

func (s *Service) fresh(conn Connection) bool {
	return conn.ExpiresAt.After(s.Now().Add(refreshSkew))
}

func (s *Service) open(sealed []byte, purpose string) (string, error) {
	plain, err := s.Sealer.Open(sealed, purpose)
	if err != nil {
		return "", ErrDependency
	}
	return string(plain), nil
}

func (s *Service) refresh(ctx context.Context, conn Connection, rejected string) (string, error) {
	key := lockPrefix + conn.ID
	owner, current, err := s.lock(ctx, key, conn, rejected)
	if err != nil {
		return "", err
	}
	if current != nil {
		return s.open(current.AccessCT, accessPurpose)
	}
	defer func() { _ = s.Locker.Release(context.WithoutCancel(ctx), key, owner) }()

	// Re-read under the lock: another caller may have refreshed while we waited.
	conn, err = s.connection(ctx, conn.UserID)
	if err != nil {
		return "", err
	}
	if s.settled(conn, rejected) {
		return s.open(conn.AccessCT, accessPurpose)
	}
	return s.rotate(ctx, conn)
}

// settled reports whether no rotation is needed: the token is fresh, or differs from the rejected one.
func (s *Service) settled(conn Connection, rejected string) bool {
	if rejected == "" {
		return s.fresh(conn)
	}
	return s.replaced(conn, rejected)
}

func (s *Service) replaced(conn Connection, rejected string) bool {
	current, err := s.open(conn.AccessCT, accessPurpose)
	return err == nil && current != rejected
}

// lock waits a bounded time; it returns a fresh connection instead if someone else refreshed first.
func (s *Service) lock(ctx context.Context, key string, conn Connection, rejected string) (string, *Connection, error) {
	waitCtx, cancel := context.WithTimeout(ctx, s.lockWait)
	defer cancel()
	for {
		owner, ok, err := s.Locker.Acquire(ctx, key, lockTTL)
		if err != nil {
			return "", nil, ErrDependency
		}
		if ok {
			return owner, nil, nil
		}

		latest, err := s.connection(ctx, conn.UserID)
		if err != nil {
			return "", nil, err
		}
		if s.settled(latest, rejected) {
			return "", &latest, nil
		}
		select {
		case <-waitCtx.Done():
			return "", nil, ErrDependency
		case <-time.After(s.lockPoll):
		}
	}
}

func (s *Service) rotate(ctx context.Context, conn Connection) (string, error) {
	refreshToken, err := s.open(conn.RefreshCT, refreshPurpose)
	if err != nil {
		return "", err
	}

	tokens, err := s.Figma.Refresh(ctx, refreshToken)
	if errors.Is(err, figma.ErrRejected) {
		_ = s.Store.MarkReconnectRequired(ctx, conn.ID, s.Now())
		return "", ErrReconnectNeeded
	}
	if err != nil {
		return "", ErrDependency
	}

	accessCT, err := s.Sealer.Seal([]byte(tokens.Access), accessPurpose)
	if err != nil {
		return "", ErrDependency
	}
	var refreshCT []byte
	if tokens.Refresh != "" {
		if refreshCT, err = s.Sealer.Seal([]byte(tokens.Refresh), refreshPurpose); err != nil {
			return "", ErrDependency
		}
	}
	if err := s.Store.SaveTokens(ctx, conn.ID, accessCT, refreshCT, tokens.ExpiresAt, s.Now()); err != nil {
		return "", ErrDependency
	}
	return tokens.Access, nil
}
