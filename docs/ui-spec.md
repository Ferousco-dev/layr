# Interface Specification: LAYR Server M1.1

| Field | Value |
| --- | --- |
| Document ID | UI-LAYR-M1.1-v1 |
| Status | proposed interface baseline |
| Interface type | text-based: HTTP JSON, command line, JSON Lines logs, process signals |
| Primary users | developer/operator; automated health probes and scripts |
| Frequency | health probes: frequent; startup/log review: daily; migrations/recovery: occasional |
| Environment | local Docker Compose v2 on macOS/Linux; CI |
| Requirements baseline | SRS-LAYR-M1.1-v1 |
| Design baseline | SDD-LAYR-M1.1-v1, DES-001..DES-030 |

## 1. Choice and controlled vocabulary

M1.1 has no end-user graphical interface. Its users operate and inspect a server, so scriptable text interfaces are the appropriate surface. Human-readable prose appears in documentation; runtime output remains deterministic and machine-parseable. These terms are normative:

| Term | Meaning | Do not substitute |
| --- | --- | --- |
| health | the server process is alive and can serve HTTP | readiness, healthy dependencies |
| ready | PostgreSQL and Redis both answered their bounded probes | alive |
| check | the state of one readiness dependency | service health |
| up | the dependency probe succeeded | healthy, ok |
| down | the dependency probe failed before its deadline | dead |
| timeout | the dependency probe exceeded its deadline | down |
| migration | one ordered, explicit SQL schema change | ORM sync, schema push |
| request ID | opaque correlation identifier in `X-Request-ID` | trace ID, correlation token |

JSON property names, error codes, event names, command names, and exit semantics are compatibility surfaces. Human messages may be clarified without changing their associated stable code.

## 2. Shared output contract

- HTTP response media type is `application/json`; UTF-8 is implied by JSON.
- Every HTTP response carries `X-Request-ID`. A valid inbound `X-Request-ID` is preserved; missing or invalid input is replaced and never echoed.
- Runtime JSON contains no stack trace, SQL text, DSN, internal hostname, credential, or secret value.
- JSON object property order is not significant. Consumers must ignore additional properties but may rely on documented properties and enum values.
- HTTP methods other than `GET` on the two M1.1 endpoints return the router's JSON error contract, never HTML.
- CLI diagnostics and logs are one complete JSON object per line on stderr. Successful command summaries are one complete JSON object on stdout. No terminal colour, cursor control, or animation is used.
- Timestamps use UTC RFC 3339 with fractional seconds; durations are integer milliseconds.

The shared non-success HTTP envelope is:

```json
{
  "error": {
    "code": "METHOD_NOT_ALLOWED",
    "message": "This endpoint accepts GET requests only.",
    "request_id": "01K..."
  }
}
```

`error.code` and `request_id` are non-empty strings. `message` is safe for an operator and does not expose implementation detail.

## 3. Interface elements and states

### UI-001 — `GET /health`: process liveness

**Satisfies:** REQ-005, REQ-006, REQ-008, NFR-004, NFR-008.

Request: no body, query parameters, authentication, or dependency access. A supplied valid `X-Request-ID` is optional.

Success — HTTP 200:

```json
{"status":"ok"}
```

| State | Observable result |
| --- | --- |
| default/active | immediate 200 and `status=ok` while the process can serve HTTP |
| focus/disabled/loading/empty | not applicable to a request/response API |
| wrong method | 405, `Allow: GET`, error code `METHOD_NOT_ALLOWED` |
| handler encoding failure | 500, error code `INTERNAL_ERROR` when a response can still be written; safely logged |
| shutdown | after listener closure, the connection is refused/closed; no false 200 is promised |

### UI-002 — `GET /ready`: dependency readiness

**Satisfies:** REQ-002, REQ-003, REQ-005, REQ-007, REQ-008, NFR-005, NFR-008.

Request: no body, query parameters, or authentication. PostgreSQL and Redis checks are independent and both are reported. The overall response completes within one second when unhealthy; each check is bounded by at most 500 ms.

Ready — HTTP 200:

```json
{
  "status": "ready",
  "checks": {
    "postgres": "up",
    "redis": "up"
  }
}
```

Not ready — HTTP 503:

```json
{
  "status": "not_ready",
  "checks": {
    "postgres": "up",
    "redis": "timeout"
  }
}
```

Each check value is exactly `up`, `down`, or `timeout`. No driver error text is returned.

| State | Observable result |
| --- | --- |
| default | probes have not started; no response emitted yet |
| loading | connection remains pending only within the one-second overall budget; no streaming body |
| active/ready | 200 only when both checks are `up` |
| error/not ready | 503 when either check is `down` or `timeout`; both states included |
| focus/disabled/empty | not applicable; the `checks` object is never omitted or empty |
| shutdown | readiness becomes unavailable as the listener stops; shutdown logs provide the reason |

### UI-003 — configuration and startup

