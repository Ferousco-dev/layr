package main

import (
	"log/slog"

	"github.com/ferousco-dev/layr/server/internal/account"
	accountstore "github.com/ferousco-dev/layr/server/internal/account/pgstore"
	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/ferousco-dev/layr/server/internal/credential"
	"github.com/ferousco-dev/layr/server/internal/postgres"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

// newAccount wires AI key storage and account deletion.
func newAccount(cfg config.Config, pg *postgres.Pool, log *slog.Logger) (*account.Service, error) {
	sealer, err := credential.New(cfg.Auth.CredentialKey)
	if err != nil {
		return nil, err
	}
	files, err := workspace.NewManager(cfg.WorkspaceRoot, cfg.Import.MaxSnapshotBytes, cfg.Import.MaxWorkspaceBytes)
	if err != nil {
		return nil, err
	}
	return account.New(accountstore.New(pg.Pgx()), sealer, files, log), nil
}
