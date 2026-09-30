# M1 Failure matrix

For each failure: what the client sees, what Layr does inside, whether it retries, what is cleaned up, and the final import state. Codes are in [error-catalogue.md](error-catalogue.md). "Failed" imports keep no partial `design-ir.json`; their workspace is removed on failure or by the TTL sweep.

| Failure | Public error | Internal behaviour | Retry | Cleanup | Final state |
| --- | --- | --- | --- | --- | --- |
| Figma 401 | `FIGMA_AUTH_REQUIRED` | One forced token refresh under the Redis lock, then one replay; a second 401 stops | Once (the replay) | Workspace removed | `failed` |
| Figma 403 | `FIGMA_PERMISSION_DENIED` | Typed error, no detail from Figma shown | No | Workspace removed | `failed` |
| Figma 404 | `FIGMA_FILE_NOT_FOUND` or `FIGMA_NODE_NOT_FOUND` | Typed error | No | Workspace removed | `failed` |
| Figma 429 | `FIGMA_RATE_LIMITED` | Waits `Retry-After` when it is within the cap, otherwise returns at once; the importer never retries it again | Up to the Figma client's 3 attempts | Workspace removed | `failed` |
| Figma 502/503/504 | `FIGMA_UNAVAILABLE` | Backoff and retry inside the Figma client | Up to 3 attempts | Workspace removed | `failed` |
| Figma 500 | `FIGMA_UNAVAILABLE` | Not retried | No | Workspace removed | `failed` |
| Figma timeout | `FIGMA_REQUEST_TIMEOUT` | Per-request client timeout; typed | No | Workspace removed | `failed` |
| Malformed Figma data | `FIGMA_IMPORT_FAILED` or a design warning | Decoder is lenient: unknown fields/types are kept and flagged; only unreadable bodies fail | No | Workspace removed on failure | `failed` or `completed` with warnings |
| No importable frame | `FIGMA_NO_FRAMES`, `FIGMA_UNSUPPORTED_NODE` | Rejected before rendering | No | Workspace removed | `failed` |
| Asset CDN unavailable | `ASSET_DOWNLOAD_FAILED` | 3 attempts with backoff, expired URLs re-resolved once per asset | Yes, bounded | Partial files aborted | `failed` |
| Asset URL unsafe (private address, bad scheme, redirect to internal) | `ASSET_URL_BLOCKED` | Refused before or during connect, redirects revalidated | No | Nothing written | `failed` |
| Asset too large | `ASSET_TOO_LARGE`, `ASSET_BUDGET_EXCEEDED` | Aborted while streaming; siblings stopped | No | Partial files removed | `failed` |
| Invalid or hostile SVG/image | `ASSET_INVALID_CONTENT` | Sniffed and sanitized before the final rename | No | Nothing kept | `failed` |
| Workspace unavailable | `WORKSPACE_FAILED` | Directory or write error; root guard unchanged | No | Best-effort removal | `failed` |
| Snapshot or workspace size limit | `SNAPSHOT_TOO_LARGE` | Write stops at the limit; temp file removed | No | Temp file removed | `failed` |
| Design too large or too deep | `DESIGN_IR_LIMIT_EXCEEDED`, `TOO_MANY_SCREENS` | Checked before any IR file exists | No | Workspace removed | `failed` |
| Design IR validation failure | `DESIGN_IR_VALIDATION_FAILED` | Validator runs before the IR is written | No | No IR written | `failed` |
| Preview file missing | `PREVIEW_NOT_AVAILABLE` (404) | Path is resolved from the stored IR only | No | None | unchanged |
| Workspace expired | `DESIGN_DATA_EXPIRED` (410) | Deliberate state; import again | No | Already swept | `expired` (design view) |
| PostgreSQL unavailable | `DEPENDENCY_UNAVAILABLE` (503, `Retry-After`) on project, import, design and auth routes; `/ready` 503, `/health` 200 | Classified by `postgres.IsUnavailable`; a running import that cannot record its result is later retired by the sweep | Client retries | Nothing lost | `failed` (`IMPORT_INTERRUPTED`) if a job was mid-flight |
| Redis unavailable | `DEPENDENCY_UNAVAILABLE` (503) from the rate limiter, sign-in and token refresh | Fails closed instead of skipping limits; `/ready` 503, `/health` 200 | Client retries | None | No new import starts |
| Import exceeds `IMPORT_TIMEOUT` | `IMPORT_TIMEOUT` | Context deadline reaches every stage | No | Workspace removed | `failed` |
| Server shutting down | `IMPORT_CANCELLED` | Base context cancelled; workers finish or abort within the shutdown window | No | Partial files aborted | `failed` |
| Client disconnects | none | The background job uses its own context and continues | n/a | None | continues |
| Process crash or restart mid-import | `IMPORT_INTERRUPTED` | Row stays `pending`/`processing` until the sweep retires it | Start a new import | Workspace swept by TTL | `failed` |
| Two imports for one project at once | `IMPORT_CONFLICT` (409) for the second | Partial unique index decides the race | Client retries after the first ends | None | first continues |
| Repeated identical POST | `IMPORT_CONFLICT` while one runs; otherwise a new import | No shared workspace; history is kept | n/a | None | independent imports |
| Too many concurrent imports | `IMPORT_BUSY` (503, `Retry-After`) | Bounded worker slots; no unbounded queue | Client retries | None | not started |
| Panic inside an import job | `INTERNAL_ERROR` | Recovered per job; other imports unaffected | No | Workspace removed | `failed` |
