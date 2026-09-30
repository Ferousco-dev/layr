# Risk Register

## RSK-001 — Credential or private-design exposure

- Likelihood: possible
- Impact: high
- Mitigation: least-privilege OAuth, encrypted persisted credentials, secure sessions, strict ownership checks, log redaction, temporary-data cleanup, and security-focused verification.
- Status: open

## RSK-002 — Operational complexity exceeds maintainer capacity

- Likelihood: possible
- Impact: medium
- Mitigation: modular monolith, minimal infrastructure, explicit migrations, reproducible Docker setup, automated tests, structured logs, and documented recovery.
- Status: open
