# Request and resource limits

Limits apply to API routes under the configured base URL. Static files and health checks remain available independently.

| Boundary                                                                                  | Limit                                                                                                  |
| ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| HTTP headers                                                                              | 32 KiB; 10-second read-header timeout                                                                  |
| Keep-alive idle connections                                                               | 60 seconds                                                                                             |
| Request-body and response-write idle time                                                 | 60 seconds, refreshed on each read/write rather than a total transfer deadline                         |
| Concurrent API requests                                                                   | 64                                                                                                     |
| Concurrent login/signup work                                                              | 4                                                                                                      |
| Concurrent search, preview, raw/archive, subtitle, command and recursive-listing requests | 8                                                                                                      |
| Login/signup per direct peer                                                              | Burst 10, replenished at 15 per minute                                                                 |
| Renewal per direct peer                                                                   | Burst 20, replenished at 120 per minute                                                                |
| Rate-limit bookkeeping                                                                    | At most 4,096 peer/class buckets, idle buckets expire after 10 minutes                                 |
| Authentication and other metadata bodies                                                  | 1 MiB                                                                                                  |
| Direct file and TUS upload size                                                           | 10 GiB by default; configurable positive `maxUploadSize` in bytes, capped at 1 PiB for safe arithmetic |
| WebSocket command message                                                                 | 16 KiB, initial read 30 seconds; output writes 10 seconds                                              |
| Command lifetime/output line                                                              | 10 minutes / 256 KiB; executable commands remain trusted shell-equivalent capability                   |
| Subtitle conversion                                                                       | 10 MiB input, 16 MiB output, two simultaneous conversions                                              |
| Search                                                                                    | Two-minute deadline; 200 results per UI page, API maximum 1,000; cursor offset up to 1,000,000                                                                    |
| reCAPTCHA verification                                                                    | 10-second overall timeout, request cancellation, 64 KiB response; no redirect following                |

Performance boundaries also include 200 mounted file rows, a 5 MiB CSV preview body, bounded worker results, a 100,000-entry archive manifest, weighted image admission, and a bounded preview cache. [Performance behavior and limits](performance.md) documents the exact policies and how to request originals or smaller archive selections.

Rate and concurrency exhaustion returns 429 with Retry-After. Known oversized request bodies return 413 before the handler; streamed over-limit writes fail while preserving the prior destination through private staging. Metadata bodies are buffered only within their cap. TUS length is checked against the upload limit before staging. Use `--maxUploadSize`, `FB_MAX_UPLOAD_SIZE`, or the server configuration to accommodate larger files; zero/negative configuration uses the default and does not disable the boundary.

Client-IP throttling uses the direct socket peer, never arbitrary X-Forwarded-For/Forwarded headers. Behind a reverse proxy this groups requests by proxy address, so configure additional per-client throttling at the trusted edge and account for shared users when sizing deployment traffic. Limits are per server process; Redis upload state does not provide a distributed rate limiter. The existing forwarded address used in logs does not establish authentication or throttle trust.

Streaming deadlines are idle policies: large uploads/downloads may run longer while making progress. WebSockets retain their protocol-specific deadlines. Body, peer-bookkeeping, concurrency, forwarding-spoof, deadline-reset and CAPTCHA cancellation regressions cover these boundaries. The production browser regression exercises normal editor uploads and media transfers through the same middleware.
