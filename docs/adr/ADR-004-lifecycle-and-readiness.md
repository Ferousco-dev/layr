# ADR-004: Fail closed on startup and require both dependencies for readiness

| Field | Value |
| --- | --- |
| Status | accepted |
| Date | 2026-09-29 |
| Deciders | Stakeholder (DEC-004/006), Ìlànà architect |
| Ledger ID | DEC-004, DEC-006 |

## Context

REQ-002/003 require verified dependencies, REQ-006 separates liveness from dependencies, and REQ-007 plus DEC-004 require PostgreSQL and Redis for readiness. NFR-001/005/009 impose startup, probe, and shutdown bounds.

## Options considered

### A. Fail startup and readiness closed; probe dependencies concurrently

Open and ping both dependencies before listening. At runtime, liveness does no I/O while readiness runs independent ≤500 ms probes concurrently inside a ≤1 s aggregate bound and distinguishes `timeout` from `down`. This satisfies the accepted policy with encoding margin.

### B. Start degraded and make Redis optional

Listen when PostgreSQL succeeds and advertise partial service if Redis fails. This can increase availability for endpoints that do not need Redis, but M1.1 has no route capability model and DEC-004 explicitly makes Redis mandatory.

### C. Sequential probes or liveness with dependency probes

Sequential checks are simpler but consume nearly the full 1 s bound. Dependency-backed liveness can restart a healthy process during an external outage and conflates process and service health.

## Decision and reasons

Choose A. It makes the accepted dependency contract truthful, keeps liveness cheap, and preserves time for JSON encoding and scheduling within NFR-005. Status output never includes raw dependency errors.

## Consequences

Accepted: Redis outages remove an instance from readiness even before Redis stores feature state; operators need both services locally; concurrent probe result collection must not leak goroutines. Timeout is externally distinct from immediate/down failure but carries no host/error detail.

Option B becomes reasonable only through a requirement change that defines which routes can function without Redis. Option C is reconsidered only if a client proves concurrency-unsafe or timing evidence supports a stricter sequential budget.

## Revisit trigger

Reopen when route-specific degraded operation is approved, or production measurements demonstrate that mandatory Redis readiness causes unacceptable availability without protecting any active capability.
