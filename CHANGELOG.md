# CHANGELOG

## Unreleased

 * **CSS at-rules & `@media` queries**:
   * Skip statement at-rules (`@import`, `@charset`, `@namespace`) up to `;` and non-`@media` block at-rules (`@font-face`, `@keyframes`, `@supports`, `@layer`, `@page`) with brace matching (`@supports` bodies are skipped, not evaluated).
   * Parse nested `@media` blocks (up to depth 8, 1,024-byte prelude) and evaluate media query lists (`screen`, `all`, `not`, `only`, `and`, `,`, and `width`/`min-width`/`max-width` in supported CSS length units) at render and TTY layout time (`columns * 10` px in TTY mode) while counting all parsed rules against `Limits.Rules`.
 * **Dynamic pseudo-classes in selector lists**:
   * Accept `:visited`, `:hover`, `:active`, and `:focus` (without arguments) in selector parsing so they never match without dropping the rest of a comma-separated selector list.
 * **HTML presentational attributes**:
   * Map HTML Standard (§15.3) presentational hints below author CSS rules (including specificity `0,0,0`): `bgcolor` (including legacy hex without `#`), `align`, `valign`, `cellpadding`, `width`/`height` on `table`/`td`/`th`/`tr`, `border="N"` on `table`, `<center>`, `<font color/size/face>`, and `<body text/link>`.
 * **Images with empty or missing `alt`**:
   * Omit `[image: ]` fallback text across native, TTY, and `typeset` routes when `alt` is empty or absent; reserve an empty `width`×`height` box when both dimension attributes are present so spacer images work in layout.
 * **Inline whitespace, kerning, & link regions**:
   * Measure whole words with OpenType kerning, collapse whitespace across inline element boundaries without extending link/underline styles into trailing spaces, and merge contiguous link hit regions on a line.
 * **Canvas background & `nttypeset` bridge**:
   * Expose `Result.Background()`, `Document.ExtractTypeset`, and `typeset.BuildWithSources` for nested section decorations and source-anchor mapping.

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
