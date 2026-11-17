# Building File Browser 3

File Browser 3 is the ToolsLib fork of archived upstream File Browser. Its review and hardening work are tracked in [fix status](docs/reviews/fix-status.md).

## Project Structure

The backend side of the application is written in [Go](https://golang.org/), while the frontend (located on a subdirectory of the same name) is written in [Vue.js](https://vuejs.org/). Due to the tight coupling required by some features, basic knowledge of both Go and Vue.js is recommended.

- Learn Go: [https://github.com/golang/go/wiki/Learn](https://github.com/golang/go/wiki/Learn)
- Learn Vue.js: [https://vuejs.org/guide/introduction.html](https://vuejs.org/guide/introduction.html)

We encourage you to use git to manage your fork. To clone the main repository, just run:

```bash
git clone https://github.com/ToolsLib/filebrowser3
```

We use [Taskfile](https://taskfile.dev/) to manage the different processes (building, releasing, etc) automatically.

## Build

Use Go 1.27.4 for normal builds; Go 1.26.11 is the tested minimum. Node 26.13.1 and the pnpm version pinned in `frontend/package.json` build the assets. CI tests both Go toolchains on Linux and Windows and fails if an unpatched compiler is substituted. `GOTOOLCHAIN=local` makes a chosen test compiler explicit.

With Node 26.13.1 and npm on your PATH, install the project's pinned pnpm once:

```text
npm --prefix frontend run setup:pnpm
```

The bootstrap reads the exact version and SHA-512 from `frontend/package.json`, verifies the npm archive before installation, and installs pnpm globally with lifecycle scripts disabled. Docker and CI use the same bootstrap. Corepack is not required. If you previously installed Corepack globally, remove it with `npm uninstall --global corepack` before bootstrapping; rerun the bootstrap after changing Node installations. An optional `-- --prefix <directory>` installs pnpm under a custom npm prefix instead. Add that prefix's `bin` directory to PATH (the prefix itself on Windows); CI does this explicitly for its temporary installation.

You can fully build the project in order to produce a binary by running:

```bash
task build
```

Without Task, run `pnpm install --frozen-lockfile` and `pnpm build` in `frontend`, then `go run ./tools/build` in the repository root. The helper emits `filebrowser.exe` on Windows and `filebrowser` on other platforms. An exact version tag supplies release metadata; otherwise the version is `0.0.0-dev+<commit>`, with `.dirty` when appropriate. A source archive without Git reports `0.0.0-dev+unknown`. It never needs a tag or remote to build locally. Container/release builds can supply paired `-version` and `-commit` overrides; see [releases](docs/releases.md).

Frontend/unit/browser checks are documented in [frontend maintenance](docs/frontend-maintenance.md). Backend checks are `go test ./...`, `go vet ./...`, and `golangci-lint run --timeout 5m` (v2.15.0). Linux also runs the race detector. `govulncheck` v1.9.0 and full-tree `pnpm audit` are release prerequisites. Vulnerabilities must be fixed or receive a specific, reviewed applicability decision; do not disable checks to ship an update.

## Development

For development, there are a few things to have in mind.

### Frontend

We use [Node.js](https://nodejs.org/en/) on the frontend to manage the build process. Prepare the frontend environment:

```bash
# From the root of the repo, go to frontend/
cd frontend

# Install the pinned package manager (once for this Node installation)
npm run setup:pnpm

# Install the dependencies
pnpm install --frozen-lockfile
```

If you just want to develop the backend, you can create a static build of the frontend:

```bash
pnpm run build
```

If you want to develop the frontend, start a development server which watches for changes:

```bash
pnpm run dev
```

Access the interface through the frontend development server. For loopback HTTP backend development, use `--insecureCookies`; leave it off in production. The Vite development server proxies API/WebSocket requests to the backend.

### Backend

First prepare the backend environment by downloading all required dependencies:

```bash
go mod download
```

You can now build or run File Browser as any other Go project:

```bash
# Build
go build

# Run
go run .
```

## Documentation

Documentation lives in [`docs`](docs) as plain Markdown and is no longer built into a site. The command line reference in [`docs/cli`](docs/cli) is generated from the commands themselves:

```bash
task docs:cli:generate
```

## Release

No release tags or remote are created by normal builds. After an operator configures the intended remote and explicitly creates a version tag, the GitHub Actions release job waits for frontend/browser tests, backend lint, Windows/Linux tests, Redis integration, both Compose deployment tests, vulnerability scans and a local build. It publishes binary archives and checksums to that repository using a job-scoped contents-write token. Inherited upstream Docker Hub and Homebrew publishing targets were removed. `Dockerfile.source` is the only Dockerfile; Compose defaults to app-only, with Redis enabled by the explicit `compose.redis.yaml` override. See [deployment](docs/deployment.md) for commands and `node docker/local/smoke-test.mjs` for both runtime checks.

GoReleaser is pinned to v2.20.2. Its full `check`/release validation requires a configured SCM remote. In the intentionally remote-free local repository, its configuration is checked against that version's JSON schema; live release publishing remains unexercised. A remote should be configured only when the operator is ready to publish.

## Translations

The Transifex integration stopped on 2026-09-01 and translations submitted there no longer reach this repository. Locale files live in [`frontend/src/i18n`](frontend/src/i18n) and can be edited directly.

## Authentication Provider

To build a new authentication provider, you need to implement the [Auther interface](auth/auth.go), whose method will be called on the login page after the user has submitted their login data.

```go
// Auther is the authentication interface.
type Auther interface {
    // Auth is called to authenticate a request.
    Auth(r *http.Request, s *users.Storage, root string) (*users.User, error)
}
```

After implementing the interface you should:

1. Add it to the [`auth` directory](auth).
2. Add it to the [configuration parser](cmd/config.go) for the CLI.
3. Add it to [`authBackend.Get`](storage/bolt/auth.go).

If you need to add more flags, please update the function `addConfigFlags`.
