# Changelog

## 0.1.0 - M1.1 server foundation (minor, unreleased)

Type: adaptive/perfective (new capability). Traces to REQ-001..REQ-012, NFR-001..NFR-015, DOM-001..DOM-003.

- Typed configuration, bounded PostgreSQL and Redis clients, embedded SQL migrations.
- `GET /health`, `GET /ready`, request IDs, structured redacted logging, panic recovery, body limit.
- Graceful and forced shutdown; DEF-001 fixed (corrective, regression `TestRunLifecycleServeFailureClosesResources`).
- CR-001: `APP_ENV`, CORS from `FRONTEND_URL`, API security headers, `TEMP_WORKSPACE_ROOT`.

## 0.2.0 - M1.2 identity (minor, unreleased)

Traces to CR-002.

- Figma OAuth with PKCE, users, encrypted Figma connections, hashed server-side sessions.
- `GET /auth/figma`, `GET /auth/figma/callback`, `POST /auth/logout`, `GET /api/v1/me`.
- Token refresh under a Redis lock; auth rate limiting; same-origin check for unsafe methods.

## 0.3.0 - M1.3 projects (minor, unreleased)

Traces to CR-003. Owner-scoped project CRUD with keyset pagination; migration 00003.

## 0.3.1 - project safety (patch, unreleased)

Traces to CR-004. Soft delete with 30-day restore (`POST /api/v1/projects/{id}/restore`), lazy purge, and rate limits on all `/api/v1` routes.
- Follow-up: restore keeps the original update time; rate limits are configurable (`RATE_LIMIT_*_PER_MINUTE`).

## 0.4.0 - M1.4 Figma API client (minor, unreleased)

Traces to CR-005. Typed `GetFile`, `GetFileNodes`, `GetImageFills`, `RenderNodes`; typed errors; bounded retries with Retry-After; one token replacement on 401.

- Follow-up: `Snapshot` raw-body capture and extra typed fields (layout grids, style refs, hyperlinks, overrides, bound variables).
- Follow-up: lenient decoding (`Node.Extra`, `Node.Drift`), drift report and logging, nesting-depth guard, mutation and fuzz tests.

## 0.5.0 - M1.5 Figma importer (minor, unreleased)

Traces to CR-006. `POST /api/v1/projects/{id}/import`, status and frame-selection endpoints, temporary workspaces, migration 00005.

## 0.6.0 - M1.6 asset pipeline (minor, unreleased)

Traces to CR-007. Real images and SVGs stored per import with a versioned manifest; reference render downloaded to `reference/reference.png`; migration 00006.

## 0.7.0 - M1.7 Design IR (minor, unreleased)

Traces to CR-008. Design IR v1 (`design/design-ir.json`), deterministic normalizer, multi-screen selection (`node_ids` / `all`), per-screen reference renders (manifest `references`), migration 00007.

## 0.8.0 - M1.8 design API (minor, unreleased)

Traces to CR-009. `GET /api/v1/projects/{id}/design` and screens, flows and preview endpoints with public DTOs; workspace `Existing` and `OpenRegular` read helpers.

## 0.9.0 - M1.9 hardening (minor, unreleased)

Traces to CR-010. Corrective: abandoned imports are failed with `IMPORT_INTERRUPTED` (DEF-002); database outages return 503 `DEPENDENCY_UNAVAILABLE` on project, import and design routes (DEF-003). Perfective: Design IR is compact JSON; validator no longer allocates per field; `Dir.Open` never follows a link. Added stress, benchmark, fuzz and SaaS end-to-end tests and the M1 architecture, security and failure-matrix documents.

## 0.10.0 - client dashboard (minor, unreleased)

Traces to CR-011. Vite and React client: landing page with a cleaner Login with Figma button, projects dashboard (list, search, grid and list views), paste-a-link import with screen selection, previews, delete with undo. Backend: CORS now allows PATCH and DELETE.

## 0.11.0 - client structure, SEO and brand assets (minor, unreleased)

Traces to CR-012. Client folders reorganized by responsibility. Added canonical, Open Graph and Twitter Card tags, JSON-LD, generated sitemap.xml and robots.txt, web manifest, full favicon and app-icon set, and a 1200x630 share image. Logo re-cut from the full-size original (the previous crop clipped the L). Footer now rests at the bottom of every screen.

## 0.11.1 - loaders (patch, unreleased)

