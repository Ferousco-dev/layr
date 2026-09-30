# Module Specification: Configuration (DES-019)

| Field | Value |
| --- | --- |
| Design level | detailed |
| Owner | `internal/config` |
| Requirements | REQ-001, REQ-005, NFR-001..003, NFR-005..006, NFR-009, NFR-014 |
| Depends on | Go standard library |
| Depended on by | DES-021..DES-023, DES-028..DES-030 |

## Responsibility and interface

Produce one complete, immutable typed configuration value from a supplied environment lookup.

| Operation | Inputs | Outputs | Errors | Preconditions | Postconditions |
| --- | --- | --- | --- | --- | --- |
| `Load(lookup)` | `func(string)(string,bool)` | `Config` | `FieldError{Name,Kind}` | lookup non-nil/stable for call | Success means every critical field is present and all bounds positive |

`Config` groups `HTTP`, `Postgres`, `Redis`, and `Log` values. Secret-bearing URL fields must use a named type without `String`/JSON/text marshaling. Errors contain the canonical variable name and category (`missing`, `malformed`, `out_of_range`) but never the supplied value. The returned value is copied, never mutated.

## Algorithm and errors

Walk the fixed variable descriptor table once, parse durations/integers/bytes, apply documented defaults, then run cross-field checks. Time O(v), space O(v), where v is the small documented variable count. Accumulate only safe field errors or fail on the first—construction may choose one policy but must test and document it consistently.

## Concurrency and test hooks

Safe for concurrent reads after construction; `Load` itself uses only local state. Inject lookup rather than changing the process environment in unit tests. Table tests cover defaults, every invalid class, maximums, and seeded-secret absence in error/log formatting.

