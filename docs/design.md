# Software Design Description: LAYR Server M1.1 Foundation

| Field | Value |
| --- | --- |
| Document ID | SDD-LAYR-M1.1-v1 |
| Baselined SRS | SRS-LAYR-M1.1-v1 |
| Author | Ìlànà architect |
| Input reviewers | Ìlànà analyst (requirements), ethics officer (security/privacy), 2026-09-29 |
| Date | 2026-09-29 |
| Scope | M1.1 only |

## 1. Introduction

This design turns the approved M1.1 requirements into a small Go modular monolith. It defines configuration, PostgreSQL, Redis, explicit SQL migrations, HTTP lifecycle, health surfaces, correlation, redacted structured logging, graceful shutdown, and a reproducible local stack. It deliberately does not design or create identity, OAuth, project, import, asset, Design IR, generation, export, or sandbox features.

Normative inputs are `docs/srs.md`, `.ilana/decisions.md`, `.ilana/ethics.md`, and `docs/data-model.md`. Exact human-visible response and command contracts are baselined in Phase 03; examples here identify architectural constraints rather than replacing that interface specification.

## 2. Architecture design — level 1

### 2.1 Decomposition style

**DES-001 — Single-process modular monolith.** One server process is composed from inward-facing Go packages and replaceable outbound adapters. Package boundaries, constructor injection, and narrow consumer-owned interfaces provide separation without introducing an M1.1 network boundary. This satisfies REQ-001..REQ-012, NFR-011, NFR-012, NFR-015, and DOM-001.

**DES-002 — Ordered lifecycle.** Startup is a bounded sequence—configuration, logger, PostgreSQL, Redis, HTTP assembly, listen—and shutdown reverses ownership—stop HTTP intake/drain, close Redis, close PostgreSQL, finish logging. Startup and cleanup return classified errors rather than terminating from library packages. This satisfies REQ-001..REQ-003, REQ-005, REQ-009, REQ-010, NFR-001, NFR-002, NFR-009, NFR-013, and DOM-001.

**DES-003 — PostgreSQL adapter and authority.** A bounded `pgxpool` is the only durable application-data connection; callers see only the health/close capabilities they consume. This satisfies REQ-002, REQ-007, NFR-001, NFR-005, NFR-006, DOM-001, and DOM-002.

**DES-004 — Redis ephemeral adapter.** A bounded `go-redis` client provides an M1.1 connectivity/readiness boundary but owns no durable record. This satisfies REQ-003, REQ-007, NFR-001, NFR-005, NFR-006, DOM-001, and DOM-002.

**DES-005 — Bounded HTTP edge.** A standard-library `net/http` server owns request limits, deadlines, liveness/readiness routing, correlation, stable error encoding, and completion logging. This satisfies REQ-005..REQ-009, NFR-002..NFR-005, NFR-007, NFR-008, NFR-013, and DOM-001.

**DES-006 — Structured observability boundary.** Standard-library `log/slog` emits JSON to an injected writer. Call sites use an allow-list of safe fields; a redacting handler is a defense-in-depth boundary, not permission to pass raw configuration or headers. This satisfies REQ-001, REQ-002, REQ-003, REQ-004, REQ-006..REQ-010, NFR-007, and NFR-008.

**DES-007 — Explicit migration and local-operations boundary.** Versioned `.sql` files are embedded into a dedicated migration command using `goose`; Docker Compose supplies only PostgreSQL and Redis. Documentation and verification commands make the local system reproducible. This satisfies REQ-004, REQ-011, REQ-012, NFR-010..NFR-015, and DOM-003.

ADRs: [ADR-001](adr/ADR-001-modular-monolith.md), [ADR-002](adr/ADR-002-http-and-dependencies.md), [ADR-003](adr/ADR-003-sql-migrations.md), and [ADR-004](adr/ADR-004-lifecycle-and-readiness.md).

### 2.2 Component view