Traces to CR-012. One shared Layr loader replaces every spinner; project cards, previews and the project list use shimmering skeletons; the footer is much smaller.

## 0.12.0 - profile page (minor, unreleased)

Traces to CR-013. Profile page (identity, Figma connection state, AI keys for Anthropic, OpenAI and xAI, GitHub placeholder, delete account, logout). Backend: encrypted per-user AI keys that are never returned, account deletion that cascades through every table and clears temporary import files, CORS allows PUT. The header keeps its original layout; Projects and Profile live in the account menu.

## 0.12.1 - legal pages (patch, unreleased)

Traces to CR-014. Terms and Conditions and Privacy Policy replace the placeholders, with a summary and contents on each page, a consent line under the login button, and sitemap entries. Contact email and governing law come from build settings.

## 0.12.2 - legal page layout (patch, unreleased)

Traces to CR-014. Terms and Privacy use a documentation layout: fixed top bar, fixed contents list on the left with the current section highlighted, collapsible contents on phones.

## 0.13.0 - contact page (minor, unreleased)

Traces to CR-015. Contact page and footer link. The form has no backend: it opens the visitor's email app with the message ready for hello@layr.appmd.dev. Legal pages now show that address.

## 0.13.1 - style scoping (patch, unreleased)

Fixes the profile identity card layout, which picked up the Contact page's `.contact` styles. Every feature stylesheet is now scoped under its own page root so pages cannot affect each other.

## 0.14.0 - project preview and generation flow (minor, unreleased)

Traces to CR-016. Project preview page from the supplied design, Generate flow with add-key and choose-model popups, and a progress page driven by real recorded job steps. Backend: `GET .../design/tokens`, `POST/GET .../generations`, migration 00009. A race in job start (the caller and the job shared a list) was found by the race detector and fixed before release. Cards on the dashboard now open the preview page instead of a popup.

## 0.15.0 - everything from Figma on the preview page (minor, unreleased)

Traces to CR-017. Import brings in every screen; the preview page selects screens by checkbox and generation records the choice. Design tokens list every color, text style, spacing, radius and shadow. Assets tab shows all images and vectors inline. Figma comments appear read-only beside the design. Sign-in requests `file_comments:read`. Privacy policy updated.

## 0.15.1 - refresh and scrolling (patch, unreleased)

Traces to CR-018. The viewer's refresh button re-imports the design from Figma with visible progress. The project page keeps the header fixed and lets each side scroll on its own; all signed-in headers stay fixed.

## 0.16.0 - generation planner (minor, unreleased)

Traces to CR-019. M2.1: selection (one, selected, flow, all), dependency resolution from the Design IR, a validated dependency graph with parallel stages, plans pinned to an exact design version and stored immutably (migration 00011), and `POST/GET .../generation-plans`. Log lines for imports that fail Design IR validation now include the failed checks. Nothing is generated or run.

## 0.16.1 - links to pages, 500 screens, smaller home page (patch, unreleased)

Traces to CR-020. A Figma link to a page or group lists its frames instead of failing. Imports fetch and render in batches of 100, so up to 500 screens can be imported and shown (`MAX_SCREENS` now 1 to 500, default 500). The landing page is smaller and the terms line is very small.

## 0.16.2 - Figma rate limits, privacy, landing size (patch, unreleased)

Traces to CR-021. When Figma limits a person's requests, the importer remembers Figma's own wait, stops sending requests until it is over, and tells the person how long (`429 FIGMA_RATE_LIMITED` with `Retry-After`, and `error.retry_after_seconds` on a failed import). Comments are cached for 10 minutes and back off during a wait. The Children section is removed from the Privacy Policy. The landing page is larger again.

## 0.16.3 - Figma request pacing (patch, unreleased)

Traces to CR-022. Every request to Figma, retries included, is paced to 8 per minute per person (`FIGMA_REQUESTS_PER_MINUTE`), shared by imports and comments, so Layr cannot exhaust a person's Figma allowance by itself. Imports wait for a free slot; comments fail at once. The default import time limit is now 5 minutes because of the pacing.

## 0.16.4 - comments removed (patch, unreleased)

Traces to CR-023. The Figma comments feature is gone from the client and the server: no comments panel, no `GET .../comments` route, no `file_comments:read` scope. Sign-in now asks for `current_user:read` and `file_content:read` only.
