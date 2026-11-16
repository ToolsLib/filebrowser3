# Application performance review

Reviewed commit: `0f110fd`. Review date: 17 November 2026. Status: **proposed improvements; application code and deployment unchanged**.

The largest opportunities are directory/subtitle expansion, bounded rendering of file lists and CSV tables, thumbnail generation, and avoiding complete directory reloads for sorting. Network compression is also a useful, comparatively contained improvement. No additional framework, database migration, or Redis service is required for these first steps.

This review covers backend filesystem/metadata work, search, previews, downloads, uploads, frontend state/rendering, assets, and request overhead. It adds reproducible probes and evidence, without applying performance fixes or altering user files, shares or settings.

## Evidence and limits

**Measured** findings were exercised using generated fixtures. **Source confirmed** findings were traced in the current code but do not have a measured end-to-end improvement. **Candidate** changes need additional profiling before implementation. Priorities describe proposed work: P1 first, P2 follow-up, P3 lower-impact cleanup.

Backend benchmarks ran with Go 1.27.4, `GOMAXPROCS=2`, on an AMD Ryzen 7 6800H. Both Windows and an isolated Linux container were used. In-memory benchmarks isolate algorithms; native-filesystem benchmarks include scoped filesystem operations. Neither is a production load test. The complete HTTP/browser fixture used the native Windows binary, Node 24.22.2 and Chromium 159, on a separate loopback port. Its timings are **not** the Docker deployment's measured latency. Filesystem, antivirus, cache, CPU limits and storage differences materially affect those numbers.

Raw results are in [performance measurements](2026-11-17-performance-measurements.json). Scripts and reproduction instructions are in [tools/performance](../../tools/performance/README.md).

| Probe | Result |
| --- | --- |
| Linux in-memory listing, 100 videos and 100 subtitles | 11.5 ms; 1.27 MB allocated per listing |
| Linux in-memory listing, 1,000 videos and 1,000 subtitles | 1.19 s; 102.2 MB allocated per listing |
| Linux native listing, 1,000 text files with scope-resolving checker | 52.5 ms; 8.6 MB allocated; not the full HTTP authorization pipeline |
| Windows complete HTTP listing, 2,000 text files | Median 5.05 s across three requests; 389,810-byte JSON response |
| Compression of that exact JSON body | 13,607 bytes with gzip; server actually returned it uncompressed |
| File listing after 18 downward scrolls | Rendered file rows grew from 82 to 1,072 of 2,000 items |
| Select All after that scroll | About 312 ms, including browser input handling and two animation frames |
| CSV with 20,000 rows and 8 columns | 388,906 input bytes; 180,375 page DOM nodes; about 1.95 s to rows being available; a 194 ms long task |
| Same CSV browser heap sample | About 73.9 MB used; whole-page state, not CSV-exclusive allocations or a peak |
| CSV above preview limit | All 6,000,004 bytes downloaded, HTTP 200, followed by a blank preview and no size-error message |
| Eight concurrent requests for one thumbnail | Eight resize calls in an instrumented cold/no-op-cache probe |
| Store update with 3,000 retained selections | Median 609 ms existing code; 7.6 ms for an unapplied full-update Set prototype; 1.5 ms existing code with no selection |

## P1 findings

### PERF01 Subtitle discovery makes video listings quadratic

**Measured and source confirmed.** [detectSubtitles, loadSubtitles and readListing](../../files/file.go) discover subtitles for every video while constructing a directory listing. Every video scans the current directory, and every video rereads and scans the same `subs`/`subtitles` directory. With V videos, N siblings and S shared subtitles, the repeated work grows roughly as V×N + V×S, plus directory enumeration and allocations. The CPU profile shows regular-expression backtracking as a major cost in the subtitle fixture.