| Component | Responsibility | Owns data/state | Talks to |
| --- | --- | --- | --- |
| Server command | Compose and run one application instance | Process lifecycle only | Config, logger, adapters, HTTP, OS signals |
| Migration command | Apply, reverse, and inspect ordered SQL migrations | No application data; obtains migration lock/version via tool | Config, PostgreSQL, embedded SQL |
| Configuration | Parse and validate environment into immutable typed values | In-memory configuration for process lifetime | Server/migration composition roots |
| PostgreSQL adapter | Establish, verify, expose, and close bounded durable connection pool | Connections only; PostgreSQL owns durable state | PostgreSQL server, readiness |
| Redis adapter | Establish, verify, expose, and close bounded ephemeral client | Connections only; Redis data is non-authoritative | Redis server, readiness |
| HTTP edge | Enforce protocol bounds and route operational endpoints | In-flight request state only | Health, middleware, logger |
| Observability | Encode allow-listed JSON events and correlation | Output stream only; no retained business data | All lifecycle/HTTP modules |
| Docker/local ops | Reproduce dependency topology and health checks | Named developer database volumes | Docker engine, PostgreSQL, Redis |

External dependency communication is TCP. Internal communication is direct typed function calls. No internal network RPC, broker, global service locator, or mutable package singleton is introduced.

### 2.3 Conceptual integrity

LAYR M1.1 is one bounded process whose composition root owns every resource and whose internal modules expose only the capability each consumer needs. Durable truth lives only in PostgreSQL, ephemeral availability in Redis is explicit, and all ingress, waits, logs, and shutdown work are bounded and safe by construction.

### 2.4 Non-functional mechanisms

| NFR | Mechanism | Design location | Verification hook |
| --- | --- | --- | --- |
| NFR-001 | 10 s startup context covers dependency initialization; fail before listener creation | DES-002, DES-021, DES-022, DES-030 | Inject dial/probe functions and clock |
| NFR-002 | Positive typed durations wired directly to `http.Server` and shutdown context | DES-019, DES-028, DES-029 | Inspect constructed server/config tests |
| NFR-003 | `http.MaxBytesReader` at body-bearing route boundary; default 1 MiB | DES-019, DES-027 | Over-limit request test |
| NFR-004 | Liveness is constant-time in-memory encoding, with no dependency call | DES-025 | Dependency spies and benchmark |
| NFR-005 | Concurrent independent probes, each with ≤500 ms child deadline and ≤1 s aggregate | DES-024 | Blocking fakes and elapsed-time assertion |
| NFR-006 | Validated positive pool sizes passed to pgx/Redis options, default 10 | DES-019, DES-021, DES-022 | Config and pool-stat tests |
| NFR-007 | JSON `slog`, safe-field allow-list, case-insensitive redaction, never log raw URLs/config/headers | DES-020, DES-027 | Seeded-secret scan |
| NFR-008 | Validate or generate request ID once, store in context, set response header, reuse in completion log | DES-026, DES-027 | Capturing logger/handler test |
| NFR-009 | Signal cancels run context; listener stops/drains first; cleanup uses bounded context | DES-002, DES-029, DES-030 | Controlled active request and close spies |
| NFR-010 | Embedded ordered up/down SQL and single migration runner; repeat cycle three times | DES-023 | Fresh PostgreSQL acceptance cycle |
| NFR-011 | Minimal dependencies and standard Go package layout; CI runs named commands | DES-001, DES-018, DES-030 | Command exit evidence |
| NFR-012 | No Figma package, endpoint, host, credential, or test dependency in M1.1 | DES-001, DES-018 | Dependency/outbound-network review |
| NFR-013 | No per-item workers; only server-managed goroutines and bounded concurrent readiness pair | DES-024, DES-028, DES-029 | Race test and goroutine completion test |
| NFR-014 | Pinned service images, health checks, dependency ordering, documented clean-start script | DES-018 | 3 clean-stack timings |
| NFR-015 | Runtime manifest/Compose contain exactly PostgreSQL and Redis as external data services | DES-001, DES-018 | Manifest review |

### 2.5 Trust and security boundaries

