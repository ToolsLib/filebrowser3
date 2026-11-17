# Performance behavior and limits

The [implementation report](reviews/2026-11-17-performance-completion.md) records the measured results and validation for PERF01–PERF17. These optimizations retain filesystem confinement, current authorization checks, private upload staging and durable commits.

## Browsing and selection

The workspace requests 200 entries per page. File, grid and gallery views retain at most 200 mounted rows, including after scrolling. Public folders also use bounded display pages. Folder counts describe the complete authorized folder. Ctrl+A selects the loaded page; **Select entire folder** explicitly loads its metadata and selects all entries, with display still paginated. Selection survives sorting by stable URL. Complete collections sort locally and persist the preference; partial collections use server sorting. Closing an editor or preview returns to the originating page. Preview next/previous navigation can load the full metadata collection when needed.

`GET /api/resources/<path>?limit=200&offset=400` returns `total`, `limit`, `offset` and the requested `items`, with complete folder counts. Limits are 1–1,000. Omitting `limit` retains the complete-list API for bulk operations and compatibility. Listings classify known types without opening every file; unknown types retain header detection. `details=true` requests header enrichment. Video subtitle discovery occurs on the file-detail request. Enumeration, metadata authorization and sorting still inspect the directory; pagination does not supply a persistent index or a snapshot. Refresh when the folder changes between pages.

## CSV and optional previews

CSV previews check metadata before downloading. The server also enforces a 5 MiB preview-body boundary, including when a file grows after the metadata response. A worker parses permitted content and terminates on navigation or format/encoding changes. Results are bounded to 50,000 data rows, 200 columns, 200,000 cells and 256 KiB records; a truncation notice accompanies bounded results. The table displays at most 100 rows and roughly 2,000 data cells per page. Preview limits leave original downloads available. **Edit as text** uses the existing text editor limit (10 MiB); it fetches actual editor content independently of the CSV preview limit.

Markdown, math and sanitization code load when Markdown preview is requested. TIFF/RAW decoding loads only for those formats, aborts downloads on navigation and releases generated image URLs. Its browser preview admits up to 24 million pixels; larger originals remain downloadable.

## Thumbnail generation and cache

On Windows, the OS-backed cache opens readers with delete sharing and uses native replacement when a preview is already open. Active readers keep the old bytes while subsequent requests see the replacement. This retains streaming without copying entire cached previews into memory. Concurrent replacement requires filesystem support for Windows POSIX rename semantics; cache write failures leave the generated preview available to the request.

Concurrent requests for the same physical file version and preview size share generation. A caller can cancel independently; generation is canceled when no callers remain. At most 128 distinct in-flight keys are retained. Cache keys include file size, nanosecond modification time and a format version. Every HTTP request still performs current authorization before returning a cached image or 304.

Compose enables a dedicated `/cache` volume and one image processor. Other deployments can set `cacheDir`/`FB_CACHE_DIR`; an empty value keeps the no-op cache. Disk cache defaults are 512 MiB, 8,192 entries, 16 MiB per entry and seven days. Entries are atomically replaced, evicted by recent access and served from seekable files. A cache failure falls back to generation. Use one application process per cache directory, consistent with the supported Bolt database topology. Preview data is regenerable and need not be backed up.

The image service admits work against a 384 MiB weighted decoded-image estimate, using 16 bytes per source pixel alongside the worker limit. Images above that admission limit or 10,000 pixels in either dimension return 413 for resized previews. This is conservative allocation admission, not an absolute process-RSS guarantee. Original downloads remain available.

## Search, uploads and archives

Search returns up to 200 results per UI page (API maximum 1,000) using `limit` and a numeric `cursor` offset. The NDJSON stream ends with `{ "type": "page", "nextCursor": 200 }`, or zero when complete. The first result flushes immediately; subsequent writes flush at 64 results or about 35 ms, with recurring heartbeats. Requests retain a two-minute deadline and client cancellation. Cursors can advance to 1,000,000 matches, re-evaluate authorization and walk from the beginning; they are not an index or stable snapshot. Only inherited literal denials with no allow exceptions are pruned. Regex and hide-dotfile rules retain their existing semantics.

Upload preflight uses `POST /api/check-paths` with `{ "base": "/uploads", "paths": ["folder/file.txt"] }`. It checks only requested destinations, in count- and byte-bounded client batches. The server accepts at most 1,000 relative paths within a 1 MiB body. Traversal, absolute paths and denied destinations are rejected; interrupted/failed preflight does not silently authorize overwrite. Upload staging and final overwrite checks remain authoritative.

Archive manifests check request cancellation, deduplicate source paths and bound enumeration to 100,000 entries. Exceeding the limit returns 413 before archive output; select smaller folders separately. The archive library still requires a manifest, so large allowed archives have a metadata phase before their first output byte. Recursive authorization and safe entry names remain enforced.

## HTTP delivery

Large JSON responses negotiate gzip and remain private. Search stays uncompressed NDJSON. Static assets negotiate existing Brotli/gzip variants with quality values; originals are served directly when selected. Content-hashed assets and versioned Ace assets use one-year immutable caching. Branding remains on a shorter policy. HTML retains no-store and fresh request nonces; only its parsed template is cached.
