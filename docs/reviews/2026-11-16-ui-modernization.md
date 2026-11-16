# UI review and modernization

The file workspace now uses a restrained neutral palette, one indigo accent, system typography, compact file cards and rows, and consistent borders and spacing. The implementation simplifies existing Vue components and CSS without adding a UI framework. Existing files, accounts, share links and saved view preferences are preserved.

## Review and changes

| Previous behavior | Result |
| --- | --- |
| Large colorful file icons, heavy shadows and little page hierarchy | Smaller file icons, quiet surfaces, clear folder titles, item counts and section labels |
| Navigation had no persistent active state | Active navigation, a distinct account entry and create actions shown in the file workspace |
| Primary operations relied on icons | Labeled upload action on desktop and mobile; secondary actions collapse into a menu on tablets and phones |
| Empty folders showed a sad-face message | A clear empty state with an upload action for users with create permission |
| Native outlines were globally suppressed | Visible keyboard focus and reduced-motion support |
| File rows and sorting controls were focusable but lacked keyboard activation | Enter opens files/folders, Space selects, and Enter/Space operate sorting |
| Dialogs accumulated global Escape listeners and allowed focus to leave | Lifecycle-managed keyboard handling, dialog names, focus containment and focus restoration |
| Dialogs used positive tab indices | Natural tab order, with native buttons for upload and checksum actions |
| Drag-and-drop read component instances attached to every DOM node | Explicit destination URLs and drag data, separate handling of external uploads, canceled-drag cleanup, and removal of the global Vue mixin and its DOM type extension |
| JavaScript inspected and mutated stylesheet rules to size cards | Responsive CSS, with no stylesheet-inspection utility |
| Login relied on placeholders | Persistent labels, autocomplete hints and native required-field validation |
| Profile preferences had text that could not toggle the checkbox | Clickable checkbox labels; labeled password and language controls |
| An unset editor theme appeared as a blank select | An explicit automatic appearance option |
| Share downloads had a large metadata/QR panel | A focused file card with download/open actions; QR codes expand on request; guest pages omit account navigation |
| User/share tables occupied half the available desktop width | Full-width administration tables |

Mobile navigation becomes inert while closed, menus support Escape, and layout uses logical positioning for right-to-left languages. New interface copy is provided in English and French; other locales use the existing English fallback for the added strings. Existing translations remain available.

## Dependency decisions

Direct runtime dependencies decrease from **31 to 26**. No replacement packages were added.

| Removed dependency | Replacement |
| --- | --- |
| `@chenfengyuan/vue-number-input` | Native labeled number inputs with minimum, maximum, integer step and validity checks |
| `vue-lazyload` | Native image lazy loading and asynchronous decoding, with an icon fallback for failed thumbnails |
| `pretty-bytes` | The existing binary `filesize` utility, shared with the rest of the application |
| `js-base64` | Filename strings as Vue list keys; encoding was unnecessary |
| `@vueuse/integrations` | Removed because the application did not import it |

The copied progress component is replaced by a native progress element. System typography removes 21 bundled Roboto source files and their font-face declarations. Material Icons remains the shared icon system.

Native browser behavior is documented in MDN's references for [number inputs](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/input/number), [image lazy loading](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/img#loading) and [asynchronous image decoding](https://developer.mozilla.org/en-US/docs/Web/API/HTMLImageElement/decoding). Client-side number validation complements the existing server validation.

The remaining libraries have substantial responsibilities:

| Area | Libraries retained and rationale |
| --- | --- |
| Application foundation | Vue, Router, Pinia and Vue I18n provide routing, shared state and translations |
| Editing | Ace supports language modes, workers, themes and editor behavior; a homegrown replacement would be a separate product |
| Document previews | CSV Parse, Marked, DOMPurify, KaTeX and its Marked extension, EPUB.js/Vue Reader, and UTIF support parsing, sanitization, mathematics, books and TIFF images |
| Media | Video.js and its hotkey/mobile plugins preserve established playback controls and mobile behavior |
| Transfers and sharing | TUS supports resumable uploads; QRCode Vue generates share codes |
| Small shared utilities | Day.js, filesize, JWT Decode, Lodash ES and VueUse Core remain used for dates, formatting, authentication payloads, throttling and reactive browser state |
| Presentation infrastructure | Toastification, Material Icons and Normalize CSS remain shared across the app |

Preview/editor libraries remain lazy-loaded. This change does not remove supported preview formats or replace security-sensitive parsers and sanitizers with custom implementations. The existing toast API is also preserved rather than adding another notification state system.

## Build measurements

Production asset measurements from the same local toolchain, before and after this change:

| Asset | Before | After |
| --- | ---: | ---: |
| Initial application JavaScript | 209.0 kB | 187.9 kB |
| Initial JavaScript gzip | 70.9 kB | 64.4 kB |
| Main CSS | 63.5 kB | 55.9 kB |
| Emitted font assets | 20 | 2 |

These are decimal asset sizes, not measured page load times or total application transfer sizes. Optional editor, video and document chunks remain separate.

## Validation and screenshots

The completed checks passed: reproducible frozen-lockfile installation, ESLint, production build and TypeScript checks, 40 unit tests, and four Chromium browser scenarios. The pnpm audit reported zero advisories at all severities. The app-only Docker deployment was rebuilt and verified locally without replacing its data volumes.

Run the standard frontend checks from `frontend`:

```text
pnpm install --frozen-lockfile
pnpm lint
pnpm test
pnpm build
pnpm audit
```

Rebuild the Go executable after the production frontend build, then run `pnpm test:e2e`. The server uses a disposable database and fixture files under `frontend/test-results`; it never modifies the running Docker application's volumes.

The browser suite covers the existing login/session, TUS, editor save, CSV/image/EPUB/video and CSP checks, plus the updated UI: repeated dialog opening and focus restoration, keyboard selection and sorting, native number validation, public downloads, responsive widths from 320 to 1440 pixels, search, dark appearance, and right-to-left settings. Guest file/folder shares and moving a file by drag-and-drop have separate scenarios.

The UI scenario writes screenshots under `build/ui-screenshots`: login, grid/list files, dark appearance, a dialog, public file/folder shares, settings, empty folders, mobile navigation, right-to-left settings and search. Screenshots show fixture data. Browser interaction checks use Chromium; Firefox and Safari have not been separately exercised in this pass.
