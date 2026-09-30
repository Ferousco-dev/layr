# Test Cases: LAYR Server M1.1 Foundation

Document: TC-LAYR-M1.1-v1  
Designed: 2026-09-29, before verifier execution  
Basis: G1 SRS, G2 design, G3 interface baseline

Each requirement has a success/precondition-holds case and a failure, absent-precondition, boundary, or prohibition case. “Automated” is the planned status; execution results belong in `docs/test-report.md`.

| ID | Verifies | Level / type | Preconditions and test data | Steps | Expected result | Automated |
| --- | --- | --- | --- | --- | --- | --- |
| TC-001 | REQ-001 | unit / functional | Complete valid environment | Load configuration | Typed values equal inputs/defaults; validation succeeds | yes |
| TC-002 | REQ-001 | unit / negative | Required value missing plus invalid duration/integer/URL partitions | Load configuration for each partition | Classified validation errors name safe field only; no listener starts and no value leaks | yes |
| TC-003 | REQ-002 | integration / functional | Reachable PostgreSQL | Initialize, ping, then close pool | Ping succeeds; pool is bounded and closes once | yes |
| TC-004 | REQ-002 | integration / negative | Invalid/unreachable PostgreSQL | Start application with bounded context | Non-zero classified startup failure within deadline; never ready; no credential output | yes |
| TC-005 | REQ-003 | integration / functional | Reachable Redis | Initialize, ping, close client | Ping succeeds; configured pool is bounded and closes once | yes |
| TC-006 | REQ-003 | integration / negative | Invalid/unreachable Redis | Start application with bounded context | Non-zero classified startup failure within deadline; never ready; no credential output | yes |
| TC-007 | REQ-004 | integration / functional | Fresh PostgreSQL | Run up, up, status, down, up | Ordered apply reaches latest; second up no-op; version reports correctly; reversible cycle restores latest | yes |
| TC-008 | REQ-004 | integration / negative | Deliberately failing/invalid migration fixture or unavailable DB | Run migration command | Non-zero exit identifies safe migration/version; no secret; later files do not apply | yes |
| TC-009 | REQ-005 | system / functional | Valid config and free configured address | Start server; issue bounded normal request | Configured address serves; timeout/limit configuration is applied | yes |
| TC-010 | REQ-005 | integration / boundary | Body endpoint/test handler receives payload at and above configured maximum | Submit boundary and over-limit payloads | At-limit behavior is defined; over-limit receives 413 without destabilizing server | yes |
| TC-011 | REQ-006 | integration / functional | Server running; dependency spies armed | GET `/health` | 200 stable JSON; PostgreSQL/Redis spies receive zero calls | yes |
| TC-012 | REQ-006 | unit / negative | Response writer/encoder failure injected | Execute health handler | 500 where still possible and safe structured error; no panic or dependency call | yes |
| TC-013 | REQ-007 | integration / functional | PostgreSQL and Redis healthy | GET `/ready` | 200 stable JSON with both `up` | yes |
| TC-014 | REQ-007 | integration / negative | Matrix: PostgreSQL down, Redis down, each timeout | GET `/ready` per condition | 503 by deadline with truthful `down`/`timeout`; no credentials | yes |
| TC-015 | REQ-008 | integration / functional | Valid opaque client request ID | Send request with request-ID header | Same ID appears in context-observable handler, response header, completion log | yes |
| TC-016 | REQ-008 | integration / negative | Missing, empty, oversized, control-character, whitespace and malformed IDs | Send one request per partition | Fresh opaque ID is generated and echoed consistently; invalid input is never reflected | yes |
| TC-017 | REQ-009 | integration / functional | Lifecycle and HTTP requests; captured log sink | Exercise startup, dependency, request, shutdown events | Every record is one JSON object with required safe fields | yes |
| TC-018 | REQ-009 | integration / security | Seed password/secret/token/auth/cookie/database URL canaries and induce errors | Scan raw and parsed logs | Zero canary values; sensitive keys redacted; logging failure does not panic | yes |
| TC-019 | REQ-010 | system / functional | Active bounded request plus signal-capable process | Send SIGTERM while request active | New work stops; active work finishes in grace; HTTP, DB, Redis, logs close once in documented order | yes |
| TC-020 | REQ-010 | system / negative | Handler exceeds grace and a cleanup hook fails | Signal process and observe exit | Deadline forces close; timeout/failure safely logged; non-zero exit when cleanup fails | yes |
| TC-021 | REQ-011 | system / functional | Clean Docker host with images available | Validate/start stack, migrate, start server, poll readiness | Health checks pass and `/ready` becomes 200 via documented workflow | yes |
| TC-022 | REQ-011 | system / negative | Stop or poison either dependency | Probe readiness and stack health | Dependency is unhealthy and service does not advertise ready | yes |
| TC-023 | REQ-012 | acceptance / usability | Reviewer starts from prerequisites only | Follow setup, migration, verification and shutdown guidance | Clean start completes without unstated command; meanings are findable | no |
| TC-024 | REQ-012 | acceptance / negative | Simulate unavailable PostgreSQL then Redis and an unsupported state | Follow recovery guidance | Reviewer restores supported failures; unsupported state is explicitly labelled rather than guessed | no |
| TC-025 | NFR-001 | system / performance | Invalid critical config and unreachable dependency variants | Time startup until exit | Each exits non-zero within 10 seconds | yes |
| TC-026 | NFR-001 | system / negative | Dependency stalls instead of refusing | Start and time bounded initialization | Timeout still exits non-zero within 10 seconds; no background hang | yes |
| TC-027 | NFR-002 | unit / boundary | No timeout overrides | Load defaults and inspect server config | read-header ≤5s, read ≤15s, write ≤30s, idle ≤60s, shutdown ≤30s; all positive | yes |
| TC-028 | NFR-002 | unit / negative | Zero, negative, malformed and above-policy timeout values | Load each configuration | Each invalid value is rejected before listen with safe field-level error | yes |
| TC-029 | NFR-003 | integration / boundary | Default config; body at exactly 1 MiB | Submit body-bearing request | Request is not rejected merely for size at boundary | yes |
| TC-030 | NFR-003 | integration / negative | Default config; body 1 MiB + 1 byte | Submit request | 413 and bounded read; process stays available | yes |
| TC-031 | NFR-004 | system / performance | Recorded host; 1,000 in-process `/health` calls, concurrency 10 | Measure full latency distribution | p95 ≤100 ms and dependency call count zero | yes |
| TC-032 | NFR-004 | system / stress | Same load while dependencies unavailable/noisy | Repeat measurement | p95 remains ≤100 ms because liveness is dependency-free | yes |
| TC-033 | NFR-005 | integration / performance | One dependency unhealthy | Time `/ready` | Response ≤1s; individual probe deadline ≤500ms | yes |
| TC-034 | NFR-005 | integration / negative | Probe blocks until context cancellation | Call `/ready` repeatedly | Every response is 503 ≤1s; no accumulating goroutines | yes |
| TC-035 | NFR-006 | unit / functional | Pool values absent | Load config and construct clients | PostgreSQL max open =10 and Redis pool size =10 | yes |
| TC-036 | NFR-006 | unit / negative | Pool values 0, negative, malformed; then valid positive override | Load configuration | Invalid partitions rejected; positive override applied exactly | yes |
| TC-037 | NFR-007 | integration / security | Captured logs from all normal lifecycle paths | Parse every line as JSON | 100% parse as exactly one object/line | yes |
| TC-038 | NFR-007 | integration / security-negative | Seed canaries under password/secret/token/authorization/cookie/database_url and errors | Search complete captured output | Zero seeded secret values in logs or HTTP failures | yes |
| TC-039 | NFR-008 | integration / functional | Valid supplied request IDs over representative routes/statuses | Compare responses and completion events | 100% non-empty response IDs match corresponding logs | yes |
| TC-040 | NFR-008 | integration / negative | Missing/invalid IDs and handler error responses | Compare generated response/log IDs | 100% remain correlated, including failures; invalid input absent | yes |
| TC-041 | NFR-009 | system / performance | Listening process and active request | Signal and timestamp connection refusal plus cleanup | New connections stop ≤1s; cleanup ≤30s | yes |
| TC-042 | NFR-009 | system / negative | Stuck request and slow close hook | Signal process | Hard grace bound is honored; process does not hang beyond configured shutdown | yes |
| TC-043 | NFR-010 | system / regression | Three fresh databases | For each, run apply/apply/rollback/apply and query versions | All 3 cycles match expected versions | yes |
| TC-044 | NFR-010 | integration / negative | One migration cycle interrupted or DB unavailable | Execute cycle and resume safely | Non-zero failure is explicit; no false latest version; recovery is deterministic | yes |
| TC-045 | NFR-011 | system / quality | Constructor handoff tree | Run format-diff, vet, test, race, build for all packages | Zero changed format files and every command exits 0 | yes |
| TC-046 | NFR-011 | system / negative | Inspect CI and force/observe a representative failing test where safely supported | Evaluate pipeline behavior | Failure produces non-zero visible result; no step suppresses/ignores it | yes |
| TC-047 | NFR-012 | system / isolation | Network observation/source scan and empty OAuth/Figma env | Run full automated suite | Suite passes with zero live Figma calls and zero OAuth credentials | yes |
| TC-048 | NFR-012 | system / negative | Poison Figma/OAuth endpoint and unset credentials | Run suite | No attempted call, prompt, or credential-dependent skip masquerades as pass | yes |
| TC-049 | NFR-013 | review+system / concurrency | Source plus normal load | Inspect loop launch sites; run race suite and shutdown | Zero unbounded goroutine-per-item loops; all background work exits ≤30s | yes |
| TC-050 | NFR-013 | system / stress-negative | Repeated slow/timed-out readiness and HTTP work | Measure goroutines before/after shutdown | No sustained growth; process terminates within bound | yes |
| TC-051 | NFR-014 | system / reproducibility | Images available; clean stack for each run | Start, migrate and reach ready three times with stopwatch | 3/3 runs ready within 120s | yes |
| TC-052 | NFR-014 | system / negative | Clean host missing image/network or with one unhealthy service | Follow documented start | Failure is visible and readiness never falsely succeeds; recovery steps apply | partly |
| TC-053 | NFR-015 | review+system / scope | Repository dependency/config/runtime scan | Enumerate external runtime services | Exactly PostgreSQL and Redis are used | yes |
| TC-054 | NFR-015 | review / prohibition | Search source/config/Compose for object stores, brokers, Figma/AI/GitHub services, Chromium | Inspect matches | Zero such runtime services/processes in M1.1; documentation mentions do not count as runtime use | yes |
| TC-055 | DOM-001 | unit+integration / functional | Cancelable fake and real request/shutdown I/O | Cancel contexts at each blocking boundary | Calls return promptly with cancellation semantics; contexts are passed, not stored | yes |
| TC-056 | DOM-001 | review+integration / negative | Blocking dependency/handler; source inspection | Cancel parent and inspect long-lived structs | No uncancelable I/O or stored context; shutdown bound holds | yes |
| TC-057 | DOM-002 | integration / functional | Write any M1.1 durable metadata if present; restart/reset Redis | Inspect PostgreSQL and behavior after Redis reset | PostgreSQL remains sole durable authority; readiness recovers without durable loss | yes |
| TC-058 | DOM-002 | review / prohibition | Source/schema/config inventory | Search Redis writes and durable models | No application record exists solely in Redis; if M1.1 has no records, result explicitly N/A with design evidence | yes |
| TC-059 | DOM-003 | review+integration / functional | Repository migration inventory and fresh DB | Compare ordered SQL files to applied schema history | Every schema change is represented by ordered explicit SQL migration | yes |
| TC-060 | DOM-003 | review / prohibition | Source/dependency scan | Search ORM auto-migrate/schema mutation APIs | Zero automatic ORM schema mutation paths | yes |

## Execution rules

- Record exact command, environment, time, count, and result in `docs/test-report.md`.
- A planned case may be blocked, but never silently converted to pass.
- Any observed product defect receives a full `DEF-###` entry; the verifier does not fix it.
- Acceptance cases TC-023 and TC-024 remain blocked until executed by the user/client.
