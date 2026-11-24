# Command Execution

> [!CAUTION]
>
> The **hook runner** and **interactive shell** functionalities have been disabled for all existent and new installations by default from version v2.33.8 and onwards, due to continuous and known security vulnerabilities. You should only use this feature if you are aware of all of the security risks involved. For more up to date information, consult issue [#5199](https://github.com/filebrowser/filebrowser/issues/5199).

## Hook Runner

The hook runner is a feature that enables you to execute any shell command you want before or after a certain event. Right now, these are the events:

* Copy
* Rename
* Upload
* Delete
* Save

Also, during the execution of the commands set for those hooks, there will be some environment variables available to help you perform your commands:

* `FILE` with the full absolute path to the changed file.
* `SCOPE` with the path to user's scope.
* `TRIGGER` with the name of the event.
* `USERNAME` with the user's username.
* `DESTINATION` with the absolute path to the destination. Only used for **copy** and **rename.**

At this moment, you can edit the commands via the command line interface, using the following commands \(please check the flag `--help` to know more about them\):

```bash
filebrowser cmds add before_copy "echo $FILE"
filebrowser cmds rm before_copy 0
filebrowser cmds ls
```

Or you can use the web interface to manage them via **Settings** → **Global Settings**.

## Command console

Toggle the shell (`< >` icon at the top right) to open the command console. Enable it with `FB_DISABLE_EXEC=false` or `--disableExec=false`. It remains disabled by default. The user also needs Execute permission and an explicit command allowlist, including administrators. Hooks keep their existing behavior; the transport change described here applies to the console.

The console runs one command at a time and streams combined stdout/stderr, including output without a newline. It displays nonzero exit status and errors. This is a command runner, not a PTY with interactive stdin. The browser retains about one million characters of recent output per command.

Use **Cancel** or Escape to stop an active command. Closing the console, navigating to another route, logging out or unloading the page also cancels the request. Server-side cancellation terminates the command and ordinary descendants in its Unix process group or Windows Job Object, then reaps the command before releasing the request's work slot. The same cleanup applies to the ten-minute runtime limit, response write/flush failures and normal parent completion. Deliberately detached Unix processes can escape the process group; command execution is not a sandbox. Enable it only for trusted users and commands.

By default no commands are available as the command list is empty. To enable commands these need to either be done on a per-user basis (including for the Admin user).

You can do this by adding them in Settings > User Management > (edit user) > Commands or to *apply to all new users created from that point forward* they can be set in Settings > Global Settings

## HTTP streaming API

The local migration replaces `GET /api/command/<directory>` WebSocket upgrades with **`POST /api/command/<directory>`**. Send the command as a UTF-8 `text/plain` body with the regular `X-Auth` access-token header. The limit is 16 KiB for the complete request body. Cookie-only POSTs are not accepted. Authentication, execution permissions, the allowlist and request/concurrency limits remain enforced. Empty or malformed commands fail before streaming (400); permission/allowlist failures return 403 and oversized bodies return 413. Process startup failures return 500.

A successful response is `application/x-ndjson; charset=utf-8`, containing one JSON object per line:

```json
{"type":"start"}
{"type":"output","data":"partial output\n"}
{"type":"heartbeat"}
{"type":"exit","code":0}
```

Output arrives in bounded chunks with UTF-8 boundaries preserved. Quiet commands send a heartbeat every 15 seconds. Runtime failures after headers are sent use an `error` event, for example `{"type":"error","data":"Command timed out."}`. An `exit` event carries the process exit code. EOF without a terminal event is an interrupted stream, not successful completion. The browser reports it without retrying the command: it may already have executed. External clients should follow the same rule.

The endpoint has no WebSocket fallback. Deploy the backend and bundled assets together, reload open browser tabs, and migrate any custom WebSocket clients to POST and NDJSON. Hooks and other file APIs do not change. No replacement WebSocket dependency is introduced; `github.com/gorilla/websocket` is removed. `github.com/gorilla/mux`, the separate HTTP router, remains.

## Reverse proxies and buffering

The server flushes each event, sets `Cache-Control: no-store, no-transform` and `X-Accel-Buffering: no`, and bounds stalled writes with the shared one-minute write deadline. Heartbeats help keep quiet streams open. The proxy must forward them promptly and propagate client disconnects; an application header cannot override every proxy or CDN's buffering policy.

For Nginx, `X-Accel-Buffering: no` disables response buffering unless the administrator ignores that header. An explicit command location can also enforce streaming:

```nginx
location /api/command/ {
    proxy_pass http://filebrowser:8080;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_cache off;
    gzip off;
    proxy_read_timeout 60s;
    proxy_ignore_client_abort off;
}
```

Adjust the location to include any configured base URL, and preserve the deployment's existing authentication/access controls. Nginx's [`proxy_buffering` documentation](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_buffering) explains the header and directive behavior.

Traefik can forward this as ordinary HTTP without WebSocket configuration. **Do not attach its [Buffering middleware](https://doc.traefik.io/traefik/reference/routing-configuration/http/middlewares/buffering/) to the command route.** It reads the response before forwarding it and prevents incremental console output; `X-Accel-Buffering` does not disable it. If buffering is required for other routes, give command requests a separate router that retains all existing authentication/security middleware while omitting response buffering and compression. Set proxy/CDN idle timeouts above 15 seconds and permit ten-minute responses. If a proxy intentionally keeps reading after its client disconnects, prompt cancellation cannot be guaranteed from the backend alone.

## Validation

Backend regressions cover preflight authorization, cookie-only POST rejection, request bounds, UTF-8 and partial-line output, exit codes, flushing through a reverse proxy, heartbeats after stdout closes, runtime timeout, flush/write errors, and cancellation of a parent and descendant through direct/proxied HTTP. Linux runs include the race detector; Windows exercises Job Object cleanup.

Frontend tests cover split/coalesced NDJSON, split UTF-8, interrupted/oversized/invalid streams, errors and aborts without retry. The production browser test checks incremental rendering, header authentication with a base URL, exit status and descendant cleanup after Cancel, console close and navigation.

After `pnpm build` in `frontend` and `go run ./tools/build` in the repository root, run the optional real-proxy smoke check from the root:

```text
node tools/testing/command-stream-proxy.mjs
```

It requires Docker, Node 26 and available ports 18090–18094. It starts only generated test data and uniquely named temporary Nginx/Traefik containers, then removes those containers and stops the fixture app. It checks Nginx header-driven streaming, explicit `proxy_buffering off`, normal Traefik forwarding and abort propagation, plus a negative control demonstrating delayed output with Traefik Buffering enabled. The fixture binds on all host interfaces so the containers can reach it; its account and files are disposable. `FB_TEST_TRAEFIK_IMAGE` can select another Traefik image for comparison. Ordinary production browser tests still use loopback and port 18089.

> [!NOTE]
> 
> If using Docker and you want to add a new command that is not in the base image, build a custom image using `toolslib/filebrowser3` as the base. Install packages as root, then restore the unprivileged runtime user. For example:
> 
> ```docker
> FROM toolslib/filebrowser3:1.0.0
> USER root
> RUN apk add --no-cache git
> USER 1000:1000
> ```
