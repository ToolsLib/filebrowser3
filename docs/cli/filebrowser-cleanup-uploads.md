# filebrowser cleanup-uploads

Find or remove orphaned upload staging files

## Synopsis

Find orphaned upload journals beneath a root. Dry-run by default. Stop memory-backed servers before applying cleanup; provide the shared Redis URL for a Redis-backed deployment.

```
filebrowser cleanup-uploads [flags]
```

## Options

```
      --apply[=true|false]    remove candidates instead of only listing them
  -h, --help[=true|false]     help for cleanup-uploads
      --older-than duration   minimum journal age (default 24h0m0s)
      --redis string          shared upload Redis URL, required when checking a Redis-backed deployment
      --root string           served filesystem root to inspect (default ".")
```

## Options inherited from parent commands

```
  -c, --config string     config file path
  -d, --database string   database path (default "./filebrowser.db")
```

## See Also

* [filebrowser](filebrowser.md)	 - A stylish web-based file browser

