# CHANGELOG

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
