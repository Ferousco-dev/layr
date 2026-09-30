package main

import (
	"log/slog"

	"github.com/ferousco-dev/layr/server/internal/genplan"
	planstore "github.com/ferousco-dev/layr/server/internal/genplan/pgstore"

	"github.com/ferousco-dev/layr/server/internal/account"
	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/generation"
	generationstore "github.com/ferousco-dev/layr/server/internal/generation/pgstore"
	"github.com/ferousco-dev/layr/server/internal/postgres"
)

// newGeneration wires code-generation jobs to the design API and the saved AI keys.
func newGeneration(pg *postgres.Pool, designs *designapi.Service, keys *account.Service, log *slog.Logger) *generation.Service {
	return generation.NewService(generationstore.New(pg.Pgx()), designs, keys, log)
}

// newPlans wires generation plans to the design API; planning reads the stored design and never contacts Figma or an AI provider.
func newPlans(pg *postgres.Pool, designs *designapi.Service, log *slog.Logger) *genplan.Service {
	return genplan.NewService(designs, planstore.New(pg.Pgx()), genplan.DefaultLimits, log)
}
