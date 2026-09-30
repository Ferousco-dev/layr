# Test Execution Report: LAYR Server M1.1

| Field | Value |
| --- | --- |
| Build / commit | G4 handoff tree; repository has no Git commit (`NO_GIT_COMMIT`) |
| Environment | macOS darwin/arm64; Go 1.27.1 |
| Executed | 2026-09-29, stopped on user handoff request |
| Executed by | Ìlànà verifier |
| Test plan | TP-LAYR-M1.1-v1 |

## 1. Summary

| Level | Planned cases | Executed evidence | Passed | Failed | Blocked/unverified | Skipped |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Unit | 29 case-level mappings | 12 top-level Go tests (18 including subtests) | 12 top-level | 0 | remaining planned partitions not executed | 0 |
| Integration | 22 case-level mappings | in-process health/HTTP/middleware/lifecycle fakes only | covered local assertions passed | 0 | all real PostgreSQL, Redis and migration boundaries | 0 |
| System | 27 case-level mappings | local build/CLI failure/static checks | completed checks passed | 0 | live server/dependency, Docker, migration cycle, load/stress | 0 |
| Acceptance | 2 | 0 | 0 | 0 | 2 (TC-023, TC-024; user/client required) | 0 |

Counts across levels overlap because a planned case can span levels. Go runner facts are exact: 11 packages discovered; 6 packages passed tests; 5 packages reported `[no test files]`; 12 top-level tests passed, 18 pass events including subtests, 0 failures, 0 skipped tests.

Coverage by package: app 63.5%, config 82.4%, health 94.6%, httpapi 94.7%, middleware 58.3%, observability 82.9%; commands, migrations, PostgreSQL and Redis 0.0%. Coverage is not aggregated into a misleading repository percentage.

## 2. Command evidence

| Command/check | Result |
| --- | --- |
| `test -z "$(gofmt -l .)"` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test -count=1 -json ./...` | exit 0; counts above |
| `go test -race -count=1 ./...` | exit 0; 6 tested packages, 5 without tests |
| `go build ./...` | exit 0 |
| `go test -cover ./...` | exit 0; package coverage above |
| 50 repeated runs of app/health/httpapi/middleware/observability suites | exit 0; no observed flake |
| Missing-config server command | exit 1, zero stdout bytes, one valid JSON `CONFIG_MISSING` record; under 10 s |
| Invalid migration verb | exit 1, zero stdout bytes, one valid JSON `MIGRATION_USAGE` record |
| Unreachable PostgreSQL with seeded DSN secret | exit 1, valid JSON `POSTGRES_UNAVAILABLE`; sentinel scan clean |
| Canonical module identity | 17 `github.com/ferousco-dev/layr/server` references; 0 stale `github.com/ferousco/layr/server` references |
| Forbidden M1.2+ runtime scan | 0 source/config matches for Figma/AI/Chromium/brokers/object-store sample |
| Docker/container execution | NOT RUN by explicit user constraint |

No test failure output exists. One defect was found by independent review: DEF-001.

## 3. Requirement evidence and unverified scope

The design catalogue contains TC-001..TC-060, exactly two per each of 30 REQ/NFR/DOM requirements. Local evidence supports configuration defaults/failures, liveness, fake readiness states, request-ID validation, routing errors, body-limit primitive, structured-key redaction, ordered signal shutdown and second-signal force, formatting/vet/test/race/build, module identity and runtime-scope scans.

The following remain explicitly unverified and are not passes: live PostgreSQL and Redis open/ping/close; migrations against PostgreSQL including three apply/apply/down/apply cycles; real readiness with dependencies; clean Docker startup in 3/3 runs; full process signal timing against a listening server; 1,000-request health p95; readiness/goroutine stress; production-environment behavior; acceptance TC-023/TC-024. The user prohibited Docker startup and image pulls due to limited data, and execution stopped for handoff.

## 4. Defects

| ID | Title | Severity | Priority | Status | Found at level |
| --- | --- | --- | --- | --- | --- |
| DEF-001 | Runtime HTTP serve termination does not close dependencies | medium | high | New | review/system lifecycle |

Open: 0 critical, 0 high-severity, 1 medium, 0 low. No defects were closed.

## 5. Exit criteria

| # | Criterion | Target | Actual | Met |
| --- | --- | --- | --- | --- |
| 1 | Requirements with two designed cases | 30/30 | 30/30, TC-001..TC-060 | yes |
| 2 | Requirements with at least one passing executed case | 30/30 | Not established; live and acceptance requirements blocked | no |
| 3 | Open critical defects | 0 | 0 | yes |
| 4 | Open high-severity defects | 0 | 0 | yes |
| 5 | Unit pass rate | 100% | 12/12 top-level; 18/18 including subtests | yes |
| 6 | Integration boundaries exercised | 100% or named gap | real DB/Redis/migration boundaries unverified | no |
| 7 | fmt/vet/test/race/build | all exit 0 | all exit 0 | yes |
| 8 | Applicable performance/security/stress thresholds | all met | secret canary passed; performance/stress incomplete | no |
| 9 | Regression automated on every change | CI evidence | workflow runs local suite on push/PR; no live integration job | partial/no |
| 10 | Acceptance sign-off | dated user/client approval | absent | no |

## 6. Recommendation

**Do not claim verification complete.** Local construction quality is promising and all executed commands passed, but G5 is not ready because real module boundaries, quantified non-functional tests, full requirement-to-result evidence, and stakeholder acceptance are absent. Preserve this report for the receiving Claude session; resume without Docker unless the user later changes that constraint.

## 6. Addendum - local run without Docker (2026-09-29, post-CR-001)

Environment: macOS darwin/arm64; Homebrew PostgreSQL 16 and a local Redis started outside Docker; Go 1.27.1. Docker/container execution remains **NOT RUN** by explicit user constraint. This evidence is labeled "local, no Docker" and does not satisfy the Docker/Compose clean-host cases.

| Check | Result |
| --- | --- |
| `gofmt -l .`, `go vet ./...`, `go build ./...` | exit 0 |
| `go test -race ./...` (after DEF-001 fix and CR-001) | exit 0; all packages with tests pass, including new postgres, redis, migrations, workspace and middleware suites |
| `go run ./cmd/migrate up` on empty `layr` database | migrated 0 → 1; `layr_foundation` schema created |
| Server start against live PostgreSQL and Redis | `server.started` on `:8080` logged, twice |
| `GET /health` | 200 `{"status":"ok"}`; security headers present (`X-Content-Type-Options`, `Cache-Control`, `Referrer-Policy`, `Content-Security-Policy`); `X-Request-Id` returned |
| `GET /ready` | 200 `{"checks":{"postgres":"up","redis":"up"},"status":"ready"}` |
| `GET /health` with `Origin: http://localhost:3000` | `Access-Control-Allow-Origin` echoes the origin; credentials allowed; no wildcard |
| SIGTERM to running server | `shutdown.started` then `shutdown.completed`; port closed |

