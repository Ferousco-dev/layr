# Error Catalogue: LAYR Server M1.1

Every public failure has a stable code, a safe message, a recovery path, and an explicit preservation statement. HTTP errors use the envelope in `docs/ui-spec.md`; process and migration failures use the same `code` and `message` fields in JSON Lines. Diagnostic causes may be retained internally only after redaction; they are never copied into HTTP or CLI output.

| ID / stable code | Surface and condition | Status / exit | What the operator sees | Next action | Work preserved | Logged event |
| --- | --- | --- | --- | --- | --- | --- |
| ERR-001 `METHOD_NOT_ALLOWED` | non-GET method on `/health` or `/ready` | HTTP 405; `Allow: GET` | `This endpoint accepts GET requests only.` | Retry with `GET`. | No state changed. | `request.completed` |
| ERR-002 `NOT_FOUND` | unknown route | HTTP 404 | `The requested endpoint does not exist.` | Check the documented path and retry. | No state changed. | `request.completed` |
| ERR-003 `REQUEST_TOO_LARGE` | body exceeds configured limit on a body-bearing route | HTTP 413 | `The request body exceeds the allowed size.` | Send a smaller request within the documented limit. | Server remains available; no partial request state is retained. | `request.completed` |
| ERR-004 `INTERNAL_ERROR` | response cannot be completed safely | HTTP 500 | `The server could not complete the request. Use the request ID to find the matching log.` | Retry once; inspect the matching redacted log if it repeats. | Existing durable state is unchanged. | `request.completed` |
| ERR-005 `DEPENDENCY_NOT_READY` | either readiness check is down or timed out | HTTP 503 | `/ready` body uses `status=not_ready` and named check states; no diagnostic string | Restore the named dependency and retry `/ready`. | Existing database state and configuration are unchanged. | `readiness.failed` |
| ERR-006 `CONFIG_MISSING` | required environment variable absent | non-zero process exit | `Required configuration is missing.` plus safe `field` name | Set the named variable and restart. | HTTP never starts; dependency/data state is unchanged. | `startup.failed` |
| ERR-007 `CONFIG_INVALID` | value malformed, non-positive, or outside a safe bound | non-zero process exit | `Configuration is invalid.` plus safe `field` name | Correct the named variable using `.env.example`; restart. | HTTP never starts; invalid value is not printed. | `startup.failed` |
| ERR-008 `POSTGRES_UNAVAILABLE` | PostgreSQL startup probe fails or times out | non-zero process exit | `PostgreSQL is unavailable during startup.` | Confirm the service and configured connection source, then restart. | HTTP never starts; Redis is closed if already opened. | `startup.failed` |
| ERR-009 `REDIS_UNAVAILABLE` | Redis startup probe fails or times out | non-zero process exit | `Redis is unavailable during startup.` | Confirm the service and configured connection source, then restart. | HTTP never starts; PostgreSQL is closed if already opened. | `startup.failed` |
| ERR-010 `LISTEN_FAILED` | configured address cannot be bound | non-zero process exit | `The HTTP listener could not start.` | Check the configured address/port and competing process; restart. | Dependencies are closed; no requests were accepted. | `startup.failed` |
| ERR-011 `MIGRATION_USAGE` | missing or unknown migration verb | non-zero command exit | `Usage: migrate <up|down|status>` | Choose exactly one documented verb. | Database is not opened for mutation. | `migration.failed` |
| ERR-012 `MIGRATION_CONNECT_FAILED` | migration command cannot reach PostgreSQL | non-zero command exit | `The migration database is unavailable.` | Restore PostgreSQL or correct configuration, then retry. | No migration is applied by this attempt. | `migration.failed` |
| ERR-013 `MIGRATION_AT_BASE` | `down` requested at version zero | non-zero command exit | `No applied migration is available to reverse.` | Run `status`; use `up` if migrations are pending. | Database remains at version zero. | `migration.failed` |
| ERR-014 `MIGRATION_IRREVERSIBLE` | latest migration has no valid down operation | non-zero command exit | `The latest migration cannot be reversed automatically.` plus safe version | Stop and follow the migration's documented recovery procedure. | Current version is retained; no rollback begins. | `migration.failed` |
| ERR-015 `MIGRATION_DIRTY` | migration store reports an incomplete/dirty version | non-zero command exit | `Migration state is incomplete.` plus safe version | Stop automatic migration and follow documented recovery; do not force a version. | Dirty state is not concealed or advanced. | `migration.failed` |
| ERR-016 `MIGRATION_APPLY_FAILED` | an `up` migration fails | non-zero command exit | `Migration could not be applied.` plus safe version | Correct the migration/dependency fault and retry according to recovery docs. | Transactional work is rolled back where supported; prior successful versions remain. | `migration.failed` |
| ERR-017 `MIGRATION_ROLLBACK_FAILED` | a `down` migration fails | non-zero command exit | `Migration could not be reversed.` plus safe version | Stop and use the named version's recovery procedure. | Prior versions remain; no further rollback is attempted. | `migration.failed` |
| ERR-018 `MIGRATION_STATUS_FAILED` | version/status cannot be read | non-zero command exit | `Migration status could not be read.` | Restore PostgreSQL or repair documented migration metadata, then retry. | No schema mutation is attempted. | `migration.failed` |
| ERR-019 `SHUTDOWN_TIMEOUT` | grace period expires with active work | non-zero process exit | `Shutdown exceeded its grace period; remaining HTTP work was stopped.` | Inspect bounded request logs, correct the blocking operation, then restart. | Completed durable work remains; incomplete requests receive no success claim. | `shutdown.forced` |
| ERR-020 `SHUTDOWN_FAILED` | one or more resources fail to close | non-zero process exit | `Shutdown completed with cleanup errors.` plus safe resource category | Inspect the matching redacted event before restarting. | Each close hook is attempted once; existing durable data remains. | `shutdown.failed` |

