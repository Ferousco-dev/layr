# Usability Review Record: LAYR Server M1.1

| Field | Value |
| --- | --- |
| Date | 2026-09-29 |
| Facilitator | Ìlànà interaction-designer |
| Participant role | no representative user participated |
| Interface version | UI-LAYR-M1.1-v1 |
| Method | specification heuristic walkthrough |

This is an expert specification review, not a user study. At rigour 3, representative-user review is not a G3 requirement. No claim of observed user performance is made.

## Tasks walked through

| # | Task | Result | Evidence / remaining validation |
| --- | --- | --- | --- |
| 1 | Determine whether the process is alive | specified | one `GET /health`; runtime validation pending construction |
| 2 | Determine why the service is unready | specified | one `GET /ready` shows both named checks; runtime validation pending |
| 3 | Start a clean local stack | specified | five-step UI-007 flow; documentation walkthrough pending |
| 4 | Inspect/apply/reverse migration state | specified | three verbs, JSON results, one-step reversible `down`; runtime validation pending |
| 5 | Diagnose startup/shutdown failure | specified | stable codes, event names, next actions, no secret values; runtime validation pending |

## Five-principle findings

| ID | Finding | Principle | Severity | Disposition |
| --- | --- | --- | --- | --- |
| USAB-001 | `health` and `ready` could be confused without controlled definitions. | structure | medium | Resolved in vocabulary and distinct response schemas. |
| USAB-002 | Shell-oriented output that mixes prose and JSON would be fragile for automation and screen readers. | simplicity / accessibility | medium | Resolved: one JSON object per line, no ANSI/cursor output. |
| USAB-003 | A generic 503 would not identify the dependency an operator must restore. | visibility / feedback | high | Resolved: both named checks and bounded states are always returned. |
| USAB-004 | Unbounded or multi-step rollback could destroy more schema state than intended. | tolerance | high | Resolved: `down` reverses exactly one migration and `up` is the undo; reset/force excluded. |
| USAB-005 | Error detail useful for diagnosis could leak credentials or implementation internals. | feedback / tolerance | high | Resolved: stable safe messages plus request/event correlation; secret-bearing detail prohibited. |
| USAB-006 | Migration and startup feedback over one second has no graphical progress display. | feedback | low | Accepted: lifecycle JSON events provide observable progress without untrustworthy percentages. |

## Accessibility and parseability review

- Pass by specification: no colour-only meaning, mouse dependency, animation, cursor rewriting, or layout-dependent tables at runtime.
- Pass by specification: stable text labels, JSON properties, exit statuses, HTTP statuses, and request IDs support automation and assistive reading.
- Not yet evidenced: actual command output, help/usage output, HTTP headers, and logs require construction tests and an operator documentation walkthrough.
- Not applicable: visual contrast, focus ring, pointer target, and reduced-motion controls because no GUI is shipped in M1.1.

## Destructive-action review

| Action | Safeguard | Verdict |
| --- | --- | --- |
| migration `down` | explicit verb, one step only, resulting versions shown, `up` undo | acceptable |
| SIGINT/SIGTERM shutdown | deliberate signal, idempotent handling, bounded drain, restart recovery | acceptable |
| forced shutdown on timeout | configured grace period, failure event and non-zero exit | acceptable with verification |

No M1.1 command may drop the database, reset all migrations, force a migration version, or erase application data.

## Review conclusion

The interface specification is internally reviewable at rigour 3 and covers each M1.1 human/developer-facing requirement. G3 remains dependent on architecture cross-reference and confirmation that construction adopts the specified contracts. Representative-user evidence is explicitly absent rather than fabricated.
