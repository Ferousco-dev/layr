# Rollback plan

## Application

Redeploy the previous tagged build. The server is stateless; nothing else changes.

## Database

Migrations are reversible. From `server/`, with the production `POSTGRES_URL` set:

```sh
go run ./cmd/migrate status
go run ./cmd/migrate down   # one step per call
go run ./cmd/migrate status
```

Rolling back migration 00002 drops `users`, `figma_connections` and `sessions`, which signs everyone out and discards stored Figma authorization. Take a database backup first.

## Rehearsal

| Date | Environment | Steps | Result |
| --- | --- | --- | --- |
| 2026-09-29 | local PostgreSQL 16, throwaway `layr_test` database, no Docker | `down`, `down`, `up`, `up`, `down`, `up`, `status` | 2→1→0→2, second `up` applied 0, final `current=2 dirty=false` |

Staging rehearsal with the production topology has not been performed.
