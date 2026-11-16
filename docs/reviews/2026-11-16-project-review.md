# Filebrowser Next Project Review

Review date: 16 November 2026, Europe/Paris.

The first priorities are destructive file operations, upload integrity, and vulnerable dependencies. The frontend builds successfully, but backend tests fail on Windows and several security defects remain in the reviewed source. Session revocation and filesystem confinement need changes beyond package updates.

This review covers security and hardening, frontend dependencies and assets, Go dependencies and migration candidates, and other correctness and delivery issues. All 28 findings are accepted as **To fix** under the active implementation goal. See the [fix status](fix-status.md) for current progress and validation evidence. The findings and measurements below preserve the original baseline review.

## Baseline and evidence

The reviewed source was imported into a local Git repository on branch `main`. Baseline commit: `d810b7c` (`chore: establish local project baseline`). No remote or release tag was added. The commit records the supplied source snapshot; it does not reconstruct upstream history or identify a released version.

The project uses Go, Vue 3, TypeScript, Vite 8, Vue Router 5, ESLint 10, and pnpm. Its module remains `github.com/filebrowser/filebrowser/v2`. The frontend package version is `3.0.0`; the backend reports an untracked version without build metadata. The upstream archive warnings in the README still apply to the inherited project. They do not imply that the local changes proposed here have been made.

Evidence labels distinguish the strength of each finding:

- **Reproduced:** exercised against this source with temporary test data.
- **Source confirmed:** traced in the source, without a complete runtime exploit or browser reproduction.
- **Audit reported:** identified by a dependency scanner; applicability still needs checking.
- **Investigation:** a hardening or design concern whose practical impact needs further testing.

P0 means address before relying on the affected file operation, P1 means high priority for the next work, and P2 means scheduled follow-up. These are project priorities, not CVSS scores. Upstream advisories retain their own severity ratings.

## Validation results

| Check                                 | Result                                                                                                                                                 |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Locked frontend installation          | Passed with pnpm 10.33.4 and `--frozen-lockfile --ignore-scripts`; package manifests and lockfile were unchanged                                       |
| Frontend lint                         | Passed                                                                                                                                                 |
| Frontend type checking                | Passed                                                                                                                                                 |
| Frontend tests                        | Passed: 5 files, 29 tests                                                                                                                              |
| Production frontend asset compilation | Passed; output was written outside the project                                                                                                         |
| `go test ./...`                       | Failed on Windows in the `files` and `http` packages; other tested packages passed                                                                     |
| Isolated review probes                | Four reproduced defects; the symlink probe was skipped because Windows denied symlink creation                                                         |
| Full frontend dependency audit        | 80 reported findings: 1 critical, 48 high, 22 moderate, 9 low                                                                                          |
| Production dependency audit           | 48 reported findings: 1 critical, 34 high, 8 moderate, 5 low                                                                                           |
| Go vulnerability scan                 | 34 reachable advisory matches using local Go 1.26.1, plus 9 findings in imported packages and 10 in required modules without detected vulnerable calls |

The frontend checks were executed through the installed tools' Node entry points because the sandboxed Corepack shim attempted a blocked registry download. Registry checks and dependency installation succeeded with the required filesystem/network access. No deployed binary or production service was examined. Race-enabled Go tests, a Linux test run, and browser end-to-end tests were not completed in this pass.

Audit totals can include the same advisory at multiple package versions or dependency paths. Production dependency classification also does not establish that every affected function reaches the browser bundle. In particular, the critical DOMPurify advisory requires options and DOM/shadow-root processing that the current Markdown sanitization call does not use. Treat these totals as an update and triage backlog, not a count of independently verified exploits.

The [Go vulnerability snapshot](2026-11-16-go-vulnerabilities.txt) retains all advisory summaries and the scanner's separate symbol, package, and module result groups. Detailed call traces are omitted. The frontend snapshots contain 64 package/advisory records and 38 direct-package update entries; these record counts differ from the scanner's affected-version totals.

## Security and hardening findings

### SEC01 Failed upload cleanup can delete existing directories

**P0 — Reproduced.** `resourcePostHandler` accepts an existing directory as an overwrite target. `writeFile` fails when opening the directory for writing; the error path then calls `RemoveAll` on the requested path. The removal does not require `Perm.Delete` and does not authorize descendants.

