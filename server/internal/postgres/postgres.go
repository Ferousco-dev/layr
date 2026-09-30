// Package postgres owns the durable connection pool (DES-021).
package postgres

import (
	"context"
	"errors"
	"sync"

	"github.com/ferousco-dev/layr/server/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnavailable = errors.New("postgres unavailable")

type Pool struct {
	p    *pgxpool.Pool
	once sync.Once
}

func Open(ctx context.Context, c config.Postgres) (*Pool, error) {
	pc, e := pgxpool.ParseConfig(string(c.URL))
	if e != nil {
		return nil, ErrUnavailable
	}
	pc.MaxConns = c.MaxConnections
	p, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		return nil, ErrUnavailable
	}
	x := &Pool{p: p}
	if e = p.Ping(ctx); e != nil {
		x.Close()
		return nil, ErrUnavailable
	}
	return x, nil
}

func (p *Pool) Ping(ctx context.Context) error {
	if e := p.p.Ping(ctx); e != nil {
		return ErrUnavailable
	}
	return nil
}

func (p *Pool) Close() { p.once.Do(p.p.Close) }

// Pgx exposes the pool to stores that own explicit SQL.
func (p *Pool) Pgx() *pgxpool.Pool { return p.p }
