# Module Specification: PostgreSQL Adapter (DES-021)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/postgres` |
| Requirements | REQ-002, REQ-007, NFR-001, NFR-005..006, DOM-001..002 |
| Depends on | DES-019; `pgx/v5/pgxpool` |
| Depended on by | DES-024, DES-029, DES-030 |

## Responsibility and interface

Own one bounded PostgreSQL pool from verified creation through idempotent close.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `Open(ctx,cfg)` | bounded context, validated config | pool owner | `connect`, `timeout`, `cancelled` | cfg max > 0 | Success implies a ping succeeded; failure closes partial pool |
| `Ping(ctx)` | caller-bounded context | error/nil | typed dependency class | owner open | Honors cancellation; no DSN in error |
| `Close()` | none | none | none | any state | Safe once/repeated; connections released |

Translate errors at the adapter boundary without driver error text entering HTTP or normal logs. Pool stats are observable for tests. Construction/ping is O(1) locally and network-bound. `pgxpool` handles request concurrency; owner lifecycle is single-writer and close is once-guarded.

Test hooks: narrow pool/dial factory, controlled pinger, pool stats, close spy. Integration verification uses an isolated PostgreSQL instance and cancellation deadlines.

