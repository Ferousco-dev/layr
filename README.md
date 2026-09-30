# LAYR

LAYR turns structured Figma designs into developer-owned frontend code. Users sign in with Figma and organise their work in projects.

The repository currently contains the backend. Figma importing, design analysis, code generation and export are planned and not built yet.

## Repository map

```text
LAYR/
├── server/    # Go API: authentication, sessions, projects (see server/README.md)
├── client/    # web application (not started)
├── docs/      # requirements, design, operations and user guidance
├── .ilana/    # engineering ledger, gates, traceability and registers
└── README.md
```

## Getting started

Everything needed to configure, run (with or without Docker), test and deploy the backend is in [server/README.md](server/README.md).

## Documentation

- [Requirements](docs/srs.md)
- [Design](docs/design.md) and [decision records](docs/adr)
- [Error catalogue](docs/error-catalogue.md)
- [Runbook](docs/runbook.md)
- [Test plan](docs/test-plan.md) and [test report](docs/test-report.md)
- [Changelog](CHANGELOG.md)

## Status

Local unit tests and live PostgreSQL/Redis tests pass. Docker images have not been built or run yet, and the sign-in flow has not been tried against a real Figma app. Evidence is in [docs/test-report.md](docs/test-report.md).

## Contributing and license

Contribution policy and licensing are not yet established.