1. **Environment → configuration (DES-019):** all environment text is untrusted. Parse by an explicit name table, validate types/ranges, and report only the variable name and reason. Never place raw values in errors.
2. **Network client → HTTP edge (DES-026..DES-028):** method, path, headers, and bodies are untrusted. Only `GET /health` and `GET /ready` exist; body limits are installed before any future decoder; invalid request IDs are replaced, not reflected.
3. **Application → PostgreSQL/Redis (DES-021..DES-024):** connection URLs are secret-bearing configuration. Adapters accept typed configuration, use bounded contexts, and translate errors into categories without returning DSNs.
4. **Application → logs (DES-020):** every field crossing this boundary must be public operational metadata. Authorization, cookie, token, session, password, secret, and database URL keys/values are forbidden and defense-in-depth redacted.
5. **Host → process lifecycle (DES-029..DES-030):** SIGINT/SIGTERM are the only shutdown control inputs. Cleanup is idempotent and each owned resource closes at most once.
6. **Repository → dependencies (ADR-002/003):** direct libraries are licence-recorded and version-pinned in `go.mod`; dependency vulnerability review belongs to construction/SCM evidence.

M1.1 contains no authentication or tenant data; health surfaces disclose only stable status labels and never build versions, hosts, connection errors, addresses, or credentials.

### 2.6 Failure and degradation behaviour

| Dependency/event | Behaviour | Externally visible effect |
| --- | --- | --- |
| Invalid/missing configuration | Classified startup error; no listener or dependency initialization | Process exits non-zero within NFR-001 |
| PostgreSQL unavailable at startup | Bounded probe fails; close partially built resources; do not listen | Process exits non-zero; secret-free event |
| Redis unavailable at startup | Same fail-closed startup policy | Process exits non-zero; secret-free event |
| Either dependency fails after startup | `/health` remains 200; `/ready` is 503 with only per-service state | Orchestrator may remove instance from service |
| Readiness probe hangs | Child timeout marks that check `timeout`; aggregate completes within 1 s | 503, never indefinite wait |
| Logging write fails | Request/lifecycle continues; never retry in an unbounded loop or panic | Missing log record; observable only through injected test writer/host stderr behaviour |
| Response encoding fails before headers | Safe 500 where possible; secret-free error event | 500 stable error surface |
| Client sends excessive body | Stop reading at configured bound | 413 for body-bearing routes; health routes remain available |
| SIGINT/SIGTERM | Stop new intake, drain within grace, then close Redis/PostgreSQL once | In-flight work may finish; timeout yields non-zero exit |
| Migration failure | Stop at failing version; report version/direction/category without SQL URL | Migration command exits non-zero; server does not auto-migrate |
| Docker dependency unhealthy | Health-gated server start/readiness remains unavailable | Documented recovery; no false-ready state |

## 3. High-level design — level 2

### 3.1 Modules

| ID | Package/module | Single responsibility | Requirements |
| --- | --- | --- | --- |
| DES-008 | `internal/config` | Produce one validated immutable configuration value | REQ-001, REQ-005, NFR-001..003, NFR-005..006, NFR-009, NFR-014 |
| DES-009 | `internal/observability` | Construct a JSON logger that permits only safe structured fields | REQ-009, NFR-007..008 |
| DES-010 | `internal/postgres` | Open, verify, and close a bounded PostgreSQL pool | REQ-002, REQ-007, NFR-001, NFR-005..006, DOM-001..002 |
| DES-011 | `internal/redis` | Open, verify, and close a bounded ephemeral Redis client | REQ-003, REQ-007, NFR-001, NFR-005..006, DOM-001..002 |
| DES-012 | `internal/migrations` | Run ordered embedded SQL migration operations | REQ-004, NFR-010, DOM-003 |
| DES-013 | `internal/health` | Evaluate and encode liveness/readiness state | REQ-006..007, NFR-004..005 |
| DES-014 | `internal/httpapi/middleware` | Bound and correlate inbound HTTP work and emit completion events | REQ-005, REQ-008..009, NFR-003, NFR-007..008, DOM-001 |
| DES-015 | `internal/httpapi` | Assemble the M1.1 router and configured HTTP server | REQ-005..007, NFR-002..005, NFR-013 |
| DES-016 | `internal/app` | Coordinate signal-driven run and deterministic cleanup | REQ-010, NFR-001, NFR-009, NFR-013, DOM-001 |
| DES-017 | `cmd/server`, `cmd/migrate` | Compose concrete dependencies and map terminal errors to exit status | REQ-001..REQ-010, NFR-001, NFR-011..013 |
| DES-018 | Docker, Makefile/tasks, README | Reproduce and explain local operation and verification | REQ-011..REQ-012, NFR-010..015 |

