# Layr server

The Layr backend is a Go HTTP API that signs users in with Figma, keeps their Figma authorization safe, and stores their projects. It is one deployable program (a modular monolith) backed by PostgreSQL and Redis.

## Technology

| Purpose | Choice |
| --- | --- |
| Language | Go 1.24, standard library `net/http` router |
| Database | PostgreSQL 16 or newer, `pgx` connection pool, explicit SQL |
| Migrations | `goose`, SQL files embedded in the binary |
| Cache and short-lived state | Redis 7 or newer via `go-redis` |
| Logging | `log/slog` JSON with automatic secret redaction |
| Encryption | AES-256-GCM from the Go standard library |
| Containers | Multi-stage `Dockerfile`, `docker-compose.yml` for PostgreSQL and Redis |

## Layout

```text
cmd/server         the API process
cmd/migrate        migration command (up, down, status)
internal/
  config           typed environment configuration, validated at startup
  postgres, redis  connection pools and small helpers (locks, counters)
  migrations       embedded SQL files
  httpapi          routes, middleware (request ID, logging, recovery, CORS, rate limit)
  auth             Figma login, sessions, Figma token refresh
  figma            Figma OAuth client and the typed Figma REST API client
  figmaurl         parses pasted Figma URLs into a file key and node ID
  imports          Figma import orchestration (lifecycle, target choice, snapshot)
  workspace        temporary per-import directories and safe atomic writes
  assets           asset discovery, secure downloads, SVG sanitizing, manifest
  designir         Design IR v1 schema, validator, screen selection, JSON codec
  normalize        deterministic Figma snapshot + manifest to Design IR
  designapi        the frontend-facing read layer: design summary, screens, flows, previews
  credential       encryption of secrets at rest
  oauthstate       single-use login state and PKCE verifier
  project          projects: rules and ownership-scoped SQL
  health, observability, workspace, app
```

## How it works

**Startup.** Load and validate configuration, prepare the temporary workspace directory, open PostgreSQL and Redis (failing clearly if either is down), then listen. On `SIGINT` or `SIGTERM` it stops accepting requests, lets in-flight ones finish within `HTTP_SHUTDOWN_TIMEOUT`, then closes Redis and PostgreSQL. A second signal forces the exit.

**Every request** gets a request ID (returned as `X-Request-ID`), is logged as one JSON line with method, route, status and duration (never bodies or secrets), and is protected by panic recovery, security headers, CORS, a same-origin check for non-GET methods and a body size limit.

**Sign in with Figma.**
1. `GET /auth/figma` creates a random state and PKCE verifier, stores them in Redis for a few minutes, and redirects to Figma.
2. Figma redirects to `GET /auth/figma/callback`. The state is consumed exactly once, the code is exchanged for tokens, and the Figma identity is fetched.
3. The user and their Figma connection are saved in one short transaction. Figma tokens are encrypted before they reach the database.
4. A session is created. The browser receives a random opaque cookie; only its SHA-256 hash is stored. The browser never sees Figma tokens.
5. The user is redirected to the frontend.

**Figma tokens** are refreshed on demand by `auth.Service.AccessToken`, which uses a Redis lock so concurrent callers refresh once. If Figma rejects the refresh, the connection is marked as needing reconnection and the user stays signed in to Layr.

**Projects** belong to exactly one user. Every query filters by project ID and owner ID, and a foreign project returns the same 404 as a missing one.

## Configuration

Everything comes from environment variables and is validated at startup. Start from a template:

- `.env.example` for local development
- `.env.production.example` for production

| Variable | Purpose | Default |
| --- | --- | --- |
| `APP_ENV` | `development` or `production` | `development` |
| `HTTP_ADDRESS` | Listen address | required |
| `FRONTEND_URL` | Only origin allowed by CORS and used for login redirects | `http://localhost:3000`; required in production |
| `POSTGRES_URL`, `REDIS_URL` | Connection URLs, never logged | required |
| `POSTGRES_MAX_CONNECTIONS`, `REDIS_POOL_SIZE` | Pool sizes | 10 |
| `FIGMA_CLIENT_ID`, `FIGMA_CLIENT_SECRET` | Figma OAuth app credentials | required |
| `FIGMA_REDIRECT_URI` | Callback URL registered in Figma; https in production | required |
| `CREDENTIAL_ENCRYPTION_KEY` | Base64 of 32 random bytes; encrypts Figma tokens | required |
| `SESSION_COOKIE_NAME`, `SESSION_TTL` | Cookie name and lifetime | `layr_session`, 168h (max 720h) |
| `SESSION_COOKIE_SECURE` | `Secure` cookie flag; production forces true | `true` |
| `OAUTH_STATE_TTL` | Login attempt lifetime | 10m (max 30m) |
| `TEMP_WORKSPACE_ROOT` | Scratch directory, created owner-only | `/tmp/layr/jobs`; required in production |
| `TRUSTED_PROXY_CIDRS` | Proxy networks whose `X-Forwarded-For` is trusted | empty |
| `RATE_LIMIT_LOGIN_PER_MINUTE`, `RATE_LIMIT_API_PER_MINUTE`, `RATE_LIMIT_WRITE_PER_MINUTE`, `RATE_LIMIT_IMPORT_PER_MINUTE` | Per-minute budgets (1 to 100000) | 20, 300, 30, 6 |
| `MAX_IR_BYTES`, `MAX_IR_NODES`, `MAX_SCREENS` | Largest serialized Design IR, nodes per design, screens per import (1 to 500) | 64 MiB, 200000, 500 |
| `MAX_ASSET_SIZE_BYTES`, `MAX_IMPORT_ASSET_BYTES` | Largest single asset and total asset bytes per import | 50 MiB, 256 MiB |
| `ASSET_DOWNLOAD_CONCURRENCY`, `MAX_IMPORT_ASSETS` | Parallel downloads (1 to 16) and files per import | 4, 300 |
| `TEMP_WORKSPACE_TTL` | How long an import workspace lives before cleanup (1s to 720h) | 24h |
| `IMPORT_TIMEOUT` | Time limit for one import (max 10m); imports are paced, so a very large one takes minutes | 5m |
| `FIGMA_REQUESTS_PER_MINUTE` | Layr's own ceiling on requests to Figma per person, retries included (1 to 600) | 8 |
| `MAX_SNAPSHOT_BYTES`, `MAX_WORKSPACE_BYTES` | Largest single snapshot file and total per import | 128 MiB, 512 MiB |
| `HTTP_*_TIMEOUT`, `HTTP_MAX_BODY_BYTES`, `HTTP_MAX_HEADER_BYTES` | Server bounds | see `.env.example` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

