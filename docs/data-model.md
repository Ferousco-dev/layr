# LAYR Server M1.1 Data Model

Status: architecture baseline candidate, 2026-09-29. Scope: M1.1 only.

## Durable state

M1.1 defines **no application-domain entities and no application-owned tables**. Users, identities, OAuth credentials, sessions, projects, Figma resources, imports, assets, workspaces, Design IR, generation jobs, and exports are excluded by the approved SRS and DEC-007.

PostgreSQL remains the sole durable authority (DOM-002). The selected migration tool may create one tool-owned metadata relation to serialize migrations and record the current schema version. Application packages must not query, update, or depend on its physical columns; only the migration runner may interact with it through the tool.

| Logical record | Owner/writer | Readers | Purpose | Retention | Sensitivity |
| --- | --- | --- | --- | --- | --- |
| Migration metadata | `goose` through DES-023 | Migration runner/status command | Lock/record ordered schema version and dirty/failure state where supported | Life of database; removed only with database teardown | Operational metadata; no secrets or personal data |

The exact relation name and fields are implementation details of the pinned `goose` version and must be verified during construction against the real migration cycle. They are not copied here because doing so would create a false application contract around third-party internals.

## Foundation migration

`000001_foundation.up.sql` and `.down.sql` must be reversible and must create no speculative domain table. Preferred order:

1. If the migration library records an otherwise empty SQL version correctly, use a comment-only migration documenting the boundary.
2. If it requires a database change, create a dedicated application schema marker with a symmetric `DROP` in the down migration, but no entity columns or records.
3. Do not create placeholder `users`, `projects`, `sessions`, or generic key/value tables.

The apply/apply/down/apply sequence must be demonstrated against a fresh PostgreSQL database three times (NFR-010). The HTTP server never applies migrations implicitly.

## Redis state

M1.1 creates no Redis keys and defines no cache, session, queue, or lock schema. Redis exists only as a required connectivity and readiness dependency. Flushing or replacing Redis therefore loses no application record and cannot change PostgreSQL state.

## In-memory/request state

| Structure | Owner | Lifetime | Invariants |
| --- | --- | --- | --- |
| Typed `Config` | DES-019 | Process | Immutable after load; secrets have no string dump; all bounds positive |
| Request ID | DES-026 | One request | Opaque, bounded syntax/length, same value in context/header/log |
| Readiness result | DES-024 | One request | Fixed dependency names/states; no raw errors, hosts, timings, or credentials |
| Lifecycle resource set | DES-029/030 | Process | Composition root is sole owner; every acquired resource closes at most once |

## Ownership, access, and retention boundary

- PostgreSQL owns durable migration history; DES-023 is its only application-side writer in M1.1.
- Redis owns no application record and may be destroyed between runs.
- Configuration exists only in process memory and is discarded on exit; no module persists environment secrets.
- Logs may retain public operational metadata according to the host's policy, but contain no credentials, database URLs, cookies, authorization values, or future design data.
- Docker named volumes are development-only and removed using the documented explicit teardown operation; ordinary server shutdown does not delete them.
- Data-subject deletion, asset cleanup, credential retention, and tenancy rules require new requirements before M1.2+ data models are approved.

## Review boundary

Persistence design has been checked against DEC-007, DOM-002, DOM-003, and the ethics finding against speculative retention. No database engineer or operations specialist is staffed; therefore implementation must validate the third-party migration metadata shape and Docker behaviour with executable evidence before G4. This limitation is disclosed, not treated as review evidence.
