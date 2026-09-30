# Module Specification: Health (DES-024, DES-025)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/health` |
| Requirements | REQ-006, REQ-007, NFR-004, NFR-005, DOM-001 |
| Depends on | Consumer-owned `Checker { Ping(context.Context) error }` |
| Depended on by | DES-028 |

## Responsibility and interfaces

Report process liveness and bounded aggregate dependency readiness.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `Liveness(w,r)` | request/writer | stable 200 JSON | safe 500 on encode failure | process is serving | Zero checker calls |
| `Readiness(w,r)` | request/writer, exactly pg+redis checkers | 200 or 503 stable JSON | raw probe errors suppressed | aggregate and child bounds configured | Both results settle or time out by bound |

Liveness emits only `{status:"ok"}`. Readiness starts at most two goroutines, each with a child context no longer than 500 ms, collects exactly two buffered results, and classifies each as `up`, `down`, or `timeout`; overall status is ready only for two `up` results. The aggregate context is no longer than 1 s and response encoding leaves timing margin. Complexity O(1) time/space.

Handlers are concurrency-safe and store no request state. Buffered result channels plus context-aware checkers prevent goroutine leaks. Tests use call-counting, immediate-failure, and blocking checkers plus injectable clock/deadline settings. Raw errors and endpoints never enter the response. Interface trace: UI-001 liveness and UI-002 readiness (Phase 03).

