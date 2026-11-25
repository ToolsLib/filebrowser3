# Gorilla WebSocket masking advisory review

Reviewed locally on 24 November 2026, against `f7f943aeec9ecf34faa91a67ae8ea8af5eb80a7b` plus the proposed changes below. This review does not publish an application update.

Final local disposition: the initially tested pseudo-version update below has been superseded by [HTTP command streaming](../command-execution.md#http-streaming-api). The working tree removes `github.com/gorilla/websocket` from production, tests, `go.mod` and `go.sum`, rather than retaining that pin. The applicability and proxy-test diagnosis below describe the original dependency investigation.

## Finding and applicability

[GO-2026-6278](https://pkg.go.dev/vuln/GO-2026-6278), also identified as GHSA-w67g-5rqw-f597, concerns predictable WebSocket **client** masking keys generated with `math/rand`. Client masks must be unpredictable to protect intermediaries from attacker-controlled wire data; they are not encryption. See [RFC 6455, sections 5.3 and 10.3](https://www.rfc-editor.org/rfc/rfc6455.html).

The installed dependency, `github.com/gorilla/websocket v1.5.3`, contains the weak generator and falls within the Go advisory's affected range. The optional Go scan correctly reports the dependency, but its call traces through shared reader/writer interfaces and server methods do not establish that client masking executes in this application.

No production client-masking path was found:

- `http/commands.go` is the only production importer of Gorilla WebSocket, including the application's production dependency graph. It uses `websocket.Upgrader`, which constructs a server connection (`isServer=true`).
- Gorilla calls `newMaskKey` only for outgoing client frames (`!c.isServer`), including control frames. Server frames are unmasked.
- `frontend/src/api/commands.ts` uses the browser's native `window.WebSocket`; it does not use Gorilla's Go client implementation.
- Go client dialing appears only in `http/request_bounds_test.go`, against temporary loopback test servers.
- The command endpoint additionally requires both server-side command execution to be enabled and the user's Execute permission. Execution remains disabled by default. The server-only role, rather than this default alone, is the reason the affected masking branch is not reached.

Assessment: the dependency is affected, but the reviewed production usage does not exercise the vulnerable client-mask generator. There is no evidence from this advisory of exploitable client masking in File Browser 3. Reassess if a Go WebSocket client is introduced.

## Proposed local hardening

Pin the direct dependency to **`v1.5.4-0.20240701034025-d67f41855da4`**, the pseudo-version for the [official upstream fix](https://github.com/gorilla/websocket/commit/d67f41855da42d7bccd9ef050c49f7e54e783b95). That commit replaces the generator with `crypto/rand`. The version and checksums were resolved through the Go module proxy and checksum database. No replacement fork or vendored patch is used, and no other dependency versions change.

As of this review, `go list -m github.com/gorilla/websocket@latest` still resolves to the affected stable tag `v1.5.3`. The Go advisory explicitly covers v1.5.3 through versions before this fix. The linked fork advisory's claim that v1.5.3 is patched conflicts with the actual v1.5.3 source and Go database range; it was not used to select the version.

The fixed snapshot is not a new stable tag. Its [delta from v1.5.3](https://github.com/gorilla/websocket/compare/v1.5.3...d67f41855da42d7bccd9ef050c49f7e54e783b95) also includes client/proxy refactoring, response-controller hijacking, an HTTP 426 response for a missing Upgrade header, and subprotocol preference changes. Our upgrader has no configured subprotocols. The application handshake, permission checks, incoming-message bounds and outgoing replies are covered by the local tests. Prefer a suitable official stable tag when one becomes available, after normal compatibility checks.

Add a live loopback WebSocket regression that sends a masked client command and verifies the server's permission-denial text reply with an empty command allowlist. It executes no external process. Existing authorization-before-upgrade and oversized-message regressions remain in place.

## Validation and limits

Using Go 1.27.4 and govulncheck 1.9.0:

- `go mod tidy`: only the Gorilla version/checksum entries changed.
- `go test ./...`: passed on Windows, including the new live WebSocket regression.
- `go vet ./...`: passed on Windows.
- `golangci-lint run --timeout=5m` (v2.15.0): zero issues on Windows.
- `govulncheck ./...`: no vulnerabilities found for Windows/amd64 and Linux/amd64 package analysis.
- `go test github.com/gorilla/websocket -skip '^TestProxy(Authorization)?Dial$' -count=1`: passed on Windows.

The unfiltered upstream package suite fails `TestProxyDial` and `TestProxyAuthorizationDial` with both the original v1.5.3 and the proposed fixed version. Their fixtures return from a CONNECT handler and then attempt a second HTTP request on that connection without hijacking it. Go 1.27's `net/http/server.go` explicitly sets `closeAfterReply` for CONNECT. These are baseline fixture incompatibilities, not newly introduced failures. The remaining upstream tests pass; this is not a claim that the unfiltered upstream suite is green. The application has no production Go proxy-dialing path.

Follow-up verification: an isolated loopback test with a proper CONNECT proxy (hijacked socket and bidirectional relay to a separate WebSocket echo server) passes with both v1.5.3 and the fixed snapshot, with and without proxy authentication. This supports the fixture diagnosis; it does not cover every proxy implementation. The same CONNECT-close rule is also present in the locally installed Go 1.26.11 source, so it is not unique to our Go 1.27 migration. A Traefik reverse proxy forwarding inbound WebSocket upgrades uses a different path from the Go client's forward-proxy CONNECT dialing tested here.

Linux vulnerability analysis is not a native Linux test or race-detector run. No remote CI or image build/publication was triggered. Main CI's vulnerability checks remain optional under the existing policy. Published releases and Docker images are unchanged; this patch takes effect only in a later build containing it.
