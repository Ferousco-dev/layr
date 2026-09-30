# LAYR Server Coding Standard

Status: M1.1 construction baseline. Applies to `server/`.

## Go source

- Run `gofmt`; CI rejects any file that it would change.
- Keep imports in standard-library, blank-line, third-party/project groups.
- Use conventional Go names: exported identifiers in `CamelCase`, local names
  concise but descriptive, initialisms consistently capitalized.
- Prefer small consumer-owned interfaces and constructor injection. Do not use
  mutable package globals or service locators.
- Pass `context.Context` as the first argument to blocking work. Never retain a
  context in a long-lived struct.
- Return or handle every error. Wrap errors only with safe categories; driver
  errors and secret-bearing configuration never enter HTTP or routine logs.
- Use early returns and focused helpers. `goto` and commented-out code are not
  permitted.

## Readability limits

The project uses `gofmt` rather than a hard 80-column formatter. Aim for 100
columns and split long calls, conditionals, and structured log records across
lines. Blank lines separate setup, work, and outcome. A function should perform
one task; functions over roughly 40 lines or with more than five branches need
review, not automatic rejection. These modern Go limits preserve the principle
behind the course's 80-column/10-line guidance without creating fragmented code.

Comments explain package responsibility, invariants, concurrency, or a decision's
reason. They do not restate obvious operations. Tests use visible arrange, act,
and assert sections when the distinction is not already obvious.

## Boundaries and verification

Production modules cite their `DES` element in package comments. User-visible
codes and messages come verbatim from `docs/error-catalogue.md`. New runtime
dependencies require a version and licence entry in `docs/dependencies.md`.
Before handoff run formatting, vet, unit tests, race tests, build, Docker Compose
validation, and a secret scan.
