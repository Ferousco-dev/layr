# Module Specification: Redis Adapter (DES-022)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/redis` |
| Requirements | REQ-003, REQ-007, NFR-001, NFR-005..006, DOM-001..002 |
| Depends on | DES-019; `go-redis/v9` |
| Depended on by | DES-024, DES-029, DES-030 |

## Responsibility and interface

Own one bounded Redis client used only for connectivity/readiness in M1.1.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `Open(ctx,cfg)` | bounded context, validated config | client owner | `connect`, `timeout`, `cancelled` | cfg pool > 0 | Success implies ping; failure closes client |
| `Ping(ctx)` | caller-bounded context | error/nil | typed dependency class | owner open | No URL/raw error escapes |
| `Close()` | none | error/nil | `close` category | any state | At most one physical close |

No Get/Set/key interface is exposed; adding one requires a requirement and key/retention design. Operations are O(1) locally/network-bound. `go-redis` is concurrency-safe; lifecycle ownership is once-guarded. Inject client factory/pinger/close spy and prove Redis reset has no durable application effect.

