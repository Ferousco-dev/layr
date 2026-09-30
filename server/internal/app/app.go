// Package app controls ordered server lifecycle (DES-029).
package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type Redis interface{ Close() error }
type Postgres interface{ Close() }

type serverControl interface {
	Serve(net.Listener) error
	Shutdown(context.Context) error
	Close() error
}

func Run(
	ctx context.Context,
	force <-chan struct{},
	s *http.Server,
	rd Redis,
	pg Postgres,
	log *slog.Logger,
	grace time.Duration,
	stop func(context.Context),
) error {
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		_ = rd.Close()
		pg.Close()
		log.Error(
			"startup.failed",
			slog.String("code", "LISTEN_FAILED"),
			slog.String("message", "The HTTP listener could not start."),
		)
		return errors.New("listen failed")
	}

	log.Info("server.started", slog.String("address", s.Addr))
	return runLifecycle(ctx, force, s, listener, rd, pg, log, grace, stop)
}

func runLifecycle(
	ctx context.Context,
	force <-chan struct{},
	server serverControl,
	listener net.Listener,
	rd Redis,
	pg Postgres,
	log *slog.Logger,
	grace time.Duration,
	stop func(context.Context),
) error {
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()

	select {
	case serveErr := <-serveDone:
		runStop(stop, grace)
		closeErr := closeDependencies(rd, pg)
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return errors.Join(errors.New("listen failed"), closeErr)
		}
		return closeErr
	case <-ctx.Done():
	}

	log.Info("shutdown.started")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	var out error
	cleanupFailed := false
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(shutdownCtx) }()

	select {
	case shutdownErr := <-shutdownDone:
		if shutdownErr != nil {
			out = errors.New("shutdown failed")
			cleanupFailed = true
		}
	case <-force:
		_ = server.Close()
		log.Warn("shutdown.forced")
		out = errors.New("shutdown forced")
	case <-shutdownCtx.Done():
		_ = server.Close()
		log.Error("shutdown.forced", slog.String("code", "SHUTDOWN_TIMEOUT"))
		out = errors.New("shutdown timeout")
	}

	runStop(stop, grace)
	if closeErr := closeDependencies(rd, pg); closeErr != nil {
		out = errors.Join(out, closeErr)
		cleanupFailed = true
	}

	if out != nil {
		if cleanupFailed {
			log.Error("shutdown.failed", slog.String("code", "SHUTDOWN_FAILED"))
		}
		return out
	}
	log.Info("shutdown.completed")
	return nil
}

// runStop lets background work (imports) finish or cancel before its dependencies close.
func runStop(stop func(context.Context), grace time.Duration) {
	if stop == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	stop(ctx)
}

// closeDependencies releases Redis then PostgreSQL on every terminal path.
func closeDependencies(rd Redis, pg Postgres) error {
	var err error
	if closeErr := rd.Close(); closeErr != nil {
		err = errors.New("redis close failed")
	}
	pg.Close()
	return err
}
