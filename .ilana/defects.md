# LAYR M1.1 Defect Log

Owner: Ìlànà verifier  
Initialized: 2026-09-29 before test execution

## DEF-001 - Runtime HTTP serve termination does not close dependencies

| Field | Value |
| --- | --- |
| Reported by / date | Ìlànà verifier / 2026-09-29 |
| Found in | G4 handoff tree; no Git commit exists |
| Found during | Independent code review / system lifecycle analysis |
| Component | DES-029, `server/internal/app/app.go` |
| Related requirement | REQ-010, NFR-013 |
| Severity | medium |
| Priority | high |
| Status | Closed (retest passed 2026-09-29) |
| Assigned to | constructor / next maintainer |
| Root-cause phase | Coding error - phase 04 |
| Regression test | `TestRunLifecycleServeFailureClosesResources` in `server/internal/app/app_test.go` |

**Description:** `runLifecycle` returns directly when `Serve` terminates before the run context is cancelled, whether with an unexpected error or `http.ErrServerClosed`. Unlike the signal path, that branch does not close Redis or PostgreSQL. `Run` no longer has deferred dependency closes. The operating system will reclaim sockets on process exit, but deterministic lifecycle ownership and close-once behavior are not satisfied and embedded/reused execution can leak resources.

**Steps to reproduce:** inspect or exercise `runLifecycle` with a fake `serverControl` whose `Serve` immediately returns an error while fake Redis/PostgreSQL record closes.

**Expected:** Redis and PostgreSQL close exactly once on every terminal path after successful initialization; unexpected serve error returns a safe non-nil error.

**Actual:** the serve-result branch returns before either dependency close method is called.

**Evidence:** `server/internal/app/app.go`, first `select` in `runLifecycle`. No live Docker/dependency execution was needed. Workaround: run only as a top-level process and rely on OS cleanup; this is not sufficient to close the defect.

**Resolution (2026-09-29):** `closeDependencies` in `server/internal/app/app.go` now runs on the serve-termination branch and the signal branch. Local `go vet`, `go test -race ./internal/app/` pass. Retested by the verifier: regression passes under `-race`; closed.

## DEF-002 - Imports stuck in `processing` after the server dies

| Field | Value |
| --- | --- |
| Reported by / date | M1.9 audit / 2026-09-30 |
| Found during | Crash and restart review |
| Severity | Medium: status API reports "importing" forever; project blocked until a new import supersedes it |
| Cause | Stale imports were retired only when a new import started |
| Fix | `Sweep` (startup and hourly) fails imports untouched for longer than any live import can run (`IMPORT_INTERRUPTED`) |
| Regression tests | `TestSweepFailsImportsAbandonedByACrashedProcess`, `TestFailAbandonedRetiresOnlyOldRunningImports` (live PostgreSQL) |
| Status | Fixed, verified locally |

## DEF-003 - Database outage returned a generic 500 on project, import and design routes

| Field | Value |
| --- | --- |
| Reported by / date | M1.9 audit / 2026-09-30 |
| Severity | Low: no leak, but clients could not tell a retryable outage from a bug, and the import handler logged no request ID |
| Fix | `postgres.IsUnavailable` and a shared `unexpected` responder: 503 `DEPENDENCY_UNAVAILABLE` with `Retry-After`, request ID in every failure log |
| Regression tests | `TestIsUnavailableSeparatesOutagesFromOrdinaryErrors`, `TestDatabaseOutageIsA503WhileOtherFailuresStayAGeneric500` |
| Status | Fixed, verified locally |
