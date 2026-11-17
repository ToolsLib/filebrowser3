# Performance implementation report

All 17 performance findings are implemented and validated. The local application is available at http://127.0.0.1:8000 with its existing account, files, settings and shares. No remote, dependency migration, additional framework or required Redis service was introduced. The app-only Compose default remains supported; Redis remains optional upload-state persistence.

The baseline is `dca4446`, containing the [original performance review](2026-11-17-performance-review.md). [Finding status](performance-fix-status.md) maps every PERF identifier to its implementation and evidence. [Operational documentation](../performance.md) specifies limits, selection semantics and API contracts. Raw final results are preserved in [performance measurements after implementation](2026-11-17-performance-after.json).

## Measured results

| Fixture | Before | After |
| --- | --- | --- |
| Linux in-memory listing, 1,000 videos plus 1,000 shared subtitles | 1.193 s; 102.2 MB allocated | 0.748 ms; 0.892 MB allocated; subtitle discovery moved to video details |
| Windows HTTP folder, 2,000 text files | 5.05 s median, complete metadata | 2.38 s complete lean listing; 2.41 s first 200-entry page |
| Same folder transfer | 389,810 bytes, uncompressed | 13,871 bytes for the complete compressed listing; 1,687 bytes for its first compressed page |
| File rows after 18 downward scrolls | 82 initially, growing to 1,072 | 200 initially and after scrolling |
| Browser Select All including input and two animation frames | 312 ms | 45 ms for the loaded 200-entry page |
| CSV, 20,000 rows × 8 columns | 1.95 s; 180,375 whole-page DOM nodes | 133 ms; 1,285 whole-page DOM nodes; 100 displayed rows |
| CSV sampled main-page JS heap | About 73.9 MB | About 16.5 MB; worker parsing peak is not included |
| CSV long-task sample | 194 ms long task observed | No main-page long tasks recorded in the probe |
| Oversized CSV | Downloaded all 6,000,004 bytes; blank preview | 186 bytes of metadata, explicit size message, no preview-body request |
| Eight concurrent requests for one cold thumbnail | Eight resize calls | One shared resize call |
| Store update retaining 3,000 selections | 609 ms median | 7.6 ms median |

These are generated-fixture measurements, not production service-level promises. Windows HTTP/browser runs use the native binary, Node 24.22.2 and Chromium 159 on the same host as the baseline. Linux algorithm benchmarks use Go 1.27.4, GOMAXPROCS=2 and the same in-memory/native fixture structure. Timings vary with storage, antivirus, cache and concurrent activity. File timestamps make exact compressed byte counts vary slightly between runs. The first-page transfer saving combines pagination and compression; compression alone reduces the complete listing by about 96%. Select All now selects the loaded page, so that browser timing is not a 2,000-selection benchmark; the separate store benchmark measures 3,000 retained selections.

A one-thread Go JSON microbenchmark checks the compression tradeoff independently of filesystem time. For 200 representative records, identity and gzip both take about 0.31 ms; gzip allocates about 71 KB versus 89 KB for the recorder's identity body. For 2,000 records, gzip takes 3.32 ms versus 3.03 ms, allocating about 690 KB versus 876 KB. Four reusable compressors bound retained codec state. These are steady-state synthetic serialization timings on Windows, not measurements of the Docker application's CPU consumption or network latency.

## Implemented changes

Directory listings avoid repeated subtitle scans and unnecessary header opens for known types. File details retain subtitle and content detection. The workspace uses server pages of 200 entries, with bounded DOM in list, grid and gallery views; public folders use display pages too. Explicit entire-folder selection fetches the full metadata collection, keeps selection by stable identity and retains bounded rendering. Closing previews/editors restores the originating page. Complete collections sort locally; partial collections ask the server to sort.

CSV preview metadata is checked before body retrieval and the server independently caps preview reads. Parsing runs in a cancellable worker with row, column, cell and record bounds. Encoding/delimiter changes restart parsing, page controls bound the table, and limits/errors are visible. Downloads remain available; CSV text editing fetches the real editor content separately, including files above the preview limit.

