# Layr client

The Layr web app. Signed-out visitors see the landing page and can sign in with Figma through the Layr backend. Signed-in people land on their projects dashboard, where they can paste a Figma link to import a design, choose which screens to import, preview imported screens, and delete projects. It is a small single-page app built with Vite and React, with no runtime dependencies besides React itself.

## Technology

| Purpose | Choice |
| --- | --- |
| UI | React 19 |
| Build and dev server | Vite 8 with `@vitejs/plugin-react` |
| Styling | One plain CSS file with custom properties, no CSS framework |
| Routing | A 20-line path router (`src/router.jsx`), no routing library |
| Fonts | System font stack, nothing to download |
| Icons and art | Inline SVG, one 2 KB WebP logo, one 2.5 KB SVG background |
| Tests | Node's built-in test runner, no test framework |

Weight of a production build: about 75 KB of gzipped JavaScript (almost all of it React) and 3 KB of gzipped CSS for the landing page. The dashboard is a separate chunk (about 5 KB of JavaScript and 3 KB of CSS gzipped) that loads only after sign-in.

## Layout

```text
index.html              static SEO and social tags (read by crawlers that do not run JavaScript)
vite.config.js          dev server on port 3000, React plugin, SEO plugin
vite.seo.js             fills the site URL into index.html; writes sitemap.xml and robots.txt at build
brand/
  generate.py           builds the logo, favicons and social image from the full-size logo
  logo-master.png       the mark cut out with transparency
public/                 copied as-is: favicons, app icons, og-image.png, site.webmanifest, _redirects
src/
  main.jsx              mounts the app
  app/                  App.jsx (chooses the screen), router.jsx, useSession.js, usePageMeta.js
  config/               env.js: API and site URLs from the environment
  lib/
    api/                http.js (request, ApiError), auth.js, projects.js, imports.js, designs.js
    format.js           relative time, Figma link check, project name from a link (+ tests)
  components/           shared UI: AppHeader, PageHeader, ProfileMenu, Brand, Logo, SiteFooter, Loader, Skeleton, icons
  features/
    profile/            Profile.jsx, profile.css, components/ (IdentityCard, ApiKeysCard, KeyRow, IntegrationsCard, DangerZone, DeleteAccountDialog), hooks/useProfile.js
    landing/            Landing.jsx, landing.css, messages.js (+ tests)
    dashboard/
      Dashboard.jsx     page composition
      components/       ProjectCard, PreviewDialog, SelectScreens, SearchBar, Toast
      hooks/            useProjects, useImporter, useDesign
      dashboard.css
    project/            ProjectPage.jsx, project.css, components/ (Viewer, Thumbnails, DetailsCard, Palette, AddKeyDialog, ChooseModelDialog, ProjectSkeleton), hooks/ (useProjectData, useViewer, useGenerateFlow)
    generation/         GenerationPage.jsx, generation.css, progress.js (+ tests), components/StepList.jsx, hooks/useGenerationProgress.js
    contact/            ContactPage.jsx, contact.css, message.js (+ tests)
    legal/              LegalPage.jsx, legal.css, useActiveSection.js, content/terms.js, content/privacy.js (+ tests)
    pages/              InfoPage.jsx (Docs placeholder)
  styles/base.css       tokens, reset, page shell, footer
  assets/               brand/logo.webp, art/artwork.svg
```

Rules of thumb: styles are plain global CSS, so every feature stylesheet is scoped under its page root (`.landing`, `.dashboard`, `.profile-page`, `.contact-page`, `.legal-page`) and generic names like `.primary` or `.contact` never leak between pages; only `styles/base.css` and `components/header.css` are global by design. A feature folder owns its screens, hooks and styles; anything used by two features lives in `components/` or `lib/`; only `lib/api` talks to the backend.

## How it works

The page never handles Figma tokens or passwords. Sign-in is a normal browser navigation to the backend, which owns the OAuth secret, the state and the session cookie.