Generate a key with `openssl rand -base64 32`. Losing it makes stored Figma tokens unreadable, so back it up in a secret manager.

**Figma app.** In the Figma developer console create an OAuth app, enable the scopes `current_user:read` and `file_content:read`, and register the callback that equals `FIGMA_REDIRECT_URI`.

## Run without Docker

Prerequisites: Go 1.24+, PostgreSQL 16+ and Redis 7+ running locally.

```sh
# one-time database setup
psql -d postgres -c "create role layr login password 'layr_local'"
psql -d postgres -c "create database layr owner layr"

cp .env.example .env          # then fill in the Figma values and a real encryption key
set -a; . ./.env; set +a
go run ./cmd/migrate up
go run ./cmd/server
```

The default `.env` points at `localhost:5432` and `localhost:6379`. Check it is up:

```sh
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
```

## Run with Docker

Prerequisites: Docker with Compose v2.

Dependencies in containers, server on your machine:

```sh
cp .env.example .env
set -a; . ./.env; set +a
docker compose up -d --wait      # PostgreSQL and Redis with a persistent volume
go run ./cmd/migrate up
go run ./cmd/server
docker compose down              # stop; the data volume is kept
```

The server itself in a container:

```sh
docker build -t layr-server .
docker run --rm --env-file .env layr-server layr-migrate up
docker run --rm -p 8080:8080 --env-file .env layr-server
```

Inside a container `localhost` is the container itself, so set `POSTGRES_URL` and `REDIS_URL` in the env file to hosts the container can reach (for example `host.docker.internal`, or a shared Docker network). The image runs as a non-root user and contains only the two binaries.

## Migrations

SQL files live in `internal/migrations/sql` and are embedded in the binary.

```sh
go run ./cmd/migrate status
go run ./cmd/migrate up       # apply all pending
go run ./cmd/migrate down     # revert one step
```

To add one, create the next numbered file (`00004_name.sql`) with `-- +goose Up` and `-- +goose Down` sections, and raise `Latest` in `internal/migrations/migrations.go`. Always test `down` on a throwaway database.

## API

Errors always look like `{"error":{"code","message","request_id"}}` with a stable `code`; the browser routes redirect to the frontend with `?auth_error=<CODE>` instead. See `docs/error-catalogue.md`.

