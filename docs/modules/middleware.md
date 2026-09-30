# Module Specification: HTTP Middleware (DES-026, DES-027)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/httpapi/middleware` |
| Requirements | REQ-005, REQ-008, REQ-009, NFR-003, NFR-007..008, DOM-001 |
| Depends on | DES-020; `net/http`; `crypto/rand` |
| Depended on by | DES-028 |

## Responsibility and interfaces

Bound, correlate, and observe each HTTP request without owning route behaviour.

| Operation | Inputs | Outputs | Failure response | Invariant |
| --- | --- | --- | --- | --- |
| `RequestID(next)` | next handler | wrapped handler | generated fallback on invalid input | Same ID in context, `X-Request-ID`, completion log |
| `AccessLog(next)` | logger/next | wrapped handler | logging cannot alter response | Exactly one completion event when handler returns |
| `LimitBody(next,max)` | positive bytes/next | wrapped handler | 413 when downstream reads beyond limit | Reader cannot consume > max |

Accept a client ID only when ASCII token syntax and a conservative maximum length pass; otherwise generate 128 random bits with `crypto/rand` and lowercase hexadecimal encoding. Random-source failure returns a stable 500 and does not invent a weak ID. Validation is O(n) with n capped; generation/logging is O(1). Capture status/bytes through a response-writer wrapper that preserves required optional interfaces only after tests establish them.

Middleware order is request ID outermost, access logging, body limit at body-bearing route group, then handler. No header/body/query values are logged. Everything is request-local and safe for concurrent use; random source and logger/writer are injectable.

