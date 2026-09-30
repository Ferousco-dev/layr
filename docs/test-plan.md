# Test Plan: LAYR Server M1.1 Foundation

| Field | Value |
| --- | --- |
| Document ID | TP-LAYR-M1.1-v1 |
| Written on | 2026-09-29, before verifier execution |
| Author | Ìlànà verifier |
| Approved by | Pending G5 review |
| Baselined SRS | `docs/srs.md`, approved at G1 on 2026-09-29 |
| Design inputs | `docs/design.md`, `docs/data-model.md`, `docs/modules/*`, ADR-001..ADR-004 |
| Interface inputs | `docs/ui-spec.md`, `docs/error-catalogue.md` |

## 1. Objectives

Independently establish whether M1.1 starts and stops safely, exposes truthful bounded health signals, preserves request correlation, emits parseable logs without seeded secrets, applies reversible SQL migrations, and can be reproduced with PostgreSQL and Redis. Testing also checks that failures are bounded and classified rather than silently degraded.

## 2. Scope

### 2.1 Features to be tested

| Feature | Requirements | Level(s) | Priority |
| --- | --- | --- | --- |
| Typed configuration and startup | REQ-001, NFR-001, NFR-002, NFR-003, NFR-006 | unit, integration, system | high |
| PostgreSQL and Redis lifecycle | REQ-002, REQ-003, NFR-005, DOM-001, DOM-002 | unit, integration, system | high |
| SQL migrations | REQ-004, NFR-010, DOM-003 | unit, integration, system | high |
| HTTP, liveness, readiness | REQ-005..REQ-007, NFR-004, NFR-005 | unit, integration, system | high |
| Correlation and safe structured logs | REQ-008, REQ-009, NFR-007, NFR-008 | unit, integration, system | high |
| Graceful shutdown and concurrency | REQ-010, NFR-009, NFR-013, DOM-001 | unit, integration, system | high |
| Local stack and operator guidance | REQ-011, REQ-012, NFR-014, NFR-015 | system, acceptance | high |
| Build quality and isolation | NFR-011, NFR-012 | system | high |

### 2.2 Features not to be tested

| Feature | Why not | Risk accepted | Accepted by |
| --- | --- | --- | --- |
| Figma OAuth, user/session, projects, import, assets, Design IR, generation | Explicitly outside M1.1 | Later milestones remain unverified | Stakeholder through G1 baseline |
| Production cloud topology and production traffic | No production environment exists in M1.1 | Environment-specific defects may remain | Pending release decision |
| Real secrets or customer data | Prohibited test data | Provider-specific edge cases may remain | Ethics control ETH-001..003 |

## 3. Strategy and levels

| Level | In scope | Technique and tools | Owner | Entry criteria | Exit criteria |
| --- | --- | --- | --- | --- | --- |
| Unit | Yes | Go table tests, fakes, boundary and negative tests; `go test ./...` | constructor authors; verifier independently executes and inspects | packages compile | all unit tests pass; failure paths represented |
| Integration | Yes | Real PostgreSQL/Redis and HTTP process where feasible; migration cycles; cancellation/shutdown | verifier | G4 handoff and dependencies available | every declared module boundary has evidence or an explicit gap |
| System | Yes | Build/run complete service, probe endpoints, signals, Docker validation, security/performance checks | verifier | unit/integration suite passes sufficiently to run | SRS-wide checks executed; high/critical defects block recommendation |
| Acceptance | Planned, not executable by verifier | Stakeholder follows README/runbook clean-start and recovery scenarios | user/client, never constructor or verifier | system evidence available | dated business sign-off |

Acceptance is intentionally absent from verifier execution because it is a stakeholder judgement. Until signed, G5 criterion 10 and the plan's acceptance exit criterion remain unmet; risk is that technical correctness may not match operator expectations.

## 4. Environment

