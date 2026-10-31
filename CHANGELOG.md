# CHANGELOG

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
