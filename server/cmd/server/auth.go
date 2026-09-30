package main

import (
	"log/slog"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/auth/pgstore"
	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/ferousco-dev/layr/server/internal/credential"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/httpapi"
	"github.com/ferousco-dev/layr/server/internal/oauthstate"
	"github.com/ferousco-dev/layr/server/internal/postgres"
	"github.com/ferousco-dev/layr/server/internal/project"
	projectstore "github.com/ferousco-dev/layr/server/internal/project/pgstore"
	red "github.com/ferousco-dev/layr/server/internal/redis"
)

// newAuth wires the identity stack from config and the opened dependencies.
func newAuth(cfg config.Auth, pg *postgres.Pool, rd *red.Client, log *slog.Logger) (*httpapi.AuthOptions, *auth.Service, error) {
	sealer, err := credential.New(cfg.CredentialKey)
	if err != nil {
		return nil, nil, err
	}
	service := auth.New(auth.Deps{
		Store: pgstore.New(pg.Pgx()),
		Figma: figma.New(figma.Config{
			ClientID:     cfg.FigmaClientID,
			ClientSecret: string(cfg.FigmaClientSecret),
			RedirectURI:  cfg.FigmaRedirectURI,
		}),
		States:     oauthstate.New(rd, cfg.StateTTL),
		Sealer:     sealer,
		Locker:     rd,
		SessionTTL: cfg.SessionTTL,
		Log:        log,
	})
	return &httpapi.AuthOptions{
		Projects:     project.NewService(projectstore.New(pg.Pgx())),
		Service:      service,
		Limiter:      rd,
		CookieName:   cfg.CookieName,
		CookieSecure: cfg.CookieSecure,
	}, service, nil
}
