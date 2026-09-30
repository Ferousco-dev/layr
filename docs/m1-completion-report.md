# M1 Completion report (M1.9)

Date: 2026-09-30. Scope: hardening, performance and end-to-end verification of M1.1–M1.8. No product features were added and nothing from M2 was started.

## Executive summary

M1 is technically ready to be the foundation for M2, with the listed residual items. The complete path from Figma sign-in to a validated Design IR, asset manifest and reference renders works, was attacked with hostile and oversized input, and is deterministic. The audit found two real defects (imports stuck after a crash, database outages reported as generic 500) and one memory/size inefficiency; all are fixed and tested. Docker was **not run**, so Docker-based deployment paths remain unverified.

## Architecture and pipeline

See [m1-architecture.md](m1-architecture.md). Verified end to end: OAuth session, project, URL parse, import, multi-frame selection, one batched render request, image and vector assets (sanitized), reference renders, Design IR, validation, Design API and preview streaming.

## Multi-screen and large designs

| Case | Result |
| --- | --- |
| SaaS fixture: 7 product screens (a section of 3 and 4 dashboard frames) plus a kit page | 11 screens, section preserved, components shared across screens with correct instance counts, one asset for the repeated background |
| 100 and 300 screens | Normalized and validated; 300 screens (17,100 nodes) in about 280 ms, byte-identical on repeat |
| Thousands of nodes in one screen | Passes |
| Screen and node limits | Exactly at the limit passes, one over gives `DESIGN_IR_LIMIT_EXCEEDED` |
| 5,000 nested levels | Rejected with a typed error, no crash |

## Audit findings and fixes

| Finding | Type | Fix |
| --- | --- | --- |
| Imports stuck in `processing` after the process dies (DEF-002) | Reliability | `Sweep` fails abandoned imports with `IMPORT_INTERRUPTED`; live PostgreSQL test |
| Database outage returned 500 on project, import, design routes; import failure log had no request ID (DEF-003) | Reliability, observability | `postgres.IsUnavailable` and a shared responder: 503 `DEPENDENCY_UNAVAILABLE` with `Retry-After`, request ID in every failure log |
| Validator built a path string for every field of every node | Performance | Path built only on failure: 17.7 MB to 0.44 MB, 302k to 67 allocations, 10.4 ms to 5 ms |
| Design IR was indented JSON, 2.7 times larger; `MAX_IR_BYTES` (64 MiB) was reachable at about a third of the node limit | Performance, consistency | Compact JSON, about 380 bytes per node |
| `Dir.Open` followed file-level symlinks while the read path did not | Security (defense in depth) | Opens with `O_NOFOLLOW`; hostile-name and link tests added |
| Child nodes decoded into a temporary and copied | Performance | Decoded in place: 17% less allocation |
| Live tests were silently skipped when the wrong environment variable names were used | Test process | Correct `TEST_DATABASE_URL` and `TEST_REDIS_URL`; suite shows 0 skipped |

Checked and found already sound (with tests): IDOR across users and mismatched nested IDs, mass assignment, parameterized SQL, session and OAuth state handling, token encryption, refresh single-flight, Figma retry ownership, SSRF and redirect revalidation, SVG sanitization, path traversal, symlinks, cleanup guards, production config rules, CORS, security headers, private preview caching and ETag, pagination bounds, body limits, secret-free logs.

## Security

Invariants and their tests: [m1-security.md](m1-security.md). No known straightforward path remains for cross-user access, token leakage, arbitrary URL fetch, workspace escape, arbitrary file read, unsafe recursive deletion or unbounded asset download. This is a statement about what was tested, not a guarantee.

Residual risks: SVGs are sanitized but not proven safe for inlining; raw Figma JSON in the workspace is protected by permissions and TTL only; one static encryption key with no rotation; Redis outage stops rate-limited routes (fails closed); a maximum-size Figma body with minimal nodes could use close to 1 GB while decoding (a `Node` is about 1 KB in memory whatever its JSON size).

## Reliability

