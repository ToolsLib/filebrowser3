# Performance review probes

Run these from the repository root after installing the frontend dependencies. The probes generate their own fixtures; they do not benchmark or modify the running Docker instance. Results describe the current source and machine, not performance targets.

## Backend listings

```powershell
$env:GOMAXPROCS = '2'
go test -p 1 ./files -run '^$' -bench BenchmarkPerformance -benchmem -benchtime=150ms -count=1
```

`BenchmarkPerformanceListing` isolates file-type/subtitle expansion in an in-memory filesystem. `BenchmarkPerformanceNativeListing` uses an actual scoped filesystem and optionally a checker that resolves each path; it does not substitute for the complete HTTP authorization pipeline. Benchmark-generated native files live in Go's temporary directories.

To record a CPU profile on Windows, quote flags containing output paths and use absolute paths:

```powershell
go test ./files -run '^$' -bench 'BenchmarkPerformanceListing/video-subs/1000$' -benchtime=2x '-cpuprofile=C:/path/to/project/build/listing.cpu' '-o=C:/path/to/project/build/files.test.exe'
go tool pprof -top C:/path/to/project/build/files.test.exe C:/path/to/project/build/listing.cpu
```

## Thumbnail work amplification

```text
go test -tags performance_review ./http -run TestPerformancePreviewColdConcurrency -v -count=1
```

This explicit opt-in probe sends eight concurrent requests for one thumbnail with a cold/no-op cache and a counted fake image service. It reports resize invocations and verifies successful responses. The service deliberately waits for concurrent calls, with a bounded timeout, so this is not an image-generation latency benchmark.

## Frontend selection

```text
node tools/performance/selection.mjs
```

Node 24 executes the source TypeScript store. The script compares the existing store with no selection, the artificial worst case where every item remains selected, and an unapplied Set-based prototype that performs the complete state update. Each figure is the median of three calls. The normal file fetch flow clears selection first, so the all-selected result must not be described as the cost of every navigation.

## Production browser workflows

Build the frontend and then the embedded Go executable:

```text
pnpm --dir frontend build
go build -o filebrowser.exe .
node tools/performance/browser.mjs
```

On Linux, build `filebrowser` instead. Chromium must already be installed through Playwright. The script runs an isolated server at `127.0.0.1:18091`, using generated files and a disposable database under ignored `build/performance-browser`. It terminates that server after the run; fixture directories are kept for inspection.

The browser probe records complete listing response times and sizes, hypothetical gzip sizes of those exact bodies, retained DOM rows after scrolling, Select All time including two animation frames, CSV node/heap/long-task measurements, and oversized CSV response behavior. Browser and API times include work beyond the targeted component. Heap snapshots are process state at the sampling point, not component-exclusive allocations. Output is written to `build/performance-browser.json`.
