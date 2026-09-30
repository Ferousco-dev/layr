package postgres

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUnavailable reports whether err means the database cannot be reached right now, so callers can answer 503 and the client can retry.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var connect *pgconn.ConnectError
	var pg *pgconn.PgError
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return false
	case errors.As(err, &connect), errors.As(err, &netErr), errors.Is(err, context.DeadlineExceeded), errors.Is(err, io.ErrUnexpectedEOF):
		return true
	case errors.As(err, &pg):
		return strings.HasPrefix(pg.Code, "08") || strings.HasPrefix(pg.Code, "57P") || pg.Code == "53300"
	}
	return false
}
