# Docker image and build optimization

Reviewed baseline: `61c408e`, 17 November 2026. The production image was already a multi-stage build with a stripped, static Go binary. Optimizations target build inputs and cache reuse; the runtime image remains byte-for-byte identical in the standalone comparison.

| Measurement | Before | After |
| --- | --- | --- |
| Runtime image size, Linux/amd64 | 56,268,735 bytes (56.3 MB) | Same |
| Application binary | 47.4 MB | Same SHA-256 |
| Build context | 473 files; 7,724,178 bytes | 313 files; 2,406,930 bytes (69% fewer bytes) |
| Documentation-only rebuild | 3.80 s; backend source copy and build reran | 0.81 s; frontend/backend compilation stayed cached |
| Warm dependency reinstall with dependency-step networking disabled | No cross-install package-store mount | All 367 packages reused; zero downloads |

Timings are single local samples, not a portable speed guarantee. The important cache assertion is that documentation changes no longer execute compilation. Runtime size is the uncompressed Docker image size, not registry transfer size or container memory use. Of its 56.3 MB, 47.4 MB is the binary and embedded frontend; the remaining runtime includes Alpine, certificates and support for the existing entrypoint/health check.

## Changes

- Added separate project-scoped BuildKit cache mounts for Corepack and the pnpm store; the build command uses the same package-manager cache.
- Kept frozen-lockfile installation, using package copies across mount boundaries.
- Limited the frontend stage to production inputs rather than copying its browser tests and tooling.
- Separated backend source, frontend embedding code and compiled frontend assets. Root-wide frontend copying no longer invalidates the backend source layer.
- Excluded documentation, local agent/tooling files, Go/frontend tests, fixtures and test output from the Docker context. Both runtime Go embedded resources remain included, including the common-password denial list.

The runtime stage, base-image digests, application functionality, security options, writable volumes, entrypoint and health check are unchanged. No executable packing, reduced editor-language coverage or compression-only static packaging was added. Those would trade compatibility or runtime work for size; this review does not claim a runtime-size reduction.

## Validation

- Production Docker builds succeed and Docker's build checks report no warnings.
- Standalone baseline, optimized and warm offline-dependency builds produce the same image digest: `sha256:adf74833d0b96049e00a8dd8cb2296cb36d896cb132da4b16019ef09561d8f2b`.
- The binary SHA-256 remains `61ee5ac44f8c1bbd0177fdc99b14650dc0886c266e5dcadf569cf98820ba0a04`.
- A disposable documentation edit confirms compilation remains cached. A manifest whitespace edit was restored byte-for-byte after testing installation reuse. Dependency installation was also rerun with `--offline` and networking disabled for that step; it reused all 367 packages without downloads. Fresh/uncached builders still require downloads.
- Both isolated app-only and optional-Redis Compose setups pass authentication, runtime isolation, direct/TUS uploads, overwrite safety, persistent files/sessions, preview caching/ETags and restart behavior. Disposable stacks and their volumes are cleaned up by the existing harness.
- The local deployment remains healthy. Its existing public download, Brotli assets, fresh HTML nonces and original data-volume identities are verified.

Detailed measurements are in [Docker optimization measurements](2026-11-17-docker-optimization.json). Build logs and disposable probes remain under ignored `build/docker-*` paths.

The cache strategy follows the official [Docker cache guidance](https://docs.docker.com/build/cache/optimize/) and [pnpm Docker guidance](https://pnpm.io/docker). The provided pinned Dockerfile frontend supports the backend's `COPY --exclude` instruction.
