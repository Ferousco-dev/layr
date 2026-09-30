# Decision Register

## DEC-001 — Use FLEET mode

- Date: 2026-09-29
- Status: accepted
- Decision: Run the gated lifecycle for the greenfield LAYR server foundation.
- Basis: The user selected FLEET after reviewing the mode options.

## DEC-002 — Set rigour to 3 of 5

- Date: 2026-09-29
- Status: accepted
- Decision: Apply production-SaaS gates and revisit the level before public launch.
- Basis: LAYR will handle OAuth credentials, sessions, personal identity metadata, and proprietary private designs, but it is not safety-critical and no regulatory regime has yet been identified.

## DEC-003 — Use hybrid process and regulated ceremony

- Date: 2026-09-29
- Status: accepted
- Decision: Use iterative construction with plan-driven architecture, security, traceability, and verification. Escalate ceremony because M1 includes authentication, cryptography, and personal data.
- Basis: The product will evolve, while its security boundaries require explicit review evidence.

## DEC-004 — PostgreSQL and Redis are required for readiness

- Date: 2026-09-29
- Status: accepted by stakeholder
- Decision: `/ready` returns ready only when both PostgreSQL and Redis pass bounded probes.

## DEC-005 — Local platform baseline

- Date: 2026-09-29
- Status: accepted by stakeholder
- Decision: M1.1 local development targets Docker Compose v2 and supported Go toolchains on macOS and Linux.

## DEC-006 — Graceful-shutdown default

- Date: 2026-09-29
- Status: accepted by stakeholder
- Decision: Use a configurable 30-second default graceful-shutdown ceiling until measured traffic justifies revision.

## DEC-007 — Migration boundary

- Date: 2026-09-29
- Status: accepted by stakeholder
- Decision: M1.1 establishes explicit migration infrastructure without speculative identity, OAuth, project, or import tables.
