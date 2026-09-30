# Elicitation Plan — LAYR Server M1.1

## Stakeholders

| Stakeholder | Role | Interest/influence | Consultation |
| --- | --- | --- | --- |
| Project owner | commissioner, developer, operator, initial user | high/high | written interview/read-back, 2026-09-29 |
| Limited testers | later operators/users | reproducibility/diagnostics; medium | acceptance observation after build |
| Public developers/designers | eventual users, affected not yet consulted | availability/confidentiality; high | deferred until workflows exist |
| Future security/SRE reviewers | absent reviewers/operators | controls/recovery; medium | review before public launch |

## Techniques

| Technique | Applied to | Result |
| --- | --- | --- |
| Written stakeholder interview | intake | users, harms, success, schedule, ownership |
| Document analysis | 1,970-line brief | scope, constraints, sequencing, failure paths |
| Repository observation | workspace | placeholder README; no implementation or Git history |
| Scenario analysis | startup/readiness/migration/shutdown | observable acceptance and failure cases |

## Validation

1. Stakeholder reviews the SRS, DEC-004..DEC-007 proposals, and numeric thresholds.
2. Architect checks feasibility without changing intent.
3. Verifier derives a test for every matrix row.
4. A limited tester follows clean-start and dependency-recovery instructions before closure.

Before M1.2, elicit session lifetime, OAuth scopes, encryption/key rotation, privacy jurisdiction, retention/deletion, and incident response. No UI prototype is justified for machine-facing M1.1; Docker clean-start observation is its operational prototype.
