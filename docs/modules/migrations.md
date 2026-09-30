# Module Specification: SQL Migrations (DES-023)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/migrations` |
| Requirements | REQ-004, NFR-010, DOM-003 |
| Depends on | DES-019; `goose/v3`; embedded `migrations/*.sql` |
| Depended on by | DES-030 migration command |

## Responsibility and interface

Execute one explicitly requested ordered SQL migration operation.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `Run(ctx,db,op,out)` | bounded context, SQL DB, `up|down|status`, writer | status/version output | invalid operation, open, migration version/direction | DB reachable; embedded FS valid | Up reaches latest, down reverses one version, status reports current history |

`status` is the sole version-reporting interface; no redundant `version` command is designed. Validate the operation before opening/executing. Goose owns metadata/locking; project SQL owns schema changes. Error presentation may contain the migration number and direction, never DSN or raw secret-bearing driver data. Time O(m) for traversed migrations, memory O(1) excluding tool internals.

Run is not safe to invoke concurrently from the same process; database/tool locking is still required across processes. Tests inject DB/tool runner/output, test unknown operation without DB calls, and run fresh apply/apply/down/apply cycles three times. Interface trace: UI-004 (Phase 03).

