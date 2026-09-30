# M1 Security notes and invariants

This is a practical description of what M1 protects and how it is tested. It is not a claim that the system cannot be attacked; residual risks are listed at the end.

## System invariants

| # | Invariant | Where it is enforced | Proof |
| --- | --- | --- | --- |
| I-1 | A user reaches only their own projects | Every query filters by `user_id`; foreign IDs return 404 | `TestCrossUserAccessIsDeniedAsNotFound`, `TestLiveCrossUserIsolation` |
| I-2 | A user reaches only imports of their own projects | Import queries join `projects.user_id` | `TestOwnershipIsCheckedBeforeAnyWork`, `TestLiveImportFlowAndIsolation` |
| I-3 | A user reaches only previews of their own designs; project, import and screen must match | Design lookups are scoped to the owner's current import | `TestOnlyTheOwnerCanReachADesign`, `TestEveryLookupIsScopedToTheOwner`, `TestSelectingFromAnotherImportsCandidatesIsRejected` |
| I-4 | Figma tokens never reach the browser | Tokens live only in `figma_connections`, encrypted | `TestImportJSONShapesAreSafe`, `TestDTOsNeverContainInternalDetail` |
| I-5 | Figma tokens are never logged | Errors carry labels only | `TestTokenNeverAppearsInErrorsOrLogs`, `TestSnapshotsNeverContainCredentials` |
| I-6 | Session tokens are never stored in plaintext | Only the SHA-256 is stored | `internal/auth/pgstore` live tests |
| I-7 | Temporary Figma URLs are not durable | URLs are used once; `render.json` and the manifest hold none | `TestImportWithNodeIDCompletes`, `TestManifestHoldsNothingSensitiveAndIsSorted`, `TestSaaSProductImportsEndToEndWithEveryInvariant` |
| I-8 | Asset and preview paths cannot escape the import workspace | Strict name pattern, `O_NOFOLLOW`, symlink refusal | `TestIDsAndNamesCannotEscapeTheRoot`, `TestHelpersRefuseHostileNamesAndLinks`, `TestPreviewIdentifiersCannotBeUsedToReachFiles`, `FuzzHostileNamesNeverLeaveTheWorkspace` |
| I-9 | Raw Figma data is not the API contract | Public DTOs are built from the IR only | `TestDTOsNeverContainInternalDetail` |
| I-10 | The IR holds no credentials and no absolute paths or URLs | Validator rejects unsafe paths; test scans stored artifacts | `TestValidatorRejectsBrokenDesigns`, `TestSaaSProductImportsEndToEndWithEveryInvariant` |
| I-11 | IR and manifest bytes are deterministic | Sorted output, hashed IDs, no timestamps in structure | `TestNormalizationIsByteForByteStable`, `TestOutputDoesNotDependOnConcurrency`, `TestSaaSProductOutputIsReproducibleAcrossRuns`, 300-screen determinism test |
| I-12 | IR asset references resolve to manifest entries backed by real, checksummed files | Validator plus manifest verification | `TestVerifyStoredDetectsAnyChange`, `TestSaaSProductImportsEndToEndWithEveryInvariant` |
| I-13 | The Design API needs no Figma call | `designapi` has no Figma dependency | `TestDesignAPINeverDependsOnTheFigmaClientOrTheImporterWorkers` |
| I-14 | One failed import does not corrupt another | Unique workspace per import, one running import per project | `TestConcurrentStartsAllowExactlyOne`, `TestConcurrentRunsUnderTheRaceDetector`, `TestPanicInsideAnImportFailsThatImportOnly` |
| I-15 | Cleanup never deletes outside `TEMP_WORKSPACE_ROOT/imports` | Root guard plus symlink unlink | `TestCleanupIsRepeatableAndStaysInsideTheRoot`, `TestRemoveRefusesPathsOutsideImports`, `TestSymlinkedWorkspaceIsUnlinkedNotFollowed` |
| I-16 | No import stays `processing` after its process dies | `Sweep` fails abandoned imports | `TestSweepFailsImportsAbandonedByACrashedProcess`, `TestFailAbandonedRetiresOnlyOldRunningImports` |

## Controls by threat

- **Authentication.** Figma OAuth with PKCE and a single-use, short-lived state consumed atomically; sessions are 32 random bytes, only the hash is stored, cookies are `HttpOnly`, `SameSite=Lax`, `Secure` in production (production refuses to start otherwise). Unsafe methods require a same-origin request.
- **Token storage.** AES-256-GCM with a random nonce and the purpose as associated data; the key must be exactly 32 bytes. Refresh runs under a Redis lock with a re-check after acquiring it.
- **Authorization (IDOR).** Ownership is part of each SQL statement, not a separate check, and foreign resources answer 404 so existence is not revealed.
- **SQL injection.** All statements are parameterized; there is no dynamic ordering or search text in SQL.
- **Mass assignment.** Requests decode strictly into explicit DTOs (`name`, `figma_url`, selection fields); unknown fields such as `user_id`, `status` or `workspace_path` are rejected with 400 before any service runs (`TestProjectValidation`, `TestStartValidation`, `TestSelectValidation`).
- **URL parsing.** Only `https` URLs on `figma.com` and `www.figma.com` of kind design, file or proto are accepted, with strict key and node-id patterns. Lookalike, Unicode, userinfo, trailing-dot and encoded hosts are rejected (`TestParse`, `FuzzParseNeverPanics`).
- **SSRF.** The downloader accepts only provider-returned https URLs, blocks private, loopback, link-local and metadata addresses on the resolved IP at connect time, and revalidates every redirect (`TestControlChecksTheResolvedAddress`, `TestEveryRedirectIsRevalidatedUnderTheStrictPolicy`).
- **SVG policy.** SVGs are sanitized before storage: scripts, event handlers, `foreignObject`, external references and DTDs are removed. Layr never inlines them in a page in M1; a future consumer must still treat them as untrusted (`FuzzSanitizeSVGNeverPanicsAndLeavesNoActiveContent`).
- **Resource exhaustion.** Bounded bodies, snapshots, assets, IR size, node count, depth, screens, concurrency, timeouts and per-user rate limits; excess fails with a typed error and cleans partial files.
- **Files.** There is no generic file-serving route. Previews are streamed by screen ID, resolved through the owner's design to a stored path, with `Cache-Control: private` and ETag.
- **Secrets in logs.** Logs carry identifiers, counts, durations and error codes only; the access log records the route pattern, not raw URLs.

## Residual risks

- SVGs are sanitized, not proven safe; any later inlining needs its own review or a sandboxed image context.
- Raw Figma JSON in the workspace can contain proprietary design content; it is protected by file permissions, the private directory and the TTL only.
- OAuth tokens are encrypted with a single key held in configuration; there is no key rotation yet.
- A compromised host can read the workspace and configuration; disk encryption and access control are deployment concerns.
- Redis outage disables rate limiting and the refresh lock; dependent endpoints answer 503 rather than failing open.
- Docker-based deployment paths were not executed in M1.9.
