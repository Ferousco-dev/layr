package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ferousco-dev/layr/server/internal/config"
)

func TestOpenRejectsMalformedURL(t *testing.T) {
	_, err := Open(context.Background(), config.Postgres{URL: "not a url", MaxConnections: 2})

	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestOpenReportsUnreachableServerWithoutLeakingURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := Open(ctx, config.Postgres{
		URL:            "postgres://user:secret@127.0.0.1:1/db",
		MaxConnections: 2,
	})

	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if err.Error() != ErrUnavailable.Error() {
		t.Fatalf("error text carries detail: %q", err.Error())
	}
}

func TestIsUnavailableSeparatesOutagesFromOrdinaryErrors(t *testing.T) {
	for name, err := range map[string]error{
		"connection refused":  &net.OpError{Op: "dial", Err: errors.New("connection refused")},
		"server shutdown":     &pgconn.PgError{Code: "57P01"},
		"connection failure":  &pgconn.PgError{Code: "08006"},
		"too many connection": &pgconn.PgError{Code: "53300"},
		"deadline":            context.DeadlineExceeded,
		"wrapped":             fmt.Errorf("query: %w", &pgconn.PgError{Code: "57P03"}),
	} {
		if !IsUnavailable(err) {
			t.Errorf("%s should count as unavailable", name)
		}
	}
	for name, err := range map[string]error{
		"nil":           nil,
		"plain":         errors.New("boom"),
		"unique":        &pgconn.PgError{Code: "23505"},
		"syntax":        &pgconn.PgError{Code: "42601"},
		"client leaves": context.Canceled,
	} {
		if IsUnavailable(err) {
			t.Errorf("%s must not count as unavailable", name)
		}
	}
}