## M1.2 identity codes (CR-002)

| Code | HTTP / delivery | Meaning |
| --- | --- | --- |
| `AUTH_REQUIRED` | 401 JSON | No session cookie was sent. |
| `INVALID_SESSION` | 401 JSON, cookie cleared | Session is unknown, expired or revoked. |
| `OAUTH_STATE_INVALID` | 302 to `FRONTEND_URL/?auth_error=` | State is missing, unknown, expired or already used; these are deliberately indistinguishable. |
| `OAUTH_CALLBACK_FAILED` | 302, same form | Callback lacked a code or state, or failed unexpectedly. |
| `FIGMA_AUTH_FAILED` | 302, same form | User denied access or Figma rejected the code exchange. |
| `FIGMA_IDENTITY_FAILED` | 302, same form | Figma identity lookup failed or did not match the token owner. |
| `FIGMA_RECONNECT_REQUIRED` | internal (`auth.ErrReconnectNeeded`) | Stored Figma authorization was revoked; the user must sign in again. |
| `RATE_LIMITED` | 429 JSON with `Retry-After` | More than 20 requests per minute per address to an auth start or callback route. |
| `FORBIDDEN` | 403 JSON | State-changing request came from another origin. |
| `DEPENDENCY_UNAVAILABLE` | 503 JSON or 302 | Redis, PostgreSQL or Figma was temporarily unavailable. |

## M1.3 project codes (CR-003)

| Code | HTTP | Meaning |
| --- | --- | --- |
| `INVALID_PROJECT_ID` | 400 | Path ID is not a UUID. |
| `INVALID_PROJECT_NAME` | 400 | Name missing, blank, over 120 characters, or contains control characters. |
| `INVALID_REQUEST` | 400 | Bad JSON, unknown or extra fields, empty PATCH, or invalid `limit` or `cursor`. |
| `PROJECT_NOT_FOUND` | 404 | No such project for this user; identical for foreign projects. |
| `REQUEST_TOO_LARGE` | 413 | Body exceeds `HTTP_MAX_BODY_BYTES`. |
| `RATE_LIMITED` | 429 | Also returned by `/api/v1` routes: 300 per address per minute, 30 writes per user per minute. |

## M1.4 Figma client codes (CR-005)

Returned by the Go client as `figma.Error.Kind`; they are not HTTP responses yet.

