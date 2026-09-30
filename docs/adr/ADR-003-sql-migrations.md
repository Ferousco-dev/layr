# ADR-003: Use embedded explicit SQL migrations through goose

| Field | Value |
| --- | --- |
| Status | accepted |
| Date | 2026-09-29 |
| Deciders | Ìlànà architect; stakeholder constraints via approved SRS |
| Ledger ID | DEC-007 |

## Context

REQ-004 and DOM-003 require ordered SQL, apply/status/reverse operations, repeatability, and no ORM mutation. DEC-007 forbids speculative domain tables. The command should work without installing a separate migration executable in the runtime image.

## Options considered

### A. `pressly/goose/v3` library with embedded SQL

Use a dedicated Go command whose allowed operations are `up`, `down`, and `status`; status is the version-reporting surface. Apache-2.0 permits the intended use. Embedded files couple a released command to the exact migrations it operates. Cost: tool-owned metadata exists and the library is a direct dependency.

### B. `golang-migrate/migrate/v4`

This mature MIT option supports explicit SQL and up/down/version. Its library and source/database driver arrangement is suitable, but the M1.1 command needs a status-oriented operator surface that would require more local presentation logic. It remains a viable substitute.

### C. Hand-written migration runner / do nothing

Either manually track versions/locks or execute loose SQL. This avoids a library but transfers locking, dirty-state, transaction, and idempotence risk to project code. Doing nothing directly fails REQ-004.

## Decision and reasons

Choose A. It most directly supplies the three approved operator operations behind one small command, handles explicit SQL, and supports embedding. Migrations do not run during server startup.

## Consequences

Accepted: Apache-2.0 notices and exact version licence verification are required; migration metadata is tool-owned; rollback safety still depends on authored down SQL. The foundation migration records infrastructure only and creates no domain entities.

Option B should be reconsidered if construction finds goose cannot satisfy context cancellation, embedded-file, PostgreSQL-lock, or status semantics with the pinned version. Option C remains rejected unless PostgreSQL gains a reviewed native migration facility meeting every requirement.

## Revisit trigger

Reopen if the 3-cycle fresh-database test fails due to tool behaviour, if exact-version licence review conflicts with distribution, or if a future deployment system requires a different migration packaging model.

