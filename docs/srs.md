# Software Requirements Specification: LAYR Server M1.1 Foundation

| Field | Value |
| --- | --- |
| Document ID | SRS-LAYR-M1.1-v1 |
| Status | baselined |
| Author | Ìlànà analyst |
| Baselined on | 2026-09-29 (G1 PASS) |

## 1. Purpose and scope

This document defines M1.1 only: the Go server foundation on which later LAYR Figma capabilities depend. It covers environment configuration, PostgreSQL and Redis connectivity, explicit SQL migrations, HTTP lifecycle, structured logging, request correlation, liveness/readiness, graceful shutdown, reproducible Docker development, verification, and operator documentation.

LAYR is ultimately a public SaaS that imports private Figma designs and deterministically converts them to a framework-independent Design IR. M1.1 processes no Figma credentials or design data, but its boundaries must be safe for later M1 phases.

### Sources and repository evidence

- Product/engineering brief and stakeholder intake supplied 2026-09-29.
- Repository inspection 2026-09-29: placeholder `README.md`; no implementation, migrations, configuration, or Git history.

## 2. Overall description

The initial operator is one developer assisted by coding/review agents; limited testers follow and the eventual audience is public. Go, PostgreSQL, Redis, and Docker are mandatory. PostgreSQL is durable truth; Redis is used only for justified ephemeral coordination. The deployment is a modular monolith whose workers can later separate. Secrets come from the environment and are never committed or logged.

### Assumptions requiring conductor decisions

- **DEC-004:** readiness treats PostgreSQL and Redis as mandatory.
- **DEC-005:** local development targets Docker Compose v2 and a supported Go toolchain on macOS/Linux.
- **DEC-006:** the default graceful-shutdown ceiling is 30 seconds until traffic data exists.
- **DEC-007:** M1.1 creates migration infrastructure and a reversible foundation migration, but no speculative identity/project tables.

### Out of scope

M1.1 excludes users, sessions, Figma OAuth/token handling, projects, Figma URL/client work, imports, workspaces, assets, rendering, Design IR, AI/generation, Chromium, exports, object storage, production deployment, and production rate limiting. Per CR-001, the empty workspace root (`TEMP_WORKSPACE_ROOT`) is in scope; import-specific workspace structure remains out of scope. Later interfaces that M1.1 must not obstruct are constructor dependency injection, `context.Context`, `/api/v1`, stable typed errors, correlation fields, and separable workers. These are design inputs, not authorization to implement later phases.

## 3. Functional requirements

Each source is the stakeholder brief dated 2026-09-29 unless noted.

### REQ-001 — Configuration

The server shall load documented environment variables into a typed value and reject missing, malformed, or unsafe critical values before listening. **Acceptance:** valid `.env.example` values reach dependency initialization; invalid values exit non-zero with the variable name but no secret value. **Failure:** do not start HTTP. **Depends:** none.

### REQ-002 — PostgreSQL

The server shall create, verify, and close a bounded PostgreSQL pool. **Acceptance:** reachable PostgreSQL pings; invalid/unreachable PostgreSQL produces a classified startup failure; shutdown closes the pool. **Failure:** never advertise readiness or wait past the initialization deadline. **Depends:** REQ-001.

### REQ-003 — Redis

The server shall create, verify, and close Redis. **Acceptance:** reachable Redis pings; invalid/unreachable Redis produces a classified startup failure; shutdown closes the client. **Failure:** never advertise readiness or wait past the initialization deadline. **Depends:** REQ-001, DEC-004 proposed.

### REQ-004 — Explicit migrations

The repository shall provide repeatable commands to apply ordered SQL migrations, report version, and reverse the latest reversible migration without ORM schema mutation. **Acceptance:** empty database apply reaches latest; second apply is a no-op; rollback removes latest; apply restores it. **Failure:** exit non-zero and identify the migration without secrets. **Depends:** REQ-002, DEC-007 proposed.

### REQ-005 — Bounded HTTP server