Detailed contracts are in [`docs/modules/`](modules/README.md).

### 3.2 Public module interfaces

| Module | Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- | --- |
| Config | `Load(lookup)` | Environment lookup | `Config` | Named missing/invalid variable | Lookup must not mutate while loading | Fully valid value or zero usable config |
| Observability | `New(writer, level)` | Writer, validated level | `*slog.Logger` | Configuration error only | Writer non-nil | JSON line handler with redaction |
| PostgreSQL | `Open(ctx, cfg)` | Context, secret-bearing DB config | Pool handle | startup category, no DSN | Valid config; bounded context | Ping succeeded or resource closed |
| PostgreSQL | `Ping(ctx)`, `Close()` | Bounded context / none | nil or error / none | dependency category | Open handle | Close is idempotent |
| Redis | `Open(ctx, cfg)` | Context, secret-bearing Redis config | Client handle | startup category, no URL | Valid config; bounded context | Ping succeeded or resource closed |
| Redis | `Ping(ctx)`, `Close()` | Bounded context / none | nil or error / error | dependency/close category | Open handle | Close is idempotent |
| Migrations | `Run(ctx, operation)` | Context, `up|down|status` | Status including current version | operation/version category | DB config valid | SQL history advanced/reversed or unchanged |
| Health | `Liveness(w,r)` | HTTP request | Stable JSON response | encoding/internal | Process running | Does not call dependencies |
| Health | `Readiness(w,r)` | HTTP request, injected checkers | Stable JSON check map | represented as 503; encoding/internal | Checkers present | All checks settle by aggregate deadline |
| Middleware | `RequestID(next)` | HTTP handler | HTTP handler | none | next non-nil | Exactly one validated/generated ID propagated |
| Middleware | `AccessLog(next)` | HTTP handler/logger | HTTP handler | none | Request-ID middleware outside it | One safe completion event per completed request |
| Middleware | `LimitBody(next,bytes)` | HTTP handler, positive limit | HTTP handler | 413 at read boundary | Installed before body decoder | Reader cannot consume beyond limit |
| HTTP API | `NewHandler(deps)` | Health handlers/logger | `http.Handler` | assembly error | Dependencies non-nil | Only M1.1 routes registered |
| HTTP API | `NewServer(cfg,handler)` | Valid config/handler | `*http.Server` | assembly error | Timeouts positive | All configured limits installed |
| App | `Run(ctx,deps)` | Signal-aware context, owned resources | error | listen/shutdown/cleanup category | Fully composed instance | Resources closed once in defined order |

The exact HTTP JSON and CLI grammar is owned by Phase 03. The architecture requires stable machine-readable status, no raw dependency errors, and non-zero exits for failed migration/startup operations.

### 3.3 Dependency graph

```text
cmd/server (DES-017)
  -> config (DES-008)
  -> observability (DES-009)
  -> postgres (DES-010)
  -> redis (DES-011)
  -> httpapi (DES-015) -> health (DES-013)
                        -> middleware (DES-014)
  -> app lifecycle (DES-016)

cmd/migrate (DES-017)
  -> config (DES-008)
  -> observability (DES-009)
  -> migrations (DES-012) -> embedded migrations/*.sql -> PostgreSQL

health (DES-013) -> consumer-owned `Checker` interfaces
postgres/redis implement those interfaces without importing health
```

