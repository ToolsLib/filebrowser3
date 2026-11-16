# Frontend maintenance and browser support

The frontend uses pnpm (the exact version is in `frontend/package.json`) and Node 24. Do not introduce a second npm/yarn lockfile. Use `pnpm install --frozen-lockfile` for reproducible installs.

The November 2026 update migrates VueUse to 15, Pinia to 4, CSV Parse to 7, TypeScript to 6, and Vitest to 5, alongside routine updates of Vue, Router, Vite, i18n, DOMPurify and the tooling. Node type definitions stay on the Node 24 line. TypeScript 7 is outside the declared support range of Vue's type checker and typescript-eslint. Video.js stays on patched 8.24.2 because the maintained mobile UI plugin declares Video.js 8 support. KaTeX stays on 0.18, within the extension's supported peer range. These compatibility decisions require reevaluation when the dependent tools add support.

The remaining inherited transitive advisories were addressed with explicit pnpm overrides for Rollup, esbuild, xmldom and KaTeX. They are tested through a production build, unit tests, and browser exercises of Markdown, EPUB/XML parsing and media. The saved audit snapshots distinguish the original vulnerabilities, intermediate upgrade state, and final zero-advisory tree. Review overrides during each dependency update; remove them when parents resolve patched versions themselves.

Production browser support follows Vite's modern baseline: Chrome/Edge 111+, Firefox 114+, Safari 16.4+. Legacy output was removed. This also removes the legacy detector's inline/data-script requirements from CSP. The [Vite browser compatibility documentation](https://vite.dev/guide/build.html#browser-compatibility) defines these limits. Update browser data through normal lockfile maintenance; no separate legacy polyfill bundle is shipped.

Only English translations load with the application. Other message and date locales load on selection, with mappings for Norwegian, Portuguese and Swedish and protection against out-of-order language switches. Settings, preview types and the editor load on demand. The initial application chunk is about 188 KB (64 KB gzip), compared with the original review baseline's 792 KB application plus 829 KB translations. Video.js remains a large optional chunk (655 KB/188 KB gzip) and loads when opening media. These are build measurements, not a total page transfer budget.

The [UI modernization review](reviews/2026-11-16-ui-modernization.md) records the design, component and dependency decisions. System fonts replace the bundled Roboto faces. Native image loading, number inputs and progress replace small plugins and copied component code; existing formatting utilities are shared. Five direct dependencies were removed without adding a UI framework. New common controls should use native semantics and the existing CSS tokens before introducing another package.

Ace modes, themes and workers are emitted under `/static/ace/` from the locked package, including compressed variants. The editor uses the application's configured static path; no CDN is required. Ace strict CSP mode is enabled. The HTML response uses a unique nonce and `Cache-Control: no-store`; the policy wraps the router so SPA fallbacks are covered. Styles retain inline support for Vue/Ace layout. Uploaded raw content retains the additional `script-src 'none'` policy. reCAPTCHA adds only the configured HTTPS origin and required Google resource origins when enabled; it remains an optional network integration.

Checks from `frontend`:

```text
pnpm install --frozen-lockfile
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm audit
pnpm exec playwright install chromium
pnpm exec playwright test
```

Build the Go binary in the repository root after the frontend build and before Playwright (`go build -o filebrowser.exe .` on Windows, `go build -o filebrowser .` on Linux). Browser tests start a disposable loopback server under a nested base path and use fixture-only credentials/database. Alongside editor, upload, preview and CSP regressions, they check keyboard/dialog behavior, mobile/tablet layouts, themes, right-to-left settings, public sharing and drag-and-drop. UI screenshots are saved under `build/ui-screenshots`. Test output is ignored by Git.
