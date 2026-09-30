# Module Specification: Application Lifecycle (DES-029)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/app` |
| Requirements | REQ-010, NFR-001, NFR-009, NFR-013, DOM-001 |
| Depends on | DES-020..DES-022, DES-028 |
| Depended on by | DES-030 server command |

## Responsibility and interface

Run one composed server instance and close its resources in deterministic bounded order.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `Run(ctx,resources)` | signal-aware context, server, Redis, PostgreSQL, logger | terminal error/nil | listen, timeout, cleanup category | resources fully initialized | New intake stopped; all resources close at most once |

Start listening, distinguish expected `http.ErrServerClosed`, then await cancellation or listen failure. On shutdown create a fresh timeout context (not the already-cancelled run context), call HTTP `Shutdown`; on deadline call `Close`; then close Redis and PostgreSQL. Aggregate safe error categories while preserving non-zero terminal status. Time O(k) for k resources plus drain time; space O(k).

One goroutine may report listen termination through a buffered channel; no unbounded goroutine creation exists. `sync.Once`/single ownership prevents duplicate cleanup if listen and signal race. Tests inject server/resource interfaces, controlled active requests, a fake clock/deadline, ordered close recorder, and repeated cancellation.

