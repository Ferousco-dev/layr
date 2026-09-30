package auth

import (
	"context"
	"errors"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

// FigmaTokens adapts the credential lifecycle to figma.TokenSource without letting figma import auth.
type FigmaTokens struct{ Service *Service }

func (t FigmaTokens) AccessToken(ctx context.Context, userID string) (string, error) {
	token, err := t.Service.AccessToken(ctx, userID)
	return token, mapTokenError(err)
}

func (t FigmaTokens) RefreshRejected(ctx context.Context, userID, rejected string) (string, error) {
	token, err := t.Service.RefreshRejected(ctx, userID, rejected)
	return token, mapTokenError(err)
}

func mapTokenError(err error) error {
	if errors.Is(err, ErrReconnectNeeded) {
		return figma.ErrReconnectRequired
	}
	return err
}