Thumbnail generation coalesces identical in-flight keys, shares results, tracks interested callers and stops abandoned queued work. Compose enables a dedicated persistent preview cache and one image worker. Weighted pixel admission prevents a single permitted-dimension image from bypassing the memory estimate. Cache entries have byte/count/age quotas, atomic replacement, seekable serving and validators. Cache failure falls back to generation. Authorization runs before both cached responses and 304s.

Search returns bounded result pages, flushes its first result promptly, batches later flushes and sends recurring heartbeats. It stops on cancellation and only prunes literal denied subtrees when no allow exception can apply. Upload preflight checks actual submitted destinations through `/api/check-paths`, splitting batches by count and encoded size; cancellation and failure stop preflight without silently authorizing overwrite. Dropping onto a folder no longer fetches its complete listing first. Final staged upload checks remain authoritative.

Raw/checksum response mode is chosen before content expansion. Raw encoding bodies stream and checksum tests count one full data read plus header detection. Archive manifests check cancellation, deduplicate overlapping selections, close enumeration handles promptly and stop at 100,000 discovered entries. Existing recursive authorization and traversal-safe archive names remain enforced.

Large JSON responses negotiate gzip and remain private. Static serving negotiates Brotli/gzip quality values, streams selected seekable assets, serves original bytes directly and compresses CSS. Hashed assets and versioned Ace paths use immutable caching. Theme metadata is generated from the locked Ace package into a 3.59 KB module (0.83 KB gzip), so settings do not load Ace. Markdown/math/sanitization load only for Markdown preview; UTIF loads for TIFF/RAW formats and releases canceled requests/object URLs. The HTML template is parsed once and executed with fresh settings, auth data and request nonces.

## Validation

- Full Windows Go 1.27.4 test suite, `go vet` and the repository Go lint checks pass.
- Full Linux Go suite passes; the final Linux Go 1.26.11 suite also passes with the race detector, including confinement, symlink/rule checks, archive-name safety, upload staging/durability and the new cache/preview concurrency tests.
- Frontend type checking, lint, production build and 41 unit tests pass.
- Five production Playwright workflows pass under a nested base URL and CSP, covering login, editor/save/Markdown, CSV/encoding preview, TIFF/image/video/EPUB, shares, uploads, folder drops, responsive layouts, themes, pagination, complete-folder selection and return navigation. Browser errors remain empty.
- The generated performance browser probe enforces bounded file/CSV DOM, editor return to page 2 and metadata-only oversized CSV behavior. The eight-request thumbnail probe asserts one resize.
- Both isolated Docker Compose setups pass health, authentication, non-root/read-only isolation, direct/TUS uploads, existing-file safety, durable files/sessions, writable persistent preview cache and ETags across restart. Memory upload state restarts safely; optional Redis state resumes and its ACL checks pass. Disposable stacks and test volumes are removed.
- The existing local deployment is rebuilt, healthy and checked with its existing public download/share. Served files, configuration and database volumes are retained.

## Deliberate limits

Directory sorting and authorization still enumerate metadata for the complete folder. The native scoped-checker benchmark remains approximately 51.5 ms for 1,000 files versus 52.5 ms before; no general secure-filesystem speedup is claimed. Root-handle reuse, persistent indexes and snapshot pagination need separate profiling and correctness work. Confinement and alias-aware rules were retained.

Search cursors rewalk and skip prior matches rather than using a persistent index. Directory/search pages can change if the underlying tree changes. Entire-folder selection and preview-neighbor navigation can still load complete metadata; these explicit operations keep DOM bounded but are not constant-memory catalogs. Archive output retains a bounded manifest phase before its first byte because the archive interface requires it. Image admission estimates decoded work rather than guaranteeing whole-process peak RSS. CSV and TIFF browser limits trade preview completeness for bounded work; originals remain available.

No dependency versions changed in this performance work. Existing optional media/editor chunks remain sizable, but their loading is narrower. Disk preview storage is regenerable and not part of the user-data backup requirement. Timing assertions are kept out of permanent tests; structural bounds and protocol correctness are asserted instead.

Generated screenshots: `build/performance-screenshots/01-paginated-files.png` and `build/performance-screenshots/02-paginated-csv.png`. Reproduction scripts and commands are in [tools/performance](../../tools/performance/README.md).