The server shall listen on the configured address and enforce configured read-header, read, write, idle, shutdown, and request-body limits. **Acceptance:** configured address is used; over-limit bodies receive 413; server timeout values are test-covered. **Failure:** reject offending work and retain availability where safe. **Depends:** REQ-001.

### REQ-006 — Liveness

`GET /health` shall report process liveness without querying dependencies. **Acceptance:** while running it returns 200 with stable JSON and dependency spies receive zero calls. **Failure:** encoding failure returns 500 and is safely logged. **Depends:** REQ-005.

### REQ-007 — Readiness

`GET /ready` shall check PostgreSQL and Redis within bounded deadlines, returning 200 only when both succeed and otherwise 503 with stable per-dependency states. **Acceptance:** tests cover both healthy, either down, and timeout, without credentials in output. **Failure:** return 503 by the deadline. **Depends:** REQ-002, REQ-003, REQ-005.

### REQ-008 — Request correlation

Every request shall accept a syntactically valid client request ID or generate an opaque ID, put it in context, return it in a response header, and attach it to request logs. **Acceptance:** valid supplied, missing, and invalid cases yield one matching context/response/log ID. **Failure:** replace invalid input rather than echo it. **Depends:** REQ-005.

### REQ-009 — Structured redacted logs

The server shall emit structured startup, shutdown, dependency, and HTTP-completion events with timestamp, level, event/message, request ID where applicable, method, route, status, and duration, and redact sensitive names/values. **Acceptance:** events parse as JSON and seeded token/session/secret values never appear. **Failure:** logging errors neither expose values nor panic handling. **Source:** brief plus intake. **Depends:** REQ-001, REQ-008.

### REQ-010 — Graceful shutdown

On SIGINT/SIGTERM the server shall stop new work, allow active requests within the grace period, then close HTTP, PostgreSQL, Redis, and logging resources in deterministic order. **Acceptance:** integration test observes new work stop, active work complete within grace, and each close hook once. **Failure:** after deadline force close, log timeout, and exit non-zero if cleanup fails. **Depends:** REQ-002, REQ-003, REQ-005, REQ-009.

### REQ-011 — Reproducible local stack

The repository shall provide Docker definitions and one documented workflow to start isolated PostgreSQL and Redis with health checks and start/connect the Go server with non-secret defaults. **Acceptance:** Docker validation succeeds and a clean-host run reaches `/ready` 200 after migrations. **Failure:** unhealthy dependencies prevent readiness. **Source:** brief plus intake. **Depends:** REQ-002, REQ-003, REQ-004, REQ-007.

### REQ-012 — Operator documentation

The repository shall document prerequisites, variables, startup, migration apply/rollback/status, health/readiness meaning, verification, shutdown, and common dependency recovery. **Acceptance:** a reviewer follows clean start without unstated commands and can find recovery steps. **Failure:** unsupported states are labeled, not guessed. **Source:** brief plus intake. **Depends:** REQ-001..REQ-011.

## 4. Non-functional requirements