| Route | Auth | Purpose |
| --- | --- | --- |
| `GET /health` | none | Process is alive (does not touch dependencies) |
| `GET /ready` | none | PostgreSQL and Redis reachable; 503 otherwise |
| `GET /auth/figma` | none, rate limited | Start Figma sign-in |
| `GET /auth/figma/callback` | none, rate limited | Finish sign-in, set cookie |
| `POST /auth/logout` | cookie | Revoke session, expire cookie; safe to repeat |
| `GET /api/v1/me` | cookie | Current user |
| `POST /api/v1/projects` | cookie | Create; body `{"name":"Landing Page"}` |
| `GET /api/v1/projects?limit=20&cursor=` | cookie | Your projects, newest update first; `limit` 1 to 100 |
| `GET /api/v1/projects/{id}` | cookie | One project |
| `PATCH /api/v1/projects/{id}` | cookie | Rename; body `{"name":"New name"}` |
| `DELETE /api/v1/projects/{id}` | cookie | Delete (recoverable for 30 days); returns 204 |
| `GET /api/v1/projects/{id}/design` | cookie | The project's design: status, screens, flows, warnings, counts |
| `GET /api/v1/projects/{id}/design/screens?limit=&offset=&q=&flow=` | cookie | Paged screen summaries (search by name, filter by flow) |
| `GET /api/v1/projects/{id}/design/screens/{screenId}` | cookie | One screen's details |
| `GET /api/v1/projects/{id}/design/screens/{screenId}/preview` | cookie | The screen's reference image (PNG bytes) |
| `GET /api/v1/projects/{id}/design/flows/{flowId}` | cookie | One flow with its screens |
| `POST /api/v1/projects/{id}/import` | cookie | Start a Figma import; body `{"figma_url":"..."}`; returns 202 |
| `POST /api/v1/projects/{id}/import/refresh` | cookie | Run the project's import again from the Figma file its last import came from (no body); returns 202 like a new import |
| `GET /api/v1/projects/{id}/import` | cookie | Latest import of the project |
| `GET /api/v1/projects/{id}/imports/{importId}` | cookie | One import |
| `POST /api/v1/projects/{id}/imports/{importId}/select` | cookie | Choose screens; body `{"node_id":"1:2"}`, `{"node_ids":["1:2","1:3"]}` or `{"all":true}` (exactly one); returns 202 |
| `GET /api/v1/projects/{id}/design/tokens` | cookie | Everything the design defines: colors (hex, alpha, use count), text styles (family, weight, size, line height, letter spacing), fonts, spacing, corner radii and shadows |
| `GET /api/v1/projects/{id}/design/assets` | cookie | Every image and vector Figma provided: name, kind, format, size, dimensions, the screens it is used on, and a file URL |
| `GET /api/v1/projects/{id}/design/assets/{assetId}/file` | cookie | The asset file itself, streamed privately (cacheable only per browser, checksum as ETag) |
| `POST /api/v1/projects/{id}/generations` | cookie | Start a code-generation job; body `{"provider":"anthropic","screen_ids":["screen_..."]}` (`openai` or `xai`; `screen_ids` is optional and means all screens when left out); 202 with the plan; needs a saved key for that provider and a finished design |
| `POST /api/v1/projects/{id}/generation-plans` | cookie | Plan a generation without running it; body `{"design_version":"dv_...","selection":{"mode":"one\|selected\|flow\|all","screen_id\|screen_ids\|flow_id":...},"target":{"framework":"nextjs","language":"typescript"}}`; 201 with the plan: units, dependencies, stages, assets, summary |
| `GET /api/v1/projects/{id}/generation-plans/{planId}` | cookie | The stored, immutable plan |
| `GET /api/v1/projects/{id}/generations/{generationId}` | cookie | The job's steps and status; poll it to show progress |
| `POST /api/v1/projects/{id}/restore` | cookie | Undo a delete within 30 days; returns the project |
| `GET /api/v1/profile` | cookie | Profile page data: user, Figma connection state (`active`, `reconnect_required`, `missing`) and, per AI provider, whether a key is saved and its last four characters |
| `PUT /api/v1/profile/ai-keys/{provider}` | cookie | Save or replace a key for `anthropic`, `openai` or `xai`; body `{"api_key":"..."}` |
| `DELETE /api/v1/profile/ai-keys/{provider}` | cookie | Remove a saved key; 204, safe to repeat |
| `DELETE /api/v1/profile` | cookie | Delete the account and everything in it; body `{"confirm":"delete my account"}`; 204 and the session cookie is cleared |

Success bodies are `{"data": ...}`. A project is `{"id","name","created_at","updated_at"}`. Lists also return `"next_cursor"` (null on the last page). Project names are trimmed, 1 to 120 characters, and need not be unique. Deleting a project hides it (soft delete). The owner can restore it for 30 days, and it returns to its previous place in the list (restoring is not an edit, so the update time is unchanged); projects past that window are permanently removed the next time anyone deletes a project, so no scheduler is needed.

**AI keys.** Each person can bring one key per provider. A key is checked only for its shape (provider prefix, length, safe characters); Layr does not call the provider to verify it. It is encrypted with AES-256-GCM using the provider name as bound context, so a stored key cannot be moved to another provider or person, and only its last four characters are ever returned. Nothing returns, logs or echoes the key. Server code that later needs a key reads it through `account.Service.OpenKey`, which is not exposed over HTTP.

**Deleting an account** removes the user; projects, imports, sessions, the Figma connection and AI keys go with it through foreign keys, and the temporary files of the person's imports are cleared. It needs the exact phrase in the request body. Figma tokens are dropped from Layr, but Layr does not revoke the app's access on Figma's side; the person can do that in their Figma settings.

**Generation plans (M2.1).** A plan says what to build, what each piece depends on, and what can run together, from the stored Design IR of an exact design version. It calls neither Figma nor an AI provider and runs nothing. See [docs/generation-plan.md](../docs/generation-plan.md).

**Generation jobs.** `POST .../generations` checks the provider, that the person has a saved key for it, and that the design is finished; records the plan as steps; and runs the job in the background. Each step is saved the moment it starts and ends, so every poll shows what really happened: `read_design` (loads the design summary), `extract_tokens` (reads colors and fonts), `prepare_model` (decrypts the saved key on the server; nothing is sent to the provider), and `write_code`. The last step is where the AI call belongs and is not built yet, so it stops the job with `GENERATION_NOT_AVAILABLE`; the milestone that adds it replaces that stage and the rest of the flow does not change. One job may run per project (`GENERATION_RUNNING`), jobs end on shutdown or after five minutes, and abandoned jobs are failed at startup (`GENERATION_INTERRUPTED`). The saved key never appears in a response, log or step detail.

