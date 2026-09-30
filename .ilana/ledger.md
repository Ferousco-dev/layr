# LAYR Engineering Ledger

Append-only process record for the LAYR server.

## 2026-09-29 | INTAKE | conductor | FLEET SELECTED
Evidence:
  - Product and Milestone 1 brief: user-supplied specification dated 2026-09-29.
  - Repository inspection: README.md is the only project file; no Git history or existing implementation was found.
  - Intake answers: public SaaS target, proprietary design data, OAuth credentials, no hard deadline, primary maintainer is one developer.
Decision:
  - DEC-001: FLEET mode.
  - DEC-002: Rigour 3 of 5 because this is production SaaS handling sensitive credentials and proprietary customer data, but is not regulated or safety-critical.
  - DEC-003: Hybrid process with regulated ceremony for authentication, cryptography, sessions, and personal-data work.
Open:
  - Fleet execution awaits approval of the proposed plan.

## 2026-09-29 | G0 | conductor | GATE PASS
Evidence:
  - User approved the 12-role FLEET plan for M1.1.
  - Intake answers cover users, sensitive data, harms, leak impact, schedule, success measures, and maintenance ownership.
  - Process registers are recorded in .ilana/state.json.
Decision: enter Phase 01 Requirements with analyst as phase owner.

## 2026-09-29 | G1 | analyst | GATE PASS
Evidence:
  - docs/srs.md defines 12 functional, 15 numeric non-functional, and 3 domain requirements for M1.1.
  - .ilana/traceability.csv contains 30 structurally valid requirement rows.
  - Stakeholder approved the requirements baseline and thresholds on 2026-09-29.
  - DEC-004..DEC-007 disposition the four proposed assumptions.
  - ETH-001..ETH-003 impose preventive controls; no ethics halt exists.
Decision: advance to Phase 02 Architecture.

## 2026-09-29 | G2 | architect | GATE PASS
Evidence:
  - docs/design.md defines DES-001..DES-030 across architecture, high-level, and detailed design.
  - docs/modules/ contains ten module specifications; docs/adr/ records four decisions and rejected alternatives.
  - Every approved requirement and NFR has a design mechanism; bidirectional orphan audit reports no gaps.
  - Design stays within M1.1 and creates no speculative domain schema.
Decision: accept the architecture baseline and proceed to interface baseline.

## 2026-09-29 | G3 | interaction-designer | GATE PASS
Evidence:
  - docs/ui-spec.md defines UI-001..UI-009 for HTTP, CLI, logging, configuration, and lifecycle surfaces.
  - docs/error-catalogue.md defines ERR-001..ERR-020 with safe messages and recovery paths.
  - Architecture and interface agree on `up|down|status` migration commands and `up|down|timeout` readiness states.
  - docs/reviews/usability-m1-1.md records a heuristic specification review without claiming a user study.
Decision: advance to Phase 04 Construction.

## 2026-09-29 | G4 | constructor | GATE PASS
Evidence:
  - `server/` contains the M1.1 Go module, explicit migration, local infrastructure definitions, and unit tests.
  - Independent verifier review found and constructor remediated forced-shutdown, lifecycle-testability, import-style, and module-identity issues before gate close.
  - Local `gofmt`, `go vet`, `go test`, `go test -race`, and `go build` commands exited 0 across 11 packages.
  - Canonical module path is `github.com/ferousco-dev/layr/server`; stale former-account scan found 0 references.
  - Targeted repository secret scan found 0 live credential assignments.
Limit:
  - By stakeholder direction, no Docker images or containers were pulled or started; live dependency acceptance remains unverified.
Decision: advance to Phase 05 Verification.

## 2026-09-29 | M1.2 | constructor | CONSTRUCTED (gate not run)
Evidence:
  - CR-002 approved by stakeholder in chat; M1.2 scope follows the stakeholder brief only.
  - Figma OAuth details taken from developers.figma.com (authorize, token, refresh, scopes, `/v1/me`); PKCE S256 added because Figma documents it. Refresh does not rotate the refresh token.
  - `go vet`, `gofmt`, `go test -race ./...` pass; live PostgreSQL 16 and Redis 7.4 tests pass with `TEST_DATABASE_URL` and `TEST_REDIS_URL`.
