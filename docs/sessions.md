# Sessions and credential transport

Sessions are persisted in the existing Bolt database. Every protected request verifies the signed access token, the session record, its idle and absolute expirations, and a fingerprint of the current password, authentication method and signing key. Current user permissions are loaded from the user store. Restarting the server preserves sessions and revocations; losing or restoring the database changes that state.

`tokenExpirationTime` controls the inactivity/refresh window (default two hours). Nonpositive or invalid values fall back to the default. A session can never extend beyond 24 hours after login, regardless of that setting or renewal frequency. A valid access token alone cannot renew itself. Refresh credentials contain a random session ID and a separate 256-bit secret; only a hash of the secret is stored. Rotation consumes the previous secret in a database transaction. Replaying a consumed secret rejects the request and revokes the session, including its access tokens. Browser renewal is serialized within a tab and across tabs using Web Locks to avoid accidental replay.

`POST /api/logout` revokes the current session. `POST /api/logout-all` revokes all sessions for the authenticated user; the profile page provides the corresponding button. Password changes through the API revoke all of that user's sessions. Password changes made through the CLI or another database writer also invalidate sessions because each request compares the credential fingerprint. Account deletion, authentication method changes, and signing-key changes invalidate affected sessions. Revocation applies to subsequent requests; an already-authorized stream or running operation may finish within its own bounds.

The database allows at most 20 active sessions per user and 10,000 overall; expired records are pruned during creation. Sessions with old credential fingerprints are removed when the same user signs in with updated credentials. Hitting a limit returns 429. Storage failures fail closed.

## Browser transport

The access token is held only in application memory. The old `localStorage.jwt` value is removed, and the legacy JavaScript `auth` cookie is expired. Existing stateless tokens require a new login after upgrading.

The server issues host-only, HttpOnly, SameSite=Strict cookies: `fb_access` at the configured application base path, and `fb_refresh` at its `/api/` path. Cookies are Secure by default, including when TLS terminates at a reverse proxy. Serve the browser application over HTTPS. For loopback HTTP development only, `--insecureCookies` (or `FB_INSECURE_COOKIES=true`) permits non-Secure cookies for a localhost/loopback hostname; remote hostnames still receive Secure cookies. Keep this option disabled behind a production proxy.

Normal mutation APIs continue to require the in-memory `X-Auth` access token; GET/media requests may use the access cookie. Login, refresh, logout and logout-all require `X-Requested-With: FileBrowser` or the explicit native transport header below. Cross-site Fetch Metadata and foreign Origin values are rejected. The server does not enable cross-origin credential access. Successful credential responses have `Cache-Control: no-store` and `Pragma: no-cache`. Cookie paths reduce accidental exposure across applications; applications sharing an origin remain within the same browser security boundary.

## Header clients

Native clients can use the following protocol without cookies:

1. Send `POST /api/login` with JSON credentials and `X-Session-Transport: header`.
2. Read the access JWT from the response body and refresh credential from `X-Refresh-Token`. Keep both secret.
3. Authenticate ordinary requests with `X-Auth: <access JWT>`.
4. Renew with `POST /api/renew`, `X-Session-Transport: header`, and `X-Refresh-Token: <refresh credential>`. Replace both returned credentials and discard the consumed refresh credential. Serialize renewals.
5. Revoke with `POST /api/logout` or `/api/logout-all`, `X-Session-Transport: header`, and `X-Auth`.

Clients using only the former JWT renewal protocol must migrate. Sessions and tokens are scoped to the configured authentication database and signing key.

## Process and database topology

The supported authentication topology is one server process per Bolt database. Bolt holds an exclusive process lock; do not put the database on a shared network filesystem or copy it to independent active replicas. Redis coordinates resumable-upload state and file locks coordinate shared staging, but neither makes user accounts, permissions or sessions in separate database copies consistent. A multi-process deployment requires a shared transactional identity/session backend before it can be supported. This limitation is explicit and fail-closed: an independent database does not recognize a session it did not create. Store-level tests verify transaction atomicity between independent store instances on the same database, restart persistence, and immediate revocation visibility.