**Scopes.** Sign-in asks Figma for two read-only scopes: `current_user:read` (name, email, picture) and `file_content:read` (files, nodes, renders, image fills and SVG exports). Figma only grants scopes the OAuth app has enabled. Nothing else is requested.

**Assets.** Assets come from the stored design and manifest: each file is served only from the owner's import workspace, only if the design lists it, only as a viewable image type, and never through a symlink.

## Figma API client

`figma.API` (`internal/figma`) is the only code that talks to Figma's REST API. Callers pass a Layr user ID; the client gets that user's token from `auth.FigmaTokens`, so there is a single token lifecycle and callers never see credentials.

| Method | Figma endpoint | Returns |
| --- | --- | --- |
| `GetFile(ctx, userID, fileKey, FileOptions)` | `GET /v1/files/:key` | metadata and node tree, components, component sets, styles |
| `GetFileNodes(ctx, userID, fileKey, nodeIDs, opts)` | `GET /v1/files/:key/nodes` | requested subtrees plus a `Missing` list |
| `GetImageFills(ctx, userID, fileKey)` | `GET /v1/files/:key/images` | `imageRef` to temporary URL |
| `RenderNodes(ctx, userID, fileKey, nodeIDs, RenderOptions)` | `GET /v1/images/:key` | node ID to temporary PNG, JPG or SVG URL, plus a `Failed` list |

All four need the `file_content:read` scope Layr already requests. The client returns Figma's facts as typed Go structs (auto layout, paints including `imageRef`, effects, text styles, constraints, component data) and does no interpretation. Node types stay plain strings; `Node.Known()` is false for types Layr does not model, and unknown JSON fields are ignored, so new Figma features never break decoding. Colors stay as Figma's 0 to 1 floats. Node IDs must be canonical (`120:450`); URL parsing is not done here. Image and render URLs are temporary Figma URLs: download promptly and never store or ship them. This client does not download bytes.

**Robust to what Layr was not written for.** Figma's JSON is decoded leniently, so a new or changed feature degrades one field instead of failing the file:
- an unknown property on a node is kept, exactly as sent, in `Node.Extra`;
- a known property that arrives in a different shape (say `cornerRadius` becomes an object) is zeroed, kept in `Node.Extra`, and named in `Node.Drift`, while the rest of the node survives;
- a child that is not a proper node is skipped and named in `Drift`; one unreadable node entry becomes `FileNodes.Malformed` and does not spoil the others; odd values in image and render maps are treated as failures for that item only;
- unknown node types, paint types and effect types pass through as plain strings (`Node.Known()` tells you);
- only a response with no usable `document` or `nodes` fails, as `FIGMA_BAD_RESPONSE`;
- hostile nesting deeper than 2000 levels is rejected before it can exhaust the stack.

`figma.Analyze(nodes...)` returns a `DriftReport` (unknown node types, extra fields, mismatched fields), and every call that meets drift logs one `figma.schema_drift` warning, so gaps in the model get noticed. Tests corrupt every single value of every fixture in six ways and require the call to survive, and a fuzz test (`go test -fuzz FuzzDecodeNeverPanics ./internal/figma`) checks the decoder never panics. Decoding runs at about 35 MB/s, so a 50 MB file takes around a second and a half.

**Nothing is lost.** The typed structs model what Layr needs (including grids, shared-style references, hyperlinks, instance overrides and raw variable bindings) and skip the rest. To keep everything Figma sent, set `Snapshot` (an `io.Writer`, for example a temporary file) in `FileOptions` or `NodesOptions`: the untouched JSON of a successful response is copied to it while it is decoded, with no extra memory. Later stages can read fields the structs do not model from that snapshot. Discard the snapshot if the call returns an error.

**Requests.** One shared `http.Client` (60 s timeout, redirects refused, so a request can never be steered to another host). The base URL is fixed to `https://api.figma.com` in code; only tests override it. Query values are built with `url.Values`. File keys and node IDs are validated before any network call (at most 100 node IDs per call; duplicates are removed). Response bodies are decoded as a stream and capped at 128 MiB, which fits large files while stopping runaway responses. Context cancellation aborts the request and any wait.

**Errors.** Failures are `*figma.Error` with a stable `Kind`, HTTP status, `Retryable` and, for rate limits, `RetryAfter`, `PlanTier` and `RateLimitType`. They never contain the token, URL query or Figma's response body. Use `figma.KindOf(err)`.

| Kind | Cause |
| --- | --- |
| `FIGMA_AUTH_REQUIRED` | The user must reconnect Figma (stored authorization revoked) |
| `FIGMA_TOKEN_EXPIRED` | Figma answered 401 even after one token replacement |
| `FIGMA_PERMISSION_DENIED` | 403: valid user, no access to that file |
| `FIGMA_FILE_NOT_FOUND` | 404 |
| `FIGMA_RATE_LIMITED` | 429 |
| `FIGMA_BAD_REQUEST` | 400 |
| `FIGMA_BAD_RESPONSE` | Malformed, oversized or unexpected response, redirect |
| `FIGMA_UNAVAILABLE` | 5xx, connection failure, or credentials temporarily unavailable |
| `FIGMA_REQUEST_TIMEOUT` / `FIGMA_REQUEST_CANCELLED` | Timeout or cancelled context |