Prefer discovering subtitles when opening a video, where the file-detail request already exists. If listing clients need the subtitle fields, build a directory/subtitle index once per listing and reuse it across entries. Preserve per-file authorization when serving subtitle content. Validate video-only folders, language variants, shared subtitle directories, and unavailable files. The measured 10× increase in videos and subtitles produced about 103× the listing time.

### PERF02 Large directories require complete metadata and header work

**Measured and source confirmed.** [resourceGetHandler](../../http/resource.go), [readListing](../../files/file.go), [ScopedFs.resolve/rooted](../../files/scoped.go) and [rule checks](../../http/data.go) process the complete directory before returning JSON. Header detection opens each regular file by default; scoped opens and rule checks repeatedly resolve paths and the scope root. The browser receives every item even when initially rendering only dozens of rows.

Introduce a lean listing contract and bounded server pagination, with explicit detail/header enrichment where needed. Pagination alone will not eliminate full enumeration if sorting still needs every entry's metadata. Profile request-scoped root-handle reuse and redundant resolution separately. Do not improve timings by disabling confinement, alias-aware deny rules, or traversal/race protections. Reuse/caching changes need the existing symlink, Windows case and authorization regression suites.

### PERF03 File-list scrolling retains every row it has revealed

**Measured and source confirmed.** [FileListing](../../frontend/src/views/files/FileListing.vue) increases `showLimit` as the page scrolls. It does not remove off-screen rows. The generated 2,000-item folder grew from 82 rendered rows to 1,072 after 18 downward scrolls; further scrolling can retain the whole directory. Every row also depends on shared selection state.

Use viewport virtualization or explicit UI pages so rendered rows remain bounded. Keep selection by stable identity, independent of whether the row is mounted. Cover list/grid/gallery modes, keyboard navigation, returning to the previous item, drag targets, image loading and accessibility. Vue's [performance guidance](https://vuejs.org/guide/best-practices/performance#virtualize-large-lists) describes this strategy; shallow immutable metadata can also reduce reactive overhead after the current item mutations are accounted for.

### PERF04 CSV preview limits bytes but does not bound rendering

**Measured and source confirmed.** [CsvViewer](../../frontend/src/components/files/CsvViewer.vue) parses the full input and renders every row/cell. A file under 0.4 MB generated about 180,000 DOM nodes. A 5 MB limit therefore does not bound table size or main-thread work.