1. **On load** the app calls `GET /api/v1/me` with cookies included. A `401` shows the landing page with **Login with Figma**; a `200` shows the dashboard. If the backend cannot be reached within six seconds the landing page stays usable and shows a short notice.
2. **Login** is a link to `GET /auth/figma`. The backend sends the browser to Figma, then back to its own callback, sets the session cookie, and redirects to this app's `/`.
3. **Sign-in errors.** When the callback fails, the backend redirects to `/?auth_error=<CODE>`. The app shows a friendly message for that code, then removes the query string. Unknown codes get a generic message; the code itself is never printed.
4. **Projects.** The dashboard lists `GET /api/v1/projects` (newest first, 100 per page with **Load more**). Typing plain text filters the list by name.
5. **Import.** Pasting a Figma design link into the same box and submitting runs the whole flow: create a project, `POST .../import`, poll `GET .../import` every 1.5 seconds, and when the file has several screens open a dialog to choose them (`POST .../imports/{id}/select`). When the import completes the project is renamed to the Figma file name and its card appears with a preview. If the import fails, the backend's message is shown and the empty project is removed.
6. **Previews.** Each card asks `GET .../design` when it scrolls into view and shows the first screen's preview image. Previews are plain `<img>` requests; the session cookie authorizes them. Clicking a card opens a larger viewer with a thumbnail per screen.
7. **Delete** calls `DELETE /api/v1/projects/{id}` and offers **Undo** for eight seconds, which calls the restore endpoint.
8. **Log out** sends `POST /auth/logout` and returns to the landing page.
9. **Profile** (`/profile`, from the account menu) loads `GET /api/v1/profile`: identity, the Figma connection state, and one row per AI provider. Saving a key sends `PUT .../ai-keys/{provider}`; the key is cleared from the page at once and only its last four characters come back. **Remove** deletes it. **Delete account** asks the person to type `delete my account`, calls `DELETE /api/v1/profile` and returns to the landing page. **Logout** signs out. GitHub shows "Coming soon" because the backend has no GitHub connection yet.
10. **Legal pages.** `/terms` and `/privacy` render from plain data files in `features/legal/content/`, so the text is edited without touching components. They use a documentation layout: a fixed top bar, a contents list fixed on the left that highlights the section being read, and the text scrolling on the right (on phones the list folds into a collapsible "On this page" bar). They are indexable and listed in the sitemap, and the landing page shows a consent line linking to them. `/docs` is still a placeholder.
11. **Project preview** (`/projects/{id}`, opened from a project card) shows a pan and zoom viewer (buttons, drag, keyboard, fullscreen), every screen as a thumbnail with a checkbox, and a details card. **Import brings in every screen** of the Figma file (no more picking); on this page you tick the screens to work on or leave all ticked. The **Design tokens** tab lists every color (with use counts), text style, spacing value, corner radius and shadow; the **Assets** tab shows every image and vector Figma provided, inline, with filters. The **↻ button** on the viewer re-imports the design from Figma (`POST .../import/refresh`), shows its progress ("Reading your Figma file again…", "Importing all N screens…") and then reloads the screens, tokens and assets in place; the zoom percentage resets the view. On wide screens the top bar is fixed and the two sides scroll independently; on smaller screens the page scrolls normally. Delete offers Undo.
12. **Generate.** The button checks the person's saved keys (`GET /api/v1/profile`): no key opens a popup to choose a provider, paste a key and save it; one key starts straight away; several keys open a popup to choose the model (the last one used is preselected). It then calls `POST .../generations` with the ticked screens (left out when every screen is ticked) and moves to `/projects/{id}/generations/{generationId}`.
13. **Progress page.** It polls the job every 0.7 seconds and shows each step as the server recorded it (in progress, done with its detail, or stopped), with the model named. Because real steps can finish in milliseconds, each recorded change is revealed about 0.6 seconds after the previous one so it can be read; nothing is invented, and the order and details are the server's. A run that finished long ago is shown at once. Today the final step, writing the code, reports that it is not available yet.
14. **Contact** (`/contact`, linked from the footer) has a name, email and message form with inline checks. There is no backend for it yet: **Send message** opens the visitor's email app with a message addressed to `VITE_CONTACT_EMAIL` and the details filled in, and the page says clearly that nothing is sent until they press Send there. A copy button and an "open again" button cover people without a mail app. When a mail service exists, replace `sendMessage` in `features/contact/message.js` with a call to it; the form and checks stay as they are.