- **NFR-001 Startup:** critical configuration/dependency failure exits non-zero within **10 seconds**.
- **NFR-002 HTTP timeouts:** defaults are at most **5 s** read-header, **15 s** read, **30 s** write, **60 s** idle, and **30 s** shutdown; each is configurable and positive.
- **NFR-003 Body bound:** body-bearing endpoints default to a maximum **1 MiB** unless a later route requirement sets a smaller limit.
- **NFR-004 Health latency:** `/health` completes in **≤100 ms p95** across 1,000 in-process requests at concurrency 10 on the recorded CI runner.
- **NFR-005 Readiness bound:** unhealthy `/ready` completes in **≤1 second** and each dependency probe deadline is **≤500 ms**.
- **NFR-006 Pools:** PostgreSQL maximum open and Redis pool size each default to **10**, configurable as positive integers.
- **NFR-007 Log confidentiality:** **100%** of test log records parse as one JSON object/line and contain **0** seeded secret values for names matching password/secret/token/authorization/cookie/database_url.
- **NFR-008 Correlation:** **100%** of completed middleware-test requests return a non-empty ID matching their completion log.
- **NFR-009 Shutdown:** new connections stop within **1 second** of signal and cleanup finishes within the configured default **30 seconds**.
- **NFR-010 Migrations:** apply/apply/rollback/apply passes on **3 consecutive** fresh-database cycles with expected versions.
- **NFR-011 Quality:** `go fmt` changes **0 files** and `go vet`, `go test`, `go test -race`, and `go build` each exit **0** for all packages.
- **NFR-012 Isolation:** automated tests make **0** live Figma calls and require **0** OAuth credentials.
- **NFR-013 Concurrency:** there are **0** unbounded goroutine-per-item loops and all background goroutines exit within the **30-second** shutdown default.
- **NFR-014 Reproducibility:** with images available, a clean Docker host reaches healthy dependencies, migrations, and readiness within **120 seconds in 3/3 runs**.
- **NFR-015 Simplicity:** M1.1 uses exactly **2 external runtime data services** (PostgreSQL and Redis) and **0** object stores, brokers, Figma/AI/GitHub services, or Chromium processes.

Measurement methods are respectively process tests; config inspection; HTTP boundary tests; benchmark; timed dependency doubles; pool-stat tests; log scan; middleware tests; signal integration test; PostgreSQL integration test; command evidence; outbound-network review; race/shutdown inspection; acceptance script; runtime manifest review.

## 5. Domain requirements

- **DOM-001 — Go cancellation:** blocking request/shutdown I/O shall accept and honor `context.Context`; contexts shall not be stored in long-lived structs. **Source rule:** Go standard-library convention adopted by the brief. **Consequence:** leaks/delayed shutdown. **Verification:** interface review and cancellation tests.
- **DOM-002 — Durable authority:** PostgreSQL shall be the sole durable application-data authority; M1.1 persists no application record only in Redis. **Source rule:** stakeholder product constraint. **Consequence:** unrecoverable data loss. **Verification:** storage review and Redis-reset test.
- **DOM-003 — Schema history:** all PostgreSQL schema changes shall be ordered SQL migration files; automatic ORM mutation is prohibited. **Source rule:** stakeholder database constraint. **Consequence:** unrepeatable environments. **Verification:** repository and migration tests.

No sector-specific regulation was identified for M1.1. NDPR/GDPR and OAuth-provider rules require applicability review before M1.2 stores identity or credentials.

## 6. Interfaces and approval

Operational interfaces are unauthenticated `GET /health` and `GET /ready`, configured PostgreSQL/Redis URLs, and SIGINT/SIGTERM. Later application routes use `/api/v1`; none exist now. Traceability lives in `.ilana/traceability.csv`; requirement changes require a change request and IDs are never reused.

| Role | Date | Status |
| --- | --- | --- |
| Ìlànà analyst | 2026-09-29 | complete |
| Stakeholder representative | 2026-09-29 | approved at G1 |
| Architect | 2026-09-29 | feasibility confirmed at G2 |

## Change CR-001 — Request boundary and environment (2026-09-29)

Approved by the stakeholder in chat. Adds four items to M1.1:

- `APP_ENV` (`development` or `production`); production requires `FRONTEND_URL` and `TEMP_WORKSPACE_ROOT` to be set explicitly.
- CORS restricted to the single `FRONTEND_URL` origin, never a wildcard, with credentials allowed.
- API security headers: `X-Content-Type-Options`, `Cache-Control: no-store`, `Referrer-Policy`, and a deny-all `Content-Security-Policy`.
- `TEMP_WORKSPACE_ROOT` validated and created owner-only (0700) at startup.

The same change wires panic recovery and the request body limit into the handler chain; both were specified but had not been attached.

## Change CR-002 — M1.2 identity (2026-09-29)

Approved by the stakeholder in chat. The M1.1 out-of-scope list is superseded for exactly this milestone: Figma OAuth, users, sessions and the Figma credential lifecycle. Projects, imports, Design IR, AI and export remain out of scope.
