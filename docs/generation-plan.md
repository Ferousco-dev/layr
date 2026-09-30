# Generation Plan v1 (M2.1)

M2.1 decides **what will be built, what each piece depends on, and what may run at the same time**. It builds no code, runs no jobs, calls no AI provider and never contacts Figma. The input is the stored Design IR; the output is an immutable, validated plan.

```
Design IR (pinned version) -> Selection -> Dependency resolution -> Units and edges -> Validated DAG -> Plan v1
```

Code: `server/internal/genplan` (planner, validator, service), `genplan/pgstore` (storage), `httpapi/generation_plans.go` (routes), `designapi/pin.go` (version pinning).

## Selection

The person chooses; the server decides everything else.

| Mode | Body | Resolves to |
| --- | --- | --- |
| `one` | `{"mode":"one","screen_id":"..."}` | that screen |
| `selected` | `{"mode":"selected","screen_ids":[...]}` | the listed screens, duplicates removed |
| `flow` | `{"mode":"flow","flow_id":"..."}` | the screens of that flow (a Design IR section, from M1's deterministic grouping) |
| `all` | `{"mode":"all"}` | every eligible screen |

Eligible means the screen has a visible, sized root. The resolved screens are always listed in **design order**, never in request order, so equal choices give equal plans. Malformed IDs, an empty list, unknown modes, extra fields for a mode, screens or flows that are not in the pinned design, and screens that cannot be generated are rejected (`INVALID_SELECTION`, `EMPTY_SELECTION`, `SCREEN_NOT_FOUND`, `FLOW_NOT_FOUND`). Names are never identity.

`all` is not one task. It resolves to many screen units that share the components they use.

## Design version pinning

A request carries `design_version` (`dv_...`, from `GET .../design`). `designapi.Service.Pin` proves ownership, loads the project's current design and returns it only if its version is exactly the requested one. A replaced design or one whose temporary files expired is `DESIGN_VERSION_UNAVAILABLE`, never a different design. The plan stores the version and the import ID. Because the plan holds everything it needs, reading a plan does not require the temporary design files.

## Units

Only types the Design IR can justify exist.

| Type | ID | When |
| --- | --- | --- |
| `foundation` | `foundation` | always, once |
| `design_foundation` | `design_foundation` | only if the design has tokens or styles; carries their counts |
| `component` | `component_<id>` | every component the selection uses, once, however many screens use it |
| `screen` | `screen_<id>` | one per selected screen |
| `integration` | `integration` | always, once |
| `verification` | `verification_<screen id>` | one per selected screen that has a reference render |

There is no `layout` unit: the Design IR has no layout model, and inventing shared layouts is left to later milestones. A screen without a reference render gets a `REFERENCE_MISSING` warning and no verification unit.

IDs are deterministic. A source ID with unsafe characters is replaced by a hash, so an ID is never usable as a path.

Units hold references, not copies: `ir` (screen ID, node ID, origin `screen`, `definition` or `instance`), `component_ids`, `asset_ids`, and a logical `reference` (`ref_<screen id>`, checksum and size, never a path or URL).

## Dependency resolution

One walk over each selected screen (iterative, never recursive), tracking the nearest enclosing component:

- an instance or definition of component C directly on a screen makes the screen depend on C;
- one inside component P makes P depend on C (`Dashboard -> Sidebar -> Avatar`);
- the walk then follows components used by components until nothing new appears, reading a component from its definition in the design, or from its first instance when the master lives outside the imported screens;
- assets are attributed to the nearest component, else to the screen; a one-screen plan includes only what that screen and its components use;
- a component used by ten screens is one unit.

Roots attach to the design foundation (or the foundation). Every screen feeds the integration unit, which every verification unit waits for.

## DAG

`NewGraph` validates unit IDs (safe, unique), unit types, edges to missing units, self-dependencies and duplicate edges, then computes levels with Kahn's algorithm. Ties break by canonical unit order, so results never depend on map iteration. A cycle returns `GENERATION_DEPENDENCY_CYCLE` naming the units; the algorithm cannot loop. A reference to a component or asset that is not in the design returns `GENERATION_DEPENDENCY_INVALID`.

`order` is a topological order. `stages` are the levels: units in one stage may run together once earlier stages are done. The graph stays authoritative. For a later executor, `Plan.Graph()` offers `DependsOn`, `Dependents` and `Ready(done)`.

## Example

Selected: Dashboard and Settings, where Dashboard has AppShell (Sidebar (Avatar), Navbar) and Chart, and Settings has AppShell and a form with Button.

```
foundation -> design_foundation -> Avatar, Navbar, Chart, Button   (no dependencies of their own)
Avatar   -> Sidebar
Sidebar, Navbar -> AppShell
AppShell, Chart -> Dashboard
AppShell, Button -> Settings
Dashboard, Settings -> integration -> verification_Dashboard, verification_Settings
```

The exact edges follow what the design contains; nothing is added that the design does not show.

## Determinism, fingerprint, immutability

Equal Design IR, selection and target give the same unit IDs, edges, order, stages and fingerprint. The fingerprint is a SHA-256 over design version, resolved selection, target, units, edges and assets; it leaves out database IDs, the project and timestamps. A stored plan is never updated: a different choice, target or design version makes a new plan. Two identical requests create two records with the same fingerprint (there is no idempotency layer); the fingerprint is there for later duplicate detection and caching.

## Validation

`Validate` runs on every plan before it is stored and again when it is read. It checks schema version, design version, supported target (`nextjs` with `typescript`; anything else is `GENERATION_TARGET_UNSUPPORTED`), a non-empty resolved selection, unique units, the graph, that order and stages match the edges, exactly one foundation and integration, that every selected screen has a unit, that component and asset references resolve inside the plan and, when the design is at hand, inside the design, that verification units have a reference, and that the summary and fingerprint match.

## Storage

Table `generation_plans` (migration 00011): metadata columns (project, import, design version, fingerprint, selection mode, target, status, schema version) plus the plan as JSON. The plan is kept in PostgreSQL rather than in the import's workspace because the workspace is deleted after about 24 hours and a plan must outlive it for later job tracking. It holds identifiers and counts only. Status is `planned` (or `invalid`, reserved; invalid plans are rejected, not stored). Runtime statuses belong to later milestones.

## API

| Route | Notes |
| --- | --- |
| `POST /api/v1/projects/{id}/generation-plans` | body `{design_version, selection, target?}`; 201 with the plan |
| `GET /api/v1/projects/{id}/generation-plans/{planId}` | the stored plan |

The response has `summary` (screens, shared components, assets, units, dependencies, stages, widest stage), `units`, `dependencies`, `order`, `stages`, `assets`, `warnings`. The summary is structural; it makes no cost claim. There is no route to change a plan; unknown body fields such as `units`, `dependencies` or `user_id` are rejected.

## Limits

Planning work is bounded by a step budget (4,000,000 node visits), 20,000 units and 16 MB of plan JSON, on top of the import limits the design already respects. Exceeding a bound returns `GENERATION_PLAN_TOO_LARGE`.

## Security review

- **IDOR.** Every read and write goes through the project owner check (`designapi.Pin`, and the store joins `projects.user_id`). The owner is the session user; a user ID in the body is rejected. A stranger gets `PROJECT_NOT_FOUND` or `GENERATION_PLAN_NOT_FOUND`.
- **Cross-design references.** Screens and flows are looked up only in the pinned design; a screen from another design is `SCREEN_NOT_FOUND`.
- **Resource exhaustion.** Iterative walks, a work budget, unit and size limits, a request cap of 5,000 IDs, and the global body limit.
- **Plan tampering.** The client sends a choice only; unknown fields are rejected; the server computes the dependencies.
- **Leakage.** Plans hold IDs, names, counts and checksums; no paths, URLs, tokens or file keys (tested).
- **Determinism.** No map order reaches the output.
- **Dependency integrity.** Missing references and cycles fail validation; a plan is validated before storing and after reading.

Residual risks: component and screen names are the owner's own text and are stored in the plan; instance swaps in a design can create a real component cycle that stops planning; the design-foundation unit lists global counts because the Design IR has no per-screen token data; nothing verifies that a plan matches its design after the temporary files expire.

## Deliberately deferred

Executing units, queues, providers and keys (M2.2), code generation, builds, layout units, cost estimates, and de-duplicating identical plan requests.