The header stays minimal (logo left, account menu right); Projects, Profile / Settings and Log out are all in the menu.

What the dashboard does not have yet, because the backend has no matching feature: favorites, "recently viewed", "shared with me", project rename by hand, and code generation. The filter bar therefore shows only **All projects**.

## Legal pages

The Terms and Privacy Policy describe what Layr actually does today: the two Figma permissions it requests, what it stores and for how long, the single essential cookie, encrypted AI keys, and self-service account deletion. When behavior changes (for example code generation, GitHub, payments or analytics), update the content files and the effective date in the same change. `npm test` checks that the pages state key facts, have unique sections, and use only known inline tokens. The text is a plain-language draft and should be reviewed by a lawyer before launch.

## SEO and sharing

- **Tags.** `index.html` carries the title, description, canonical link, Open Graph and Twitter Card tags, JSON-LD, theme color and icon links. Link previews on WhatsApp, Slack, LinkedIn, X and Facebook do not run JavaScript, so these tags are static on purpose.
- **Share image.** `public/og-image.png` (1200 × 630) is used for `og:image` and `twitter:image`.
- **Icon cache.** Browsers keep a favicon per host for a long time, so a site on `localhost` can keep showing the icon of an older project. The icon URLs carry a version that changes on every build and dev start (`vite.seo.js`). If a tab still shows a wrong icon, close and reopen the tab (Safari: Develop, Empty Caches; Chrome: hard reload).
- **Icons.** `favicon.ico` (16, 32, 48), `favicon-16x16.png`, `favicon-32x32.png`, `apple-touch-icon.png` (180), `icon-192.png`, `icon-512.png`, a maskable `icon-maskable-512.png`, and `site.webmanifest` for installable-app metadata.
- **Sitemap and robots.** Generated at build from `VITE_SITE_URL`: `sitemap.xml` lists the home page, and `robots.txt` allows crawling and points to the sitemap. The placeholder pages and the signed-in dashboard are marked `noindex` while the page is open.
- **Per page.** `usePageMeta` updates the title, description, canonical URL, robots flag and share tags as the page changes.
- **After deploy,** check the result with the social platforms' link debuggers (Facebook Sharing Debugger, LinkedIn Post Inspector, X card validator) and submit `sitemap.xml` in Google Search Console. Some platforms cache previews, so a change to the image may take time to appear.

## Brand assets

The logo, favicons and share image are generated from one full-size logo file so they always agree:

```sh
pip install pillow numpy scipy
npm run brand -- path/to/full-size-logo.png
```

This extracts the whole mark with transparency (`brand/logo-master.png`), then writes `public/*` and `src/assets/brand/logo.webp`. The share-image text and colors live in `brand/generate.py`.

## Configuration

Copy `.env.example` to `.env` (optional for local development):

| Variable | Default | Meaning |
| --- | --- | --- |
| `VITE_API_URL` | `http://localhost:8080` | Base URL of the Layr API, without a trailing slash |
| `VITE_SITE_URL` | `https://layr.appmd.dev` | Public URL of the site, used for canonical links, share tags, sitemap and robots |
| `VITE_OPERATOR_NAME` | `Layr` | Name of whoever runs Layr, used in the Terms and Privacy Policy |
| `VITE_CONTACT_EMAIL` | `hello@layr.appmd.dev` | Contact address on the Contact page and in the legal pages |
| `VITE_GOVERNING_LAW` | empty | Country whose law governs the Terms; when empty the Terms use a general wording |

