# Releases

File Browser 3 is published from https://github.com/ToolsLib/filebrowser3.
The public Docker Hub image is `toolslib/filebrowser3`.
The current release candidate is **[v1.0.0-rc.4](https://github.com/ToolsLib/filebrowser3/releases/tag/v1.0.0-rc.4)**.
It moves file/folder creation from the sidebar into a New menu beside the
current-folder heading, with keyboard and mobile support. Releases
provide standalone archives, checksums and a multi-platform image; check the
release page and linked Actions run for publication status.

## Release channels

- GitHub tags use `v1.0.0-rc.4`; container tags omit `v`, for example
  `toolslib/filebrowser3:1.0.0-rc.4`.
- Release candidates are GitHub prereleases and get only their exact version
  image tag. They never update `latest`, major or minor tags.
- Stable releases get their exact image tag, then major/minor/`latest` aliases
  after validation. Published Git tags and exact-version image tags must not
  be moved to another commit. Use the published image digest for immutable
  deployments.

## Maintainer setup

The repository needs:

1. Repository variable **DOCKERHUB_USERNAME**, set to the Docker Hub login that
   owns the publishing token. An organization namespace is not necessarily the
   login name.
2. Repository secret **DOCKERHUB_TOKEN**, a Docker Hub access token with write
   access to `toolslib/filebrowser3`. Configure it in GitHub's Actions secrets
   settings; it does not belong in source files, shell history or chat.
3. A public `toolslib/filebrowser3` repository on Docker Hub, with access granted
   to that login. Enable immutable exact-version tags where supported.

The `v*` tag workflow validates version syntax and missing publishing credentials.
It requires frontend unit/browser checks, backend lint and Windows/Linux tests,
Go vulnerability checks, Redis integration, both source Compose configurations,
build metadata checks and release tag-policy tests.

On the exact tag commit, it builds linux/amd64 and linux/arm64 candidate images,
checks embedded version/revision metadata, and tests each architecture in both
Compose configurations on native amd64/arm64 runners. These native image tests
also run for ordinary branch and pull-request checks. After all checks pass, the
release job rebuilds the same tagged source with explicit version/revision build
arguments, creates draft binary archives/checksums, pushes
the multi-platform exact-version image with SBOM/provenance attestations, attaches
its immutable digest and publishes the GitHub release. Stable aliases move last.
No image is pushed by pull-request or ordinary branch workflows.

The GitHub token is read-only by default; only the release job receives contents
write access for release artifacts. Docker Hub credentials are exposed only to
the configuration check and login action. Third-party actions are pinned to
reviewed full commit SHAs.

## Preparing a release

Before creating the tag:

1. Review the source diff, changelog, migration notes and attribution. Keep
   `LICENSE` and `NOTICE` in distributions. Scan the complete Git history for
   credentials; generated data and bootstrap secrets stay ignored.
2. Run the normal checks from CONTRIBUTING.md and the release metadata tests:
   `node --test tools/release/metadata.test.mjs`.
3. Build image candidates from `Dockerfile.source` with `VERSION` and `VCS_REF`
   build arguments. Native Node/Go build stages cross-compile the backend; the
   runtime base matches the target platform.
4. Confirm the target commit is on `main`, the tree is clean, and Docker Hub
   publishing credentials are configured.
5. Create an annotated tag and push it, for example:

   ```text
   git tag -a v1.0.0-rc.4 -m "File Browser 3 v1.0.0-rc.4"
   git push origin v1.0.0-rc.4
   ```

The workflow repeats its required checks for the tagged commit. A failed run
does not promote stable aliases. Fix code with a new candidate tag; do not
retarget a published release. If a publishing step fails after artifacts were
uploaded, inspect the existing draft/tag/image before rerunning.

Let the workflow publish its draft before editing custom release notes. Preserve
the exact `tag_name` explicitly on API updates and verify the tag association
afterward. If finalization needs recovery, reuse the verified release ID and
published image digest; do not rebuild or replace a published exact-version tag.

## Installing an image release

Docker Compose and Node 26 are needed for the repository's secret helper; a Go
compiler is not required when using `--image`:

```text
node docker/local/prepare-secrets.mjs --image toolslib/filebrowser3:1.0.0-rc.4
docker compose -f compose.release.yaml -f compose.loopback.yaml up -d --wait
```

This explicit loopback HTTP preview is available at http://127.0.0.1:8000. Save
the generated admin password. For deployment behind a TLS/authenticating reverse
proxy, omit `compose.loopback.yaml` so Secure cookies remain enabled.
`FILEBROWSER_VERSION` selects a different published exact-version tag.

Optional Redis is supported with the same override:

```text
node docker/local/prepare-secrets.mjs --redis --image toolslib/filebrowser3:1.0.0-rc.4
docker compose -f compose.release.yaml -f compose.redis.yaml up -d --wait
```

Keep the same project name/directory and preserve all data volumes when
upgrading or switching between source/release Compose files. Back up files,
database and configuration together. Existing stateless clients must sign in
again and follow [session migration](sessions.md). The preview cache is
regenerable. See [deployment](deployment.md) for operating boundaries.

The CLI executable remains named `filebrowser`. The inherited Go module path
is retained as an internal build identity; use release archives, container
images or a source checkout rather than `go install` against the fork's v1 tags.
