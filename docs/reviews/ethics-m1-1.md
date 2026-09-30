# M1.1 Ethics Review

| Field | Value |
| --- | --- |
| Scope | LAYR server M1.1 foundation |
| Reviewer | ethics-officer |
| Date | 2026-09-29 |
| Trigger | G0 intake and approved FLEET execution |
| Verdict | Proceed with controls; no ethics halt |

## People and harm

Developers, designers, and their organizations could be harmed by unauthorized access to Figma resources, cross-user exposure of private designs, loss or corruption of project metadata, reputational damage, or disclosure of unreleased product work. Future leakage of Figma or GitHub OAuth credentials could widen unauthorized access; leakage of a BYOK AI key could also cause financial charges. Incorrect frontend generation is not expected to cause physical harm, and M1.1 implements no generation or external identity flow.

## Data and exposure

The planned service may hold user email, display name, avatar metadata, stable Figma identity, application sessions, encrypted OAuth credentials, project identifiers, private design structure, images, SVGs, reference renders, and later generated source. The approved policy is data minimization: identity, connection, and necessary project metadata may be durable; raw designs, assets, renders, and workspaces are temporary unless a later explicit requirement changes that rule. BYOK keys are outside M1 and should not be persistently stored by default.

| Dimension | Present | Assessment |
| --- | --- | --- |
| Personal data | Planned after M1.1 | Identity and session metadata require access control, minimization, and log redaction. |
| Credentials | Planned after M1.1 | OAuth and session secrets are high-value; plaintext persistence or client exposure is prohibited by the brief. |
| Proprietary designs | Planned in later M1 | Commercial and reputational impact is plausible if tenant isolation or cleanup fails. |
| Temporary files | Planned in later M1 | Worker-local storage is acceptable only with restrictive isolation, limits, and recovery cleanup. |
| Medical, children's, or safety-critical data | Not identified | No basis for rigour 4 or 5 from current scope. |
| Automated decisions affecting people | No | Generation is out of M1.1 and is not described as consequential decision-making. |
| Legal/regulatory obligation | Unspecified | Do not claim formal regulatory compliance; identify applicable privacy obligations before public launch. |

## Five-issue assessment

| Issue | Assessment | Evidence / action |
| --- | --- | --- |
| Honesty and integrity | Conditionally compliant | The brief prohibits hidden uncertainty and false success claims. No test or build results exist yet; all later claims need command evidence. See ETH-003. |
| Confidentiality | Material engineering risk, no current breach | Intake and brief identify secret handling, redaction, encryption, authorization, and retention expectations. Convert them to testable requirements. See ETH-001 and ETH-002. |
| Quality and safety | Proceed | There is no deadline pressure and the user explicitly prefers scope reduction over bypassing security or verification. |
| Intellectual property | Not yet evidenced | No dependencies or third-party code exist in the repository. Licence review begins when dependencies are selected. See ETH-003. |
| Professional responsibility | Proceed with independent evidence | A single maintainer assisted by agents raises review concentration risk; automated tests, explicit migrations, documented recovery, and later human security review are proportionate mitigations. |

## M1.1 boundary conditions

- Environment examples must use placeholders and must not contain working credentials.
- Configuration errors and structured logs must redact secret values, including URLs if they embed credentials.
- Health/readiness responses must expose status only, not database URLs, Redis URLs, environment dumps, or internal error detail.
- PostgreSQL and Redis development defaults must not be represented as production-safe settings.
- M1.1 must not claim OAuth, session security, encryption, tenant isolation, retention cleanup, or regulatory compliance as implemented; those belong to later scoped work unless explicitly added through change control.
- Dependencies must be justified, licence-recorded, and used consistently with their terms.
- Verification reports must state exactly which commands ran, their observed outcomes, and any unavailable infrastructure.

## Public-interest assessment

The product has legitimate developer-tooling value and no identified safety-critical or public-infrastructure role. Its main public-interest duty is preventing avoidable credential and proprietary-data exposure. The present plan supports that duty through least privilege, separation of application sessions from provider tokens, temporary-data minimization, ownership checks, and gated verification. These are planned controls until implementation and tests provide evidence.

## Findings and disposition

| ID | Severity | Required disposition |
| --- | --- | --- |
| ETH-001 | major | Trace confidentiality controls into requirements, design, implementation, and tests before affected functionality advances. |
| ETH-002 | major | Define and verify the full temporary-workspace lifecycle before workspace functionality ships. |
| ETH-003 | minor | Maintain licence evidence and truthful command-result reporting through closure. |

No halt is raised. The findings are preventive engineering obligations; none establishes an actual confidentiality breach, licence violation, fabricated verification result, or unsafe release decision.
