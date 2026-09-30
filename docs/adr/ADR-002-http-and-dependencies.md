# ADR-002: Prefer the Go standard library and three focused dependencies

| Field | Value |
| --- | --- |
| Status | accepted |
| Date | 2026-09-29 |
| Deciders | Ìlànà architect; stakeholder constraints via approved SRS |
| Ledger ID | DEC-003 |

## Context

REQ-002/003 require PostgreSQL and Redis clients; REQ-005..009 require a small HTTP and observability edge. NFR-011 and the single-maintainer constraint favor a small dependency surface. Article 8 requires licence awareness.

## Options considered

### A. Standard library edge plus focused clients

Use `net/http`, `log/slog`, `context`, `crypto/rand`, `pgx/v5` (MIT), and `go-redis/v9` (BSD-2-Clause). This covers required capabilities with direct, context-aware clients and no framework. Cost: small middleware and validation functions are maintained locally.

### B. Web/config/logging framework stack

Use a web framework, environment decoder, logging framework, UUID library, and ORM. This supplies conveniences, but duplicates standard-library capabilities, expands vulnerability/licence review, and risks hidden lifecycle/schema behaviour. No M1.1 requirement needs its advanced features.

### C. Only standard library including hand-written wire clients

Avoid all third-party libraries. This would mean implementing PostgreSQL and Redis protocols or shelling out, which is outside scope and significantly less safe.

## Decision and reasons

Choose A. First, it is operationally legible to one maintainer. Second, it keeps SQL/schema and HTTP lifecycle explicit. Third, it minimizes licence and update obligations while using mature clients where reimplementation would be irresponsible.

## Consequences

Accepted: local code owns request-ID validation, middleware order, and configuration parsing; exact module versions and transitive licences must be recorded later.

Option B is revisited if approved interface requirements produce demonstrably repetitive routing/decoding/error code. Option C remains rejected while direct database/cache protocols are required.

## Revisit trigger

Reopen if M1.2 route complexity creates repeated, defect-prone standard-library glue measured in review/defect evidence, or an approved security policy mandates a different vetted component.

