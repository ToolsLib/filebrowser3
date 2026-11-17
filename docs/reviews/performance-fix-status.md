# Performance implementation status

All PERF01–PERF17 findings from the [performance review](2026-11-17-performance-review.md) are fixed. Baseline: `dca4446`. Security, durability and existing user data remain constraints.

| Finding | Status | Implementation and evidence |
| --- | --- | --- |
| PERF01 Subtitle discovery | Fixed | Video listings defer subtitle scans to file details; subtitle detail regression and Linux scaling benchmark pass. |
| PERF02 Directory metadata and pagination | Fixed | Lean type detection, cancellable enumeration, bounded API pages; scoped filesystem authorization remains intact. |
| PERF03 Bounded file-list rendering | Fixed | 200-row pages in workspace and public folders; bulk selection and return-to-page browser regressions pass. |
| PERF04 CSV limits and rendering | Fixed | Metadata/body guard, cancellable worker, bounded parse results and table pages; oversized and editing regressions pass. |
| PERF05 Preview reuse and cancellation | Fixed | Coalesced versioned work, last-waiter cancellation, persistent Compose cache; eight requests produce one resize. |
| PERF06 Image memory admission | Fixed | 384 MiB weighted pixel estimate alongside workers; header-only rejection and queued cancellation tests pass. |
| PERF07 Sorting | Fixed | Complete collections sort locally with stable selections; partial collections use server sorting; browser regression passes. |
| PERF08 JSON compression | Fixed | Negotiated gzip, private responses and four retained compressors; protocol and single-thread CPU/allocation probes pass. |
| PERF09 Selection | Fixed | URL/index Sets and batched selection; 3,000 retained selections update in about 7.6 ms. |
| PERF10 Search | Fixed | Bounded NDJSON result pages, batched flushes, recurring heartbeat, cancellation and conservative permission pruning. |
| PERF11 Upload conflict preflight | Fixed | Path-only, count/byte-bounded batches with cancellation and failure handling; actual destination checks preserve overwrite safety. |
| PERF12 Unused content reads | Fixed | Response mode chosen first, streamed raw bodies and metadata-only requests; checksum read-count test confirms one content pass. |
| PERF13 Archive enumeration | Fixed | Cancellable bounded manifest, deduplicated sources, closed enumeration handles and preserved safe archive names. |
| PERF14 Bounded disk cache | Fixed | Byte/count/age quotas, bounded bookkeeping, atomic writes, seekable reads, startup cleanup and concurrency/restart tests. |
| PERF15 Static delivery | Fixed | Brotli/gzip quality negotiation, compressed CSS, direct originals and versioned immutable assets; protocol/CSP tests pass. |
| PERF16 Optional frontend imports | Fixed | Generated 3.59 KB theme metadata; Markdown/math and TIFF decoder defer loading; production preview/editor tests pass. |
| PERF17 Parsed HTML templates | Fixed | Template parsed once per handler, fresh request data/nonces, duplicate inner auth lookup removed; nonce tests pass. |

The [completion report](2026-11-17-performance-completion.md) records before/after results, validation and remaining architectural costs. [Operational limits and API behavior](../performance.md) describe pagination, admission limits and compatibility.

