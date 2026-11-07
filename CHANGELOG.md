# CHANGELOG

## Unreleased

- Evaluate width media ranges (`width >= 1012px`, reversed comparisons and
  same-direction chained bounds) with fractional breakpoints. Restore desktop
  horizontal headers and closed mobile menus; malformed/unsupported ranges
  remain nonmatching, including under negation.

- Reuse intrinsic widths within each layout using a 2,048-entry cache keyed
  by node, basis and style; retain the existing measurement-work ceiling.

- Honor flex/grid display on list containers instead of forcing vertical list
  layout, restoring horizontal component navigation.

- Parse bounded CSS cascade layers with document-wide named/anonymous/nested
  ordering, conditional media ordering, reversed important precedence and
  inline-style priority. Preserve rule/work gates; cap layers at 256 and layer
  declarations at 4,096. Raise the opt-in maximum rule count to 32,768 to admit
  large component stylesheets whose layer blocks were previously skipped.

- Exclude CSS visibility-hidden boxes from link/source hits, keyboard link
  regions and text search; retain visible descendants and transparent links.
  Clip link hit and focus regions to overflow bounds.

- Retain up to 1,024 declarations per rule and 2,048 custom properties per
  computed environment so large theme blocks retain foreground colors.
- Preserve decorative aria-hidden SVG fallback height without emitting labels.
- Treat fixed-position boxes as root-positioned document snapshots outside
  normal flow; they do not remain pinned during viewport scrolling.

- Extend indexed CSS matching to root and positive functional selectors, and
  skip rules with required subject/ancestor keys absent throughout the document.
  Preserve full-scan cascade results and the matching-work ceiling.
- Permit hosts to request up to 16,384 CSS rules (default remains 256).
- Fix recursive intrinsic sizing of anonymous text children in nested flexboxes.

- Index CSS rules by necessary rightmost-subject ID/class/attribute/tag keys,
  preserving source order, specificity, pseudo-elements and media behavior.
  Charge matching work only for candidate rules without raising its ceiling.
- Charge inherited custom-property environments only when a local declaration
  allocates a copy. Shared inherited maps no longer exhaust the computed-entry
  budget on large styled pages; local copies remain bounded.

- Measure nested flex rows using their child widths and gaps, and use intrinsic
  SVG icon dimensions instead of a generic diagram width. Footer links and
  icon-bearing slogans no longer wrap despite sufficient available space.
- Resolve embedded SVG `currentColor` paint attributes and inline paint styles
  from the surrounding HTML color and local SVG color overrides, preserving
  identifiers/references and existing diagram resource gates.

- Match escaped CSS class/ID identifiers, including Tailwind arbitrary-value
  classes, with bounded Unicode escape decoding and escape-aware rule scanning.
- Preserve nested flex/grid layouts when blockifying flex and grid items, fixing
  rows whose labels and links should be separated by `justify-content`.
- Paint rounded block backgrounds and uniform solid borders with `border-radius`
  shorthand/longhands, elliptical/percentage radii, and viewport-local masks.
  Curved nonuniform/dashed borders and rounded descendant clipping remain outside
  the supported subset. Add a colored-row fixture and pixel/cascade/band tests.

## v0.5.0 (2026-11-06)

This release improves CSS style and content preservation in the shared
HTML-to-nttypeset reading path, with independent Chromium probes and a documented
subset roadmap.

**Adoption:** Upgrade to this version to receive the fixes through `typeset.Build`,
`BuildWithSources` and `BuildChunked`. The bridge now uses published nttypeset
v0.3.0. Eligible tables automatically carry per-cell styles; existing CSS page
rendering and host-painted viewport APIs remain available. This release does not
switch any host's renderer.

