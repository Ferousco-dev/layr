// Command server composes and runs the M1.1 modular monolith (DES-030).
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ferousco-dev/layr/server/internal/app"
	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/health"
	"github.com/ferousco-dev/layr/server/internal/httpapi"
	"github.com/ferousco-dev/layr/server/internal/observability"
	"github.com/ferousco-dev/layr/server/internal/postgres"
	red "github.com/ferousco-dev/layr/server/internal/redis"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

func main() {
	if e := run(); e != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, e := config.Load(os.LookupEnv)
	level := slog.LevelInfo
	if e == nil {
		level = cfg.LogLevel
	}
	log := observability.New(os.Stderr, level)
	if e != nil {
		var fe *config.FieldError
		field := "configuration"
		code := "CONFIG_INVALID"
		message := "Configuration is invalid."
		if errors.As(e, &fe) {
			field = fe.Name
			if fe.Kind == "missing" {
				code = "CONFIG_MISSING"
				message = "Required configuration is missing."
			}
		}
		log.Error(
			"startup.failed",
			slog.String("code", code),
			slog.String("message", message),
			slog.String("field", field),
		)
		return e
	}

	if e := workspace.Prepare(cfg.WorkspaceRoot); e != nil {
		log.Error(
			"startup.failed",
			slog.String("code", "WORKSPACE_UNAVAILABLE"),
			slog.String("message", "The temporary workspace root could not be prepared."),
		)
		return e
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()
	pg, e := postgres.Open(ctx, cfg.Postgres)
	if e != nil {
		log.Error(
			"startup.failed",
			slog.String("code", "POSTGRES_UNAVAILABLE"),
			slog.String("message", "PostgreSQL is unavailable during startup."),
			slog.String("dependency", "postgres"),
		)
		return e
	}
	rd, e := red.Open(ctx, cfg.Redis)
	if e != nil {
		pg.Close()
		log.Error(
			"startup.failed",
			slog.String("code", "REDIS_UNAVAILABLE"),
			slog.String("message", "Redis is unavailable during startup."),
			slog.String("dependency", "redis"),
		)
		return e
	}
	authOpts, authSvc, e := newAuth(cfg.Auth, pg, rd, log)
	if e != nil {
		_ = rd.Close()
		pg.Close()
		log.Error(
			"startup.failed",
			slog.String("code", "CONFIG_INVALID"),
			slog.String("message", "Configuration is invalid."),
			slog.String("field", "CREDENTIAL_ENCRYPTION_KEY"),
		)
		return e
	}
	figmaLimiter := figma.NewLimiter(cfg.Import.FigmaRequestsPerMinute)
	importSvc, designSvc, e := newImports(cfg, pg, authSvc, authOpts.Projects, figmaLimiter, log)
	if e != nil {
		_ = rd.Close()
		pg.Close()
		log.Error(
			"startup.failed",
			slog.String("code", "WORKSPACE_UNAVAILABLE"),
			slog.String("message", "The import workspace could not be prepared."),
		)
		return e
	}
	accountSvc, e := newAccount(cfg, pg, log)
	if e != nil {
		_ = rd.Close()
		pg.Close()
		log.Error(
			"startup.failed",
			slog.String("code", "CONFIG_INVALID"),
			slog.String("message", "Configuration is invalid."),
			slog.String("field", "CREDENTIAL_ENCRYPTION_KEY"),
		)
		return e
	}
	authOpts.Account = accountSvc
	generationSvc := newGeneration(pg, designSvc, accountSvc, log)
	generationSvc.Sweep(context.Background())
	authOpts.Generations = generationSvc
	authOpts.Plans = newPlans(pg, designSvc, log)
	authOpts.Imports = importSvc
	authOpts.Design = designSvc
	authOpts.TrustedProxies = cfg.HTTP.TrustedProxies
	authOpts.Limits = httpapi.Limits{
		Login:  cfg.HTTP.RateLimits.Login,
		API:    cfg.HTTP.RateLimits.API,
		Write:  cfg.HTTP.RateLimits.Write,
		Import: cfg.HTTP.RateLimits.Import,
	}
	svc := health.New(pg, rd, cfg.ReadinessProbeTimeout, cfg.ReadinessTimeout, log)
	srv := httpapi.NewServer(cfg.HTTP, httpapi.NewHandler(svc, log, httpapi.Options{
		FrontendOrigin: cfg.FrontendOrigin,
		MaxBodyBytes:   cfg.HTTP.MaxBodyBytes,
		Auth:           authOpts,
	}))

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	force := make(chan struct{})
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	go func() {
		<-signals
		cancelRun()
		<-signals
		close(force)
	}()

	return app.Run(runCtx, force, srv, rd, pg, log, cfg.HTTP.ShutdownTimeout, func(ctx context.Context) {
		importSvc.Close(ctx)
		generationSvc.Close(ctx)
	})
}
