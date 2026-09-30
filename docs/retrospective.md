# Retrospective — M1.1 and M1.2

## What did the process catch?
- DEF-001 (dependencies not closed when the server stops on its own) was caught by independent review at G4 and fixed with a named regression test. In production it would leak connections on every abnormal exit.
- Scope gaps in the approved SRS (CORS, security headers, workspace root, `APP_ENV`) were caught by comparing the stakeholder brief with the code and went through CR-001.

## What did the process miss?
- Panic recovery and the request body limit were specified and reviewed as done at G4, yet neither was attached to the handler chain. They were found only by a later read. G4 code review did not check that middleware was wired, only that it existed.
- An accidental `migrate down` hit the development database because the shell environment was reused. Tooling should refuse destructive commands against a database named without an explicit flag.

## Where did the process cost more than it returned?
- G6 to G8 ceremony (tags, CI logs, independent audit) produced no findings for a single-maintainer, pre-release repository and ended as overrides.

## The one change for the next run
- Add a wiring check to G4: every middleware and route in the design must be exercised by one end-to-end request in a test. Owner: constructor. Start: M1.3. Metric: zero specified-but-unwired components found after G4.

## Closing number
Defects found before G5: 3 (DEF-001 plus the two unwired components). Found after G4 review: 2 of those 3. Found in production: 0 (not released). Prevention ratio 1/3 = 33%. Baseline for next run; target 80%.
