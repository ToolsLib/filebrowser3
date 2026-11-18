# Download speed limits

Available starting with v1.0.0-rc.3.

Administrators configure **Global settings → Download speed limits**. This is a default for each user, not a shared server-wide bandwidth pool. The initial default is unlimited. User settings and defaults for newly created users offer three choices:

- **Inherit global default** follows later changes to the global speed.
- **Custom speed** is an administrator-selected override, higher or lower than the default.
- **Unlimited** explicitly exempts that user from the default speed limit.

Enter bytes per second, such as `500 KiB/s` or `10 MiB/s`. `1 MiB/s` is 1,048,576 bytes per second (about 8.39 Mbit/s). The largest setting is 1 TiB/s. Existing users inherit the global default; existing installations remain unlimited until an administrator enables a limit. Ordinary users cannot change these policies through the UI or API.

## Enforcement

One in-memory token bucket is keyed by user ID and shared by all their transfers within the server process. More tabs, sessions, parallel connections, and Range/resume requests do not create extra budgets. Public shares use the current share owner's ID: anonymous visitors and the owner's private downloads compete for the same budget. A signed-in visitor does not change ownership of a public-share budget.

The limiter paces outgoing response-body bytes for original files, generated archives, media, image previews, subtitles and file-content APIs (including encoded CSV/editor reads). Directory listings, metadata-only requests, authentication, health checks and static application assets remain outside the download budget. Archives count compressed output bytes. This is independent of maximum file size and daily/monthly quotas.

Transfers stream in bounded chunks without buffering an entire file or archive. A bucket permits a quarter-second burst, capped at 64 KiB (at least one byte for very low speeds). The rate is an aggregate upper bound over time; transfers are not guaranteed equal shares or minimum throughput. Canceled clients stop waiting. Policy changes are reloaded within roughly one second at the next write; a failed policy read stops the transfer instead of allowing unlimited traffic. Bytes already sent to the network/proxy cannot be recalled.

Active budgets remain until transfers finish. Limited idle budgets keep their tokens for one minute, so reconnecting does not create a fresh burst. Tracking is capped at 4,096 users per process. New users receive 503 with Retry-After at capacity; active budgets are never evicted. Unlimited idle budgets are removed immediately. A process restart resets budgets and permits the small initial burst; this is not persistent usage accounting.

The stream writer preserves status, Content-Length, ETags, Range and streaming while preventing copy/sendfile fast paths from bypassing accounting. Throttled content responses use `Cache-Control: private, no-store`; unlimited preview caching retains its behavior. Previously cached client data cannot be recalled. Configure proxies/CDNs to stream downloads and honor cache headers: externally cached responses do not pass through the limiter. Proxy/socket buffering may cause short client-side bursts. Keep proxy idle timeouts compatible with slow streaming.

## Multiple containers: planned work

**Download bandwidth is currently coordinated only within one container/process.** Independent containers each provide their own user budget, so distributing traffic across N containers can yield roughly N times the configured speed. Sticky routing does not guarantee distributed enforcement. The existing Redis upload-cache option does not change this.

Before supporting this topology, implement and validate:

- A shared transactional identity/session backend. The supported topology is currently one process per Bolt database; Redis download coordination alone does not make active replicas safe. See [sessions](sessions.md#process-and-database-topology).
- A Redis-backed atomic token bucket or bounded bandwidth-lease backend used by all replicas. Namespace keys by installation and stable user identity, including public-share ownership.
- Consistent policy changes, aggregate bursts, idle expiry and cancellation. Any local token prefetch must have a documented aggregate overshoot bound.
- Explicit Redis-outage behavior that fails closed for limited users. Define clock authority and recovery; do not silently fall back to independent or unlimited budgets.
- Tests with at least two real app containers: simultaneous private/public/range transfers, policy changes, disconnects, Redis restart and network partitions. Verify combined throughput across replicas.

The local controller/stream-writer boundary is the integration point for that future backend. This work is intentionally deferred; the current implementation makes no multi-container download-speed guarantee.
