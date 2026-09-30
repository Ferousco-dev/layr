# ADR-001: Use a single-process modular monolith

| Field | Value |
| --- | --- |
| Status | accepted |
| Date | 2026-09-29 |
| Deciders | Ìlànà architect; stakeholder constraints via approved SRS |
| Ledger ID | DEC-003 |

## Context

M1.1 needs one HTTP API foundation operated initially by one maintainer. NFR-011 requires straightforward Go verification, NFR-013 requires bounded concurrency, and NFR-015 permits only PostgreSQL and Redis external data services. The product brief asks that later workers can separate without making current modules distributed.

## Options considered

### A. Single-process modular monolith

Use acyclic Go packages, constructor injection, consumer-owned interfaces, and one composition root. It satisfies NFR-011/013/015 with low operational burden and introduces no internal network failures. Cost: boundaries are enforced by package/API discipline rather than deployment isolation.

### B. API service plus worker service

Deploy separate binaries/services now. It could later support independent scaling, but M1.1 contains no background job and would add deployment, RPC, retry, tracing, and failure concerns without a requirement. Cost and risk are highest for a single maintainer.

### C. Unstructured single package

Put all logic in `main` or one package. Initial file count is small, but testing seams, ownership, and future domain boundaries become implicit. It fails the requested clear package boundaries and makes later extraction a rewrite.

## Decision and reasons

Choose A. It is the smallest architecture satisfying current lifecycle and testability needs while retaining replaceable adapter boundaries. The decision ranks operability and conceptual integrity above speculative distribution.

## Consequences

Accepted: package boundaries require review; shared-process failure can affect all modules; future extraction will need a transport contract when a real worker requirement exists.

Rejected options remain rejected until B has an approved workload requiring independent scale/deploy/fault isolation, or C is shown through repository evidence to reduce rather than increase maintenance risk.

## Revisit trigger

Reopen when an approved milestone introduces background work with measured independent scaling or isolation requirements, not merely because workers appear on the roadmap.

