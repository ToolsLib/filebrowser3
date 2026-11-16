# Deployment

## Local-source Compose deployment

`compose.yaml` builds this checkout using `Dockerfile.source`; it does not run an upstream `latest` image. Node, Go, Alpine and Redis base images are pinned by digest. Build the local Go binary first (`go run ./tools/build` after building frontend assets), then prepare new secrets:

```text
node docker/local/prepare-secrets.mjs
docker compose build
docker compose up -d --wait
```

The preparation command generates a random initial admin password and Redis secret, stores only the admin password hash, and prints the admin password once. Save it in your password manager. It refuses to overwrite an existing directory. Do not commit `secrets/`; it is ignored by Git and excluded from Docker build context. On Unix the directory is mode 0700; files are 0644 inside that private parent so file-based Compose secret mounts can be read by both non-root container users. Keep the parent private and do not move those files into a shared directory. On Windows restrict the directory's NTFS access to the operator account and administrators.

The app is published only at `127.0.0.1:8000` by default (`FB_HTTP_PORT` changes the port). Put a TLS reverse proxy in front of it and preserve the external Host header. Secure cookies are enabled by default, including behind the proxy. Direct loopback HTTP development can use a Compose override setting `FB_INSECURE_COOKIES=true`; keep it off for deployment. `FB_USERNAME` selects the initial account name (default `admin`). The initial password hash is used only when creating a new database; changing the secret file later does not reset an existing account.

Both services run as unprivileged users, drop all capabilities, enable no-new-privileges, use read-only root filesystems, and have process/memory/CPU limits. Separate named volumes persist `/srv`, `/config`, `/database` and Redis AOF data. Back up served files, configuration and the database together; a database restore also restores the session/revocation state in that backup. `docker compose down` preserves volumes. `down --volumes` deletes them and is for disposable test stacks only.

Redis has no host port and sits on a private internal network. Its default user is disabled; the application user can access only `filebrowser:upload:v2:*` with the commands needed by the cache. Administrative commands and other keyspaces are denied. Redis uses AOF persistence and a no-eviction memory limit; exhausted cache capacity fails uploads closed rather than discarding active state. The application also has a web network for the loopback published port and optional outbound integrations such as reCAPTCHA.

The supported authentication topology is one server process per Bolt database. Redis coordinates upload state; it does not share accounts or sessions across independent database copies. See [sessions](sessions.md), [upload recovery](uploads.md), and [request limits](request-limits.md). Keep the command runner and external-symlink option disabled. Only operator-controlled server/user root directories and ancestors belong in the filesystem trust boundary.

Validation of the supplied stack covers source build, health, UID/capability/root-filesystem settings, denied Redis administration/foreign keys, authenticated TUS upload, and session/partial-upload persistence across service restart. Existing upstream Dockerfiles are retained as legacy packaging references; the current local-source Compose path uses `Dockerfile.source`.

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