| Element | Test | Production | Difference | Blind spot |
| --- | --- | --- | --- | --- |
| Host | macOS, local Go and Docker versions recorded in report | Undecided | OS/runtime topology may differ | signal, networking, filesystem behaviour |
| PostgreSQL | repository-pinned container | Undecided | version/configuration may differ | extensions, permissions, resource limits |
| Redis | repository-pinned container | Undecided | version/configuration may differ | eviction, TLS, authentication |
| HTTP | loopback and ephemeral ports | public ingress unknown | no proxy/TLS/CDN | forwarded headers and ingress timeouts |
| Data | synthetic sentinel values only | proprietary design/account data later | scale and shape differ | production cardinality and unusual inputs |

Test data uses synthetic records and obvious canary secrets such as `SENTINEL_SECRET_DO_NOT_LOG`; no OAuth credential, live Figma request, or personal/customer data is permitted.

## 5. Roles

| Role | Person/agent | Responsibility |
| --- | --- | --- |
| Test lead and independent tester | Ìlànà verifier | plan, adversarial execution, report, defects, G5 recommendation |
| Developer | constructor | implementation and constructor-owned automated tests; defect fixes |
| Acceptance tester | user/client | business acceptance and operator walkthrough |
| Defect triage | conductor with verifier evidence | priority decision; verifier assigns technical severity |

## 6. Risks

| ID | Risk | Likelihood | Impact | Mitigation | Owner |
| --- | --- | --- | --- | --- | --- |
| RSK-T01 | Docker daemon/images unavailable | medium | high | validate static Compose config; record integration checks blocked, never infer pass | verifier |
| RSK-T02 | Same-host verification differs from production | high | medium | record versions and blind spots; require deployment smoke suite later | release manager |
| RSK-T03 | Timing tests are noisy | medium | medium | repeat samples, state host/load, use generous SRS thresholds | verifier |
| RSK-T04 | Stakeholder acceptance unavailable at G5 | high | high | preserve unsigned criterion and require explicit release risk acceptance | conductor |
| RSK-T05 | Concurrent construction changes evidence during execution | medium | high | execute only after G4; record commit/tree state and exact commands | verifier |

## 7. Schedule

| Milestone | Date | Depends on |
| --- | --- | --- |
| Plan and cases baselined | 2026-09-29 | G1-G3 artifacts |
| Test execution begins | after G4 | constructor handoff |
| Defect retest, if needed | after constructor fix handoff | named regression test |
| G5 attempt | after report completion | all feasible evidence and explicit gaps |

## 8. Deliverables

- `docs/test-plan.md`
- `docs/test-cases.md`
- `docs/automation-strategy.md`
- `docs/test-report.md`
- `.ilana/defects.md`
- `.ilana/gates/G5.md` after G4 exists

## 9. Exit criteria

| # | Criterion | Target | Actual | Met |
| --- | --- | --- | --- | --- |
| 1 | Requirements with at least two designed cases | 30/30 | Pending execution report | Pending |
| 2 | Requirements with at least one passing case | 30/30 | Pending | Pending |
| 3 | Open critical defects | 0 | Pending | Pending |
| 4 | Open high defects | 0 | Pending | Pending |
| 5 | Unit pass rate | 100% | Pending | Pending |
| 6 | Integration boundaries exercised | 100% or named gap per boundary | Pending | Pending |
| 7 | `fmt`, `vet`, `test`, `test -race`, `build` | all exit 0 | Pending | Pending |
| 8 | Applicable performance/security/stress thresholds | all met | Pending | Pending |
| 9 | Regression suite automated on every change | CI evidence present and green | Pending | Pending |
| 10 | Acceptance sign-off | dated user/client approval | Pending | Pending |

## 10. Suspension and resumption

Suspend execution if the repository changes materially after evidence capture, the build cannot compile, required infrastructure is unavailable for all integration checks, or a critical confidentiality/data-loss defect is observed. Resume from a recorded tree state after a constructor handoff, with infrastructure healthy and any critical containment documented. Never delete or suppress a failing test.