**401 handling.** On a 401 the client asks the token service to replace that exact token (`RefreshRejected`) and retries once. If another caller already replaced it, the newer token is used and no second refresh happens. A second 401 returns `FIGMA_TOKEN_EXPIRED`; a revoked refresh returns `FIGMA_AUTH_REQUIRED`. There is no refresh loop.

**Retries and rate limits.** At most 3 requests per call. Retried: 429, 502, 503, 504 and connection failures, with exponential backoff (500 ms, 1 s) and jitter. Not retried: 400, 403, 404, 500, timeouts. `Retry-After` (seconds or HTTP date) is honoured; if it is longer than 10 s the client does not sleep and returns the rate-limit error with `RetryAfter` set, so a job scheduler can decide. Figma limits files and images to tier 1 (roughly 10 to 20 requests a minute per user on full seats), so batch node IDs into one call and cache results instead of re-fetching.

**Logging.** One line per call: `figma_operation`, `user_id`, `file_key`, `node_count`, `status`, `attempts`, `rate_limited`, `duration_ms`, and `code` on failure. Never tokens or payloads.

**Tests.** Unit tests use `httptest` servers and sanitized fixtures in `internal/figma/testdata`; no Figma account is needed. `internal/auth/figmaapi_test.go` exercises the client with the real token service.

## Figma import

An import connects a project to a Figma design. `POST /api/v1/projects/{id}/import` checks that the caller owns the project, records the import and returns **202** at once; the work continues in the background and the client polls `GET .../import` (or `.../imports/{importId}`).

**URLs.** Only `https://figma.com` and `https://www.figma.com` are accepted, with the paths `/design/:key`, `/file/:key` or `/proto/:key` (a branch URL `/design/:key/branch/:branchKey` uses the branch key). Board (FigJam) and Make links are rejected. `node-id=120-450` becomes the API form `120:450`; `120:450` and instance paths such as `I5-1;2-3` also work. The parser only extracts a file key and node ID; every Figma request goes through the controlled client, so a pasted URL can never make Layr fetch an arbitrary address.

**Lifecycle.**

```text
pending -> processing -> completed
                     \-> awaiting_selection -> processing -> completed
any unfinished state -> failed
```

Clients cannot set a status; each change is a guarded update in the database. A project has one running import at a time (a second start returns 409 `IMPORT_CONFLICT`). Older imports stay as history. Starting a new import retires one that was waiting for a frame choice (`IMPORT_SUPERSEDED`) and one that has been stuck for 10 minutes.

**Choosing the target.** With a `node-id` the importer fetches that node and checks it is a FRAME, COMPONENT or SECTION (`FIGMA_UNSUPPORTED_NODE` otherwise). A link to a page or a group is not an error: the import lists the frames on that page and waits for a choice, like a link without a `node-id`. Nodes and renders are requested from Figma 100 at a time, so one import can hold up to 500 screens. Without one it reads the file's pages and lists the visible top-level frames, components and sections. Exactly one candidate is imported automatically; several put the import in `awaiting_selection` with `frames: [{id, name, type}]`, and `POST .../select` continues it with one screen, several, or all of them. Every chosen node must be one of the stored candidates, and the number of screens is limited by `MAX_SCREENS`. Choosing a Figma *section* imports each frame inside it as a screen, grouped as a section. No candidates at all fails with `FIGMA_NO_FRAMES`. Nothing is ever guessed.

**Result.** A completed import reports the file name, Figma version, node ID and name, `reference_render` (`format`, `scale`), `screens` and `design` (`screens`, `nodes`, `warnings`). A failed one reports `error.code` and a fixed message; provider text is never stored or returned.

