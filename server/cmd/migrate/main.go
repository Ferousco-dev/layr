// Command migrate runs explicit embedded SQL migrations (DES-023, DES-030).
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"

	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/ferousco-dev/layr/server/internal/migrations"
	"github.com/ferousco-dev/layr/server/internal/observability"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if e := run(os.Args[1:], os.Stdout, os.Stderr); e != nil {
		os.Exit(1)
	}
}

func run(args []string, out, errOut *os.File) error {
	log := observability.New(errOut, slog.LevelInfo)
	if len(args) != 1 || (args[0] != "up" && args[0] != "down" && args[0] != "status") {
		emit(log, "migration.failed", "MIGRATION_USAGE", "Usage: migrate <up|down|status>")
		return errors.New("usage")
	}
	c, e := config.Load(os.LookupEnv)
	if e != nil {
		emit(log, "migration.failed", "CONFIG_INVALID", "Configuration is invalid.")
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.StartupTimeout)
	defer cancel()
	db, e := sql.Open("pgx", string(c.Postgres.URL))
	if e != nil {
		emit(log, "migration.failed", "MIGRATION_CONNECT_FAILED", "The migration database is unavailable.")
		return e
	}
	defer db.Close()
	if e = db.PingContext(ctx); e != nil {
		emit(log, "migration.failed", "MIGRATION_CONNECT_FAILED", "The migration database is unavailable.")
		return e
	}
	if e = migrations.Run(ctx, db, args[0], out); e != nil {
		if errors.Is(e, migrations.ErrAtBase) {
			emit(log, "migration.failed", "MIGRATION_AT_BASE", "No applied migration is available to reverse.")
		} else {
			emit(log, "migration.failed", migrationCode(args[0]), migrationMessage(args[0]))
		}
		return e
	}
	log.Info("migration.completed", slog.String("operation", args[0]))
	return nil
}
func emit(l *slog.Logger, event, code, message string) {
	l.Error(event, slog.String("code", code), slog.String("message", message))
}
func migrationCode(op string) string {
	switch op {
	case "up":
		return "MIGRATION_APPLY_FAILED"
	case "down":
		return "MIGRATION_ROLLBACK_FAILED"
	default:
		return "MIGRATION_STATUS_FAILED"
	}
}
func migrationMessage(op string) string {
	switch op {
	case "up":
		return "Migration could not be applied."
	case "down":
		return "Migration could not be reversed."
	default:
		return "Migration status could not be read."
	}
}