Arrows point from composition/consumer to dependency. The health package owns the smallest interface it consumes, preventing adapter packages from depending on HTTP. Adapter packages do not import one another. Configuration exposes values but imports no adapter. There are no accepted cycles.

### 3.4 Data flows

**Startup:** environment → DES-008 validation → DES-009 logger → DES-010 PostgreSQL bounded open/ping → DES-011 Redis bounded open/ping → DES-013/014/015 HTTP assembly → DES-016 listen. Any failure unwinds only resources already created and never exposes configuration values.

**Liveness/readiness request:** client → DES-026 correlation → DES-027 access logging → DES-015 route → DES-025 liveness (no I/O) or DES-024 two bounded probes → stable encoder → response/header → completion event carrying the same request ID.

**Migration:** operator argument → DES-008 database config → DES-023 `up|down|status` allow-list → embedded ordered SQL → PostgreSQL transaction/lock semantics provided by selected migration tool → status output including current version. Migration is never an implicit server-start side effect.

**Shutdown:** OS signal/context cancellation → DES-029 stop intake and bounded drain → close Redis → close PostgreSQL → finish logging/output → DES-030 return terminal status. A second signal or expired deadline forces server close; cleanup remains once-only.

## 4. Detailed design — level 3

| ID | Detailed element | Core invariant | Complexity | Specification |
| --- | --- | --- | --- | --- |
| DES-019 | Typed configuration | A returned config is complete, positive, bounded, and contains no logged/stringified secret | O(v), v = documented variables | [configuration](modules/configuration.md) |
| DES-020 | Safe JSON logging | One JSON object per event; unsafe keys/values never cross output boundary | O(a), a = attributes | [observability](modules/observability.md) |
| DES-021 | PostgreSQL pool | Successful construction implies bounded pool and successful ping | O(1) per operation, network-bound | [postgres](modules/postgres.md) |
| DES-022 | Redis client | Successful construction implies bounded ephemeral client and successful ping | O(1), network-bound | [redis](modules/redis.md) |
| DES-023 | SQL migration runner | Only known operations execute ordered embedded SQL; no auto-mutation | O(m), m = migrations traversed | [migrations](modules/migrations.md) |
| DES-024 | Readiness evaluator | 200 iff both independent probes succeed before bounds; result contains no raw errors | O(1), at most two bounded goroutines | [health](modules/health.md) |
| DES-025 | Liveness handler | Never performs dependency I/O | O(1) time/space | [health](modules/health.md) |
| DES-026 | Request correlation | Exactly one accepted/generated opaque ID is shared by context, response, and log | O(n), n = supplied ID length, capped | [middleware](modules/middleware.md) |
| DES-027 | Body/access middleware | Body reads are bounded and each completed request yields one safe event | O(1) aside from downstream work | [middleware](modules/middleware.md) |
| DES-028 | HTTP assembly/server | Only declared routes exist and every server timeout is positive/configured | O(r), r = routes (constant in M1.1) | [httpapi](modules/httpapi.md) |
| DES-029 | Shutdown coordinator | Intake stops first; resources close at most once within one deadline | O(k), k = owned resources | [lifecycle](modules/lifecycle.md) |
| DES-030 | Composition and local operations | Commands contain wiring only; service manifest has exactly two external data services | O(1) composition | [composition](modules/composition.md) |

All request operations are safe for concurrent invocation through immutable configuration, concurrency-safe standard/library clients, request-local context, and no mutable global state. Construction and shutdown are single-owner operations; `Close` paths are guarded for idempotence. Detailed error tables and test seams live in the linked module specifications.

## 5. Data model

M1.1 has no domain entities or application tables. The only persistent relation is migration-tool metadata used to serialize and record schema history; its exact physical shape is tool-owned and not application-readable. See [data model](data-model.md). Redis holds no required application state in M1.1 and may be flushed without durable loss.

## 6. Package and repository structure

