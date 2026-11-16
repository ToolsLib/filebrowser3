# Project Review Fix Status

Started: 16 November 2026.

All 28 findings from the [project review](2026-11-16-project-review.md) were accepted as to fix and are now resolved. Validation evidence and local commits are recorded below. No remote or release tag is configured. See the [completion report](2026-11-16-completion-report.md) for results, migration notes and verification limits.

Status values: **To fix**, **In progress**, **Fixed**. A finding is fixed only after its implementation or verified disposition and appropriate checks are recorded.

| Finding | Work                                                              | Status | Evidence                                                                                                                                                                                   |
| ------- | ----------------------------------------------------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| SEC01   | Failed upload cleanup can delete existing directories             | Fixed  | Permanent upload integrity tests and existing TUS tests pass on Windows; atomic sibling writes preserve interrupted replacements.                                                          |
| SEC02   | Command WebSocket reads without a size limit before authorization | Fixed  | Authorization precedes upgrade; 16 KiB messages, read/write deadlines, bounded command lifetime/output, and command boundary tests pass.                                                   |
| SEC03   | Content Security Policy does not reach the application HTML       | Fixed  | Outer-router nonce policy, nested-base HTML/static regression and production Chromium login/editor/CSV/image/EPUB/video checks pass with no CSP violations.                                |
| SEC04   | Sessions lack server side revocation                              | Fixed  | Persistent bounded sessions; absolute lifetime, atomic refresh rotation/replay revocation, logout/all and API/CLI password invalidation tested; topology documented in sessions.md.        |
| SEC05   | In scope symlink aliases can bypass path deny rules               | Fixed  | Resolved alias targets and lexical paths both undergo deny rules; read/write/public-share regressions and Linux race suite pass.                                                           |
| SEC06   | Filesystem confinement checks and operations are separate         | Fixed  | Native filesystem operations use os.Root directory handles; concurrent symlink-swap regression, virtual-adapter guard, Windows suite and Linux race suite pass.                            |
| SEC07   | Subtitle conversion has unbounded memory use                      | Fixed  | Subtitle input/output and concurrent conversions are bounded; oversized-input and string-writer limit regressions pass.                                                                    |
| SEC08   | Browser credentials need stronger handling                        | Fixed  | Memory-only access tokens; server-set Secure/HttpOnly path-scoped cookies, explicit loopback opt-in and CSRF checks; production browser reload and logout-all pass.                        |
| SEC09   | Request abuse controls need defined limits                        | Fixed  | Peer throttling, bounded API/auth/work concurrency, metadata/upload caps, streaming idle deadlines, search and CAPTCHA bounds; permanent regressions and Windows suite pass.               |
| FRONT01 | Resolve dependency advisories across the full tree                | Fixed  | Direct upgrades plus tested transitive overrides; full-tree pnpm audit reports zero vulnerabilities (saved final snapshot).                                                                |
| FRONT02 | Separate routine updates from major migrations                    | Fixed  | VueUse 15, Pinia 4, CSV 7, TypeScript 6 and Vitest 5 tested; incompatible TS 7/Video.js 10 disposition documented in frontend-maintenance.md.                                              |
| FRONT03 | Large bundles and old browser data increase asset costs           | Fixed  | Lazy routes/translations/preview tools; main chunk about 209 KB versus 792 KB plus 829 KB translation baseline; modern browser baseline documented.                                        |
| FRONT04 | Ace loads runtime assets from an external CDN                     | Fixed  | Locked Ace modes/themes/workers emitted locally; production editor save/reload works with no external script requests.                                                                     |
| GO01    | Establish a patched and consistent Go toolchain                   | Fixed  | Go minimum 1.26.11/preferred 1.27.4 aligned in module, CI, container and docs; both versions tested/scanned on Windows and Linux with zero findings.                                       |
| GO02    | Update vulnerable compression dependencies                        | Fixed  | Brotli 1.3.0 and compress 1.20.4; Windows tests/vet and govulncheck report no vulnerabilities.                                                                                             |
| GO03    | Other backend updates are available                               | Fixed  | Backend dependency graph updated; full Windows and Linux race suites pass; govulncheck reports no vulnerabilities.                                                                         |
| GO04    | Migrate the direct YAML import to the maintained project          | Fixed  | Direct YAML import uses go.yaml.in/yaml/v3 3.0.6; full tests and vet pass.                                                                                                                 |
| BUG01   | Invalid TUS upload length truncates an existing file              | Fixed  | Permanent upload integrity tests and existing TUS tests pass on Windows; atomic sibling writes preserve interrupted replacements.                                                          |
| BUG02   | Concurrent TUS chunks exceed the declared length                  | Fixed  | Private generation staging, cross-process OS locks, Redis CAS state, expiry/restart cleanup, and two-replica tests; Windows suite and Linux race suite pass.                               |
| BUG03   | Oversized chunk rollback fails on Windows                         | Fixed  | Permanent upload integrity tests and existing TUS tests pass on Windows; atomic sibling writes preserve interrupted replacements.                                                          |
| BUG04   | Editor route navigation can continue after a failed save          | Fixed  | Actual editor setup regression verifies failed save blocks navigation, preserves the buffer, and permits retry; frontend tests pass.                                                       |
| BUG05   | Upload cache backends differ in cleanup and failure behavior      | Fixed  | Private generation staging, cross-process OS locks, Redis CAS state, expiry/restart cleanup, and two-replica tests; Windows suite and Linux race suite pass.                               |
| BUG06   | Long logout timers may survive renewal                            | Fixed  | Cancelable timer follows every scheduled segment; fake-clock renewal tests pass beyond 24 hours.                                                                                           |
| CI01    | Windows failures are invisible to the Linux only test job         | Fixed  | Windows/Linux matrix tests both Go versions; local Windows suites and explicit Linux race suites pass on both versions; POSIX symlink cases exercised on Linux.                            |
| CI02    | Release does not depend on frontend tests                         | Fixed  | Release requires frontend unit/browser checks, backend lint, platform tests, Redis integration, security scans and build; workflow passes actionlint.                                      |
| CI03    | Automated dependency security checks are missing                  | Fixed  | Full-SHA actions, read-only default token, release-only contents write, pinned tools, full-tree audit/govulncheck and weekly Dependabot configuration; actionlint passes.                  |
| OPS01   | Compose defaults need production hardening                        | Fixed  | Digest-pinned local-source build, generated secret files, non-root/read-only/capability limits, private Redis ACLs and persistent volumes; health, TUS, ACL and restart smoke checks pass. |
| OPS02   | New local history has no release metadata                         | Fixed  | Portable untagged build helper emits dev+commit metadata; CI follows main; no remote/tags created; release targets this fork archives rather than inherited upstream registries.           |

## Completed work

1. Protect existing files and serialize uploads, including cleanup and backend failures.
2. Apply bounded request handling, filesystem confinement, and a functional HTML CSP.
3. Implement revocable sessions and secure credential transport.
4. Update frontend/backend dependencies and assets with compatibility checks.
5. Complete editor, CI, deployment, and build metadata fixes.
6. Run the final checks, record before/after results, and write the completion report.
