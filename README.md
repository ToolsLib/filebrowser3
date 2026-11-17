# File Browser 3

File Browser 3 is ToolsLib's independent fork of [File Browser](https://github.com/filebrowser/filebrowser), based on upstream v2.63.23. It provides a minimal web workspace for uploading, browsing, editing, previewing and sharing files.

This fork includes subsequent security fixes, dependency updates, a revised Vue interface and performance improvements. Upstream File Browser binaries and images do not contain these changes. The inherited Apache 2.0 license and contributor attribution are preserved.

- Persistent, revocable login sessions and confined filesystem operations.
- Staged direct/resumable uploads with optional Redis upload-state persistence.
- File editing, media previews, public sharing and bounded file/CSV pages.
- An unprivileged, read-only Docker deployment with a bounded preview cache.

## Release candidate

The first candidate is **v1.0.0-rc.1**. Check [Releases](https://github.com/ToolsLib/filebrowser3/releases) for availability and [Actions](https://github.com/ToolsLib/filebrowser3/actions) for its validation status.

Planned public image: [toolslib/filebrowser3](https://hub.docker.com/r/toolslib/filebrowser3), with linux/amd64 and linux/arm64 variants. Candidate tags never update `latest`.

## Try locally

Once the candidate image is published, use Docker Compose and Node 26:

```text
git clone https://github.com/ToolsLib/filebrowser3.git
cd filebrowser3
node docker/local/prepare-secrets.mjs --image toolslib/filebrowser3:1.0.0-rc.1
docker compose -f compose.release.yaml -f compose.loopback.yaml up -d --wait
```

Open http://127.0.0.1:8000 and sign in as `admin` with the generated password. Save that password; the helper stores only its hash. The explicit loopback override permits cookies on local HTTP. Omit it when deploying behind TLS.

To try the project before image publication, follow the [source build instructions](docs/installation.md#docker) using `compose.yaml` and the explicit `compose.loopback.yaml` override for local HTTP. For published images, use `compose.release.yaml` and pin an exact version or digest. Files, accounts, settings and shares live in persistent volumes; retain the same Compose project when upgrading.

## Deployment and security

Place public deployments behind a TLS reverse proxy with external authentication. Keep command execution and external symlink traversal disabled. The command runner, when explicitly enabled, is trusted shell-equivalent functionality.

Read [deployment](docs/deployment.md), [session migration](docs/sessions.md), [request limits](docs/request-limits.md) and [performance limits](docs/performance.md). Redis coordinates pending uploads; it does not replicate accounts or sessions.

Report vulnerabilities privately through [this repository's security advisories](https://github.com/ToolsLib/filebrowser3/security/advisories/new), following [SECURITY.md](SECURITY.md). General bugs belong in [this fork's issue tracker](https://github.com/ToolsLib/filebrowser3/issues).

## Development and release information

- [Build and contribute](CONTRIBUTING.md)
- [Documentation](docs/README.md)
- [Release process and image tags](docs/releases.md)
- [Changelog](CHANGELOG.md)
- [Security review and fix status](docs/reviews/fix-status.md)
- [UI modernization](docs/reviews/2026-11-16-ui-modernization.md)
- [Performance implementation report](docs/reviews/2026-11-17-performance-completion.md)

The CLI remains `filebrowser`. This project is independent of the archived upstream project; report fork-specific issues here.

## License

[Apache License 2.0](LICENSE). Copyright File Browser Contributors and subsequent contributors. See [NOTICE](NOTICE) for derivation and attribution.
