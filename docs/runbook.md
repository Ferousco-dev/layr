# LAYR Server M1.1 Local Operations Runbook

> **Status: implemented; Docker-backed acceptance unverified.** Local Go format, vet,
> unit, race, and build checks have passed. Containers were not run under the current
> no-container constraint.

This runbook covers local M1.1 operation only. For required behavior and limits, use the
[approved SRS](srs.md); for target flows, use [workflows.md](workflows.md). It does not cover OAuth,
Figma imports, users, projects, generation, exports, or production deployment.

## 1. Supported environment

Use Go 1.24 or newer and Docker Compose v2 on macOS or Linux. Images are pinned in
`server/docker-compose.yml`; development ports are PostgreSQL 5432, Redis 6379, and HTTP 8080.
These development defaults are not production-safe.

## 2. Configuration

The canonical typed variable list and sanitized local values are in `server/.env.example`.

Never paste real credentials into documentation or commit them to the repository.

## 3. Clean start

From `server/`, the implemented clean-start sequence is:

```sh
cp .env.example .env
set -a; . ./.env; set +a
docker compose up -d --wait
go run ./cmd/migrate up
go run ./cmd/server
```

Then request `http://localhost:8080/health` and `/ready`. The Docker-backed sequence is not yet
observed and must remain marked unverified until the constraint is lifted.

## 4. Migrations

The final runbook will provide exact commands for version/status, apply, and reversing the latest
reversible migration. Verification must observe apply → apply/no-op → rollback → apply against a
fresh development database on three consecutive cycles (`NFR-010`). Migration failures must name the
migration without printing database credentials. Automatic ORM schema mutation is prohibited by
`DOM-003`.

Use `go run ./cmd/migrate status`, `up`, or `down`. `down` reverses exactly one migration;
`up` reapplies it. Never edit goose metadata or force a version after failure.

## 5. Probes

| Probe | Intended meaning | Expected result once implemented |
| --- | --- | --- |
| `GET /health` | process liveness; must not query dependencies | HTTP 200 with stable, non-sensitive JSON while the process serves |
| `GET /ready` | ability to serve with required PostgreSQL and Redis dependencies | HTTP 200 only when both pass; otherwise HTTP 503 within the SRS deadline |

Proposed exact response bodies, content type, request-ID header, and stable errors are defined in the
[interface specification](ui-spec.md) and [error catalogue](error-catalogue.md). They remain
unimplemented and unverified. Probe output must never include connection URLs, credentials,
environment dumps, or internal errors.

## 6. Logs and request correlation

The planned server emits one structured JSON object per log line. Request completion records will
carry the response request ID plus the fields approved by the interface and logging design. Logs must
exclude passwords, secrets, tokens, authorization/cookie values, and credential-bearing URLs.

Field names, destinations, level controls, and diagnostic examples are **pending implementation**.
If a diagnostic step would print the environment or a complete connection URL, do not use it.

## 7. Graceful stop

The implemented service is required to handle SIGINT/SIGTERM, stop admitting new work, bound its
drain, and close owned resources. The default ceiling approved in the SRS is 30 seconds. The exact
operator command and observed exit behavior are pending construction and signal integration tests.

Do not use forced process/container deletion as the normal documented stop path. A force-stop path,
if required for recovery, must be separately labelled with its data-loss implications.

## 8. Verification

The SRS requires format, vet, test, race-test, and build checks across Go packages, plus migration,
Docker, clean-start, probe, logging, and shutdown evidence. The verifier-owned plan and repository
task targets will be linked here when present.

**Constructor evidence:** `gofmt`, `go vet ./...`, `go test ./...`, `go test -race ./...`,
`go build ./...`, and non-starting Compose configuration validation exited zero on 2026-09-29.
Live PostgreSQL, Redis, migration-cycle, signal, and clean-host evidence remains verifier-owned.

## 9. Troubleshooting and recovery

These are required recovery scenarios, not yet verified procedures:

| Symptom | Safe first assessment | Required final guidance |
| --- | --- | --- |
| Startup rejects configuration | read the named variable and validation class; do not print its value | correct sanitized example and restart; process must remain non-serving while invalid |
| PostgreSQL unavailable | check dependency health without exposing its URL | restore PostgreSQL, confirm migration version, then re-evaluate readiness |
| Redis unavailable | check dependency health without dumping credentials | restore Redis and re-evaluate readiness; no durable application truth may rely on Redis |
| `/health` succeeds but `/ready` fails | treat process as live but remove it from service | identify the failed dependency from safe status only and follow its recovery procedure |
| Migration fails | stop; preserve the exact failure/version and avoid manual schema improvisation | use tool-specific status/recovery instructions reviewed with the migration design |
| Shutdown exceeds deadline | capture redacted shutdown events and exit status | investigate blocked work/resources; do not claim clean shutdown |

Exact health commands, log queries, container commands, reset boundaries, and rollback steps will be
added only after they can be tested. Never delete volumes or local data as a generic first response.

## 10. Operational handoff checklist

Before this runbook is marked verified:

- all placeholders above have repository-backed commands or an explicit supported exception;
- environment examples contain no working secret;
- clean start and normal stop have been observed on each supported host class;
- dependency outage and migration failure recovery have been exercised;
- every command's expected output and failure exit are documented;
- the verifier's evidence is linked; and
- the onboarding test in [the documentation plan](document-plan.md) passes.