Failure matrix: [m1-failure-matrix.md](m1-failure-matrix.md). Covered by tests: cancellation during Figma and asset work, timeout, per-job panic isolation, duplicate and concurrent starts, busy importer, partial files removed, no goroutine growth after success, failure or cancel, abandoned imports retired. Graceful and forced shutdown were verified in M1.1 and re-run in this suite.

## Performance

Measured, not promised. Baseline table in [test-report.md](test-report.md) section 9. The real Figma import (3 screens, 10 assets) took about 15 s in total: 2.3 s node fetch, 0.9 s render request, 2.6 s reference download, 9.4 s asset pipeline (network bound), 7 ms normalization. The import used 6 Figma API calls, so there is no per-screen or per-asset call pattern. Renders and image fills are batched. The Design API makes no Figma calls (enforced by a dependency test).

## Determinism

Two identical imports produced the same manifest and the same IR (apart from the import id); 300 screens are byte-stable across runs; concurrency level does not change output.

## Fuzzing, races, analysis

Seven fuzz targets ran for 20 to 30 seconds each, about 40 million executions in total, with no panic, path escape or active SVG content. `go test -race` is clean across 24 packages. `gofmt`, `go vet` and `go build` are clean. Coverage by package is in the test report; the lowest are `httpapi/middleware` (61%, exercised through `httpapi` at 98%) and `designir` (67%, its derivation is exercised through `normalize` at 92%).

## Verification commands run

`gofmt -l .`, `go vet ./...`, `go build ./...`, `go test -count=1 -race -cover -p 1 ./...` (with live PostgreSQL and Redis), `go test -bench`, `go test -fuzz` for each target, `go test -v` skip count. `git diff --check` could not be run because this directory is not a git repository; a trailing-whitespace scan of Go sources (via `gofmt`) and of the new documents found nothing.

## Real Figma smoke test

Executed by hand with a real Figma OAuth app and account (not part of automated tests): sign-in, project, import of a 13-frame file, selection of 3 frames, design overview, previews matched the canvas. FigJam links and page-level node links were correctly rejected. Not executed: token refresh expiry against Figma, very large files.

## Local infrastructure and Docker

PostgreSQL (Homebrew, `layr_test`) and Redis (private instance on port 6391) were used. **Docker was NOT run.** No Docker files were changed.

## Files changed (major)

`internal/designir` (validator, compact codec, fuzz), `internal/normalize` (stress tests and benchmarks), `internal/imports` and `internal/imports/pgstore` (abandoned-import recovery, SaaS E2E, leak test), `internal/httpapi` (503 responder, tests), `internal/postgres` (`IsUnavailable`), `internal/workspace` (`Open`, tests, fuzz), `internal/assets` (fuzz), `internal/figma` (child decode), `internal/figmaurl` (host cases), `internal/designapi` (boundary test); documents: this report, architecture, security, failure matrix, error catalogue, design IR, test report, changelog, change register, ledger, defect log.

## Known limitations

- Component masters placed at the top level of a page are treated as importable screens, like a UI kit page.
- Imports run in the API process on local disk; one replica owns a workspace.
- No object storage, so workspaces are temporary by design.
- Real-Figma coverage is one hand-run file.
- A 10-user parallel-import test was not written.
- Gates G6–G8 remain overridden, not passed.

## M2 prerequisites and recommendations

1. Add a decode-time node counter, or lower `MAX_SNAPSHOT_BYTES`, so the worst-case decode memory is bounded by configuration rather than by estimate.
2. Decide how kit-page component masters appear in the screen picker (hide, group, or label them).
3. Treat stored SVGs as untrusted in any consumer; render them as images, not inline markup.
4. Plan encryption key rotation before storing tokens for many users.
5. Keep the Design IR (`schema_version` 1) and manifest as the only inputs; M2 should not read `raw/`.
6. If imports move to separate workers, the workspace needs shared storage and the sweep needs a lease; today one process owns both.
7. Run the Docker/Compose acceptance checks that were deliberately skipped.

## Final M1 status

M1 verification complete for everything that can run without Docker. Blocked or open: Docker-based verification, gates G6–G8, and the residual items above.