```text
server/
├── cmd/
│   ├── server/main.go
│   └── migrate/main.go
├── internal/
│   ├── app/
│   ├── config/
│   ├── health/
│   ├── httpapi/
│   │   └── middleware/
│   ├── migrations/
│   ├── observability/
│   ├── postgres/
│   └── redis/
├── migrations/
│   ├── 000001_foundation.up.sql
│   └── 000001_foundation.down.sql
├── Dockerfile
├── go.mod
└── go.sum
docker-compose.yml
Makefile
```

The foundation migration is intentionally reversible and contains no speculative domain tables. If the selected migration tool requires a metadata relation, that relation is created and owned by the tool. Empty SQL with documented intent is acceptable only if the tool records a version; otherwise construction must use a harmless, reversible schema-level marker rather than inventing application data.

## 7. Dependency and licence record

| Dependency | Purpose tied to requirement | Licence | Boundary/rationale |
| --- | --- | --- | --- |
| Go standard library (`net/http`, `log/slog`, `context`, `os/signal`, `crypto/rand`) | HTTP, JSON logs, cancellation, signals, request IDs | BSD-3-Clause | Prefer built-ins; no web framework or UUID package needed |
| `github.com/jackc/pgx/v5` | PostgreSQL pool and context-aware probes (REQ-002) | MIT | Native Go PostgreSQL driver with bounded `pgxpool`; no ORM/schema mutation |
| `github.com/redis/go-redis/v9` | Redis client and context-aware probes (REQ-003) | BSD-2-Clause | Focused official Go client; Redis remains ephemeral |
| `github.com/pressly/goose/v3` | Ordered SQL up/down/status/version operations (REQ-004) | Apache-2.0 | Library embedding supports a dedicated command and explicit SQL without runtime CLI installation |
| PostgreSQL container image | Local durable service (REQ-011) | PostgreSQL Licence | Pin supported major/digest during construction |
| Redis-compatible container image | Local ephemeral service (REQ-011) | Version/distribution-specific; unverified until construction selects the image | Pin an approved version and verify its exact licence before SCM baseline; Valkey remains a possible compatible alternative if requirements and client verification permit it |

Licence strings must be verified against the exact versions selected in `go.mod` and image manifests before G4/G6; this design record does not claim a dependency has been downloaded or audited. No dotenv library, HTTP router, configuration framework, ORM, UUID library, broker, or telemetry backend is justified in M1.1.

## 8. Design objective assessment

| Objective | Assessment | Evidence |
| --- | --- | --- |
| Correctness | Every approved REQ/NFR/DOM maps to at least one DES element; startup, serving, readiness, migration, and shutdown failure paths are explicit. | Sections 2.4, 7, 9; module specs |
| Completeness | Architecture, high-level modules, interfaces, dependencies, data flow, detailed invariants, data ownership, failures, operations, and licences are defined. | Sections 2–7; `docs/data-model.md` |
| Efficiency | Health is O(1); readiness uses two bounded concurrent probes; pools are bounded; no broker/process boundary or polling loop exists. | DES-003..005, DES-021..024 |
| Flexibility | Consumer-owned interfaces and one composition root allow later adapters/workers without making M1.1 abstract or distributed. | Dependency graph; ADR-001 |
| Consistency | PostgreSQL alone is durable, Redis is ephemeral, migrations are explicit, and one owner controls every resource lifecycle. | DES-002..004, ADR-003/004 |
| Maintainability | Standard library first, three direct Go libraries, acyclic packages, one responsibility per module, explicit commands, and narrow test seams suit a single maintainer. | Sections 3, 6, 7; module specs |

## 9. Traceability and orphan check

