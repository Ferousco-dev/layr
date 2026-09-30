# M1 Architecture - Figma ingestion

M1 turns a Figma design into validated files that later milestones consume. It never calls AI. After an import, nothing downstream needs Figma again.

```text
Browser
  |  session cookie (opaque, HttpOnly)
  v
Layr API (net/http)                      request id, panic recovery, body limit, CORS, security headers, rate limits
  |
  +-- Auth                               Figma OAuth (PKCE) -> user, encrypted Figma connection, hashed session
  +-- Projects                           owner-scoped CRUD, soft delete
  +-- Importer                           one background job per import, bounded, cancellable
  |     |
  |     +-- Figma client                 typed REST, retries, token refresh under a Redis lock
  |     +-- Workspace                    /imports/<uuid>/{raw,reference,assets,design}, atomic writes
  |     +-- Asset pipeline               discovery, SSRF-safe downloader, SVG sanitizer, manifest v1
  |     +-- Normalizer (pure, offline)   snapshot + manifest -> Design IR v1, validated
  |
  +-- Design API                         public DTOs, preview streaming; reads only the workspace
  v
PostgreSQL (users, sessions, connections, projects, imports)      Redis (state, locks, rate limits)
```

## Import lifecycle

`pending` -> `processing` -> `awaiting_selection` (several frames, no `node-id`) -> `processing` -> `completed` | `failed`.

1. `POST /projects/{id}/import` validates the Figma URL, checks project ownership and inserts one row. A partial unique index allows one running import per project.
2. The job fetches the file outline once, or the target nodes once, and writes the raw response to `raw/` with an atomic rename.
3. With several candidate frames the import waits in `awaiting_selection`. `POST .../select` accepts `node_id`, `node_ids` or `all`; sections open into their frames.
4. One render request covers all selected screens (reference PNGs). Image fills and vector exports are resolved in batches, downloaded with bounded concurrency, sanitized and stored.
5. The normalizer builds the Design IR, the validator checks it, and only then is `design/design-ir.json` written and the import marked `completed`.
6. The Design API reads the stored IR and streams stored previews. It makes no Figma calls (invariant I-13).

## Artifacts per import

| File | Meaning |
| --- | --- |
| `raw/file.json`, `raw/target-node.json` | Exact Figma responses, kept for diagnosis. Never exposed by the API. |
| `assets/manifest.json` + files | Versioned manifest of real files with SHA-256; references are relative paths. |
| `reference/*.png` | Figma-rendered image of each selected screen. |
| `design/design-ir.json` | Design IR v1, compact JSON, deterministic bytes. |

Workspaces are temporary. `TEMP_WORKSPACE_TTL` removes idle ones; running imports are never swept. An expired workspace is a deliberate `expired` design state, not an error.

## Trust boundaries

| Boundary | What crosses it | Control |
| --- | --- | --- |
| Browser to API | Cookies, JSON bodies, IDs in paths | Session lookup by hash, same-origin check, body limit, ID regexes, ownership on every query |
| API to Figma | User's OAuth token | Token stays server-side, never logged; fixed base URL; no redirects |
| Figma to Layr | JSON and temporary asset URLs | Lenient decoding with drift warnings; URLs used once, never stored |
| Layr to asset CDN | Provider-returned URLs only | https-only, address policy on the resolved IP, every redirect revalidated, size limits |
| Layr to disk | Files inside the import workspace | Names from a strict pattern, `O_NOFOLLOW`, symlink refusal, root-guarded deletion |
| Stored data to browser | DTOs and preview bytes | Public DTOs distinct from the IR; previews streamed with `private` caching |

## Retry and timeout ownership

Each layer that may retry is named here so retries never multiply.

| Operation | Retry owner | Budget | Notes |
| --- | --- | --- | --- |
| Figma REST calls | Figma client | 3 attempts, `Retry-After` honoured up to a cap, one forced token refresh on 401 | The importer and asset pipeline do not retry these (tests: `TestRateLimitIsNotRetriedByTheImporter`, `TestFigmaFailuresPropagateUntouched`) |
| Asset CDN download | Asset pipeline | 3 attempts, 250 ms doubling backoff up to 10 s | Permanent failures are not retried |
| Expired asset URL | Asset pipeline | One re-resolve per asset, 20 per import | Re-resolution goes through the Figma client's own budget |
| Whole import | Importer | `IMPORT_TIMEOUT` (default 2 m, max 10 m) | Context cancellation reaches Figma calls, downloads and file writes |
| HTTP server | net/http | read-header 5 s, read 15 s, write 30 s, idle 60 s | Values are fixed upper bounds |

Worst case for one asset is bounded by these budgets and by the import timeout.

## Restart and crash behaviour

- The server marks nothing `completed` until `design-ir.json` is written and validated (atomic rename).
- A process that dies mid-import leaves a `pending` or `processing` row. `Sweep` (at startup and at most hourly when imports start) fails rows untouched for longer than any live import can run (`IMPORT_INTERRUPTED`), so the status API and the project unblock.
- Leftover temporary files are dot-prefixed and skipped by every reader; stale workspaces are swept by age.

## Limits

`MAX_SNAPSHOT_BYTES`, `MAX_WORKSPACE_BYTES`, `MAX_ASSET_SIZE_BYTES`, `MAX_IMPORT_ASSET_BYTES`, `MAX_IMPORT_ASSETS`, `ASSET_DOWNLOAD_CONCURRENCY`, `MAX_IR_BYTES`, `MAX_IR_NODES`, `MAX_SCREENS`, plus fixed depth limit 256. Exceeding one gives a typed failure, never a partial design.

## What M2 receives

Design IR + asset manifest + reference renders, all inside one workspace, described in [design-ir.md](design-ir.md). M2 needs no Figma calls and no knowledge of Figma's raw JSON.
