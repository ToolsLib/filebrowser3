# File Browser 3 repository guidance

This is ToolsLib/filebrowser3, an independent fork of archived upstream File Browser v2.63.23.

- The application release line is v1.x; the inherited Go module path remains `github.com/filebrowser/filebrowser/v2` as an internal build identity.
- Preserve filesystem confinement, alias-aware authorization, session revocation and durable upload staging.
- Keep command execution and external symlinks disabled by default. Do not remove safeguards to improve benchmarks.
- Use Go 1.27.4 for normal builds; Go 1.26.11 is the tested minimum. Frontend tooling is pinned in frontend/package.json.
- Run appropriate checks from CONTRIBUTING.md and retain Linux race coverage for filesystem and upload changes.
- Publishing is restricted to the ToolsLib repository and Docker Hub `toolslib/filebrowser3`. See docs/releases.md.
- Release candidates never update `latest`. Keep LICENSE, NOTICE and upstream attribution.
- Never commit local bootstrap secrets, databases, private file data or tokens.
- Report vulnerabilities privately through this fork's GitHub security reporting channel, not the upstream issue tracker.