**Limits:** This is not full HTML/CSS conformance. Styled tables must be rectangular,
without spans or nested block structure, top-aligned, with uniform cell font
size, separate borders, zero border spacing and transparent/opaque cell
backgrounds. Only solid cell borders are supported. Other tables retain the existing permissive reading fallback.
Column sizing, margin collapsing, percentage image caps and very tight line
heights remain gaps. See [the supported scope and migration gate](docs/typeset-subset.md).

 * Preserve cell padding, backgrounds, per-side solid borders and horizontal
   alignment on supported tables, including empty cells and single-row grids,
   in complete and chunked typesetting.
 * Honor CSS font weight in headings and table cells, including inline overrides;
   ordinary cells in header groups are no longer forced bold. Uncolored headings
   use the document text color instead of an automatic accent.
 * Preserve inherited heading line heights and explicit/inherited preformatted
   line spacing. Retain explicit vertical margins on preformatted blocks, list
   containers and simple list items, including zero and small item margins.
 * Remove extra reading-mode spacing inside bordered headings and paragraphs;
   explicitly declaring zero padding no longer changes their layout.
 * Apply positive absolute image `max-width` and `max-height` caps while retaining
   aspect ratio. Preserve standalone image-only runs between blocks and respect
   `display:none` on images and enclosing links.
 * Keep paragraph, link and image fragment IDs together on image-only blocks,
   including chunked navigation and alt-text fallback.
 * Add a separate typeset-vs-Chromium differential report and reviewed status
   baseline, plus focused CSS paint/style probes. Unsupported prerequisites and
   incomplete measurements remain explicit; native-renderer results are separate.

## v0.4.0 (2026-11-05)

This release adds opt-in CSS-preserving viewport painting without a resident whole-page raster.

**Adoption:** Existing viewers retain their behavior. Hosts opt in with `ModelOptions.HostPainted`, render with `NoImage`, and install the layout plus its first painted band atomically with `SetPaintedResult`. Use automatic paint scale (`Scale: 0`). Route subsequent `PaintRequest` messages through a serial worker with one reusable `document.Painter` per layout revision, deadlines, and cancellation. See the [integration contract](docs/host-painted.md).

**Limits:** DOM and CSS layout remain eager and bounded. Host-painted mode supports width fit, retains the existing zoom range, and requires immutable results/assets. Production CJK font throughput still needs host benchmarking before ntbrowser rollout. This release does not itself enable the new path in ntbrowser.

 * Fix host-painted TTY frame replacement, reconcile stale install geometry, retain sub-row pan input, and resize retained bands while new column geometry is painting. Initial plans now use automatic paint scale in the integration guide and example.

 * Add opt-in host-painted CSS viewports: atomic layout/first-band installation, cancellable revisioned paint requests, bounded band overlays, global hit-testing, and stable Kitty transport. Existing viewer APIs keep their behavior.
 * Add reusable `document.Painter` with bounded font caches and a y-bucketed display-operation index. See [the host integration contract](docs/host-painted.md) for ownership, worker scheduling, limits, and ntbrowser adoption.

## v0.3.0 (2026-11-05)

This release adds bounded, lazy HTML-to-nttypeset chunks for long documents and conversation histories.

**Adoption:** Existing `Build` and `BuildWithSources` callers need no changes. To opt in, use `typeset.BuildChunked` with `Engine.NewChunked` / `At`, resolve fragments with `ChunkRange`, and retain the chunk index alongside selections and anchors. `ChunkWithSources` returns a chunk-local source map.

**Limits:** Parsing remains whole-document. Oversized indivisible units return errors without truncation; ancestor section decoration repeats across chunks, and hosts manage cross-chunk selection. This does not change ntbrowser's separate CSS renderer or its long-page limits.

 * Upgrade `nttypeset` to `v0.2.0` and add opt-in `typeset.BuildChunked`, implementing its `ChunkSource` contract with lazy semantic subtree extraction/highlighting, bounded assets and source indexes, and chunk-local source maps. Existing complete-document adapters remain unchanged.

 * Preserve fragment navigation, inherited styles, sections, links, images, and stable source IDs across chunks. Add coverage for 33,000-message histories, distant access, cache eviction, cancellation, oversized units, asset bounds, and compatibility with the complete-document adapter.

## v0.2.3 (2026-11-05)

 * Upgrade `nttypeset` to `v0.1.9`, `ntfonts` to `v0.3.1`, `ntpicture` to `v0.1.5`, `ntsvg` to `v0.6.0`, and indirect dependency `terminfo` to `v1.3.1`.

 * Use upstream Bubble Tea for native builds and isolate the WASM-capable fork in `wasm.work`. Browser build scripts, tasks, and CI select this workspace automatically. Manual browser builds must set `GOWORK="$PWD/wasm.work"`; downstream WASM applications still need their own fork replacement.

 * Expand fuzz coverage for TTY search and selection, SVG sanitization, and HTML-to-nttypeset source mapping alongside HTML rendering. Add configurable `task fuzz` campaigns and run each target for 10 seconds with two workers in CI. Include the `typeset` package in WASM tests.

