# M1.1 Foundation Stories

## STORY-001 / REQ-001..REQ-003, REQ-005

As the initial maintainer, I want one validated startup path so invalid configuration and unavailable dependencies fail before traffic. Valid configuration plus PostgreSQL/Redis starts HTTP; any critical failure exits within NFR-001 without secret disclosure. Invalid startup never opens the listener.

## STORY-002 / REQ-004

As a maintainer, I want explicit reversible migrations so every environment reaches the same schema. An empty database passes apply/apply/rollback/apply; failure identifies its version without credentials. Constraint: NFR-010, DOM-003.

## STORY-003 / REQ-006, REQ-007

As an operator, I want separate liveness/readiness so dependency outage stops traffic without declaring the process dead. Health remains 200 without probes; readiness is 200 only when both dependencies pass and otherwise 503 within NFR-005.

## STORY-004 / REQ-008, REQ-009

As a maintainer, I want correlated structured logs so failures are diagnosable without leaking credentials. Response and JSON completion log share one request ID; invalid IDs are replaced; seeded secrets have zero matches.

## STORY-005 / REQ-010

As an operator, I want bounded graceful shutdown so active work can finish and resources close once. New work stops within one second, active work inside the deadline completes, and close hooks run once; deadline expiry forces closure and reports failure.

## STORY-006 / REQ-011, REQ-012

As a contributor, I want a reproducible documented stack so I can start, migrate, test, stop, and recover it without hidden knowledge. A clean Docker run reaches readiness within NFR-014; unhealthy dependencies remain unready with documented recovery.

## Shared definition of done

Trace links are current; relevant tests pass; NFR-011 commands pass; documentation and `.env.example` match behavior; each story includes its failure path above.
