# Command console HTTP streaming migration

Implementation and local validation completed on 24 November 2026, based on `f7f943aeec9ecf34faa91a67ae8ea8af5eb80a7b`. The results below record validation before submission. No release, tag or published Docker image was changed.

## Result

The command console uses authenticated HTTP POST and NDJSON output instead of WebSockets. `github.com/gorilla/websocket` is absent from the application, tests, module graph, checksums and rebuilt executable. No replacement transport dependency was added. The unrelated `github.com/gorilla/mux` HTTP router remains. This resolves the Gorilla masking advisory by removing the affected dependency; the [advisory investigation](2026-11-24-websocket-mask-advisory.md) preserves the original applicability assessment and upstream proxy-test diagnosis.

The server checks authentication, execution permission, the existing allowlist and a 16 KiB body limit before starting the process. Existing shell parsing, working-directory selection and disabled-by-default execution remain. Combined stdout/stderr streams in bounded chunks, including partial lines and split UTF-8 sequences. Records distinguish start, output, heartbeat, errors and exit codes. The frontend detects interrupted/invalid/oversized streams and never retries a command automatically.

The console provides Cancel and Escape cancellation. Closing it, navigating away or unmounting it aborts the request. Cancellation, timeout and failed response writes/flushes stop the command. Unix uses a separate process group; Windows starts the process suspended, assigns it to a kill-on-close Job Object, then resumes its initial thread. Ordinary descendants are cleaned up on cancellation and parent completion. The command is reaped before releasing the request's work slot. This is not a sandbox for commands that intentionally detach on Unix.

The stream flushes every record, sends heartbeats every 15 seconds, uses the shared one-minute write deadline and preserves the ten-minute command lifetime. Middleware now propagates flush errors through `ResponseController`; the old WebSocket/Hijacker bypass is removed. The frontend bounds retained output to about one million characters per command. The Vite development proxy also uses ordinary HTTP.

## Verification

| Check | Result |
| --- | --- |
| Windows, Go 1.27.4: `go test ./... -count=1 -timeout 120s` | Passed |
| Linux/amd64, Go 1.27.4: `go test -race ./... -count=1 -timeout 120s` | Passed in an isolated container with read-only source mount |
| `go vet ./...` | Passed on Windows |
| golangci-lint 2.15.0, Windows and Linux package targets | Zero issues |
| govulncheck 1.9.0, Windows and Linux package targets | No vulnerabilities found |
| Frontend unit suite | 14 files, 62 tests passed |
| Frontend lint, type checking and production build | Passed |
| Production Chromium browser suite | All 11 scenarios passed in one full run; a later run passed 10 and hit the previously observed share/login redirect flake, which passed its isolated rerun |
| Final command browser scenario | Passed, including incremental Unicode output, header authentication under a base URL, exit status, Cancel, Escape, close/navigation cancellation and descendant cleanup |
| Real Nginx and Traefik smoke test | Passed; details below |
| Dependency and binary inspection | Gorilla WebSocket absent; Gorilla mux retained |
| `git diff --check` | Passed |

Before submission, the change was rebased onto `6f32ea4`, including the dependency updates merged into `main`. A frozen-lockfile frontend install, lint, all 62 unit tests, type checking and the production build passed again. The rebuilt application's command browser scenario passed, including Escape cancellation. The complete browser rerun passed 10 of 11 scenarios and reproduced the same share/login redirect failure at `shares.spec.ts:126`; it remains unresolved and is not presented as an all-green run. The post-rebase evidence is saved in ignored `build/command-stream-rebase-*` logs.

Backend regressions additionally cover missing authentication, disabled execution, missing Execute permission, disallowed commands, malformed/empty/oversized requests, legacy GET rejection, cookie-only POST rejection, combined stdout/stderr, nonzero exits, UTF-8 across chunk boundaries, output arriving before completion through a Go reverse proxy, heartbeats after stdout closes, timeout, flush-error propagation, and process reaping on failed writes. Direct and proxied cancellation tests wait for a real descendant to start and verify it cannot subsequently write its marker file.

The real-proxy smoke test runs against the rebuilt application with disposable accounts/files/database:

- Nginx with `proxy_buffering on` honors `X-Accel-Buffering: no` and forwards output before completion. Client cancellation stops the command and descendant.
- Nginx with explicit `proxy_buffering off` also streams and propagates cancellation, even when configured to ignore the application header.
- Traefik 3.8.0 without buffering middleware streams and propagates cancellation.
- A separate Traefik route with Buffering middleware is a negative control: output arrives only after completion. Application headers cannot override that middleware.

The Nginx test image is pinned to `sha256:8eb2b5540eae779603cc3db84b3986a15ecb5308b54303146eee9837c137b51a`. Only uniquely named test containers were created/removed; existing app containers and data were not changed. Linux test-module/build caches are retained for reuse. Ignored logs are under `build/command-stream-*`; the repeat-browser failure and isolated share rerun are recorded separately. The share scenario's missing row was a login-page redirect after full-page navigation, matching the issue observed before this migration. No authentication or share code was changed to hide it.

## Deployment and limits

See [command execution](../command-execution.md#http-streaming-api) for the API, cancellation behavior, Nginx configuration, Traefik buffering requirements and the reproducible `node tools/testing/command-stream-proxy.mjs` check. The old command WebSocket API is removed: deploy backend and assets together, reload existing tabs and update custom clients to POST/NDJSON. Hooks and file APIs retain their current interfaces.

Response-buffering middleware or a CDN that holds the full response can delay output despite application flushes. A proxy that ignores downstream disconnects can delay backend cancellation. The tested unbuffered configurations avoid those problems. Deliberately detached Unix descendants remain outside process-group guarantees; execution remains a trusted-user feature.

During local validation, no remote CI was triggered, no application Docker image was rebuilt/published, and no Redis-backed upload deployment smoke test was rerun for this console transport change. The full backend suites, production frontend build/browser tests and actual proxy containers provide the local validation above. The existing optional vulnerability-check policy is unchanged.