## v0.2.2 (2026-10-31)

 * Upgrade `nttypeset` to `v0.1.8` and `ntfonts` to `v0.2.1` for source fallback family support.

 * Preserve page scale and letterbox background pixels in Kitty focus overlays at the top, bottom, left, and right edges. Focus patches now render complete destination cells instead of stretching a clipped source region, for both link and source focus.

## v0.2.1 (2026-10-31)

 * Add `Model.ShowFocusSource(id)` to reapply source focus without changing scroll, selection, or search after a frame replacement. `FocusSource` now scrolls only when the element is not fully visible, in both Typeset and TTY modes.

 * Add `Model.FocusSource(id)` to highlight and scroll to any element using its source boxes and text runs. Source and link focus share the existing raster ring and are mutually exclusive. TTY source text runs use reverse video; elements without mapped TTY text have no visible highlight. An empty ID clears focus, unknown IDs leave the view unchanged, and both result setters clear source focus for hosts to reapply.

## v0.2.0 (2026-10-31)

This minor release expands HTML element rendering, adds styled TTY output and coverage-aware font fallback, and improves SVG rendering and viewer navigation.

**Migration:** `task build` now produces the terminal demo at `bin/nthtml-demo`; update scripts that invoke `bin/demo`. The HTML-to-PNG CLI remains `bin/nthtml`.

 * Default nested unordered-list markers to circle at the second level and square at deeper levels, counting enclosing lists through intervening elements. Preserve author styles, inherited author values, and HTML `type` overrides consistently in raster, TTY, Blocks, and ExtractTypeset output.

 * Default buttons, selects, and button-like inputs to `box-sizing: border-box`, while preserving author overrides and content-box text inputs. Explicit control heights now include padding and borders, preventing sprite backgrounds from exposing adjacent rows.

 * Use finer viewer zoom steps: each step increases magnification by about 19%, and four steps double it. Rename the terminal demo binary produced by `task build` to `bin/nthtml-demo`.

 * Skip and cache oversized fallback faces (over 4 MiB) without stopping later candidates. The shared 16 MiB total, lookup, and face-count budgets still stop further loads; primary face limits still return ErrLimit.

 * Add Model.SetTypesetFit with width/scroll and whole-page contain modes, preserving text-index navigation, search, and selection. The TTY comparison demo toggles the Typeset fit with f and shows the active mode in its header.

 * Add CSS foreground/background colors and bold/italic/underline/strikethrough to TTY ANSI output, with plain-text style spans, inherited backgrounds, safe OSC 8/highlight composition, explicit color profiles, and a side-by-side Typeset/TTY demo. Plain text, source mappings, and search/selection coordinates remain escape-free. See docs/tty-styling.md.

 * Use one cached, source-aware fallback path without redundant bundled lookups. Try configured bundled text families after symbol fallback and before Go/Go Mono, preserving primary faces and run variants. Serif/sans order follows the primary face; monospace runs try the configured monospace face then Iosevka. Default Vietnamese, currency, and monospace box drawing now use the widened ntfonts coverage within existing font budgets.

 * Upgrade `ntfonts` to `v0.2.0` for expanded text coverage and bundled Noto symbol fallback. Hosts can continue to opt into the separate CJK module through Options.FallbackFaces.

 * Add coverage-aware raster font fallback: remaining CSS families, host-supplied Options.FallbackFaces, ntfonts.Fallbacks(), then Go/Go Mono. Missing variants try Regular; combining marks stay with their base. Layout, ink bounds, and repaint share the selected face runs and the primary baseline.
 * Add Result.MissingGlyphs for characters lacking glyphs after fallback, without status-line warnings. Coverage and run-split caches are render-local and bounded; fallback font loads share the 128-face, 64-family, and font-byte budgets and degrade gracefully on exhaustion. TTY and text export retain their original text.

 * Preserve embedded-element geometry with bordered inline placeholders for sized iframe/video/canvas/object/embed elements and audio controls. Hide unsized audio without controls, suppress sized fallback content, and never load embedded resources.

 * Draw attribute-driven inputs, password masks, checkbox/radio states, buttons, selected options, clipped textareas, fieldset frames/legends, and progress/meter bars. Author CSS overrides the defaults. Controls expose whole-control bounds in existing Result.Boxes with Tag and SourceID (no dedicated Controls slice); Document.Sources includes generated nthtml-control-N IDs for controls without an HTML id. No editing state or resource fetching is added.

 * Render closed details with only the first summary, disclosure markers and a default summary; hide closed dialogs and give open dialogs a padded border.

 * Add phrasing font and decoration defaults, nested curly quotes through generated content, and inline parenthesized ruby annotations (with rp hidden). Abbreviations use a plain underline.

 * Honor the hidden attribute at UA priority, except hidden=until-found; author display overrides it.

 * Default unknown and phrasing elements to inline, with explicit HTML block and non-rendering defaults; author display declarations still win.

 * Upgrade `ntsvg` to `v0.5.5`, `ntpicture` to `v0.1.4`, `nttypeset` to `v0.1.7`, and `oksvg` to `v0.1.6` for SVG image support and rendering fixes, including rounded SVG rectangles with only `rx` or `ry` specified. Update indirect dependencies `regexp2/v2` to `v2.12.0` and `terminfo` to `v1.3.0`.

 * Fix Kitty band coordinates for `CellToDocument`, link hit-testing, source lookup, and mouse selection across band transitions, zoom, and letterboxing. Add `Model.DocumentViewport()` for the visible document-pixel bounds. Clarify that `SetResultKeepView` clears selection and active mouse drags while preserving scroll, zoom, search, and link focus.

 * Add opt-in SVG raster images through `Options.SVGImages` (or `DefaultDiagramRenderer.Images`), with explicit decoded-resource allowlists, bounded dimensions, transforms, opacity, aspect-ratio fitting/cropping, and `svg-image` source discovery. No implicit resource loading.

 * Render supported inline SVG content while skipping unsupported subtrees, with per-element warnings and unchanged safety limits.