The limit is also enforced too late. [makeRawResource](../../frontend/src/utils/encodings.ts) downloads and decodes the body first. [Preview's isCsv](../../frontend/src/views/files/Preview.vue) becomes false above the size limit, so neither the CSV component nor the intended oversize branch is shown. The CSV component's `error` prop is also not connected to its displayed error state. A 6 MB fixture reproduced the blank preview after the full download.

Check size from metadata before fetching preview content, show the size message explicitly, and cap displayed rows/cells with pagination or virtualization. Parse large allowed inputs incrementally or in a worker if profiling still shows long tasks. Avoid retaining both an eagerly decoded string and the same raw buffer unnecessarily. Preserve encoding and delimiter changes, download access, and an option to open the original file outside the preview.

### PERF05 Thumbnail work lacks reuse and cancellation at important layers

**Measured and source confirmed.** [preview generation](../../http/preview.go) performs a cache lookup followed by independent generation on each miss, without coalescing identical in-flight keys. The instrumented probe produced eight resizes for eight simultaneous requests. Cache writes happen asynchronously, extending the miss window.

The local Compose instance has no configured `cacheDir` or environment override, so [startup](../../cmd/root.go) uses the no-op preview cache. Repeated requests that reach the server can therefore repeat decoding/resizing. Browser caching may avoid some requests, but does not supply server reuse for other clients or cache misses.

Enable a dedicated writable, bounded preview cache; coalesce concurrent work for the same versioned image/size. Pass cancellation into queued generation instead of [context.Background](../../http/preview.go). If work is shared, cancellation should stop the worker when no interested requests remain rather than letting one caller cancel everyone. Test cold/warm requests, aborted queues, concurrent tabs, authorization and invalidation. Redis currently coordinates uploads; enabling it does not supply this image cache.

### PERF06 Image concurrency is not a decoded-memory budget

**Source confirmed; memory arithmetic, not an OOM experiment.** [Image limits](../../img/service.go) allow dimensions up to 10,000×10,000, and [startup](../../cmd/root.go) defaults to four image workers. One allowed RGBA image can require about 400 MB for its pixels. Four such source buffers alone exceed the Compose [1 GB memory limit](../../compose.yaml), before transforms, compressed input, output and other requests.

Use a decoded-pixel/memory budget alongside worker limits, and choose deployment defaults consistent with available memory and CPU. A lower worker count is a useful initial setting, but does not bound one oversized decode. Prefer pixel-count/weighted admission and a deliberate limit response over OOM/restart. Validate limits with header-only fixtures and representative real images rather than intentionally exhausting the local app.

### PERF07 Sorting refetches and rebuilds the entire folder

**Source confirmed.** [FileListing.sort](../../frontend/src/views/files/FileListing.vue) persists the new sort setting and sets `fileStore.reload`, causing [Files.fetchData](../../frontend/src/views/Files.vue) to request all metadata again. A sort-only action repeats filesystem work for data already present in the client.

Sort the loaded collection locally when the complete collection is present, then persist the preference independently. With server pagination, use server-side sorting and a suitable listing/index strategy instead. Match existing natural name order, folder ordering and case behavior; handle a preference-save failure without discarding an otherwise usable listing.

## P2 findings

| ID | Finding and evidence | Proposed improvement and validation |
| --- | --- | --- |
| PERF08 | **Measured:** [JSON responses](../../http/utils.go) are uncompressed. The 2,000-item body was about 390 kB; offline gzip of that exact body was 13.6 kB. This is a byte-size comparison, not a measured latency saving. | Negotiate compression for sufficiently large JSON responses and set appropriate `Vary`. Measure CPU/latency under the one-CPU deployment limit. Keep responses private and avoid compression choices that expose secrets beside attacker-controlled reflected data. Do not indiscriminately buffer or compress the search stream. |
| PERF09 | **Measured conditional hotspot:** [updateRequest](../../frontend/src/stores/file.ts) matches every incoming item against a selected-items array. The 3,000-selected-item probe took about 609 ms versus 7.6 ms for the complete Set prototype. **Normal Files.fetchData clears selection first**, so this is not the cost of every refresh. [Row selection](../../frontend/src/components/files/ListingItem.vue) and Select All also use repeated array membership. | Use stable URL/ID Sets for membership and batch Select All rather than repeatedly pushing/searching. Keep Vue updates explicit and test filtered/reordered lists. Treat the store microbenchmark and the observed 312 ms browser Select All as separate measurements. |
| PERF10 | **Source confirmed:** [search](../../search/search.go) walks the entire tree and does not skip a denied directory; [HTTP search](../../http/search.go) flushes each result individually; [the client](../../frontend/src/components/Search.vue) retains every result. | Add result limits/cursors and batch updates/flushes while preserving quick first results and cancellation. Only prune a denied subtree when rules prove no descendant can become allowed; later allow rules make unconditional pruning unsafe. Consider a permission-filtered index only after traversal remains a measured bottleneck. |
| PERF11 | **Source confirmed:** nested-folder [upload conflict preflight](../../frontend/src/utils/upload.ts) retrieves the complete destination tree and builds a browser map. Flat-upload preflight was already improved in the earlier review. | Add a bounded batched destination-existence/conflict API for the actual submitted paths, with cancellation. Retain authoritative server overwrite checks and staged commits. Test deep uploads into a much larger existing tree, special characters and interrupted preflight. |
| PERF12 | **Source confirmed:** [resourceGetHandler](../../http/resource.go) expands file content before handling checksum or raw-encoding responses. CSV raw responses then read the file again; checksums clear content only after reading it. | Decide the response mode first, skip unused content expansion, and stream permitted raw bodies. Keep binary/encoding behavior and download authorization intact. Count file reads/allocations for text, CSV and checksum requests and preserve cancellation. |
| PERF13 | **Source confirmed:** [archive downloads](../../http/raw.go) recursively build an in-memory manifest before starting the archive. The archive writer receives request context, but manifest enumeration does not. | Check cancellation during enumeration, avoid redundant entries, and bound or stream the manifest where the archive API permits. Preserve recursive authorization and safe archive names. Measure time to first byte and canceled-request cleanup for very large trees. |
| PERF14 | **Source confirmed, conditional on enabling disk cache:** [FileCache](../../diskcache/file_cache.go) retains a mutex for every stored key, has no built-in byte/age eviction, and reads the full cached body before ServeContent can reject an unchanged request. Versioned image keys accumulate. | Add cache quotas/eviction and bounded lock bookkeeping; consider serving cached files directly with validators. Coordinate reads/writes atomically. Test concurrent generation, stale versions, eviction and steady-state memory. Include this with PERF05 rather than merely turning on an unlimited cache. |
| PERF15 | **Source confirmed:** [Vite](../../frontend/vite.config.ts) emits Brotli and gzip JavaScript, but [static serving](../../http/static.go) only uses gzip and decompresses it for clients requesting Brotli alone. CSS is outside the compression plugin's include rule. | Use existing Brotli assets, correct Accept-Encoding quality negotiation and compressed CSS. Serve original bytes directly when compression is unavailable. Give content-hashed assets long immutable caching; keep mutable branding/Ace paths versioned or on shorter policies, and retain nonce-bearing HTML's no-store behavior. |
| PERF16 | **Source confirmed build/import opportunity:** [theme settings](../../frontend/src/components/settings/AceEditorTheme.vue) import Ace; [plain-text editing](../../frontend/src/views/files/Editor.vue) imports Markdown/math support; [ordinary images](../../frontend/src/components/files/ExtendedImage.vue) import UTIF. The build contains a roughly 472 kB theme-list/shared editor chunk. | Generate a small theme metadata list at build time, load Markdown/math/sanitization only for preview, and load UTIF only for its formats. Check actual request waterfalls and compressed sizes before estimating transfer savings; shared chunks and cache reuse prevent adding file sizes as if they were all extra downloads. Retain lazy video/EPUB loading, which is already present. |

## P3 finding

**PERF17 — Source confirmed:** [handleWithStaticData](../../http/static.go) rereads and parses the HTML template for each page request and reads authentication configuration twice for JSON auth. Cache the parsed template and execute it with fresh request data/nonces; avoid duplicate lookups. This is a contained optimization, but has lower expected impact than file I/O and unbounded rendering. Do not cache fully rendered nonce-bearing HTML or stale user/global configuration. Measure before assigning it a latency target.

## Suggested implementation order

1. Remove repeated subtitle discovery, fix CSV oversize handling and bound CSV rows/cells, then avoid sort-only reloads. These remove unnecessary work without introducing a new cache or service.
2. Add controlled thumbnail caching, in-flight coalescing, cancellation and image-memory admission together.
3. Bound file-list DOM and selection work, then introduce a lean/paginated listing API and profile secure filesystem/root reuse.
4. Add negotiated JSON/static compression and narrow optional imports. Improve search, nested-upload preflight and archive cancellation against large-tree fixtures.

Keep the probes as a baseline and compare after each change with the same datasets and platform. The [Go diagnostics guide](https://go.dev/doc/diagnostics) covers CPU, allocation and blocking profiles; use those before weakening safety/durability or increasing concurrency. Upload staging, fsync, session revocation, sanitization and filesystem confinement remain correctness constraints, not overhead to remove blindly.

No performance fixes were applied in this review. Only review documentation and isolated benchmark/probe tooling were added. The local Docker application was left running and its health endpoint returned HTTP 200.
