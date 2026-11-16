# Upload Integrity and Recovery

Direct uploads and editor saves write a private sibling file, sync and close it, and rename it over the destination only after the write succeeds. Failure cleanup removes that temporary file and never recursively deletes the destination. Overwriting a directory is rejected.

Resumable TUS uploads use a private staging file and journal until every declared byte arrives. An existing destination remains readable throughout the transfer. The upload URL contains an `uploadID`; HEAD, PATCH, and DELETE must use the Location returned by POST. These URLs are generation-specific and are authorized against the uploading user. Legacy pending uploads from earlier versions cannot resume after this upgrade.

The server checks the destination's recorded size and modification time before publishing a replacement. If another operation changed or created the destination while the upload was pending, publication returns a conflict and preserves that destination. This check detects ordinary concurrent edits; it does not provide filesystem snapshots or detect an external actor deliberately restoring the same size and timestamp.

Canceling an upload removes only its staging files. The user needs Create permission and the matching upload identity; Delete permission on the final file is unnecessary because cancellation does not delete the final file. Staging files beginning with `.filebrowser-write-` are reserved and excluded from API reads, shares, and listings.

## Instances and shared storage

The memory cache supports one server process. Redis stores upload generations shared by multiple instances. Registration is atomic, state failures reach the caller, and completion/heartbeat operations compare generation IDs so delayed requests cannot affect replacements.

Concurrent chunks are serialized with an OS advisory lock beside their staging file, including requests handled by different processes. A process crash releases the OS lock. Multi-instance deployments must mount shared storage at the same absolute paths and use a filesystem whose advisory locking works across those instances. Redis alone does not make independent filesystems or storage without distributed advisory locks safe. Test the lock behavior of the intended storage before using multiple replicas.

## Expiry and outages

Upload state expires after three minutes without activity. Transfers refresh their state while active. The memory backend removes an expired generation's private staging files. Redis cleanup rechecks shared state so activity on another replica prevents removal; a Redis outage postpones cleanup instead of deleting uncertain state.

If registration or a required state operation fails, the server does not publish a replacement. A failed cache deletion after a successful final rename is logged; it cannot undo the committed file. Cleanup always targets the generation's staging names. It never removes the published destination. Completion hooks run once after publication; hook failures are logged rather than reporting a completed transfer as an uncommitted write.

A restart of a memory-backed server loses pending upload state. Redis-backed pending uploads can resume on another instance while their state is alive. Crashes can leave private journals and staging files without a cleanup callback. Use the cleanup command to collect these orphans.

## Collecting orphaned staging files

The cleanup command lists candidates by default. It recognizes bounded JSON journals and only considers their sibling staging files. It ignores destination paths stored in journals. Held OS locks and active Redis generations prevent removal; Redis errors stop the operation.

Stop memory-backed servers before applying cleanup. For Redis deployments, always provide the same Redis URL used by the servers so the command can check live state. Keep credentials in the existing deployment's protected configuration or environment when invoking administrative commands; do not commit credential-bearing shell history or scripts.

```text
filebrowser cleanup-uploads --root /srv --older-than 24h
filebrowser cleanup-uploads --root /srv --older-than 24h --apply
filebrowser cleanup-uploads --root /srv --older-than 24h --redis redis://127.0.0.1:6379 --apply
```

The minimum journal age is one hour. Inspect the dry-run output before applying it. Unknown or malformed journals are left for manual investigation rather than treated as deletion instructions.