| Requirement | Design elements |
| --- | --- |
| REQ-001 | DES-001, DES-002, DES-006, DES-008, DES-017, DES-019, DES-030 |
| REQ-002 | DES-002, DES-003, DES-010, DES-017, DES-021, DES-029, DES-030 |
| REQ-003 | DES-002, DES-004, DES-011, DES-017, DES-022, DES-029, DES-030 |
| REQ-004 | DES-006, DES-007, DES-012, DES-017, DES-018, DES-023, DES-030 |
| REQ-005 | DES-002, DES-005, DES-008, DES-014, DES-015, DES-017, DES-019, DES-027, DES-028 |
| REQ-006 | DES-005, DES-006, DES-013, DES-015, DES-017, DES-025, DES-028 |
| REQ-007 | DES-003..006, DES-010, DES-011, DES-013, DES-015, DES-017, DES-024, DES-028 |
| REQ-008 | DES-005, DES-006, DES-009, DES-014, DES-017, DES-020, DES-026, DES-027 |
| REQ-009 | DES-002, DES-005, DES-006, DES-009, DES-014, DES-017, DES-020, DES-027, DES-029 |
| REQ-010 | DES-002, DES-006, DES-016, DES-017, DES-029, DES-030 |
| REQ-011 | DES-007, DES-018, DES-030 |
| REQ-012 | DES-007, DES-018, DES-030 |
| NFR-001 | DES-002..004, DES-008, DES-010, DES-011, DES-016, DES-017, DES-019, DES-021, DES-022, DES-030 |
| NFR-002 | DES-002, DES-005, DES-008, DES-015, DES-019, DES-028, DES-029 |
| NFR-003 | DES-005, DES-008, DES-014, DES-019, DES-027 |
| NFR-004 | DES-005, DES-013, DES-015, DES-025, DES-028 |
| NFR-005 | DES-003..005, DES-008, DES-010, DES-011, DES-013, DES-019, DES-021, DES-022, DES-024 |
| NFR-006 | DES-003, DES-004, DES-008, DES-010, DES-011, DES-019, DES-021, DES-022 |
| NFR-007 | DES-005, DES-006, DES-009, DES-014, DES-020, DES-027 |
| NFR-008 | DES-005, DES-006, DES-009, DES-014, DES-020, DES-026, DES-027 |
| NFR-009 | DES-002, DES-008, DES-016, DES-019, DES-029, DES-030 |
| NFR-010 | DES-007, DES-012, DES-018, DES-023, DES-030 |
| NFR-011 | DES-001, DES-007, DES-017, DES-018, DES-030 |
| NFR-012 | DES-001, DES-007, DES-017, DES-018, DES-030 |
| NFR-013 | DES-002, DES-005, DES-014..017, DES-024, DES-028..030 |
| NFR-014 | DES-007, DES-008, DES-018, DES-030 |
| NFR-015 | DES-001, DES-007, DES-018, DES-030 |
| DOM-001 | DES-001..005, DES-010, DES-011, DES-014, DES-016, DES-021, DES-022, DES-024, DES-026..030 |
| DOM-002 | DES-003, DES-004, DES-010, DES-011, DES-021, DES-022 |
| DOM-003 | DES-007, DES-012, DES-023, DES-030 |

Reverse orphan review: DES-001..DES-030 each names at least one requirement in Sections 2, 3, 4 or its module specification. No architecture element authorizes later-milestone functionality.

## 10. Rejected alternatives and revisit conditions

- Microservices or separate workers: rejected because M1.1 has no independent scaling workload and a single maintainer. Revisit when an approved worker requirement has independent deployment/scaling or fault-isolation evidence.
- Full Go web framework/router: rejected because two exact GET routes and middleware compose cleanly with `net/http`. Revisit when approved route features make standard routing materially repetitive and measured maintenance cost exceeds dependency cost.
- ORM/automatic schema mutation: rejected by DOM-003. Revisit an ORM only for query mapping after domain requirements exist; never revisit automatic schema mutation without a requirement change.
- Auto-migrate on server startup: rejected because deployment and serving failure domains should remain separable. Revisit only if a deployment platform cannot run release commands and a change request specifies locking, rollback, and multi-instance behaviour.
- Sequential readiness probes: rejected because two independent 500 ms checks could consume the 1 s envelope with no encoding margin. Revisit only if a client proves unsafe for concurrent ping or measurement shows concurrency overhead dominates.
- Third-party request-ID/validation packages: rejected because 128-bit IDs from `crypto/rand` plus a small bounded syntax validator are sufficient. Revisit if a future cross-service trace standard mandates a different interoperable format.