## v0.1.2 (2026-10-28)

 * **Banded Kitty virtual-placement scrolling & focus-ring overlay (`Model`)**:
   * Size Kitty virtual placements to the typeset page height in cells (in bands of up to 283 rows with half-viewport hysteresis) and scroll within a band via `Picture.SetViewport` (`0` bytes of Kitty APC traffic per keypress).
   * Slice page bands via zero-copy `SubImage` views so `ntpicture` `v0.1.2` `FrameEdits` dirty-rectangle updates (`a=f,r=1,X=1`) apply to in-place band updates (`SetResultKeepView`, search highlights, and selection) and unchanged updates emit `0` bytes.
   * Render the active link focus ring in a separate transparent `1×2` cell `ntpicture.Picture` (`z=1`) spliced over `Picture.View()` so `Tab` / `Shift+Tab` link cycling updates without re-encoding or re-uploading the page band.
   * Add `Model.SetResultKeepView` (preserving scroll, pan, zoom, search state, and focused link across re-renders; selection is cleared) and `Model.CellToDocument` (viewport-aware terminal cell to document CSS pixel mapping).
 * **Dependency upgrades**:
   * Upgrade `nimbleterminal.dev/ntpicture` to `v0.1.2`, `nimbleterminal.dev/nttypeset` to `v0.1.6`, `nimbleterminal.dev/ntsvg` to `v0.5.3`, `charm.land/bubbletea/v2` to `v2.1.1` (and matching WASM Bubble Tea pin), `github.com/NimbleMarkets/oksvg` to `v0.1.3`, and pin `github.com/srwiley/rasterx` to `github.com/neomantra/rasterx v0.1.0`.
 * **Link anchor element tracking (`Link.Anchor`)**:
   * Expose `Link.Anchor *html.Node` on `document.Link` and `nthtml.Link` and preserve clickable link hit regions for `<a href>` elements wrapping empty-`alt` images with explicit dimensions.
 * **CSS custom properties & `prefers-color-scheme`**:
   * Support `--*` custom property declarations with cascade inheritance, `var(--name, fallback)` substitution (cycle-safe up to depth 16), and `@media (prefers-color-scheme: light | dark)` via `Options.ColorScheme` (`ColorSchemeLight`, `ColorSchemeDark`).
 * **Functional (`:is`, `:where`, `:has`, `:not`) & state pseudo-classes**:
   * Support forgiving `:is(...)` (taking maximum matched argument specificity) and `:where(...)` (zero specificity), selector-list `:not(s1, s2, ...)`, bounded `:has(...)` (including `>`, `+`, `~` relative selectors), and state pseudo-classes (`:checked`, `:disabled`, `:enabled`, `:required`, `:optional`, `:read-only`, `:read-write`).
 * **CSS math functions, viewport units, `opacity`, `aspect-ratio`, & `object-fit`**:
   * Support `calc(...)`, `min(...)`, `max(...)`, and `clamp(...)` over lengths and percentages; viewport units (`vw`, `vh`, `vmin`, `vmax`) against `Options.Width × Options.Height`; element/subtree `opacity` (`0..1`); `aspect-ratio: <w> / <h> | <n> | auto`; and `object-fit: fill | contain | cover | none | scale-down` on `<img>` and `<svg>`.
 * **CSS Grid layout (`display: grid | inline-grid`)**:
   * Support `display: grid` and `display: inline-grid` with `grid-template-columns` (`px`, `%`, `em`/`rem`, `fr`, `auto`, `minmax(min, max)`, `repeat(N, ...)`), `grid-column` / `grid-column-start` / `grid-column-end` (`span N`, `start / end`), and row/column `gap`.
 * **Background images, CSS gradients, & background sizing/positioning**:
   * Support `background-image` (`url(...)` via `Options.Images`, `linear-gradient(...)`, `radial-gradient(...)`) and `background` shorthand extraction along with `background-size` (`auto`, `cover`, `contain`, explicit lengths/percentages), `background-position`, and `background-repeat` (`repeat`, `repeat-x`, `repeat-y`, `no-repeat`).