An authenticated user with Create and Modify permissions, but no Delete permission, caused a directory containing an existing file to be removed. The request returned 500. This matches the unpatched upstream advisory [GHSA-c4fr-5f24-4wrj](https://github.com/filebrowser/filebrowser/security/advisories/GHSA-c4fr-5f24-4wrj).

Evidence: [resource handlers](../../http/resource.go), `resourcePostHandler`, especially lines 147–183; `writeFile`, lines 346–376. Probe: `TestReviewFailedUploadPreservesDirectory`.

Reject directory overwrite targets before writing. Cleanup must only remove temporary files created by the failed upload. Preserve existing content until a replacement has completed successfully; test denied descendants, interrupted uploads, and failed hooks.

### SEC02 Command WebSocket reads without a size limit before authorization

**P1 — Source confirmed.** `commandsHandler` upgrades the connection and calls `ReadMessage` before checking `EnableExec` and `Perm.Execute`. It never sets a read limit. An authenticated user can therefore consume memory even when command execution is disabled.

Evidence: [command endpoint](../../http/commands.go), lines 41–68. Upstream: [GHSA-39cx-23x9-5c8p](https://github.com/filebrowser/filebrowser/security/advisories/GHSA-39cx-23x9-5c8p).

Reject disabled or unauthorized execution before upgrading. Bound message size and read time. Separately decide whether to remove the runner and hooks or redesign their trust model; the inherited README already treats enabled command execution as equivalent to shell access. Dependency updates alone cannot make an executable allowlist a sandbox.

### SEC03 Content Security Policy does not reach the application HTML

**P1 — Reproduced.** The policy is installed as Gorilla mux route middleware, while the application HTML is served through `NotFoundHandler`. Requests to `/` and `/files/` returned 200 without `Content-Security-Policy`; the `/health` control route received the header.

Evidence: [HTTP routing](../../http/http.go), lines 33–47. Probe: `TestReviewCSPProtectsAppHTML`.

Apply the policy outside the router so it covers the SPA fallback. Design and test the policy together with the inline configuration bootstrap, legacy bundles, styles, editor modules, reCAPTCHA, and EPUB previews. Copying the existing policy onto HTML without adjusting those resources can break the app.

### SEC04 Sessions lack server side revocation

**P1 — Source confirmed.** The server validates self-contained JWTs. User updates trigger `X-Renew-Token`; they do not invalidate the old token. The renewal endpoint issues a new expiration to a still-valid token. Logout clears client state without revoking server-side credentials. A leaked token can remain useful after a password change and can be renewed while valid.

Evidence: [token verification and renewal](../../http/auth.go), `withUser` and `renewHandler`; [client authentication](../../frontend/src/utils/auth.ts), `parseToken`, `renew`, and `logout`. Background: [upstream session issue](https://github.com/filebrowser/filebrowser/issues/5216).

Introduce explicit session state or another durable revocation mechanism. Define logout, logout-all, password-change invalidation, refresh rotation, replay rejection, and an absolute lifetime. Persistence and multi-instance behavior must be part of the design.

### SEC05 In scope symlink aliases can bypass path deny rules

**P1 — Source confirmed; local runtime probe skipped.** Rule checks canonicalize the lexical request path. `ScopedFs` permits a symlink when its target remains inside the scope. An allowed alias can therefore name a file denied under its real path.

Evidence: [rule evaluation](../../http/data.go), `CheckRules` and `rulePath`; [scoped filesystem](../../files/scoped.go), `within`. Upstream: [GHSA-7w29-q235-57m9](https://github.com/filebrowser/filebrowser/security/advisories/GHSA-7w29-q235-57m9). Probe: `TestReviewSymlinkCannotBypassDenyRule`; Windows lacked the required symlink privilege, so this was not reproduced locally.

Define authorization for both the requested name and resolved target. Cover reads, writes, shares, searches, archive downloads, and recursive operations. Reproduce on Linux and on a Windows environment that permits symlinks.

### SEC06 Filesystem confinement checks and operations are separate

**P1 — Investigation.** `ScopedFs` resolves and checks a path, then delegates the actual operation to `BasePathFs`. A separate actor able to change symlinks or directory entries between those steps can undermine a check-then-use design. The practical race was not reproduced in this review.

Evidence: [scoped filesystem](../../files/scoped.go), `guard`, `within`, and the operation wrappers. Evaluate a descriptor-based implementation using [Go's traversal-resistant `os.Root` APIs](https://go.dev/blog/osroot), preserving afero integration and the explicit external-symlink setting. This is a migration candidate, not evidence that every current operation can escape scope.

### SEC07 Subtitle conversion has unbounded memory use

**P1 — Source confirmed.** The SRT path reads the entire file, creates normalized content, parses it, and builds another output buffer. There is no input-size ceiling in the handler. An account with download access to a sufficiently large subtitle can consume substantial server memory.

Evidence: [subtitle conversion](../../http/subtitle.go), `subtitleFileHandler`, particularly `io.ReadAll` around line 56. Upstream: [GHSA-448h-jr2h-3vhp](https://github.com/filebrowser/filebrowser/security/advisories/GHSA-448h-jr2h-3vhp).

Add input and output limits, reject oversized conversions early, and bound concurrent conversion work. Updating the subtitle parser is separate from enforcing application limits.

### SEC08 Browser credentials need stronger handling

**P2 — Source confirmed.** The JWT is persisted in `localStorage` and copied into a JavaScript-readable cookie with `Path=/` and `SameSite=Strict`. The cookie is not marked Secure and cannot be HttpOnly while being set this way from JavaScript. An XSS anywhere with the same origin can read the stored token, and the root cookie path also spans applications under other paths.

Evidence: [client authentication](../../frontend/src/utils/auth.ts), `parseToken` and `logout`; [GET token extraction](../../http/auth.go), `extractor.ExtractToken`.

Choose credential transport together with SEC04. Evaluate server-set Secure/HttpOnly cookies, appropriate path scoping, and explicit CSRF protection for cookie-authenticated mutations. Preserve the existing header-authenticated workflows where necessary.

### SEC09 Request abuse controls need defined limits

**P2 — Investigation.** Authentication already has a 1 MiB body cap, but no application login throttling was found. The HTTP server config sets a 60-second header timeout without an explicit body-read or idle policy. reCAPTCHA uses an HTTP client without an overall timeout. Some authenticated JSON handlers decode bodies without the authentication endpoint's size cap.

Evidence: [server configuration](../../cmd/root.go), around line 253; [authentication and reCAPTCHA](../../auth/json.go); [user request decoding](../../http/users.go); [share creation](../../http/share.go).

Define rate, concurrency, body-size, idle, and outbound-request limits. Uploads and downloads require streaming-aware deadlines; a blanket short read or write timeout would break legitimate large transfers. If client IPs become part of rate limiting, do not trust arbitrary forwarded headers: the current `realip` use is for logging and is not a trusted-proxy policy.

## Frontend dependencies and assets

### FRONT01 Resolve dependency advisories across the full tree

**P1 — Audit reported.** Full and production audit totals are in the validation table. The accompanying [frontend audit snapshot](2026-11-16-frontend-audit.json) records individual package/advisory entries, affected installed versions, fixes reported by the registry, dependency paths, and membership in the production audit.

The main groups are DOMPurify; XML parsers inherited from EPUB and video playback; Lodash inherited from EPUB; Vue I18n; and build/test dependencies including Vite, Rollup, Babel, PostCSS, Vitest, and glob-processing packages. The direct `lodash-es` version does not eliminate the separate `lodash` dependency inherited from `epubjs`.

`epubjs` 0.3.93 requests `@xmldom/xmldom` in the 0.7 line, while video dependencies also include 0.8.11. Updating only top-level dependencies may leave these branches behind. Evaluate parent upgrades, narrowly scoped tested overrides, or replacement libraries. Do not apply broad major-version overrides without testing untrusted files and supported formats.

The critical alert is [DOMPurify GHSA-cg2g-xh64-v4q7](https://github.com/cure53/DOMPurify/security/advisories/GHSA-cg2g-xh64-v4q7). The Markdown editor calls the normal string sanitizer and does not use the affected shadow-root/template options. This narrows current exposure but does not remove the need to update and audit other usage.

### FRONT02 Separate routine updates from major migrations

**P2 — Registry checked.** The complete direct-package update snapshot is [frontend outdated packages](2026-11-16-frontend-outdated.json). Selected targets on the review date are:

| Package                      | Locked version | Available version |
| ---------------------------- | -------------- | ----------------- |
| DOMPurify                    | 3.4.12         | 3.4.18            |
| Vue                          | 3.5.34         | 3.5.45            |
| Vue I18n                     | 11.4.2         | 11.4.20           |
| Vite                         | 8.0.13         | 8.3.8             |
| Vue Router                   | 5.0.7          | 5.5.1             |
| Ace                          | 1.44.0         | 1.45.2            |
| vue-reader                   | 1.3.4          | 1.3.9             |
| CSV parser                   | 6.2.1          | 7.0.3             |
| TypeScript                   | 5.9.3          | 7.0.2             |
| Pinia                        | 3.0.4          | 4.1.0             |
| Video.js                     | 8.23.7         | 10.4.0            |
| Vitest                       | 4.1.6          | 5.1.0             |
| VueUse core and integrations | 14.3.0         | 15.0.0            |

Vite 8, Vue Router 5, and ESLint 10 are already in use and the reviewed build/lint/type checks passed. The Vite config still uses the compatibility `rollupOptions` and `manualChunks` interface; evaluate the [Vite migration guidance](https://vite.dev/guide/migration.html) and Rolldown chunking when updating the build configuration.

Keep pnpm as the existing package manager unless there is a separate reason to change it. The lockfile is reproducible and CI already uses frozen installs. The pinned manager is pnpm 10.33.4; an update notice reported 12.21.0, which should be treated as a separate major migration. Keep Node type definitions aligned with the chosen Node baseline rather than automatically selecting the newest major.

### FRONT03 Large bundles and old browser data increase asset costs

**P2 — Build measured.** The modern production build emitted these approximate minified sizes:

| Chunk            | Uncompressed | Gzip      |
| ---------------- | ------------ | --------- |
| Main application | 792.10 kB    | 225.17 kB |
| Translations     | 828.86 kB    | 194.83 kB |
| Preview          | 1,257.51 kB  | 377.44 kB |
| Editor           | 401.01 kB    | 121.88 kB |
| Day.js           | 57.94 kB     | 15.40 kB  |

The generated HTML preloads the translation chunk. Preview/editor code is split, but heavy media and reader features still share a large preview chunk. The build also emits legacy assets and warns that browser compatibility data is seven months old.

Evidence: [Vite configuration](../../frontend/vite.config.ts), [locale initialization](../../frontend/src/i18n/index.ts), and the production build output. Measure locale-on-demand loading, separate reader/media dependencies, and the cost of retaining legacy browser support. Set a measured asset budget after agreeing on browser targets.

### FRONT04 Ace loads runtime assets from an external CDN

**P2 — Source confirmed.** The editor's Ace base path points to jsDelivr. Modes, themes, or related assets requested through that path depend on external connectivity and the selected CSP. This also needs reconciliation with the product's external-resource setting.

Evidence: [editor configuration](../../frontend/src/views/files/Editor.vue), around lines 174–178. Bundle or explicitly self-host the required assets, and verify editing with network access disabled and with the intended CSP enabled.

## Go dependencies and migrations

### GO01 Establish a patched and consistent Go toolchain

**P1 — Audit reported and configuration confirmed.** The local toolchain is Go 1.26.1. `go.mod` declares 1.25.0 and CI selects the moving 1.26 series. The 34 reachable scan matches include many standard-library findings attributable to the local 1.26.1 compiler, including HTTP, TLS, certificate, URL, and HTML-template issues.

As of the review, current patched releases are Go 1.26.11 and 1.27.4. See the [official release history](https://go.dev/doc/devel/release). A minimum `go` directive does not establish which compiler built a deployment, and the moving CI selection may already build with a patched 1.26 release.

Align development, CI, release builds, and the documented support floor. Rescan using the chosen patched compiler and inspect actual release binaries before attributing the local standard-library findings to production.

### GO02 Update vulnerable compression dependencies

**P1 — Audit reported.** The scanner detected vulnerable symbols in two additional modules:

| Module                          | Current | Minimum fix reported | Available target |
| ------------------------------- | ------- | -------------------- | ---------------- |
| `github.com/andybalholm/brotli` | v1.2.2  | v1.2.7               | v1.3.0           |
| `github.com/klauspost/compress` | v1.19.1 | v1.20.2              | v1.20.4          |

References: [GO-2026-6888](https://pkg.go.dev/vuln/GO-2026-6888), excessive memory allocation in Brotli decompression; [GO-2026-6865](https://pkg.go.dev/vuln/GO-2026-6865), a compression panic. Static call traces establish potential reachability, not an end-to-end exploit through an exposed endpoint.

`golang.org/x/net` v0.57.0 also appears in affected HTTP/2 ranges; v0.60.0 contains fixes referenced by the scan and v0.62.0 is available. Its shared standard-library findings require checking which implementation is actually used. Rerun `govulncheck` after coordinated module and compiler updates.

### GO03 Other backend updates are available

**P2 — Registry checked.** `go list -m -u` found these direct-module updates:

| Module                           | Current | Available |
| -------------------------------- | ------- | --------- |
| `github.com/asticode/go-astisub` | v0.42.0 | v0.45.0   |
| `github.com/redis/go-redis/v9`   | v9.21.0 | v9.23.0   |
| `github.com/shirou/gopsutil/v4`  | v4.26.6 | v4.26.10  |
| `github.com/spf13/pflag`         | v1.0.10 | v1.1.0    |
| `github.com/stretchr/testify`    | v1.11.1 | v1.12.1   |
| `golang.org/x/crypto`            | v0.54.0 | v0.59.0   |
| `golang.org/x/image`             | v0.44.0 | v0.48.0   |
| `golang.org/x/text`              | v0.40.0 | v0.44.0   |

Additional transitive updates include `golang.org/x/sync` v0.25.0, `golang.org/x/sys` v0.50.0, and `go.yaml.in/yaml/v3` v3.0.6. Check Go requirements and behavior changes before selecting final targets. Update related `golang.org/x` modules together where appropriate.

Old dependencies without newer releases are not automatically vulnerabilities. Storm v3.2.1, imaging v1.6.2, the 2015 shlex snapshot, realip, and go-homedir merit a maintenance review, but this pass did not establish that replacing the database or image library is required. Prefer scoped replacements with a demonstrated benefit.

### GO04 Migrate the direct YAML import to the maintained project

**P2 — Source confirmed.** `cmd/utils.go` imports `gopkg.in/yaml.v3` v3.0.1. The module graph also includes the maintained `go.yaml.in/yaml/v3` fork. The old upstream was marked unmaintained; the YAML organization now maintains the replacement. See the [maintained package](https://pkg.go.dev/go.yaml.in/yaml/v3).

Move the direct import to the maintained v3 path as a small compatibility step, with configuration import/export regression coverage. Evaluate v4 separately because it has its own migration requirements. Preserve existing configuration semantics and error behavior.

## Correctness and data integrity findings

### BUG01 Invalid TUS upload length truncates an existing file

**P0 — Reproduced.** `tusPostHandler` opens an overwrite target with `O_TRUNC` before validating `Upload-Length`. An invalid length returns 400 after the existing file has already become empty. Missing or negative lengths follow the same late-validation path, although the saved probe specifically uses an invalid string.

Evidence: [TUS handlers](../../http/tus_handlers.go), lines 85–127. Probe: `TestReviewInvalidTusLengthPreservesExistingFile`.

Validate metadata before any mutation. Stage replacement data separately so invalid, canceled, or abandoned uploads preserve the previous file. Test all invalid-length forms and successful replacement.

### BUG02 Concurrent TUS chunks exceed the declared length

**P1 — Reproduced.** Two PATCH requests at offset zero both passed the length/offset checks before writing. Both returned 204 and appended five bytes, leaving ten bytes on disk for a five-byte upload. There is no serialization around checking the current size, appending, and marking completion.

Evidence: [TUS handlers](../../http/tus_handlers.go), `tusPatchUpload`, lines 221–290; [upload cache interface](../../http/upload_cache_memory.go). Probe: `TestReviewConcurrentTusEnforcesLength`. Upstream: [GHSA-4r8p-gqj2-mwgm](https://github.com/filebrowser/filebrowser/security/advisories/GHSA-4r8p-gqj2-mwgm).

Serialize updates per upload, including retries and completion hooks. If Redis-backed multi-instance operation remains supported, a process-local mutex alone is insufficient. Verify concurrent writers against shared storage and avoid firing completion more than once.

### BUG03 Oversized chunk rollback fails on Windows

**P1 — Reproduced by existing tests.** The handler opens the target in append mode, copies up to one byte beyond the remaining length, then truncates on overflow. On Windows the truncation failed with Access is denied, returning 500 instead of 413 and failing to perform the intended rollback.

Evidence: [TUS handlers](../../http/tus_handlers.go), lines 245–276. Failing tests: `TestTusOverLengthPatchKeepsConnectionUsable` and `TestTusPatchEnforcesUploadLength`.

Use a cross-platform write/rollback strategy, ideally one that does not write excess bytes into the final target. Verify content, size, offset, HTTP status, and connection reuse after rejection.

### BUG04 Editor route navigation can continue after a failed save

**P1 — Source confirmed; browser reproduction pending.** The route-update prompt's save action calls `await save()` and then `next()`. `save` catches API errors and only rethrows when passed `true`. The route guard therefore continues after a rejected save. The explicit close prompt already calls `save(true)` and has different behavior.

Evidence: [editor](../../frontend/src/views/files/Editor.vue), lines 214–216, 269–280, and 307–312.

Use one failure contract for all save-and-leave paths. Keep the editor open with its buffer and dirty state when saving fails. Test navigation with a failed request and ensure retry still works.

### BUG05 Upload cache backends differ in cleanup and failure behavior

**P2 — Source confirmed; Redis integration testing pending.** The in-memory cache removes abandoned partial files on expiry. The Redis backend ignores the removal callback and only expires the key, leaving the partial file. Its Register/Complete/Touch operations log errors without reporting them to the upload handler. A failed registration can therefore still be followed by a successful upload-creation response, while later requests cannot find the upload.

Evidence: [memory upload cache](../../http/upload_cache_memory.go) and [Redis upload cache](../../http/upload_cache_redis.go).

Define restart, expiry, outage, and garbage-collection behavior for both backends. Propagate failures where the response depends on persisted upload state. Keep cleanup bound to the correct upload generation and target so stale cleanup cannot delete unrelated replacement content.

### BUG06 Long logout timers may survive renewal

**P2 — Investigation.** `setSafeTimeout` splits delays above 24 hours into chained timers but returns only the first timer ID. After the first interval has elapsed, clearing that original ID cannot cancel the later scheduled timer. Long token lifetimes could therefore leave an old logout callback active after token renewal.

Evidence: [API utilities](../../frontend/src/api/utils.ts), `setSafeTimeout`; [authentication](../../frontend/src/utils/auth.ts), timer cancellation in `parseToken`. The default two-hour lifetime does not enter the chained path. Add a fake-clock reproduction before changing the timer API.

## CI and deployment findings

### CI01 Windows failures are invisible to the Linux only test job

**P2 — Reproduced and configuration confirmed.** `go test ./...` failed in four named tests:

| Test                                               | Observed failure                                              | Interpretation                                                                                                  |
| -------------------------------------------------- | ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `TestScopedFs/filesystem_root_scope_allows_access` | Invalid Windows path containing a drive component beneath `/` | POSIX-root assumption in the regression test; verify Windows root semantics separately                          |
| `TestRawArchiveDoesNotManufactureTraversal`        | Archive has no entries                                        | A name legal as a single POSIX filename is interpreted as path separators on Windows; platform-specific fixture |
| `TestTusOverLengthPatchKeepsConnectionUsable`      | 500 with truncate Access is denied                            | Runtime rollback defect, BUG03                                                                                  |
| `TestTusPatchEnforcesUploadLength`                 | Expected 413, got 500                                         | Runtime rollback defect, BUG03                                                                                  |

Evidence: [CI workflow](../../.github/workflows/ci.yaml), [filesystem test](../../files/file_test.go), [archive test](../../http/raw_test.go), [TUS multi-chunk tests](../../http/tus_multichunk_test.go), and [TUS length test](../../http/tus_upload_length_test.go).

Add Windows coverage and keep race-enabled tests on an appropriate supported runner. Platform-specific fixtures should explicitly target their intended platform, while genuine runtime differences need fixes rather than skips. Run symlink tests in at least one environment that permits link creation.

### CI02 Release does not depend on frontend tests

**P1 — Source confirmed.** The release job depends on `lint-frontend`, `lint-backend`, `test`, and `build`, but omits `test-frontend`. A failing frontend test job can coexist with a release job that is otherwise eligible to run.

Evidence: [CI workflow](../../.github/workflows/ci.yaml), line 94. Include frontend tests in release prerequisites and verify the complete workflow graph.

### CI03 Automated dependency security checks are missing

**P2 — Source confirmed.** No dependency-update configuration or pnpm/govulncheck audit jobs were present in the inspected workflows. Actions are selected with mutable version tags rather than commit hashes, and the main CI workflow does not declare an explicit minimal token-permission baseline.

Evidence: [.github configuration](../../.github) and [CI workflow](../../.github/workflows/ci.yaml). Add dependency update automation, scheduled/admission security scans, least-privilege workflow permissions, and a maintainable action-pinning policy. Scope failures and exceptions so noisy non-applicable advisories do not silently disable useful checks.

### OPS01 Compose defaults need production hardening

**P2 — Source confirmed.** The example exposes the HTTP port on all interfaces, selects mutable `latest` application/Redis images, and hard-codes a Redis password with unrestricted commands and keys. It should not be presented as a hardened deployment. No running container configuration was examined.

Evidence: [Compose example](../../compose.yaml). Pin intended image versions, provide credentials outside the tracked example, restrict Redis privileges, and document proxy/TLS/authentication requirements. Verify the intended served-data mount and database/config persistence; the example mounts `/flux/vault`, while the runtime image declares `/srv`, `/config`, and `/database` volumes.

### OPS02 New local history has no release metadata

**P2 — Source confirmed after Git initialization.** The initial absence of Git tracking is resolved by the local baseline. However, `Taskfile.yml` derives VERSION from a matching `v*` Git tag, and no such tag exists in this new history. The normal metadata-producing build path needs a source-snapshot version fallback or an intentional future version/tag policy. A release tag was not invented to work around this.

Evidence: [Taskfile](../../Taskfile.yml), `build:backend`. Before adding a remote, also align the CI push-branch filter (`master`) with the local branch (`main`) and review inherited repository, registry, ownership, and release settings.

## Proposed implementation goals

| Goal                                                   | Scope                            | Completion evidence                                                                                                                                               |
| ------------------------------------------------------ | -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| G1 Preserve files during failed and concurrent uploads | SEC01, BUG01–BUG03, BUG05        | Rejected/canceled requests preserve originals; concurrent chunks cannot corrupt offsets or length; Windows/Linux tests pass; Redis behavior is defined and tested |
| G2 Close application security gaps                     | SEC02, SEC03, SEC05–SEC07, SEC09 | Disabled commands rejected before upgrade; bounded work; tested SPA CSP; symlink authorization and confinement tests pass                                         |
| G3 Implement revocable sessions                        | SEC04, SEC08, BUG06              | Logout and password changes invalidate intended sessions; refresh replay is rejected; credentials and CSRF handling are tested; timer behavior is verified        |
| G4 Update frontend dependencies and assets             | FRONT01–FRONT04                  | Advisories resolved or justified individually; frozen install, lint, types, tests, and build pass; editor works offline; asset costs are measured                 |
| G5 Update Go dependencies and toolchain                | GO01–GO04                        | Supported patched compiler used consistently; module updates and YAML migration tested; vulnerability scan reviewed for actual build targets                      |
| G6 Strengthen regression tests and delivery            | BUG04, CI01–CI03, OPS01–OPS02    | Failed editor saves block navigation; platform CI and release gates work; deployment and local build/version behavior are documented                              |

Start with G1, then security dependency updates from G4/G5 alongside G2. Keep the session redesign and larger frontend migrations in separate changes so their behavior can be reviewed and tested independently. Establish Windows coverage and release gating early enough to protect subsequent work.

## Reproduction and follow-up commands

The saved [review probe source](reproduction/http_review_test.go.txt) uses the existing HTTP test helpers. Its `.go.txt` extension prevents normal Go package discovery from treating it as a separate test package. The [PowerShell runner](reproduction/run-go-review.ps1) maps it into `http/review_test.go` with a temporary Go overlay. The probes only manipulate temporary test directories and databases; they do not write a test file into the application source directory.

From the repository root:

```powershell
pwsh -NoProfile -File docs/reviews/reproduction/run-go-review.ps1
```

At the baseline, expect failures for directory cleanup, invalid TUS length, concurrent TUS length, and HTML CSP. The symlink probe is expected to fail when it can create the link; it skips when the OS refuses link creation. A nonzero exit status is intentional until the corresponding fixes are implemented.

Standard checks for later implementation:

```text
go test ./...
go vet ./...
go test --race ./...  # on a runner with race-detector prerequisites
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

cd frontend
pnpm install --frozen-lockfile
pnpm run lint
pnpm run typecheck
pnpm test
pnpm run build
pnpm audit
pnpm audit --prod
pnpm outdated
```

The dependency snapshots describe registry data on 16 November 2026. Recheck advisories, release notes, peer dependencies, engine requirements, and available patch versions when each goal begins. A passing build is not a substitute for the behavioral regression tests associated with each finding.