**Satisfies:** REQ-001..REQ-003, REQ-005, REQ-009, NFR-001, NFR-002, NFR-003, NFR-006, NFR-007.

The canonical server command is `go run ./cmd/server`. Environment variables are the only runtime configuration input. `.env.example` is a documentation template, not an automatic production secret store.

1. Load and validate the complete typed configuration.
2. On invalid configuration, emit one `startup.failed` JSON event naming safe field(s), exit non-zero, and never listen.
3. Initialize PostgreSQL and Redis with bounded probes.
4. On dependency failure, emit `startup.failed` with a stable error code and dependency name, close already-open resources, exit non-zero within ten seconds, and never listen.
5. Only after successful initialization emit `server.started` and accept HTTP traffic.

States are `validating`, `connecting`, `listening`, and `failed`. Logs make every transition visible; secret values never do. Repeated startup is safe and must not mutate the schema implicitly.

### UI-004 — migration command

**Satisfies:** REQ-004, REQ-009, REQ-012, NFR-007, NFR-010, DOM-003.

The canonical executable interface is:

```text
go run ./cmd/migrate <up|down|status>
```

Repository convenience commands may wrap it but must preserve names and semantics.

| Command | Meaning | Success output (`stdout`) | Failure |
| --- | --- | --- | --- |
| `up` | apply all pending migrations in order | `{"status":"ok","operation":"up","from":N,"to":N,"applied":N}` | non-zero; stderr event `migration.failed` |
| `status` | report current/latest version and dirty state | `{"status":"ok","operation":"status","current":N,"latest":N,"dirty":false}` | non-zero; stderr event `migration.failed` |
| `down` | reverse exactly the latest applied reversible migration | `{"status":"ok","operation":"down","from":N,"to":N,"reverted":1}` | non-zero; stderr event `migration.failed` |

`N` denotes a non-negative integer. On an empty database, `current` is `0`. A no-op `up` succeeds with `applied:0`. `down` with no applied migration fails with `MIGRATION_AT_BASE`; it never silently succeeds. Unknown or missing commands print a concise usage line to stderr and exit non-zero with `MIGRATION_USAGE`.

`down` is destructive but deliberately limited to one migration. The exact action is visible in the command, the resulting versions are reported, and its undo is `up`. There is no M1.1 `drop`, `reset`, `force`, or multi-step rollback command.

### UI-005 — lifecycle, shutdown, and recovery feedback

**Satisfies:** REQ-009, REQ-010, REQ-012, NFR-007, NFR-009, DOM-001.

SIGINT and SIGTERM initiate the same idempotent flow. Emit `shutdown.started`, stop accepting new work within one second, drain active requests within the configured grace period, close HTTP/PostgreSQL/Redis/logging resources once, then emit `shutdown.completed`. A second signal or elapsed grace period forces HTTP closure and emits `shutdown.forced`. Cleanup failure emits `shutdown.failed` and causes a non-zero exit. No interactive confirmation is required because a signal is itself a deliberate control action and restart is the recovery path.

### UI-006 — structured operational logs

**Satisfies:** REQ-008, REQ-009, REQ-010, NFR-007, NFR-008.

Every record is one JSON object with `timestamp`, `level`, and `event`. Request completion additionally has `request_id`, `method`, `route`, `status`, and `duration_ms`. Lifecycle/dependency records add only allow-listed, low-cardinality fields such as `dependency` and `code`. Raw URL query strings, request/response bodies, headers, environment values, DSNs, SQL, and error chains are not logged.

Required stable events are `startup.failed`, `server.started`, `request.completed`, `readiness.failed`, `migration.completed`, `migration.failed`, `shutdown.started`, `shutdown.forced`, `shutdown.failed`, and `shutdown.completed`.

## 4. Primary task flows

### UI-007 — clean local start

**Frequency:** daily during development. **Satisfies:** REQ-011, REQ-012, NFR-014.

1. Copy documented non-secret local defaults from `.env.example` using the README procedure.
2. Start PostgreSQL and Redis with the documented Docker Compose command.
3. Run `go run ./cmd/migrate up`; observe `status=ok`.
4. Start the server; observe `server.started`.
5. Request `/health`, then `/ready`; observe HTTP 200 from each.

Failures preserve database state and point to the failed dependency or variable. Recovery is documented retry after correcting configuration/dependency state.

### UI-008 — inspect and recover an unready service

**Frequency:** occasional. **Satisfies:** REQ-007, REQ-012.

1. Request `/ready`; read overall status and both checks.
2. Use the failed check name and matching redacted log event to select PostgreSQL or Redis recovery steps.
3. Restore the dependency; retry `/ready`.
4. Success is 200 with both checks `up`; no restart is required solely to re-probe readiness.

### UI-009 — inspect and reverse migration state

**Frequency:** rare. **Satisfies:** REQ-004, REQ-012, NFR-010.