Limit:
  - No real Figma OAuth browser flow was run. Docker was not run by stakeholder direction. G5-G8 for M1.1 remain open.


## 2026-09-29 | G5 | verifier | GATE PASS WITH OVERRIDES
Evidence:
  - DEF-001 fixed and retested; regression passes under `-race`.
  - Live PostgreSQL and Redis integration, migration cycle, readiness outage recovery, `/health` p95 0.34 ms, SIGTERM exit 0.06 s (`docs/test-report.md` section 7).
Overrides: OVR-001 Docker runs, OVR-002 acceptance sign-off, OVR-003 sustained stress; each authorised by the stakeholder in chat.
Decision: advance to phase 06; G6 requires the stakeholder to commit and tag.

## 2026-09-29 | G6-G8 | conductor | OVERRIDDEN (not passed)
Stakeholder accepted the override decision in chat. OVR-004 (G6), OVR-005 (G7), OVR-006 (G8) recorded with unmet criteria and carried risk. `docs/retrospective.md` and `docs/maturity-assessment.md` written. M1.1 is closed by override, not by full evidence.

## 2026-09-29 | M1.3 | constructor | CONSTRUCTED (gate not run)
CR-003 implemented. Live PostgreSQL tests and a real-binary smoke run confirm cross-user GET/PATCH/DELETE return 404 and lists never mix users. No Figma calls, no Redis use in project CRUD, Docker not run.

## 2026-09-29 | CR-004 | constructor | IMPLEMENTED
Soft delete with restore and lazy purge; API rate limits. Migration 00004 rehearsed down/up on a throwaway database. `go test -race` with live PostgreSQL and Redis passes.

## 2026-09-29 | CR-004 follow-up | constructor | IMPLEMENTED
Restore no longer changes updated_at; rate limits configurable via RATE_LIMIT_LOGIN/API/WRITE_PER_MINUTE (1 to 100000). Live PostgreSQL and Redis tests pass with -race.

## 2026-09-29 | M1.4 | constructor | CONSTRUCTED (gate not run)
CR-005 implemented against current Figma docs (endpoints, scopes, tiers, Retry-After). No live Figma request was made. Docker not run. Import orchestration, URL parsing and downloads intentionally absent.

## 2026-09-29 | M1.5 | constructor | CONSTRUCTED (gate not run)
CR-006 implemented. Ownership, state guards and single-running-import rule proven against local PostgreSQL. A real-binary run confirmed 401, 400, cross-user 404 and a clean FIGMA_AUTH_REQUIRED failure with workspace removed. No live Figma import was run; Docker not run.

## 2026-09-29 | M1.6 | constructor | CONSTRUCTED (gate not run)
CR-007 implemented. Download safety proven with httptest (limits, redirects, address policy, cancellation, partial files). Manifest determinism proven across worker counts. No live Figma asset download was run; Docker not run.

## 2026-09-30 | M1.7 | constructor | CONSTRUCTED (gate not run)
CR-008 implemented. The normalizer is pure and offline; golden file, determinism (25 runs), limits, validator and race tests pass. Multi-screen support added end to end (selection, per-screen references, shared components, Select). Live tests ran against Homebrew PostgreSQL over ::1 and a private Redis on 6391 because Docker now also listens on 5432 and 6379. No live Figma run; Docker not run.

## 2026-09-30 | M1.8 | constructor | CONSTRUCTED (gate not run)
CR-009 implemented. Ownership chain, path safety, symlink refusal, private caching and ETag proven by tests. A race in a test double (fakeLimiter) was found by the race detector and fixed in the test only. No live Figma; Docker not run.

## 2026-09-30 | M1.9 | constructor | CONSTRUCTED (gate not run)
CR-010 implemented. Found and fixed: imports stuck in `processing` after a crash (DEF-002), database outages reported as generic 500 (DEF-003), validator allocating as much as the whole build (perfective), IR pretty-printing that made the byte cap reachable at a third of the node cap. Live PostgreSQL and Redis tests ran with the correct `TEST_DATABASE_URL` and `TEST_REDIS_URL`; an earlier run that set the wrong variables had silently skipped them. One real Figma import (3 screens, 10 assets, 6 Figma calls) was verified by hand. Docker not run.