Vite bakes this value into the build, so change it and rebuild for each environment. Never put secrets in `VITE_` variables; everything with that prefix is public.

The backend must agree with the client:

| Backend setting | Must be |
| --- | --- |
| `FRONTEND_URL` | this app's exact origin, for example `http://localhost:3000` locally or `https://layr.appmd.dev` in production |
| `FIGMA_REDIRECT_URI` | the backend callback URL registered in the Figma app |
| `SESSION_COOKIE_SECURE` | `true` in production (the backend refuses to start otherwise) |

CORS with cookies works only for that one origin (methods GET, POST, PATCH and DELETE are allowed), and the session cookie is `SameSite=Lax`, so the client and the API must be on the same site (for example `layr.appmd.dev` and `api.layr.appmd.dev`, or both on `localhost`).

## Run

Requires Node 20.19 or newer (or 22.12 or newer) and the backend running (see `server/README.md`).

```sh
npm install
npm run dev
```

Open `http://localhost:3000`. The dev server uses port 3000 on purpose and fails instead of moving to another port, because the backend only accepts that origin.

## Build and preview

```sh
npm run build      # writes dist/
npm run preview    # serves dist/ on port 3000
```

## Deploy

`dist/` is static files. Any static host works, with two requirements:

- **Fallback to `index.html`** for unknown paths, so `/docs`, `/privacy` and `/terms` work when opened directly. `public/_redirects` already contains the rule for Netlify and Cloudflare Pages; Vercel needs an equivalent rewrite.
- **Serve over HTTPS** in production, matching the backend's `FRONTEND_URL`.

Set `VITE_API_URL` and `VITE_SITE_URL` to the production values before building.

## Testing

```sh
npm test           # unit tests for message mapping, link checking and time formatting
npm run verify     # tests, then a production build
```

For a manual end-to-end check: start PostgreSQL, Redis and the backend, run `npm run dev`, click **Login with Figma**, approve access, and confirm you return to the page signed in. Then paste a Figma design link into the search box, choose screens if asked, and confirm the project card shows a preview. Delete it and use **Undo**. Finally click **Log out**.

## Design notes

- Layout, colors and copy follow the supplied landing design; the desktop layout was drawn at 1536 × 1024 and there is a separate phone layout below 700 px.
- The logo is the full mark, cut out with transparency (the earlier crop clipped the bottom of the L) and re-encoded to a 12 KB WebP.
- Pages with ids: `/projects/{id}` and `/projects/{id}/generations/{id}` are matched by `app/routes.js` (ids must be real UUIDs, anything else is the 404 page).
- Loading states: the Layr loader (`components/Loader.jsx`, sizable) is the only spinner, used for the session check, the lazy dashboard and import progress; content areas use shimmering skeletons (`Skeleton`, `ProjectCardSkeleton`) that match the final layout. Reduced-motion users get a slower loader and static skeletons.
- The footer is deliberately tiny (12px links, a single slim row).
- Every screen is a column at least one viewport tall, so the footer stays at the bottom on short pages and follows the content on long ones.
- The decorative background is a real SVG file referenced from the CSS, so it adds nothing to the JavaScript.
- Motion is limited to a small hover lift on the button and is turned off for people who prefer reduced motion.

## Troubleshooting

- **`Port 3000 is already in use`:** another process holds the port. Stop it; the port cannot change (see Run).
- **The page says it cannot reach Layr:** the backend is not running, or `VITE_API_URL` points somewhere else.
- **Signed in on the backend but the page shows the login button:** the cookie is not being sent. Check that the client and API share a site and that `FRONTEND_URL` matches the page's origin exactly.
- **Redirected back with a message that the sign-in link expired:** the Figma approval took longer than the backend's `OAUTH_STATE_TTL`; try again.
- **Rename, delete or undo fail in the browser but work with curl:** the backend is an older build; its CORS list must include PATCH and DELETE.
- **Logout does nothing:** the backend rejected the request as cross-site; check `FRONTEND_URL`.
# layr
