# Project review completion report

Completed: 16 November 2026, Europe/Paris.

Docker follow-up: the current setup now retains only `Dockerfile.source`, defaults to app-only Compose, and enables Redis through `compose.redis.yaml`. Both setups have automated runtime tests. See the [current deployment instructions](../deployment.md); the report below records the original remediation snapshot.

All **28 findings** in the [original review](2026-11-16-project-review.md) are resolved, with implementation, compatibility decisions and validation recorded in [fix status](fix-status.md). The work is committed on `fix/project-review`. The original import remains available on `main` at `d810b7c`. No remote, release tag or public deployment was created.

## Results compared with the baseline

| Measure                   | Original review                                                    | Completed work                                                                                    |
| ------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------- |
| Frontend dependency audit | 80 findings: 1 critical, 48 high, 22 moderate, 9 low               | **Zero**, across the full resolved tree, including development dependencies                       |
| Production frontend audit | 48 findings                                                        | Full-tree audit is clean; no production findings remain                                           |
| Go vulnerability scan     | 34 reachable vulnerabilities, plus imported/required findings      | **No vulnerabilities found** on Windows and Linux, with both Go 1.26.11 and 1.27.4                |
| Windows Go suite          | Failing fixtures and real TUS rollback failures                    | Passing on both supported toolchains                                                              |
| Linux Go suite            | Platform gaps were not covered locally                             | Passing with the race detector on both supported toolchains                                       |
| Frontend tests            | 29 tests                                                           | **40 passing tests in 9 files**, plus a production Chromium integration scenario                  |
| Application assets        | Main 792.10 KB; eager translations 828.86 KB                       | Main **209.03 KB / 71.53 KB gzip**; non-English languages and larger tools load on demand         |
| Editor runtime assets     | External Ace CDN                                                   | Locked, locally emitted modes, themes and workers                                                 |
| Sessions                  | Stateless, renewable JWTs; client-only logout                      | Persistent revocation, rotating refresh secrets, replay rejection and absolute expiration         |
| Default Compose           | Upstream latest images, shared hardcoded password, broad Redis ACL | Locally built source, digest pins, generated secrets, restricted users/ACLs and loopback exposure |

Asset figures describe individual build chunks, not total page transfer. Video.js remains an optional large chunk (654.55 KB / 188.00 KB gzip); its build warning remains visible. It loads when opening media. Historical audit evidence is retained alongside the [final frontend audit](2026-11-16-frontend-audit-final.json).

## Findings closed

| ID      | Result                                                                                                                                                                                       |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SEC01   | Failed writes clean up private staging only; existing files and directories survive rejected or interrupted uploads.                                                                         |
| SEC02   | Command authorization occurs before WebSocket upgrade/read. Message size, read/write deadlines, execution lifetime and output-line size are bounded.                                         |
| SEC03   | CSP wraps the router, covers SPA fallbacks, and uses a unique HTML nonce. Production login, editor, previews and media work under the policy.                                                |
| SEC04   | Durable sessions support logout, logout-all, credential/key invalidation, transactional refresh rotation, replay revocation and a 24-hour absolute lifetime.                                 |
| SEC05   | Rules check requested and canonical target paths, including canonical children of symlink-rooted public shares and archive downloads.                                                        |
| SEC06   | Native scoped operations use traversal-resistant `os.Root` handles; concurrent symlink swapping cannot escape the scope in the regression test. Unsupported filesystem adapters fail closed. |
| SEC07   | Subtitle conversion has input/output and concurrency limits; oversized input is rejected before conversion.                                                                                  |
| SEC08   | Access JWTs stay in memory. Server-set cookies are Secure/HttpOnly, host-only and path scoped; refresh/logout operations have explicit CSRF checks.                                          |
| SEC09   | Peer throttling, bounded concurrency, metadata/upload limits, streaming idle deadlines, search bounds and outbound CAPTCHA limits are implemented.                                           |
| FRONT01 | Full-tree advisories were removed through compatible direct upgrades and targeted transitive overrides.                                                                                      |
| FRONT02 | Major migrations were tested; unsupported compiler/player major combinations have explicit compatibility decisions.                                                                          |
| FRONT03 | Routes, messages, date locales and preview tools load on demand; obsolete legacy output was removed and browser support is documented.                                                       |
| FRONT04 | Ace runtime assets are served from this application's static path; the editor requires no CDN.                                                                                               |
| GO01    | Patched minimum/preferred Go versions are consistent across the module, CI, source container and contributor documentation.                                                                  |
| GO02    | Vulnerable Brotli/compress versions were upgraded; both compiler lines scan clean.                                                                                                           |
| GO03    | The direct-module updates identified in the review were applied and verified with platform tests, lint and scanning.                                                                         |
| GO04    | The direct YAML dependency uses maintained `go.yaml.in/yaml/v3`.                                                                                                                             |
| BUG01   | Invalid TUS lengths are rejected before the destination is touched.                                                                                                                          |
| BUG02   | Generation-specific staging, state comparisons and OS advisory locks serialize chunks across requests/processes.                                                                             |
| BUG03   | Oversized-chunk rollback works on Windows; upload handles use modes compatible with truncation.                                                                                              |
| BUG04   | Failed editor saves retain the buffer and block route/close navigation; a later successful retry can proceed.                                                                                |
| BUG05   | Memory and Redis cache lifecycle/error behavior is defined, tested and restricted to private staging cleanup.                                                                                |
| BUG06   | Canceling a logout timer cancels its current long-delay segment, including after renewal.                                                                                                    |
| CI01    | Windows/Linux tests and vulnerability scans cover both supported Go versions; Linux runs race checks.                                                                                        |
| CI02    | Release depends on frontend/browser checks, backend lint, platform tests, Redis integration, security scans and build validation.                                                            |
| CI03    | Actions use full commit pins, tools have exact versions, default permissions are read-only, and weekly Dependabot plus security checks are configured.                                       |
| OPS01   | Hardened Compose source build, persistent data/config/database volumes, generated file secrets, isolated Redis and least-privilege runtime settings were smoke tested.                       |
| OPS02   | Untagged local builds produce development version/commit metadata; CI tracks `main`; inherited upstream registry/tap publishing was removed.                                                 |