## v0.1.1 (2026-10-26)

 * **CSS at-rules & `@media` queries**:
   * Skip statement at-rules (`@import`, `@charset`, `@namespace`) up to `;` and non-`@media` block at-rules (`@font-face`, `@keyframes`, `@supports`, `@layer`, `@page`) with brace matching (`@supports` bodies are skipped, not evaluated).
   * Parse nested `@media` blocks (up to depth 8, 1,024-byte prelude) and evaluate media query lists (`screen`, `all`, `not`, `only`, `and`, `,`, and `width`/`min-width`/`max-width` in supported CSS length units) at render and TTY layout time (`columns * 10` px in TTY mode) while counting all parsed rules against `Limits.Rules`.
 * **Dynamic pseudo-classes in selector lists**:
   * Accept `:visited`, `:hover`, `:active`, and `:focus` (without arguments) in selector parsing so they never match without dropping the rest of a comma-separated selector list.
 * **HTML presentational attributes**:
   * Map HTML Standard (§15.3) presentational hints below author CSS rules (including specificity `0,0,0`): `bgcolor` (including legacy hex without `#`), `align`, `valign`, `cellpadding`, `cellspacing`, `width`/`height` on `table`/`td`/`th`/`tr`, `border="N"` on `table`, `<center>`, `<font color/size/face>`, and `<body text/link>`.
 * **Images with empty or missing `alt`**:
   * Omit `[image: ]` fallback text across native, TTY, and `typeset` routes when `alt` is empty or absent; reserve an empty `width`×`height` box when both dimension attributes are present so spacer images work in layout.
 * **Inline whitespace, kerning, & link regions**:
   * Measure whole words with OpenType kerning, collapse whitespace across inline element boundaries without extending link/underline styles into trailing spaces, and merge contiguous link hit regions on a line.
 * **CSS layout expansion & WPT conformance (`187 PASS · 0 FAIL`)**:
   * Support `visibility: visible | hidden | collapse`, `border-style: hidden`, `line-height: normal` and `font` shorthand `/line-height`, CSS-wide keywords (`inherit`, `initial`, `unset`), and collapsed table border conflict resolution, advancing the pinned 200-test WPT baseline from 177 to **187 PASS · 0 FAIL**.
 * **Canvas background, section styling, source anchors, & `nttypeset` bridge**:
   * Propagate root/body backgrounds to the canvas (`Result.Background()`).
   * Preserve nested container section backgrounds, padding, borders, and source-element anchor mappings (`Document.Sources`, `Document.ParentID`, `Document.RootID`, `Result.SourceAt`, `TTYResult.SourceAt`, `Model.NavigateSource`, `Model.ActiveSourceID`, `Document.ExtractTypeset`, `typeset.BuildWithSources`, `typeset.RenderResult`, and `cmd/nthtml -mode typeset`), including layout-table container decomposition and presentational attributes.
 * **Chromium differential conformance harness (`task differential`)**:
   * Add an optional offline headless-Chromium differential harness (`internal/conformance`, `cmd/nthtml-differential`) comparing `nthtml` `Result.Boxes` geometry, document dimensions, and `800×600` screenshots against Chromium `getBoundingClientRect()` across 20 representative WPT fixtures (`artifacts/differential/index.html` and `report.json`).

