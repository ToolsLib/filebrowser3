# Deployment

Starting with RC3, administrators can configure global/per-user [download speed limits](download-limits.md). Budgets are shared within one application process only. The existing Redis upload-cache option does not coordinate download bandwidth; multi-container enforcement is deferred and requires both shared identity/session storage and a distributed budget backend.

## Published images

Use [release installation](releases.md) and `compose.release.yaml` for `toolslib/filebrowser3`. Its runtime restrictions and data volumes match the source Compose deployment. Use the explicit `compose.loopback.yaml` override only for local HTTP previews. The public secret helper accepts `--image` so no host Go build is needed.

## Local-source Compose deployment

`Dockerfile.source` is the single Docker build definition. It builds the frontend and backend from this checkout and uses digest-pinned Node, Go and Alpine images. The default `compose.yaml` runs only the application, using the in-memory resumable-upload cache. Build the local Go binary first (`go run ./tools/build` after building frontend assets), then prepare the initial admin secret:

```text
node docker/local/prepare-secrets.mjs
docker compose build
docker compose up -d --wait
```

The preparation command generates a random initial admin password, stores only its hash, and prints the password once. Save it in your password manager. The default setup creates no Redis secret and starts no Redis service. Existing secrets are never overwritten. Do not commit `secrets/`; it is ignored by Git and excluded from Docker build context. On Unix the directory is mode 0700; files are 0644 inside that private parent so file-based Compose secret mounts can be read by non-root container users. Keep the parent private and do not move those files into a shared directory. On Windows restrict the directory's NTFS access to the operator account and administrators.

The app is published only at `127.0.0.1:8000` by default (`FB_HTTP_PORT` changes the port). Put a TLS reverse proxy in front of it and preserve the external Host header. Secure cookies are enabled by default, including behind the proxy. Direct loopback HTTP development can use a Compose override setting `FB_INSECURE_COOKIES=true`; keep it off for deployment. `FB_USERNAME` selects the initial account name (default `admin`). The initial password hash is used only when creating a new database; changing the secret file later does not reset an existing account.

The app runs as an unprivileged user, drops all capabilities, enables no-new-privileges, uses a read-only root filesystem, and has process/memory/CPU limits. Named volumes persist `/srv`, `/config` and `/database`. A separate `/cache` volume stores regenerable previews: Compose sets `FB_CACHE_DIR=/cache` and `FB_IMAGE_PROCESSORS=1` under its one-CPU/1-GiB limit. Preview entries are bounded to 512 MiB total, 8,192 entries, 16 MiB per entry and seven days; least-recently-used entries are evicted as needed. Cache deletion loses only generated previews. Backups do not need this volume. See [performance and limits](performance.md) for image admission and UI pagination. Files and login sessions survive application restarts. Pending in-memory uploads lose their resume state after a restart and must be uploaded again; existing destination files remain protected by staging. Back up served files, configuration and the database together; a database restore also restores the session/revocation state in that backup. `docker compose down` preserves volumes. `down --volumes` deletes them and is for disposable test stacks only.

### Build caching

The Node 26 build installs pnpm directly using the exact version and SHA-512 in `frontend/package.json`, without Corepack. npm's download cache and pnpm's package store use project-scoped BuildKit caches. Install layers remain keyed by the bootstrap, package-manager configuration, `package.json` and `pnpm-lock.yaml`; changed dependencies still resolve against the frozen lockfile. Cache mounts are local to the builder and should be shared only between trusted builds. A fresh builder still needs network access.

Frontend compilation copies only its production source, public assets and build/type configuration. The backend copies frontend Go embedding code and the compiled bundle separately, so unrelated frontend files do not enter its source layer. Documentation, local agent/tooling files, test sources and fixtures are excluded from the Docker context; tests continue to run from the checkout in CI and the Compose smoke harness.

The [Docker optimization review](reviews/2026-11-17-docker-optimization.md) records measured cache behavior and image parity. The runtime remains the same digest-pinned Alpine image, stripped static binary, CA certificates, shell entrypoint, non-root user and health check.

### Optional Redis upload persistence

Use `compose.redis.yaml` together with the default file when pending uploads should survive brief backend restarts. Redis stores upload metadata; files stay on disk and accounts/sessions stay in Bolt. State expires after three minutes without activity, so restart recovery must happen within that window.

For a fresh Redis-enabled setup:

```text
node docker/local/prepare-secrets.mjs --redis
docker compose -f compose.yaml -f compose.redis.yaml up -d --build --wait
```

For an existing app-only setup, the same secret command adds only the missing Redis secret and preserves the admin hash/password. Then start the combined Compose configuration. Keep the same project name/directory to reuse the app's existing volumes. New uploads use Redis; pending uploads created with the memory cache must restart. The override mounts the Redis secret, selects it through `FB_REDIS_PASSWORD_FILE`, and waits for Redis health before starting the app. If Redis is explicitly selected but its secret is missing or invalid, startup fails rather than silently switching caches.

Redis has no host port and sits on a private internal network. Its default user is disabled; the application user can access only `filebrowser:upload:v2:*` with the commands needed by the cache. Administrative commands and other keyspaces are denied. It has the same non-root/read-only/capability restrictions as the app, plus its own resource limits. Redis uses a separate AOF volume and a no-eviction memory limit; exhausted cache capacity fails uploads closed rather than discarding active state. The application retains its web network for the loopback published port and optional outbound integrations such as reCAPTCHA.

Use both `-f` arguments for subsequent Redis-enabled `up`, `stop`, `logs` and `down` commands. To return to app-only mode, stop the combined stack without deleting volumes, then start the base file:

```text
docker compose -f compose.yaml -f compose.redis.yaml down
docker compose up -d --wait
```

Completed files, settings and sessions remain in the same app volumes. Pending Redis uploads do not migrate into memory. The Redis data volume and secret can be kept for later use; the default app does not mount or use them. An external Redis server can instead be selected with the app's existing `FB_REDIS_CACHE_URL` setting in a custom override; `FB_REDIS_PASSWORD_FILE` is the convenience option for the bundled Redis service.

The supported authentication topology is one server process per Bolt database. Redis coordinates upload state; it does not share accounts or sessions across independent database copies. See [sessions](sessions.md), [upload recovery](uploads.md), and [request limits](request-limits.md). Keep the command runner and external-symlink option disabled. Only operator-controlled server/user root directories and ancestors belong in the filesystem trust boundary.

### Testing the Docker setups

After building the host binary, run:

```text
node docker/local/smoke-test.mjs
```

This builds `Dockerfile.source` once and exercises isolated app-only and Redis stacks. It checks resolved services/secrets, health, runtime isolation, authentication, direct/TUS uploads, persistent preview caching/conditional requests and durable files/sessions after restart. It verifies that memory-backed uploads require a safe restart, while Redis-backed uploads resume at the prior offset, and checks Redis ACL denials. It also tests initial secret generation, adding Redis without changing the admin hash, and overwrite protection. Disposable containers, volumes, image tags and test secrets are removed. Build output is saved in ignored `build/compose-smoke-build.log`.

CI runs the same Docker checks on Linux, and release depends on them. The legacy `Dockerfile` and `Dockerfile.s6` were removed; build commands must use `-f Dockerfile.source` or the supplied Compose configuration.

## Self-Registration (Signup)

File Browser allows you to enable user self-registration (signup). This can be enabled via **Settings → Global Settings**, or with `filebrowser config set --signup`. Self-registered users inherit the configured **user defaults**, including the scope.

> [!WARNING]
>
> By default, the user scope is the server's root, so a self-registered user could read,
> modify, and delete every file File Browser serves. To prevent this, either:
>
> a. Enable `createUserDir` so each user gets their own directory; or
> b. If users are meant to share files, set the default scope to something other than the root.

## Fail2ban

This fork includes application throttling based on the direct socket peer; see [request limits](request-limits.md). A trusted reverse proxy can add per-client throttling. Additional tools such as [fail2ban](https://github.com/fail2ban/fail2ban) can enforce operator policy, but forwarded addresses in logs are not a trusted-proxy authentication policy.

### Filter Configuration

An example filter configuration targeted at matching File Browser's logs.

```ini
[INCLUDES]
before = common.conf

[Definition]
datepattern = `^%%Y\/%%m\/%%d %%H:%%M:%%S`
failregex   = `\/api\/login: 403 <HOST> *`
```

### Jail Configuration

An example jail configuration. You should fill it with the path of the logs of File Browser, as well as the port where it is running at.

```ini
[filebrowser]

enabled = true
port = [your_port]
filter = filebrowser
logpath = [your_log_path]
maxretry = 10
bantime = 10m
findtime = 10m
banaction = iptables-allports
banaction_allports = iptables-allports
```