Final review also corrected three related cases: a null user-request data object now returns an error instead of panicking; a symlink used as a public share root preserves canonical child rules; and TUS requests use renewed session credentials on subsequent chunks and process renewal hints. The browser scenario includes a real upload through the file-selection UI.

## Implementation details and compatibility decisions

Uploads now stage replacements beside the destination, sync and close them, and publish only when complete. Existing destinations stay readable during TUS transfers. Upload URLs carry generation IDs and enforce owner identity. Redis state operations compare generations atomically; shared-filesystem advisory locks protect actual chunk writes. Expiry and outages preserve uncertain data. A dry-run-first `cleanup-uploads` command handles orphaned journals without trusting journal destination paths. See [upload integrity and recovery](../uploads.md).

Session records live in the existing Bolt database. Password, authentication-method and signing-key fingerprints are checked on protected requests. Refresh secrets are hashed in storage, consumed atomically and bounded by the original absolute lifetime. The browser serializes refresh calls across tabs with Web Locks. Native clients retain header authentication through a documented access/refresh protocol. See [sessions](../sessions.md).

The frontend retains pnpm and its single lockfile. VueUse 15, Pinia 4, CSV Parse 7, TypeScript 6 and Vitest 5 were migrated alongside routine Vue, Router, Vite, i18n, DOMPurify and tooling updates. TypeScript 7 is outside Vue/type-eslint support; Video.js 10 conflicts with the mobile UI plugin. Supported TypeScript 6.0.3 and Video.js 8.24.2 are used. Node definitions remain on the Node 24 line; KaTeX remains on the extension-compatible 0.18 line. Rollup, esbuild, xmldom and KaTeX overrides remove remaining inherited advisories and are exercised by production builds/browser tests. See [frontend maintenance](../frontend-maintenance.md).

Go 1.26.11 is the compatibility floor and Go 1.27.4 is preferred. Backend updates include astisub, Redis, gopsutil, pflag, testify, Go x libraries and compression packages. Existing account/database formats remain usable; session state adds its own database bucket. The maintained YAML import replaces the direct legacy import.

Compose builds this checkout instead of running upstream releases. The app uses UID 1000, a read-only root filesystem, no capabilities and no-new-privileges. Redis has no host port, disables its default user and grants its application user only upload-keyspace/cache commands. Named volumes persist files, configuration, database and Redis AOF data. A secret-preparation helper generates independent random credentials, stores only the initial admin password hash, and refuses overwrites. See [deployment](../deployment.md) and [request limits](../request-limits.md).

## Verification performed

