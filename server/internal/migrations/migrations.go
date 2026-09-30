// Package migrations owns embedded explicit SQL migrations (DES-023).
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io"

	"github.com/pressly/goose/v3"
)

//go:embed sql/*.sql
var files embed.FS

const Latest int64 = 11

var ErrAtBase = errors.New("migration at base")

func Run(ctx context.Context, db *sql.DB, op string, out io.Writer) error {
	goose.SetBaseFS(files)
	if e := goose.SetDialect("postgres"); e != nil {
		return errors.New("migration dialect failed")
	}
	from, e := goose.GetDBVersionContext(ctx, db)
	if e != nil {
		return errors.New("migration status failed")
	}
	result := map[string]any{"status": "ok", "operation": op}

	switch op {
	case "up":
		if e = goose.UpContext(ctx, db, "sql"); e != nil {
			return errors.New("migration apply failed")
		}
		to, x := goose.GetDBVersionContext(ctx, db)
		if x != nil {
			return errors.New("migration status failed")
		}
		result["from"] = from
		result["to"] = to
		result["applied"] = to - from
	case "down":
		if from == 0 {
			return ErrAtBase
		}
		if e = goose.DownContext(ctx, db, "sql"); e != nil {
			return errors.New("migration rollback failed")
		}
		to, x := goose.GetDBVersionContext(ctx, db)
		if x != nil {
			return errors.New("migration status failed")
		}
		result["from"] = from
		result["to"] = to
		result["reverted"] = 1
	case "status":
		result["current"] = from
		result["latest"] = Latest
		result["dirty"] = false
	default:
		return errors.New("unknown migration operation")
	}

	return json.NewEncoder(out).Encode(result)
}
