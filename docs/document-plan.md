# LAYR Server M1.1 Documentation Plan

| Field | Value |
| --- | --- |
| Owner | Ìlànà documentarian |
| Scope | Server M1.1 foundation only |
| Status | active pre-construction plan |
| Governing requirement | `REQ-012` in [the SRS](srs.md) |

## Purpose and audiences

Documentation must let a new contributor identify the approved scope and let an operator build,
start, assess, stop, and recover the M1.1 service without private instructions. Until repository
evidence exists, procedures are labelled **planned / unverified** rather than presented as fact.

| Audience | Question the documentation must answer | Primary entry point |
| --- | --- | --- |
| New contributor | What exists, what is in scope, and where is authoritative detail? | [Repository README](../README.md) |
| Local operator | How do I configure, migrate, start, probe, stop, and recover the service? | [Runbook](runbook.md) |
| Implementer/reviewer | What behavior and constraints must code satisfy? | [SRS](srs.md), architecture/interface artifacts when published |
| Verifier | What must be demonstrated and where is evidence recorded? | SRS trace links and the verification plan when published |
| Future end user | What can M1.1 do directly? | [User documentation index](user/README.md) |

## Document inventory

The plan considers the five required documentation types. It links authoritative owners instead of
copying detailed requirements or design decisions.

| Type | Artifact | Owner | Current state | Completion condition |
| --- | --- | --- | --- | --- |
| Requirements | [M1.1 SRS](srs.md), stories, `.ilana/traceability.csv` | analyst | approved at G1; header metadata requires reconciliation by its owner | stable IDs and approved acceptance criteria agree with traceability |
| Design | architecture description and [interface specification](ui-spec.md) | architect / interaction designer | interface proposed; architecture not yet published | reviewed G2/G3 artifacts linked from `docs/technical/README.md` |
| Technical | [technical index](technical/README.md), configuration and migration references | documentarian, sourced from implementation owners | skeleton / unverified | repository-verified commands, variables, formats, and dependencies documented |
| User/operator | [runbook](runbook.md), [user index](user/README.md) | documentarian | skeleton / unverified | a new operator completes the onboarding test without hidden steps |
| Testing | test plan, cases, defect/evidence records | verifier | not yet published | each requirement maps to test evidence or an explicit justified exception |

No standalone public end-user manual is justified for M1.1 because it exposes only operational
service interfaces. The runbook serves the local operator. Public product guidance is deferred to a
later milestone with user-facing behavior.

## Source hierarchy and writing rules

1. Repository behavior, tests, migrations, and build definitions outrank descriptive prose.
2. Approved requirements define intent; design artifacts define structure; neither proves runtime behavior.
3. Commands are documented only after their defining file exists and the command has been observed.
4. Examples use placeholders and must not contain credentials, connection URLs with secrets, tokens,
   cookies, or private design data.
5. Status language is evidence-based: planned, implemented, verified, unavailable, or failed.
6. Details remain in their owning artifact. Other documents link to them and summarize only what a
   reader needs for navigation or safe operation.

The SRS uses an IEEE 830-shaped specification structure, architecture is expected to use an IEEE
1016-shaped design description, and the verification plan is expected to use an IEEE 829-shaped
test structure. This plan does not claim formal conformance certification.

## Update triggers

| Trigger | Documents to inspect in the same change | Required check |
| --- | --- | --- |
| Configuration variable/default/validation changes | runbook, technical configuration reference, environment example | name, requiredness, default, safe example, and failure behavior agree |
| Build, Docker, or start/stop command changes | README, runbook, technical index | clean-start sequence is rerun |
| Migration tool or schema changes | runbook, migration reference, rollback guidance | apply/apply/rollback/apply is observed and version output recorded |
| Endpoint, status code, or response schema changes | runbook probe section, interface/API reference | examples match handler tests and contain no internals |
| Timeout, pool, body limit, or shutdown behavior changes | SRS through change control, runbook, technical reference | requirement ID and observed test evidence remain linked |
| Logging fields or redaction rules change | runbook diagnostics, technical logging reference, ethics controls | seeded-secret scan remains zero-match |
| Dependency or supported version changes | README prerequisites, runbook, toolchain decision record | license/support evidence and clean build are updated |
| Verification command or CI workflow changes | README status wording, runbook verification, test plan | local and CI instructions name the same authoritative targets |
| Known failure or recovery path changes | runbook troubleshooting | failure is reproduced or explicitly labelled unverified |
| Milestone scope changes | README, this plan, SRS through change control, user/technical indexes | M1.2+ capability is not presented as M1.1 |

The implementation owner must flag the trigger; the documentarian updates owned documents in the
same change. Reviewers treat an applicable but missed documentation update as a defect.

## Publication and review sequence

1. Before construction: maintain honest skeletons and link approved requirements.
2. After G2/G3: add architecture and interface links; reconcile terminology and operational states.
3. During construction: replace placeholders only from repository evidence; document each supported
   command and variable when introduced.
4. During verification: run every documented procedure, record failures as defects, and remove stale alternatives.
5. Before G8: perform the onboarding test below with a person unfamiliar with the implementation.

## Onboarding test plan

**Participant:** one developer/tester who did not implement M1.1.  
**Starting state:** clean checkout, supported host, Docker images available, no unpublished notes.  
**Materials:** repository README and linked documentation only.

The participant must be able to:

1. state M1.1 scope and identify excluded capabilities;
2. identify prerequisites and create a local configuration without exposing a real secret;
3. start PostgreSQL and Redis, apply migrations, and start the server;
4. distinguish `/health` from `/ready` and interpret healthy and dependency-failure results;
5. locate correlated structured logs without finding seeded secret values;
6. run the documented verification workflow and identify which checks require infrastructure;
7. stop the service gracefully;
8. recover from one PostgreSQL outage, one Redis outage, and one failed migration scenario; and
9. return the environment to a documented stopped state.

Record start/end time, host/tool versions, every unstated command, ambiguity, failed step, and help
request. Each stuck point becomes a documentation defect with severity and owner. Pass requires all
nine tasks without private verbal instructions, zero credential exposure, and no unresolved blocking
documentation defect. Time is recorded for baseline measurement, not given a pass threshold before
the first observed run.

## Open documentation work

- Link the architecture artifact when published and update the proposed interface link after G3 disposition.
- Replace runbook placeholders with observed commands and exact supported versions.
- Add configuration, migration, API/probe, and logging references from implemented behavior.
- Link the verifier-owned test plan and evidence.
- Run and record the onboarding test before closure.