| Check                   | Result                                                                                                                                                                                                                 |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Windows, Go 1.26.11     | Full Go suite passed; govulncheck v1.9.0 reported no vulnerabilities.                                                                                                                                                  |
| Windows, Go 1.27.4      | Full Go suite including actual Redis upload tests passed; vet, golangci-lint v2.15.0 and govulncheck passed with zero issues/findings.                                                                                 |
| Linux, Go 1.26.11       | Explicit local-toolchain full race suite passed; vulnerability scan clean.                                                                                                                                             |
| Linux, Go 1.27.4        | Explicit preferred-toolchain full race suite, vet and vulnerability scan passed.                                                                                                                                       |
| Filesystem regressions  | Linux exercised symlink aliases, scope escape, concurrent swaps, public shares and archive filtering; Windows exercised its native path/rollback cases.                                                                |
| Shared upload state     | Actual Redis server, independent cache instances, cross-process locks, expiry/outage/cleanup and restart recovery exercised.                                                                                           |
| Frontend                | Frozen install, strict zero-warning lint, typecheck, 40 tests and production build passed; full-tree audit clean.                                                                                                      |
| Production browser      | Nested base path; login, HttpOnly cookie privacy, reload/refresh, UI TUS upload, local Ace, editor save/reload, CSV, image, EPUB, real WebM playback and logout-all passed without external scripts or CSP violations. |
| Other release platforms | macOS, FreeBSD and OpenBSD amd64 cross-builds passed.                                                                                                                                                                  |
| CI configuration        | actionlint v1.7.12 passed; workflow formatting and release prerequisite graph reviewed.                                                                                                                                |
| Release configuration   | Parsed and validated against GoReleaser v2.20.2 JSON schema. Full SCM release checking/publishing awaits an operator-configured remote.                                                                                |
| Compose                 | Fresh source build and final rebuild passed; health, UID/read-only/capability settings, loopback binding, Redis ACL denials and authenticated TUS commit passed.                                                       |
| Persistence             | Restarting both Compose services preserved a session and resumed a partial TUS upload from offset 3 to the correct completed file.                                                                                     |
| Local metadata          | A repository with no tags built and printed `0.0.0-dev+<commit>.dirty`; clean builds omit the dirty suffix. No remote/tag was needed.                                                                                  |
| Secret helper           | Generated credentials in an isolated test directory and rejected an overwrite attempt.                                                                                                                                 |

The temporary Redis and Compose fixtures are removed after verification. Dependency/build caches and ignored local build artifacts may remain for future development. No production secrets were created or committed.

## Upgrade and operating boundaries

1. Existing stateless sessions must sign in again. Header clients must migrate renewal to the separate refresh credential; access JWTs alone cannot renew.
2. Existing pending legacy TUS URLs cannot resume. Use the generation-specific Location returned by new upload creation. Follow the dry-run recovery procedure for old staging files.
3. Browser deployment requires HTTPS with Secure cookies. The explicit insecure-cookie option is limited to loopback HTTP development. Production proxies should preserve Host.
4. Authentication supports one server process per Bolt database. Redis upload coordination does not make copied user/session databases consistent. Cross-process cache/lock tests establish component behavior, not a supported replicated identity service.
5. Command execution/hooks remain a trusted shell-equivalent feature and stay disabled by default. External symlink following remains an unsafe explicit opt-in. Root directories and ancestors must be operator controlled.
6. Default upload size is 10 GiB and configurable. Metadata, concurrency, peer-rate and streaming idle limits are documented. Behind a proxy, application peer throttling groups that proxy's traffic; add trusted edge controls where appropriate.
7. Browser support follows Vite's modern baseline: Chrome/Edge 111+, Firefox 114+, Safari 16.4+. Legacy browser output is no longer shipped.

These results cover the tested configurations and reported findings. Hosted GitHub Actions, live publication, a real reCAPTCHA account, physical BSD/macOS execution and every possible storage backend were not exercised. Windows symlink creation was unavailable locally; the corresponding symlink cases ran on Linux. Dependency scans are clean as of the review date and must continue in CI; this is not a claim that unknown vulnerabilities cannot exist.

## Local history

Baseline: `d810b7c`. Original review: `6aeab0b`. Acceptance of all findings: `4e2063b`.

| Commit    | Work                                                                        |
| --------- | --------------------------------------------------------------------------- |
| `52b45b8` | Preserve files on rejected/interrupted direct uploads.                      |
| `30ce98b` | Stage TUS replacements, coordinate generations and define cleanup/recovery. |
| `67afef7` | Bound commands/subtitles; fix editor navigation and logout timers.          |
| `3374eed` | Patch Go toolchains/dependencies and migrate YAML.                          |
| `cc6e83c` | Native filesystem confinement and alias authorization.                      |
| `3bd6a8e` | Frontend migrations, lazy/local assets, CSP and browser tests.              |
| `91686ce` | Persistent sessions, secure credential transport and resource limits.       |
| `c4d0b31` | CI, local deployment, build metadata and final regression edge cases.       |

The completion report is a subsequent documentation commit. No upstream history was fabricated, and no release tag or remote was added.