1. Run `status` and record `current`, `latest`, and `dirty`.
2. Run `down` to reverse one migration.
3. Inspect the result versions; run `up` to undo the rollback.
4. If dirty or failed, stop and use the named migration/version in the recovery documentation; do not guess or force a version.

## 5. Feedback latency policy

| Operation | Budget and feedback |
| --- | --- |
| `/health` | complete within the measured health budget; one atomic response |
| `/ready` | complete within one second when unhealthy; one atomic response containing both check states |
| startup | every transition is logged; total critical failure feedback within ten seconds |
| migration | start/completion or failure event is observable; no fabricated percentage or ETA |
| shutdown | `shutdown.started` is immediate; terminal event occurs by the configured grace ceiling |

Because these are short-lived or streaming-log text surfaces, spinners and progress bars would make automation less reliable. JSON events are the progress mechanism.

## 6. Tolerance and destructive actions

| Action | Prevention/confirmation | Undo/recovery | Work preservation |
| --- | --- | --- | --- |
| migration `down` | explicit `down`; exactly one step; no wildcard/all mode | `up` reapplies it | successful prior migrations remain applied |
| SIGINT/SIGTERM | explicit OS signal; handler idempotent | restart process | active work drains within grace period |
| forced shutdown after deadline | deadline configured and logged | restart; investigate request | completed work remains in PostgreSQL |

Invalid request IDs are replaced rather than rejected; unknown JSON properties in responses can be ignored; repeated migration `up` is a successful no-op. Configuration and migration errors never partially start the HTTP listener.

## 7. Accessibility and parseability

Traditional contrast, pointer, focus, and motion checks are not applicable because M1.1 supplies no GUI. Equivalent accessibility requirements are mandatory:

| Check | Status | Evidence expected |
| --- | --- | --- |
| meaning does not depend on colour | specified | no ANSI colour; levels/statuses are text fields |
| keyboard-only operation | specified | commands, HTTP, environment, and signals require no pointer |
| assistive/automation naming | specified | stable command names, property names, codes, and documented headings |
| reading order | specified | one object per line; no cursor rewriting |
| reduced motion | specified | no animation |
| parseable without colour | specified | JSON/JSON Lines and non-zero exit status |

Human messages use short sentences and do not rely on symbols alone. Logs are usable through pipes and screen readers without terminal-layout assumptions.

## 8. Five-principle assessment

| Principle | Assessment | Weakest point / control |
| --- | --- | --- |
| Structure | HTTP probes, migration commands, lifecycle events, and recovery docs are separate but share vocabulary and JSON rules. | Multiple surfaces can drift; contract tests and this catalogue control drift. |
| Simplicity | Common health checks are one GET; migration verbs are `up`, `down`, `status`; startup is configuration then run. | Environment setup requires recall; `.env.example` and one clean-start recipe provide recognition. |
| Visibility | Probe responses expose all relevant states; logs expose lifecycle transitions; migration status exposes version and dirty state. | Startup has no GUI; JSON events are the explicit feedback channel. |
| Feedback | Every action has HTTP status, command exit plus JSON result, or lifecycle event within a quantified bound. | Migration duration has no ETA; event feedback avoids inventing one. |
| Tolerance | Invalid request IDs are safely replaced, migration apply is idempotent, rollback is one step and reversible, and shutdown is bounded/idempotent. | Database migration failure may need manual diagnosis; force/reset controls are intentionally excluded. |

## 9. Traceability

| UI ID | Requirements | Design elements |
| --- | --- | --- |
| UI-001 | REQ-005, REQ-006, REQ-008, NFR-004, NFR-008 | DES-025, DES-026, DES-028 |
| UI-002 | REQ-002, REQ-003, REQ-005, REQ-007, REQ-008, NFR-005, NFR-008 | DES-021, DES-022, DES-024, DES-026, DES-028 |
| UI-003 | REQ-001..REQ-003, REQ-005, REQ-009, NFR-001..NFR-003, NFR-006, NFR-007 | DES-019..DES-022, DES-028, DES-030 |
| UI-004 | REQ-004, REQ-009, REQ-012, NFR-007, NFR-010, DOM-003 | DES-017, DES-018, DES-020, DES-023, DES-030 |
| UI-005 | REQ-009, REQ-010, REQ-012, NFR-007, NFR-009, DOM-001 | DES-020, DES-029, DES-030 |
| UI-006 | REQ-008..REQ-010, NFR-007, NFR-008 | DES-020, DES-026, DES-027, DES-029 |
| UI-007 | REQ-011, REQ-012, NFR-014, NFR-015, DOM-002 | DES-018, DES-021..DES-023, DES-030 |
| UI-008 | REQ-007, REQ-012 | DES-018, DES-024 |
| UI-009 | REQ-004, REQ-012, NFR-010, DOM-003 | DES-018, DES-023 |

NFR-011 through NFR-013 are verification/implementation constraints without an additional human-facing control. Every functional requirement has at least one UI element; this specification does not authorize M1.2+ application interfaces.
