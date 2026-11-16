# Project Review Fix Status

Started: 16 November 2026.

All findings from the [project review](2026-11-16-project-review.md) are accepted as to fix. The active goal is to resolve all 28 findings, keep validation evidence and local commits, and deliver a full completion report. No remote is configured.

Status values: **To fix**, **In progress**, **Fixed**. A finding is fixed only after its implementation or verified disposition and appropriate checks are recorded.

| Finding | Work                                                              | Status      | Evidence                                                                                                                                                     |
| ------- | ----------------------------------------------------------------- | ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| SEC01   | Failed upload cleanup can delete existing directories             | Fixed       | Permanent upload integrity tests and existing TUS tests pass on Windows; atomic sibling writes preserve interrupted replacements.                            |
| SEC02   | Command WebSocket reads without a size limit before authorization | Fixed       | Authorization precedes upgrade; 16 KiB messages, read/write deadlines, bounded command lifetime/output, and command boundary tests pass.                     |
| SEC03   | Content Security Policy does not reach the application HTML       | To fix      | Pending                                                                                                                                                      |
| SEC04   | Sessions lack server side revocation                              | To fix      | Pending                                                                                                                                                      |
| SEC05   | In scope symlink aliases can bypass path deny rules               | To fix      | Pending                                                                                                                                                      |
| SEC06   | Filesystem confinement checks and operations are separate         | To fix      | Pending                                                                                                                                                      |
| SEC07   | Subtitle conversion has unbounded memory use                      | Fixed       | Subtitle input/output and concurrent conversions are bounded; oversized-input and string-writer limit regressions pass.                                      |
| SEC08   | Browser credentials need stronger handling                        | To fix      | Pending                                                                                                                                                      |
| SEC09   | Request abuse controls need defined limits                        | To fix      | Pending                                                                                                                                                      |
| FRONT01 | Resolve dependency advisories across the full tree                | To fix      | Pending                                                                                                                                                      |
| FRONT02 | Separate routine updates from major migrations                    | To fix      | Pending                                                                                                                                                      |
| FRONT03 | Large bundles and old browser data increase asset costs           | To fix      | Pending                                                                                                                                                      |
| FRONT04 | Ace loads runtime assets from an external CDN                     | To fix      | Pending                                                                                                                                                      |
| GO01    | Establish a patched and consistent Go toolchain                   | To fix      | Pending                                                                                                                                                      |
| GO02    | Update vulnerable compression dependencies                        | To fix      | Pending                                                                                                                                                      |
| GO03    | Other backend updates are available                               | To fix      | Pending                                                                                                                                                      |
| GO04    | Migrate the direct YAML import to the maintained project          | To fix      | Pending                                                                                                                                                      |
| BUG01   | Invalid TUS upload length truncates an existing file              | Fixed       | Permanent upload integrity tests and existing TUS tests pass on Windows; atomic sibling writes preserve interrupted replacements.                            |
| BUG02   | Concurrent TUS chunks exceed the declared length                  | Fixed       | Private generation staging, cross-process OS locks, Redis CAS state, expiry/restart cleanup, and two-replica tests; Windows suite and Linux race suite pass. |
| BUG03   | Oversized chunk rollback fails on Windows                         | Fixed       | Permanent upload integrity tests and existing TUS tests pass on Windows; atomic sibling writes preserve interrupted replacements.                            |
| BUG04   | Editor route navigation can continue after a failed save          | Fixed       | Actual editor setup regression verifies failed save blocks navigation, preserves the buffer, and permits retry; frontend tests pass.                         |
| BUG05   | Upload cache backends differ in cleanup and failure behavior      | Fixed       | Private generation staging, cross-process OS locks, Redis CAS state, expiry/restart cleanup, and two-replica tests; Windows suite and Linux race suite pass. |
| BUG06   | Long logout timers may survive renewal                            | Fixed       | Cancelable timer follows every scheduled segment; fake-clock renewal tests pass beyond 24 hours.                                                             |
| CI01    | Windows failures are invisible to the Linux only test job         | In progress | Windows defects and POSIX-only fixtures corrected; both platform suites pass. CI matrix follows.                                                             |
| CI02    | Release does not depend on frontend tests                         | To fix      | Pending                                                                                                                                                      |
| CI03    | Automated dependency security checks are missing                  | To fix      | Pending                                                                                                                                                      |
| OPS01   | Compose defaults need production hardening                        | To fix      | Pending                                                                                                                                                      |
| OPS02   | New local history has no release metadata                         | To fix      | Pending                                                                                                                                                      |

## Work order

1. Protect existing files and serialize uploads, including cleanup and backend failures.
2. Apply bounded request handling, filesystem confinement, and a functional HTML CSP.
3. Implement revocable sessions and secure credential transport.
4. Update frontend/backend dependencies and assets with compatibility checks.
5. Complete editor, CI, deployment, and build metadata fixes.
6. Run the final checks, record before/after results, and write the completion report.