| Code | Meaning |
| --- | --- |
| `FIGMA_AUTH_REQUIRED` | User must reconnect Figma. |
| `FIGMA_TOKEN_EXPIRED` | Figma rejected the token after one replacement. |
| `FIGMA_PERMISSION_DENIED` | User cannot access that Figma file (403). |
| `FIGMA_FILE_NOT_FOUND` | File or resource not found (404). |
| `FIGMA_RATE_LIMITED` | Figma rate limit (429); `RetryAfter` says when. |
| `FIGMA_BAD_REQUEST` | Figma rejected the request (400). |
| `FIGMA_BAD_RESPONSE` | Unreadable, oversized or unexpected Figma response. |
| `FIGMA_UNAVAILABLE` | Figma or credential store temporarily unavailable. |
| `FIGMA_REQUEST_TIMEOUT` | Request timed out. |
| `FIGMA_REQUEST_CANCELLED` | Caller cancelled. |

## M1.5 importer codes (CR-006)

| Code | HTTP / where | Meaning |
| --- | --- | --- |
| `INVALID_FIGMA_URL` | 400 | Not a supported Figma design link. |
| `INVALID_ID` | 400 | Project or import ID is not a UUID. |
| `INVALID_SELECTION` | 400 | Chosen node is not one of the listed frames. |
| `IMPORT_NOT_FOUND` | 404 | No such import for this user's project. |
| `IMPORT_CONFLICT` | 409 | An import is running, or this one cannot change now. |
| `IMPORT_BUSY` | 503 | Too many imports are running; retry. |
| `FIGMA_NODE_NOT_FOUND`, `FIGMA_UNSUPPORTED_NODE`, `FIGMA_NO_FRAMES` | import `error.code` | Target missing, not importable, or no frames in the file. |
| `FIGMA_IMPORT_FAILED`, `SNAPSHOT_TOO_LARGE`, `WORKSPACE_FAILED` | import `error.code` | Unreadable Figma data, design too large, or workspace problem. |
| `ASSET_DOWNLOAD_FAILED`, `ASSET_RESOLVE_FAILED` | import `error.code` | A Figma-provided asset could not be downloaded or located. |
| `ASSET_URL_BLOCKED`, `ASSET_INVALID_CONTENT` | import `error.code` | A URL failed the safety rules, or the bytes were not a supported image. |
| `ASSET_TOO_LARGE`, `ASSET_BUDGET_EXCEEDED` | import `error.code` | One asset, or all assets of an import, exceeded the configured size. |
| `TOO_MANY_SCREENS` | import `error.code` | The selection expands to more screens than `MAX_SCREENS`. |
| `DESIGN_IR_INVALID_INPUT`, `DESIGN_IR_ROOT_MISSING`, `DESIGN_IR_VALIDATION_FAILED` | import `error.code` | The design could not be converted into a valid Design IR. |
| `DESIGN_IR_LIMIT_EXCEEDED` | import `error.code` | The design exceeds the node, depth, screen or file-size limits. |
| `IMPORT_TIMEOUT`, `IMPORT_CANCELLED`, `IMPORT_SUPERSEDED` | import `error.code` | Time limit, shutdown, or replaced by a newer import. |
| `IMPORT_INTERRUPTED` | import `error.code` | The server stopped while the import was running; start it again. |

Figma client codes (`FIGMA_AUTH_REQUIRED`, `FIGMA_PERMISSION_DENIED`, `FIGMA_FILE_NOT_FOUND`, `FIGMA_RATE_LIMITED`, `FIGMA_UNAVAILABLE`, `FIGMA_REQUEST_TIMEOUT`) are stored as the import's `error.code` unchanged.

## M1.8 design API codes (CR-009)

| Code | HTTP | Meaning |
| --- | --- | --- |
| `DESIGN_NOT_FOUND` | 404 | The project has no import yet. |
| `DESIGN_NOT_READY` | 409 | The design is still being imported, or waiting for a screen choice. |
| `DESIGN_DATA_EXPIRED` | 410 | The temporary design data was cleaned up; import again. |
| `SCREEN_NOT_FOUND`, `FLOW_NOT_FOUND` | 404 | No such screen or flow in the current design (also for malformed IDs). |
| `PREVIEW_NOT_AVAILABLE` | 404 | The screen has no stored preview image. |
| `DESIGN_IR_INVALID` | 500 | The stored design could not be read. |

