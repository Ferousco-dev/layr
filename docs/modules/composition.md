# Module Specification: Composition and Local Operations (DES-030)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `cmd/server`, `cmd/migrate`, Docker/Make/README |
| Requirements | REQ-001..REQ-012, NFR-001, NFR-009..NFR-015, DOM-001..003 |
| Depends on | DES-019..DES-029 |
| Depended on by | Operators and CI |

## Responsibility and interfaces

Wire concrete modules into two thin commands and provide reproducible local operations.

| Surface | Inputs | Outputs | Failures | Postcondition |
| --- | --- | --- | --- | --- |
| `go run ./cmd/server` | environment, SIGINT/SIGTERM | HTTP process, JSON events | non-zero safe category | Owned resources closed once |
| `go run ./cmd/migrate up|down|status` | environment, one operation | safe status/output | non-zero safe category | Requested migration action completed or stopped visibly |
| Docker Compose workflow | env example/images | healthy PostgreSQL and Redis, server connectivity | unhealthy service | Exactly two external data services |
| Verification tasks | source/toolchain/services | command evidence | original non-zero exit | No suppressed result |

Commands contain no domain logic and are the only place allowed to call `os.Exit` (after deferred cleanup has run through a `run() error` pattern). Startup acquisition records close functions immediately and unwinds in reverse on partial failure. Docker images are pinned, services have health checks, ports/default credentials are development-only, and no secret default is suitable for production.

Composition is single-threaded until server run. Tests call command `run` functions with injected lookup/writers/factories; Docker validation and 3-run clean-start timing are acceptance evidence. Interface trace: UI-003 server lifecycle and UI-004 migration CLI (Phase 03).
