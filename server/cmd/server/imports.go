package main

import (
	"context"
	"log/slog"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/httpapi/middleware"
	"github.com/ferousco-dev/layr/server/internal/imports"
	importstore "github.com/ferousco-dev/layr/server/internal/imports/pgstore"
	"github.com/ferousco-dev/layr/server/internal/postgres"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

// newImports wires the Figma client, temporary workspaces and the importer.
func newImports(cfg config.Config, pg *postgres.Pool, authSvc *auth.Service, projects designapi.Projects, limiter *figma.Limiter, log *slog.Logger) (*imports.Service, *designapi.Service, error) {
	manager, err := workspace.NewManager(cfg.WorkspaceRoot, cfg.Import.MaxSnapshotBytes, cfg.Import.MaxWorkspaceBytes)
	if err != nil {
		return nil, nil, err
	}
	api := figma.NewAPI(figma.APIConfig{
		Tokens:    auth.FigmaTokens{Service: authSvc},
		Log:       log,
		RequestID: middleware.RequestID,
		Limiter:   limiter,
	})
	store := importstore.New(pg.Pgx())
	svc := imports.NewService(store, api, manager, log, imports.Config{
		Timeout:      cfg.Import.Timeout,
		WorkspaceTTL: cfg.Import.WorkspaceTTL,
		MaxScreens:   cfg.Import.MaxScreens,
	})
	svc.SetDesign(imports.DesignConfig{MaxNodes: cfg.Import.MaxIRNodes, MaxBytes: cfg.Import.MaxIRBytes})
	svc.SetAssets(assets.NewPipeline(api, assets.NewDownloader(assets.Policy{}, cfg.Import.MaxAssetBytes), log, assets.Config{
		MaxAssetBytes:  cfg.Import.MaxAssetBytes,
		MaxImportBytes: cfg.Import.MaxImportAssetBytes,
		Concurrency:    cfg.Import.AssetConcurrency,
		MaxAssets:      cfg.Import.MaxAssets,
	}))
	svc.Sweep(context.Background())

	limits := designir.Limits{MaxNodes: cfg.Import.MaxIRNodes, MaxDepth: designir.DefaultLimits.MaxDepth, MaxScreens: cfg.Import.MaxScreens}
	design := designapi.New(projects, store, manager, log, designapi.Config{Limits: limits, MaxIRBytes: cfg.Import.MaxIRBytes})
	return svc, design, nil
}