Still unverified: dependency-outage behavior (`/health` stays 200 while `/ready` reports not ready) against real services; migration down/up cycles; load and stress cases; production-mode startup; Docker and Compose acceptance; TC-023 and TC-024.

DEF-001 is fixed locally (regression `TestRunLifecycleServeFailureClosesResources`) and awaits independent verifier confirmation.

## 7. Addendum - live boundary verification without Docker (2026-09-29, attempt 2)

Environment: macOS darwin/arm64; Homebrew PostgreSQL 16 (`layr_test` database) and Redis 7.4 started outside Docker; a second throwaway Redis on port 6390 for the outage test. Docker remains **NOT RUN**.

| Check | Result |
| --- | --- |
| Migration cycle on empty DB: down, down, up, up, down, up, status | 0→2 applied, second `up` applied 0 (idempotent), reversals stepped 2→1→0, final `current=2 dirty=false` |
| Live PostgreSQL store tests (users, connections, sessions, 8-way concurrent first login) | pass with `-race` |
| Live Redis tests (single-use `GETDEL`, owner-checked lock, rate counter) | pass with `-race` |
| End-to-end login flow against live services with fake Figma | pass; state replay rejected; no raw session token or plaintext Figma token stored |
| `GET /health` 1,000 sequential requests | p50 0.26 ms, p95 0.34 ms, p99 0.44 ms, max 6.99 ms |
| Redis stopped while server running | `/health` 200 (process alive); `/ready` 503 `redis: down`, `status: not_ready` |
| Redis restarted | `/ready` back to 200 without restarting the server |
| SIGTERM to idle server | `shutdown.started`, `shutdown.completed`, process gone in 0.06 s |
| `go test -race -count=1 ./...` with live env vars | exit 0, 15 packages with tests |

Requirement notes: closes the live PostgreSQL/Redis, migration-cycle, readiness-outage, p95 and signal-timing gaps listed in section 3. Still not executed: Docker/Compose clean-host runs, active-request SIGTERM drain, sustained stress/goroutine measurement, production-mode startup, TC-023 and TC-024 acceptance.

DEF-001 retest: regression `TestRunLifecycleServeFailureClosesResources` passes under `-race`; defect closed.

## 8. Addendum - live Figma OAuth and import verification (2026-09-30)

Environment: local server on :8080 with PostgreSQL and Redis outside Docker, a real Figma OAuth app, and a manual test page on `localhost:3000`. Docker remains **NOT RUN**.

