# Ethics Register

Scope: LAYR server M1.1 foundation. Review evidence is in `docs/reviews/ethics-m1-1.md`.

## ETH-001 — Sensitive-data controls must be requirements, not assumptions

- Severity: major
- Principle: confidentiality; public interest; product quality
- Finding: Later M1 phases will handle OAuth credentials, sessions, personal identity metadata, and proprietary private designs. M1.1 establishes the configuration, logging, persistence, and operational boundaries those phases will inherit. The supplied brief states suitable controls, but no implementation evidence exists yet.
- Required action: Before the relevant construction gate, baseline testable requirements for secret exclusion and redaction, encrypted credential persistence, secure session handling, ownership enforcement, and data minimization. M1.1 logging and configuration must be designed so secrets are neither emitted nor committed.
- Owner: analyst, architect, constructor, verifier
- Status: open; no halt. Preventive gate condition, not evidence of current mishandling.

## ETH-002 — Temporary proprietary data requires a bounded lifecycle

- Severity: major
- Principle: confidentiality; client and employer responsibilities
- Finding: Raw Figma snapshots, images, SVGs, renders, and future generated source may contain confidential customer material. The intended worker-local design is legitimate, but deletion after normal completion alone would not cover failure, cancellation, or crash abandonment.
- Required action: Before temporary workspace functionality ships, define and verify creation permissions, path isolation, retention bounds, cleanup on success/failure/cancellation, abandoned-workspace recovery, and prohibition on permanent filesystem paths in durable records. Logs and test fixtures must not contain real private designs.
- Owner: architect, constructor, verifier
- Status: open; applies to later M1 workspace work, not an M1.1 halt.

## ETH-003 — Dependency ownership and verification claims require evidence

- Severity: minor
- Principle: intellectual property; honesty and integrity
- Finding: The greenfield repository currently contains no dependency manifest or implementation. Consequently, dependency licences and quality-command outcomes are unknown, not non-compliant or passing.
- Required action: Record each introduced direct dependency and its licence in the toolchain/SCM evidence; preserve required notices. Report `fmt`, `vet`, tests, race tests, builds, Docker validation, and integration limitations only from observed command results, explicitly distinguishing unavailable infrastructure from a pass.
- Owner: configuration-engineer, quality-auditor, verifier
- Status: open; no halt.

## Halt status

No halt exists as of 2026-09-29. No secret, personal data, proprietary design, copied dependency, fabricated result, or control-defeating implementation is present in the inspected repository. Reassess at architecture, construction, verification, and before public exposure.