**Temporary workspace.** Each import gets `TEMP_WORKSPACE_ROOT/imports/<import-id>/` (owner-only permissions) with `raw/` (`file.json` when the file was read, `target-node.json`) and `reference/` (`render.json`: node, format, scale and Figma's temporary render URL). Directory names come only from the import's UUID, never from Figma or user text. Files are written atomically (temporary file, sync, rename) with the size limits above. The render URL expires, so later stages must download the image bytes and never store or ship the URL. Raw Figma JSON is never stored in PostgreSQL.

**Cleanup.** A failed or cancelled import deletes its workspace immediately. Completed and waiting workspaces stay for `TEMP_WORKSPACE_TTL` so later stages can use them, then a sweep (at startup and at most hourly while imports start) removes them unless their import is still running. Deletion refuses any path outside `imports/` and unlinks symlinks instead of following them. Deleting a project removes its import rows by cascade but cannot reach the disk, so its workspaces disappear at the next sweep; a future purge should also call the workspace cleanup.

**Limits and behaviour.** At most 4 imports run at once (the 5th gets 503 `IMPORT_BUSY`), each is limited by `IMPORT_TIMEOUT`, and starts are limited per user by `RATE_LIMIT_IMPORT_PER_MINUTE`. Shutdown cancels running imports and waits for them before closing PostgreSQL. Every request to Figma, including retries, is paced by a sliding one-minute window per person (`FIGMA_REQUESTS_PER_MINUTE`, default 8). Imports wait for a free slot; a request that would wait past the import's own time limit fails at once as `FIGMA_RATE_LIMITED` with the wait. On a Figma rate limit the import fails with `FIGMA_RATE_LIMITED` and logs the requested wait; it does not retry in a loop, so the user starts it again later. Logs carry IDs, timings and codes, never tokens or design content.

**Assets.** Before an import completes, the asset pipeline stores the design's real images and vectors, so nothing is redrawn or regenerated. Layout under `imports/<id>/`:

```text
reference/<node>-<hash>.png  Figma's render of each screen (for preview and comparison, never a website asset)
design/design-ir.json       the validated Design IR (see docs/design-ir.md)
assets/<name>-<hash>.<ext>  images and SVGs
assets/manifest.json        the Asset Manifest
```

*Discovery* walks the target node's snapshot once, in document order, skipping hidden layers. Image fills (`IMAGE` paints, in fills or strokes, at any depth, including inside instances) are collected by `imageRef`, so a picture used ten times is downloaded once. Vectors are chosen by structure, never by AI: a node is exported as one SVG when it is vector artwork with no boxes of its own (only vector shapes and plain primitives, no text, images, fills, strokes or shadows on the container) and it contains at least one real vector. The outermost such node is the export root, so a logo or illustration is one file rather than one per shape; a lone icon inside a styled button is exported by itself while the button stays CSS. A node the designer marked for SVG export is honoured. The import root itself is never exported. Groups of plain rectangles, lines and ellipses are left for CSS. Video and pattern fills, animated images and missing image references produce warnings, not failures.

*Resolution and download.* Image URLs come from one `GetImageFills` call and SVGs from batched `RenderNodes(format=svg)` calls (50 per request), so a rerun makes no Figma calls for assets it already holds. Downloads use at most `ASSET_DOWNLOAD_CONCURRENCY` workers and stream to a temporary file; nothing is read into memory whole except an SVG being sanitized (max 20 MiB). A rejected (403, 404, 410) URL is re-resolved once, transient statuses (429, 500, 502, 503, 504) and connection failures retry up to 3 attempts with backoff, and everything else fails at once. The first fatal error cancels the other downloads and removes their partial files.

*Validation.* The declared `Content-Type` must be an image (or missing, generic, XML or plain text), but the bytes decide the format: PNG, JPEG, GIF, WebP or SVG by signature, with the header decoded for width and height. HTML, scripts or anything else is rejected as `ASSET_INVALID_CONTENT`. Extensions come from the detected format, never from the URL.

*SVG.* SVGs are untrusted documents. The sanitizer keeps the drawing and removes scripts, `foreignObject`, iframes, animation elements, event attributes (`on*`), any `href`/`src` that is not a `#fragment` or a `data:image/png|jpeg|gif|webp` URI, styles that load external resources or run code, and doctypes. It changes nothing in a clean Figma export. The result is safe to serve as an image file or reference from `<img>`. If it is ever inlined into a page, use a Content-Security-Policy as well, because sanitizing is a filter, not a sandbox. Sanitized files are flagged `sanitized` with a warning.

*File names* are `slug-hash.ext`: the layer name reduced to lowercase ASCII words (at most 40 characters, `vector` or `image` if nothing remains) plus the first 6 characters of the content's SHA-256 (12 on a clash). Names are assigned after all downloads finish, in design order, so the same design gives the same names regardless of download timing. Identical bytes become one file with several sources.

*Asset Manifest, schema version 1* (`assets/manifest.json`, written last and atomically, so it only lists finished files):

```json
{
  "schema_version": 1, "import_id": "...", "source": {"file_key": "...", "node_id": "...", "node_ids": ["..."]},
  "assets": [{
    "id": "asset_<16 hex of sha256>", "kind": "image|svg", "format": "png|jpeg|gif|webp|svg",
    "media_type": "image/png", "path": "assets/hero-image-c39a17.png", "size_bytes": 582134,
    "sha256": "...", "width": 1200, "height": 800, "sanitized": false,
    "sources": [{"node_id": "22:91", "image_ref": "abc123", "role": "fill", "scale_mode": "FILL", "rotation": 0, "has_image_transform": false}]
  }],
  "references": [{"node_id": "...", "path": "reference/12-34-aabbcc.png", "media_type": "image/png", "size_bytes": 1, "sha256": "...", "width": 1, "height": 1}],
  "warnings": [{"code": "video_paint_unsupported", "node_id": "2:10", "message": "..."}]
}
```

Assets and sources are sorted, paths are relative to the workspace (never host paths), and no provider URL or token is ever written. `width` and `height` appear only when known: pixel size for raster images, declared size for SVGs. Identity (the asset) is separate from usage (each source's node, crop mode and rotation), so a later Design IR can keep how each node uses an image without duplicate files. Lookups: `AssetByID`, `AssetForImageRef`, `AssetsForNode`. Paths are framework independent; code generation maps them to its own folder.

*Reuse.* Rerunning on the same workspace verifies each listed file's size and SHA-256 and only fetches what is missing or corrupt; unlisted files are removed.

*Failures.* Resolution gaps (Figma has no image or SVG for something) and unsupported media are warnings. A URL Figma did provide that cannot be downloaded, is blocked by policy, is too large, is not a valid image or exceeds the import budget fails the import with `ASSET_DOWNLOAD_FAILED`, `ASSET_URL_BLOCKED`, `ASSET_TOO_LARGE`, `ASSET_INVALID_CONTENT` or `ASSET_BUDGET_EXCEEDED`. The workspace is then deleted. Beyond `MAX_IMPORT_ASSETS`, images are kept first and the rest are skipped with a warning.

*Download security.* The downloader is internal: only URLs that Figma's API returned during an authenticated import reach it, and no endpoint accepts a URL. It still refuses anything but `https` on port 443, URLs with credentials, and `localhost`, `.local` and `.internal` names. Every socket connection is checked after DNS resolution, so a name that resolves to loopback, private (RFC 1918), CGNAT, link-local (including the 169.254.169.254 metadata address), unique-local, multicast or unspecified addresses is refused even after a redirect or DNS rebinding. Redirects are limited to 3 and each target is validated again. Environment proxies are ignored. Figma serves images from several signed S3 and CDN hosts that can change, so hosts are not allow-listed; the https and address rules apply to all of them. Error messages and logs never contain the signed URL.

*Fonts* are not downloaded: naming a font is not permission to redistribute it. Video and pattern fills are detected and reported but not fetched.

**Design IR.** After the assets are stored, the importer converts the raw snapshot and the manifest into Design IR v1 and writes it, validated, to `design/design-ir.json`. It contains every imported screen (page-sized frame) with its exact geometry, Auto Layout, sizing, constraints, typography, colors, gradients, strokes, radii, effects, text and asset references, plus shared components, tokens and warnings. Later stages read this file and never raw Figma JSON, and can select any subset of screens from it without re-importing. The conversion uses no AI, network or database. Details, the schema, the multi-screen model and an example are in [docs/design-ir.md](../docs/design-ir.md).

## Design API

The frontend reads a project's design through stable DTOs (`internal/designapi`), never Figma or Design IR structures. Everything is derived from data already stored by the import; nothing calls Figma or an AI.

**Which design.** `GET .../design` returns the project's *current* design: its newest completed import. A newer import that is running or failed is reported beside it as `pending_import`, so a failed refresh never hides a working design. With no completed import the response describes the import in flight instead.

| `status` | Meaning |
| --- | --- |
| `ready` | A design is available (`screens`, `flows`, `warnings`, `counts`, `design_system` are filled) |
| `importing` | An import is running (assets and normalization are part of it) |
| `awaiting_selection` | The import is waiting for the user to choose screens |
| `failed` | The import failed; `error` has a stable code and a safe message |
| `expired` | The design metadata exists but its temporary workspace has been cleaned up; import again |

`design_version` identifies this design (a hash of the import and its stored content) and changes with every re-import; `source.source_version` is Figma's own version, kept separate. Screen data has the form:

```json
{"id": "screen_0123456789abcdef", "name": "Dashboard", "source_node_id": "120:450", "page": "App", "flow_id": "section_...",
 "index": 4, "width": 1440, "height": 1024, "warning_count": 0,
 "preview": {"available": true, "width": 1440, "height": 1024, "media_type": "image/png", "url": "/api/v1/projects/<id>/design/screens/<screenId>/preview?v=<design_version>"}}
```

Screens keep design order (which is often the product flow). Layr screen IDs are the product identity; `source_node_id` is provider metadata. `flows` come only from Figma sections that the designer made; screens outside a section have no `flow_id`, and nothing is grouped by guesswork. `selection.modes` lists what the UI may offer (`one`, `selected`, `all`, and `flow` when flows exist); the selection itself lives in the frontend until generation exists. The summary lists at most 300 screens (`screens_truncated` says more exist); the screens endpoint pages through all of them (`limit` up to 500).

Warnings use fixed public codes and messages, so wording inside the IR can change without leaking; unknown codes fold into `DESIGN_NOT_FULLY_SUPPORTED`. `design_system` and `counts` are summaries only; the node trees and components never leave the server.

**Previews** stream the stored PNG through the same ownership chain (session, project, current design, screen). The path comes from the trusted design, must be exactly `reference/<name>.png`, and is opened without following links; the bytes must start with the PNG signature. Responses carry `ETag` (the stored SHA-256), `Vary: Cookie` and `Cache-Control: private, max-age=86400, immutable` when the URL's `v` matches the current design version, otherwise `private, no-cache`; a matching `If-None-Match` gets `304`. Previews are private and never publicly cacheable. Temporary Figma URLs, workspace paths and asset paths are never returned, and no generic file endpoint exists.

**Errors:** `PROJECT_NOT_FOUND` (also for other people's projects), `DESIGN_NOT_FOUND`, `DESIGN_NOT_READY` (409), `DESIGN_DATA_EXPIRED` (410), `SCREEN_NOT_FOUND`, `FLOW_NOT_FOUND`, `PREVIEW_NOT_AVAILABLE`, `DESIGN_IR_INVALID` (500, the stored file is unreadable), `INVALID_REQUEST`.

**Performance.** The stored design is parsed once and kept in memory (the four most recent imports), rebuilt when the file changes, and shared read-only between requests. No Redis or extra database is involved.

**Manual test with real Figma** (not yet run): sign in, create a project, `POST` a real design link, poll the import until `completed` or `awaiting_selection`, then check `raw/target-node.json` and `reference/render.json` inside the workspace directory.

## Security notes

- **Sessions:** 32 random bytes in an `HttpOnly`, `SameSite=Lax` cookie (`Secure` in production). Only a SHA-256 hash is stored.
- **Figma tokens:** AES-256-GCM with a random nonce per value, versioned format, purpose-bound so values cannot be swapped between columns.
- **Login state:** random, single-use, expires in minutes, stored hashed in Redis, plus PKCE.
- **Redirects:** users only ever return to `FRONTEND_URL`; no redirect parameter is accepted.
- **CORS:** one exact origin (`FRONTEND_URL`), credentials allowed, never `*`. Non-GET requests from other origins get 403.
- **Rate limiting** (per minute, in Redis, tunable with the `RATE_LIMIT_*` variables): by default 20 per client address on the two login routes; 300 per client address on every `/api/v1` route, checked before authentication; 30 state-changing calls (POST, PATCH, DELETE) per signed-in user. Over the limit returns 429 `RATE_LIMITED` with `Retry-After`. `X-Forwarded-For` is ignored unless the caller is inside `TRUSTED_PROXY_CIDRS`.
- **Ownership:** every project statement filters on the owner; the owner is never read from the request.
- **Logs:** no request bodies, cookies, tokens, codes or state values; keys containing `token`, `secret`, `cookie` or `password` are redacted.

## Production

Production frontend: `https://layr.appmd.dev`. Use `.env.production.example`.

- Serve the API over https from a host under `appmd.dev` (for example `api.layr.appmd.dev`). The session cookie is `SameSite=Lax`, so the frontend and API must be on the same site; the frontend calls the API with credentials included.
- Set `FIGMA_REDIRECT_URI` to that host's `/auth/figma/callback` and register the same URL in Figma.
- If a reverse proxy sits in front, set `TRUSTED_PROXY_CIDRS` to its network.
- Run `layr-migrate up` before starting a new version, and keep a database backup before any `down`.

## Testing

```sh
go test ./...            # unit tests, no services needed
go test -race ./...
```

Live tests run against real PostgreSQL and Redis when you point them at throwaway databases with migrations applied:

```sh
export TEST_DATABASE_URL='postgres://layr:layr_local@localhost:5432/layr_test?sslmode=disable'
export TEST_REDIS_URL='redis://localhost:6379/0'
go test -race -count=1 ./...
```

The live tests share one test database and empty its tables, so use `-p 1` if you ever see cross-package interference.

Without those two variables the live tests are skipped, not failed, so check `go test -v ./... | grep -c SKIP` prints 0 when you expect a full run. Only `TEST_DATABASE_URL` and `TEST_REDIS_URL` are read; `POSTGRES_URL` and `REDIS_URL` belong to the server.

Benchmarks and fuzzing (all offline):

```sh
go test -run '^$' -bench . -benchmem ./internal/normalize ./internal/figma
go test ./internal/assets -run '^$' -fuzz '^FuzzSanitizeSVGNeverPanicsAndLeavesNoActiveContent$' -fuzztime 30s
```

Fuzz targets: `FuzzParseNeverPanics` (Figma URLs), `FuzzDecodeNeverPanics` (Figma JSON), `FuzzUnmarshalNeverPanicsAndRoundTrips` (Design IR), `FuzzFileNameIsAlwaysSafe`, `FuzzSanitizeSVGNeverPanicsAndLeavesNoActiveContent`, `FuzzSniffNeverPanics` (assets) and `FuzzHostileNamesNeverLeaveTheWorkspace` (workspace).

More on how M1 is built, secured and how it fails: [architecture](../docs/m1-architecture.md), [security and invariants](../docs/m1-security.md), [failure matrix](../docs/m1-failure-matrix.md), [completion report](../docs/m1-completion-report.md).

`make verify` runs formatting, vet, tests, race tests, build and a Compose config check. Other targets: `make run`, `make migrate-up`, `make deps-up`, `make deps-down`.

## Troubleshooting

- **Server exits with `CONFIG_MISSING` or `CONFIG_INVALID`:** the log names the variable but never its value.
- **`POSTGRES_UNAVAILABLE` or `REDIS_UNAVAILABLE` at startup:** the service is not reachable at the configured URL.
- **`/ready` returns 503:** one dependency is down; `/health` stays 200.
- **Login redirects back with `auth_error=OAUTH_STATE_INVALID`:** the login took longer than `OAUTH_STATE_TTL`, or the link was reused. Start again from `/auth/figma`.