| Check | Result |
| --- | --- |
| Figma OAuth sign-in (authorize, PKCE, token exchange, identity) | pass; `GET /api/v1/me` returned the Figma profile and the session persisted across refreshes |
| Session and CORS from `localhost:3000` to `localhost:8080` with credentials | pass |
| Create project | pass (201) |
| Import with a page node (`node-id=0-1`) | failed as designed with `FIGMA_UNSUPPORTED_NODE` |
| Import of a whole design file with 13 frames | `awaiting_selection` listing 13 frames |
| Select 3 frames | `completed` in about 16 s; 3 screens, 27 nodes, 10 assets, 0 warnings, reference render captured |
| Design overview | `ready`; 3 screens at 685x492; 3 colors and 6 text styles |
| Screen previews for the 3 screens against the Figma canvas | matched visually (manual comparison) |

Requirement notes: closes the real-Figma import gap from section 7 for the single-file, multi-frame path. Not executed: FigJam and unsupported-file rejections against real Figma, refresh-token expiry, very large files, Docker and Compose acceptance.

## 9. Addendum - M1.9 hardening and end-to-end verification (2026-09-30)

Environment: macOS darwin/arm64, Go 1.27.1; Homebrew PostgreSQL over `::1` (`layr_test`) and a private Redis on port 6391, supplied through `TEST_DATABASE_URL` and `TEST_REDIS_URL`; mock Figma and a local `httptest` CDN. Docker **NOT RUN**. Earlier runs that set `POSTGRES_URL` instead had silently skipped the live tests; this run has zero skipped tests.

| Check | Result |
| --- | --- |
| `gofmt -l .` | no files |
| `go vet ./...` | clean |
| `go build ./...` | success |
| `go test -count=1 -race -cover -p 1 ./...` with live PostgreSQL and Redis | 24 packages ok, 0 failures, 0 skipped, no data races |
| Test functions / fuzz targets / benchmarks | 353 / 7 / 5 |
| Fuzz, 20 to 30 s each: filename slug, SVG sanitizer (active-content oracle), image sniffing, Design IR decode and round trip, workspace path resolution, Figma URL parser, Figma JSON decoder | roughly 40 million executions, no crash, escape or active content |
| SaaS product E2E (7 screens across an Authentication section and a Dashboard page, 4 shared components, gradients, shadows, auto layout, repeated image fill, vectors, real asset pipeline) | 11 screens including the 4 kit components, 1 shared image asset, 7 reference renders, every IR asset resolves to a checksummed file, no URLs or host paths stored, at most 10 Figma calls |
| Reproducibility: identical input twice | manifest and IR hashes equal (import id aside) |
| 300 screens, 17,100 nodes | normalized in about 280 ms, byte-identical across two runs, 17.5 MB indented (6.5 MB compact) |
| Boundaries: screen limit, node limit, 5,000-level nesting | at the limit passes, one over fails with `DESIGN_IR_LIMIT_EXCEEDED`, nesting rejected with a typed error |
| Goroutine leak across success, failure and cancelled imports | none |
| Abandoned import recovery, live PostgreSQL | pass |
| Real Figma import by hand (OAuth, project, 13 candidate frames, 3 selected) | completed in about 15 s: 27 nodes, 10 assets, 0 warnings, 6 Figma API calls in total, previews matched the canvas visually |

Baseline on a development laptop (not a service level):

| Benchmark | Time | Allocated |
| --- | --- | --- |
| Normalize 100 screens (6,000 nodes) | 9.4 ms | 9.0 MB, 59k allocations |
| Normalize one 4,000-node screen | 6.8 ms | 5.3 MB |
| Validate the 100-screen IR | 5.1 ms | 0.44 MB, 67 allocations (was 17.7 MB, 302k) |
| Marshal and unmarshal the 100-screen IR | 29 ms | 28.8 MB (was 128.6 MB) |
| Decode a 5.3 MB Figma body with minimal nodes | 141 ms, 37 MB/s | 199 MB (was 240 MB) |

Findings fixed: DEF-002 (imports stuck after a crash), DEF-003 (database outage as generic 500, missing request ID in a failure log). Improvements: validator allocation, compact IR JSON, no-follow `Dir.Open`, child decode without a copy.

Still not executed: Docker and Compose runs, sustained load, active-request SIGTERM drain with the real server under load, a real Figma run of FigJam rejection, refresh-token expiry against Figma, very large real files. A 10-user parallel-import test was not written; concurrency is covered by `TestConcurrentStartsAllowExactlyOne`, `TestBusyImporterRejectsInsteadOfQueueingForever`, `TestConcurrentRunsUnderTheRaceDetector` and the two-user live isolation tests.
