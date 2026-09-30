# Module Specification: HTTP API and Server (DES-028)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/httpapi` |
| Requirements | REQ-005..REQ-007, NFR-002..NFR-005, NFR-013 |
| Depends on | DES-024..DES-027; `net/http` |
| Depended on by | DES-029, DES-030 |

## Responsibility and interfaces

Assemble the M1.1 route tree and a fully bounded HTTP server.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `NewHandler(deps)` | health handlers, middleware dependencies | handler | nil/missing dependency | complete deps | Only exact GET `/health` and `/ready` registered |
| `NewServer(cfg,handler)` | typed HTTP config, handler | server | invalid bound/address | positive validated config | Address/read-header/read/write/idle limits set |

Wrong methods and unknown paths use stable Phase 03 error contracts and do not disclose registered internals. Server-level `MaxHeaderBytes` and all timeouts are explicit; body limits remain route middleware because `http.Server` has no whole-request body limit. Route assembly O(r), request dispatch standard-library complexity with r=2.

Handlers run concurrently; dependencies must be concurrency-safe. Server construction/listen is single-owner. Tests inspect every server field, route/method matrix, middleware ID propagation, and oversized future body-bearing test route without adding a production route.