Public warning codes on screens: `UNKNOWN_NODE_TYPE`, `UNSUPPORTED_PAINT`, `UNSUPPORTED_EFFECT`, `UNSUPPORTED_MASK`, `UNSUPPORTED_LAYOUT_GRID`, `UNKNOWN_BLEND_MODE`, `MISSING_ASSET`, `MISSING_BOUNDS`, `MIXED_STYLE_PARTIAL`, `ROTATED_BOUNDS_APPROXIMATE`, `ASSET_NOT_FULLY_SUPPORTED`, `DESIGN_NOT_FULLY_SUPPORTED`.

## Catalogue rules

1. Stable codes are uppercase ASCII with underscores and are safe for scripts. A new condition receives a new code; an existing code is never silently repurposed.
2. Messages never contain secret values, DSNs, SQL, stack traces, driver/class names, internal hostnames, raw request data, or environment values.
3. Configuration failures may name an allow-listed environment variable but never print its value.
4. Dependency failures name only `postgres` or `redis`. Detailed internal causes are redacted before logging.
5. HTTP status communicates transport outcome; the code communicates the stable condition; the request ID provides correlation.
6. An unexpected failure maps to `INTERNAL_ERROR`; it does not expose the unexpected cause to the caller.
7. Error paths do not claim work succeeded. Migration and shutdown messages state precisely what state is known to be preserved.

## Profile codes (CR-013)

| Code | HTTP | Meaning |
| --- | --- | --- |
| `INVALID_API_KEY` | 400 | The key is missing or does not have the shape of that provider's keys. The key is never echoed. |
| `UNKNOWN_PROVIDER` | 404 | The provider in the path is not `anthropic`, `openai` or `xai`. |
| `CONFIRMATION_REQUIRED` | 400 | Account deletion needs the exact phrase `delete my account`. |
| `ACCOUNT_NOT_FOUND` | 404 | The account was already deleted. |

## Preview and generation codes (CR-016)

| Code | HTTP | Meaning |
| --- | --- | --- |
| `INVALID_PROVIDER` | 400 | No provider was sent, or it is not `anthropic`, `openai` or `xai`. |
| `KEY_REQUIRED` | 400 | The person has no saved key for that provider. |
| `DESIGN_NOT_READY` | 409 | The project has no finished design to generate from. |
| `GENERATION_RUNNING` | 409 | A generation is already running for this project. |
| `GENERATION_NOT_FOUND` | 404 | The generation does not exist for this project and person. |
| `GENERATION_NOT_AVAILABLE`, `GENERATION_INTERRUPTED`, `GENERATION_TIMEOUT` | generation `error.code` | Writing the code is not built yet; the server stopped mid-run; the job ran too long. |

## Assets and screen selection (CR-017)

| Code | HTTP | Meaning |
| --- | --- | --- |
| `ASSET_NOT_FOUND` | 404 | The asset is not part of this design, is not a viewable image, or its file is gone. |
| `INVALID_SELECTION` | 400 | The chosen screens are empty, malformed, too many, or not in the design. |

## Generation plans (CR-019)

| Code | HTTP | Meaning |
| --- | --- | --- |
| `INVALID_SELECTION` | 400 | The selection mode or its fields are not valid. |
| `EMPTY_SELECTION` | 400 | No eligible screens were chosen. |
| `INVALID_DESIGN_VERSION` | 400 | The design version is missing or not valid. |
| `GENERATION_TARGET_UNSUPPORTED` | 400 | Only Next.js with TypeScript is supported. |
| `SCREEN_NOT_FOUND`, `FLOW_NOT_FOUND` | 404 | The screen or flow is not in the pinned design. |
| `GENERATION_PLAN_NOT_FOUND` | 404 | The plan does not exist for this project and person. |
| `DESIGN_NOT_READY` | 409 | The project has no finished design. |
| `DESIGN_VERSION_UNAVAILABLE` | 409 | That design version is not the current one, or its temporary files expired. |
| `GENERATION_DEPENDENCY_INVALID`, `GENERATION_DEPENDENCY_CYCLE`, `GENERATION_PLAN_INVALID` | 422 | The design has a broken reference or components that depend on each other, or the plan failed validation. |
| `GENERATION_PLAN_TOO_LARGE` | 422 | The selection needs more planning work or output than allowed. |

| `FIGMA_RATE_LIMITED` | 429 / import `error.code` | Figma told this person to wait. The message says how long (from Figma's Retry-After); no request goes to Figma until then. |