## v0.1.0 (2026-10-25)

 * Initial release of `nimbleterminal.dev/nthtml`: a bounded, pure-Go (`CGO_ENABLED=0`) HTML + CSS document renderer and Bubble Tea v2 viewer for native terminals and WebAssembly (`js/wasm`).
 * **Dual rendering modes**:
   * **Typeset**: pixel layout and rasterization (`document.Render`, `Result.Image`, `Result.Paint`) presented in terminals via `ntpicture` (Kitty graphics with half-block glyph fallback) and in browsers on `<canvas>`.
   * **TTY**: terminal-cell document layout (`Document.TTYLayout`, `Document.TTY`) with wrapped Unicode tables, lists, headings, code blocks, and OSC 8 hyperlinks.
 * **Tiled & high-DPI rendering (`Options.NoImage`, `Result.Paint`, `Result.Bounds`, `Result.PageBreak`)**:
   * Lay out long documents without allocating a full-page image (`Limits.LayoutHeight` defaults to 262,144 CSS px and can be raised up to 1,048,576 px via `MaxLimits`), and paint arbitrary viewport bands at 1/16× to 16× device scale.
   * Configurable `Limits` (raisable up to `MaxLimits`: 16 MiB input, 200,000 nodes, 4,096 CSS rules), `Options.FontFamilies` (`Default`, `Serif`, `SansSerif`, `Monospace`), and `Options.Colors` (`Background`, `Text`, `Link` fallback for unstyled pages).
 * **HTML & CSS subset**:
   * Selectors: tag, `.class`, `#id`, `*`, `[attr]` (`=`, `~=`, `|=`, `^=`, `$=`, `*=`, `i`), compound, descendant, child (`>`), adjacent/general sibling (`+`, `~`), structural pseudo-classes (`:root`, `:first-child`, `:last-child`, `:only-child`, `:empty`, `:nth-child`, `:not`), and bounded `::before`, `::after`, and `::first-line` pseudo-elements.
   * Box model & layout: `margin` (including negative margins and vertical margin collapse), `padding`, per-side `border` (`solid`, `dashed`, `dotted`, `double`, `hidden`, `none`), `box-sizing`, `width`/`height` + `min-*`/`max-*`, `display: block | inline | inline-block | flex | table | none`, `overflow: hidden`, `text-overflow: ellipsis`, `position: relative | absolute` with integer `z-index`, and `float: left | right` + `clear`.
   * Typography, tables, & media: embedded `ntfonts` (`Options.Fonts`, `document.MinimalFonts`), `font` shorthand, `line-height`, `word-spacing`, `letter-spacing`, `text-indent`, `text-transform`, `font-variant: small-caps`, `text-align: left | right | center | justify`, `white-space`, `overflow-wrap`, content-sized tables (`colspan`, `rowspan`, `colgroup`/`col`, `border-collapse`, `border-spacing`, `caption-side`), allowlisted `Options.Images`, and pure-Go inline `<svg>` + Mermaid diagrams via `ntsvg`.
 * **Navigation, search, & per-character selection**:
   * Local `#fragment` anchors, case-insensitive whitespace-collapsing `TextIndex.Search`, per-character `TextIndex.Hit` and `TextIndex.Regions` (`TextRun.CharX`) in both Typeset and TTY modes, and OSC 52 / browser clipboard copy.
 * **Ecosystem & conformance**:
   * Optional `nimbleterminal.dev/nthtml/typeset` adapter (`typeset.Build`, `typeset.FromResult`, `typeset.Raster`) bridging into `nttypeset.Document`, plus `nthtml.NewWithOptions` (`ReflowTypeset`, multi-widget Kitty sessions/placements).
   * Pinned offline Web Platform Tests (WPT) conformance suite (`task conformance -- -strict`: **177 PASS · 0 FAIL** across all runnable visual reference tests) and 10 live WebAssembly demos hosted on GitHub Pages.
