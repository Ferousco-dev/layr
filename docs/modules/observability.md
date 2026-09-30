# Module Specification: Observability (DES-020)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/observability` |
| Requirements | REQ-009, NFR-007, NFR-008 |
| Depends on | `log/slog`, `io` |
| Depended on by | DES-023..DES-030 |

## Responsibility and interface

Construct a JSON logger that emits safe structured operational fields.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `New(writer, level)` | non-nil writer, validated level | logger | invalid setup | writer accepts bytes | Each successful record is one JSON object/line |
| Redacting handler `Handle(ctx,record)` | slog record | none | writer error per slog contract | attributes are structured | Sensitive-key values replaced; safe record forwarded |

Safe fields are an allow-list such as `event`, `request_id`, `method`, normalized route, `status`, `duration_ms`, `dependency`, and error `kind`. Raw error strings from drivers, URLs, headers, bodies, configuration structs, and arbitrary maps are prohibited. Recursive groups and case-insensitive sensitive key fragments are redacted as defense in depth.

## Algorithm, failure, concurrency

Clone/walk O(a) attributes, replace unsafe values, delegate to `slog.JSONHandler`. Logger write failure never panics request handling and causes no recursive logging. `slog` handlers must be concurrency-safe; mutable buffers belong only to test writers protected by a lock. Inject writer/handler and use decoded line capture for tests, including nested and differently cased secret keys.

