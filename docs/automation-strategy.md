# Test Automation Strategy: LAYR Server M1.1

| Field | Value |
| --- | --- |
| Owner | Ìlànà verifier for strategy; constructor for test implementation |
| Maintenance owner | Primary repository maintainer, with verifier regression review |
| Runs on | Every pull request/change through `.github/workflows/ci.yml`; local pre-release rerun |
| Written | 2026-09-29, before verifier execution |

## 1. Automation allocation

| Area | Automate | Reason | Tool |
| --- | --- | --- | --- |
| Regression | yes | highest-repeatability value | Go test runner and CI |
| Configuration, HTTP, middleware | yes | bounded deterministic input partitions | Go tests / `httptest` |
| PostgreSQL/Redis and migrations | yes | interface failures dominate foundation risk | containers plus Go/shell commands |
| Smoke and build quality | yes | fastest build-worthiness signal | `gofmt`, `go vet`, `go test`, race detector, `go build` |
| Load/stress thresholds | yes | repeated timing and concurrency are not reliable manually | Go benchmarks/tests or scripted HTTP load |
| Secret-canary scan | yes | confidentiality needs exact zero-occurrence evidence | log parser plus exact-string scan |
| Exploratory failure injection | partly/manual | novel combinations require judgement | verifier commands and observation |
| Usability/operator comprehension | no | requires representative human judgement | acceptance walkthrough |
| Final acceptance | no | business decision cannot be delegated to automation | user/client sign-off |

## 2. Pyramid targets

| Level | Target | Runtime budget | Current |
| --- | --- | --- | --- |
| Unit | majority of suite; every pure branch and numeric boundary | under 60 seconds | Pending inspection |
| Integration | every declared module boundary, real dependencies for storage paths | under 5 minutes | Pending execution |
| System | health/readiness/startup/shutdown/migration/Docker critical paths | under 20 minutes | Pending execution |
| Acceptance | three operator journeys: clean start, recover readiness, migration rollback | human scheduled | Pending user |

## 3. Tooling

| Tool | Purpose | Version | Owner |
| --- | --- | --- | --- |
| Go toolchain | format, vet, unit/integration/race/build | record from `go version` at execution | maintainer |
| Docker Compose v2 | PostgreSQL/Redis and clean-stack system checks | record at execution | maintainer |
| GitHub Actions | every-change regression gate | repository workflow revision | configuration engineer |
| POSIX shell/HTTP client | black-box lifecycle and endpoint probes | record at execution | verifier |

## 4. CI sequence and evidence

The blocking sequence is format check, vet, tests, race tests, and build. Integration jobs must start pinned PostgreSQL and Redis services, wait on health checks, apply migrations, and run system probes. Logs and artifacts must retain exact command exit codes without printing environment secrets. Any conditional skip must be visible and counted as skipped, never passed.

## 5. Maintenance and flakiness

A flaky test is `DEF-###`, with severity and owner. It is removed from the blocking path only by explicit quarantine that remains visible, then fixed or deleted only when its protected requirement is covered elsewhere. Timing assertions use bounded contexts and recorded host/load; arbitrary sleeps are rejected. Every closed defect names a regression `TC-###`.

## 6. Automation limits

Automation cannot establish that a requirement is correct, judge whether operator guidance is understandable, simulate the unknown production ingress, discover every secret shape, or provide stakeholder acceptance. Those gaps remain explicit in the test report and release decision.
