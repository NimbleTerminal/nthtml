# nthtml

**Constrained HTML/CSS documents, not a browser.**

A pure-Go document renderer for [Nimble Terminal](https://nimbleterminal.dev).
Render HTML into pixels and structured boxes, display it with Bubble Tea v2
through [ntpicture](https://github.com/NimbleTerminal/ntpicture), or compile the
same renderer to WebAssembly. No CGO, browser engine, JavaScript execution, or
implicit network/file access.

![Rendered demonstration](document/testdata/demo.png)

## Quickstart

```sh
go get nimbleterminal.dev/nthtml
go run ./examples/demo
go run ./cmd/nthtml -width 800 -o document.png examples/demo/sample.html
```

```go
result, err := document.Render(ctx, strings.NewReader(`
  <style>h1 { color: #2948a0; }</style>
  <h1>Hello, Nimble Terminal</h1><p>A portable document.</p>
`), document.Options{Width: 800})
if err != nil {
    return err
}
// result.Image is an *image.RGBA; result.Boxes contains text and pixel bounds.
viewer := nthtml.New(result)
_ = viewer
```

Import `nimbleterminal.dev/nthtml` and `nimbleterminal.dev/nthtml/document`.
Use `document.Parse` followed by `Document.Render` to reuse parsed documents.
The document package has no Bubble Tea dependency. Its image can be sent directly
to `ntpicture.Picture.SetImage`, encoded as PNG, or composed with other images.
Text boxes expose font size, weight, color, and bounds for host adapters such as
nttypeset; there is no direct nttypeset adapter yet.

## Task runner

A `Taskfile.yml` follows the neighboring Nimble Terminal projects. With Go Task
installed, `task --list` shows the available commands. Tasks disable CGO and
parent Go workspaces by default.

```sh
task                         # build CLI and terminal demo into bin/
task demo                    # platform overview; press t for TTY layout
task invoice                 # also: field-notes, status-report
task demo DEMO=field-notes
task serve                   # build and serve the browser gallery on localhost:8080
task serve PORT=8090
task render INPUT=examples/demo/documents/invoice.html OUTPUT=invoice.png
task previews                # regenerate the checked-in demo PNGs
task check                   # native tests, vet, formatting, module checks
task test -- -count=1
task ci                      # native + WASM builds/tests and browser smoke checks
```

Browser smoke tests and WASM tests require Node; `task serve` requires Python 3.
Use `task fmt` and `task go-tidy` to update formatting and module files.

## Demo gallery

Four embedded documents run through the same renderer on native Go and WASM:

| Demo | What it shows | Terminal command |
| --- | --- | --- |
| Platform overview | Cards, inline emphasis, feature table | `go run ./examples/demo` |
| Studio invoice | Nested blocks, itemized table, right-aligned totals | `go run ./examples/demo -demo invoice` |
| Field notes | Editorial typography, pull quote, long-page scrolling | `go run ./examples/demo -demo field-notes` |
| Delivery status | Dark palette, metric cards, service report | `go run ./examples/demo -demo status-report` |

The browser gallery has a document picker and shareable URLs such as
`http://localhost:8080/?demo=invoice`. **Show browser reference** opens a
side-by-side iframe containing the exact embedded HTML sent to the Go renderer.
With the reference closed, the Go view lives in a resizable container: drag its
lower-right corner, set **Container width**, or use **Fit pane**. Resize events
rerun Go layout at the available pixel width and rewrap TTY tables/text using the
measured monospace cell width. The heading shows the current pixels or columns;
zoom still magnifies the resulting layout. Resize updates are debounced.

With the reference open, both views use a fixed 800-pixel document width; the
browser uses its native fonts, styles, and layout rules. The panes scroll independently, and the comparison
stacks vertically on small screens. Close it with the same button or **Close**.
`?demo=invoice&compare=1` opens the reference immediately and keeps it open when
switching demos. The iframe is sandboxed without script or same-origin privileges.
All business names and operational data are fictional. HTML sources live in [examples/demo/documents](examples/demo/documents).

Render a standalone preview without starting a terminal UI:

```sh
go run ./cmd/nthtml -o invoice.png examples/demo/documents/invoice.html
```

Preview images: [invoice](examples/demo/previews/invoice.png),
[field notes](examples/demo/previews/field-notes.png),
[status report](examples/demo/previews/status-report.png).

## Terminal component

The component follows ntpicture's pointer-mutating convention:

- Return `viewer.Init()` from the parent's `Init`.
- Forward all messages through `viewer.Update(msg)` and run returned commands.
- Call `SetSize(columns, rows)` on resize and compose `viewer.View()` into the parent's view.
- Forward input only while focused; the parent owns focus and quit handling.
- Enable `tea.MouseModeCellMotion` for mouse-wheel scrolling.
- On exit run `tea.Sequence(viewer.Close(), tea.Quit)`; also defer `viewer.Close()` for resource cleanup.

Press **t** to toggle **Typeset** (the styled raster page) and **TTY** (readable
plain text reflowed to terminal cells). The browser gallery has the same toggle
as a button and keyboard shortcut. Its selectable TTY text reflows with the
resizable container; comparison mode uses 80 columns with horizontal scrolling
on narrower screens to preserve table alignment. Each mode retains its own scroll position. Switching back
to Typeset restores the raster zoom and pan. TTY omits CSS decorations, respects
hidden content, represents images by alt text, and draws Unicode-bordered tables with header
separators, wrapped cells, and left/center/right alignment from CSS. Columns fit
the available terminal width; very narrow layouts switch to stacked records.
Nested tables flatten inside their parent cell; colspan/rowspan remain unsupported.
TTY output is capped at 4 MiB. It strips terminal control characters from document text.

Hosts can call `viewer.SetLayoutMode(nthtml.LayoutTTY)`,
`viewer.ToggleLayout()`, or inspect `viewer.LayoutMode()`. Run returned commands
so Kitty placements are hidden/restored correctly. `viewer.Err()` reports TTY
conversion errors. Lower-level consumers can call `doc.TTY(ctx, columns)` or
`result.TTY(ctx, columns)` without using the widget. Rendered results retain their
immutable parsed document for reflow; manually constructed results have no TTY
content. “Typeset” names the existing nthtml raster layout, not integration with
the separate nttypeset engine.

Arrows / hjkl pan, Page Up / Down scroll, + / − zoom the Typeset view, and
0 / Home reset the active view.
Kitty support is detected by ntpicture, with half-block glyph fallback.
Zoom crops the raster; it does not reflow or regenerate higher-resolution text.
The example uses a full-screen component; multiple positioned Kitty widgets are
not exposed by this first wrapper.

## Support matrix

This describes the current implementation, not a roadmap. **Partial** means the
specific subset below works; it does not imply browser-compatible behavior.
**Not supported** means callers should not depend on that feature.

### HTML and CSS

| Feature | Status | What works / current boundary |
| --- | --- | --- |
| HTML parsing | Partial | Documents and fragments, entities, explicit start/end tags, and tolerant handling of unmatched tags. Bounded token-stack parser; no browser tree repair or implied paragraph/table closures. |
| Block and inline flow | Partial | Blocks, paragraphs, inline text spans, and `br`. `display: block`, `inline`, and `none`. No inline-block formatting, floats, flexbox, grid, or positioned layout. |
| Headings and emphasis | Partial | Built-in styles for `h1`, `h2`, and bold `b` / `strong` / `th`. Other headings behave as ordinary blocks unless styled. `em`, `i`, `code`, and `small` are inline, without automatic italics, monospace, or smaller type. |
| Whitespace and line breaks | Partial | Whitespace collapses; `br` inserts a break; long words wrap. No `white-space` modes, preserved `pre` formatting, hyphenation, or ellipsis. |
| Lists | Partial | List content flows as blocks. No generated bullets, numbering, nesting indentation, or CSS counters. |
| Stylesheet sources | Partial | Embedded `<style>` and inline `style` attributes. No external stylesheets, imports, or media-query evaluation. |
| Selectors and cascade | Partial | Single tag, `.class`, `#id`, `*`, and comma-separated selectors; specificity and source order for the same property; inline declarations override stylesheets. No compound selectors, combinators, attribute selectors, pseudo-classes/elements, `!important`, or full shorthand cascade rules. |
| Box spacing and sizing | Partial | Single-value `margin` / `padding`, `width`, and `height` (minimum content height). Nonnegative pixels; unitless lengths also mean pixels, not multipliers. Available width constrains boxes. No percentages, `em` / `rem`, viewport units, `calc()`, per-side spacing, margin collapse, or min/max sizing properties. |
| Borders and backgrounds | Partial | Uniform solid borders using `border`, `border-width`, and `border-color`; solid `background` / `background-color`. No individual sides, dashed/dotted styles, radius, shadows, gradients, or background images. |
| Typography | Partial | Bundled Go regular/bold fonts; `font-size`, `font-weight` (`bold` / `700` or regular), pixel `line-height`, and left/center/right `text-align`. No `font-family` selection, ntfonts provider, italic face, text decoration, justification, or letter/word spacing. |
| Unicode text | Partial | Text is drawn from the bundled fonts; TTY wrapping measures terminal cells. No full complex-script shaping, bidi layout, font fallback, or guaranteed emoji coverage. |
| Colors | Partial | `#rgb`, `#rrggbb`, `black`, `white`, `red`, `green`, `blue`, `gray`, and `transparent`. No alpha hex, `rgb()` / `hsl()`, `currentColor`, or full named-color set. |
| Tables | Partial | Explicit rows/cells with nested content. Typeset uses equal-width columns per row; TTY uses content-based column widths and wrapping. No colspan/rowspan, shared intrinsic column sizing in Typeset, border collapse, or full CSS table layout. |
| Images | Partial | Explicit `Options.Images[src]` allowlist; PNG/JPEG/GIF decode helpers and base64 data URL helper. Missing images show alt text. No automatic file/URL loading, animated playback, or browser-style inline image layout. |
| SVG and other embedded media | Not supported | No SVG fragment renderer, video/audio playback, embedded browser frames, or canvas execution in the document engine. |
| Links and forms | Not supported | Link text can render, but there is no navigation, hit testing, editable input, form state, or submission. |
| JavaScript and dynamic CSS | Not supported | No script execution, DOM/event system, animations, transitions, transforms, filters, or CSS variables. |

Unknown elements generally flow as blocks. Unsupported properties and selectors
have no supported effect; invalid values are not guaranteed to fall back as they
would in a browser. Inline spans carry text styles, not independent painted boxes.
The supported CSS properties are exactly those named in the table above, plus
`color`. This is an experimental document renderer, not a standards-conformant
HTML engine.

### Output modes and integrations

| Capability | Typeset | TTY |
| --- | --- | --- |
| Output | RGBA image and structured paint boxes | Plain text with terminal-cell wrapping |
| Visual styling | Supported font sizes, weight, colors, backgrounds, borders, and spacing | Omits font/color/box styling; respects `display: none` and table-cell alignment |
| Tables | Equal-width cells per row | Unicode borders, header separators, wrapped/aligned cells; stacked records when too narrow; nested tables flatten inside cells |
| Images | Rasterizes allowlisted images | Alt-text placeholder |
| Resize / reflow | Call `Document.Render` at a new pixel width; browser demo does this on container resize | Call `Document.TTY` / `Result.TTY` with new columns; widget and browser demo reflow on resize |
| Zoom / scroll | Widget zooms/crops the existing raster; browser zoom scales it; both scroll | Scrolls wrapped lines; no image zoom |
| Native terminal | `ntpicture` Kitty graphics with half-block fallback | Ordinary terminal text; no graphics protocol required |
| Browser demo | Go WASM raster displayed on canvas | Go-generated text in a selectable `<pre>` |

The native widget does **not** automatically rerender its Typeset document when
resized: it changes the viewport of the supplied image. The browser demo's
resizable container explicitly reruns layout. “Typeset” is the name of nthtml's
raster view; it does **not** currently use the separate nttypeset engine.

| Integration / target | Status |
| --- | --- |
| Pure-Go / no CGO common path | Supported |
| Native Go and `js/wasm` document API | Supported and tested; small font antialiasing differences are covered by the golden tolerance |
| Bubble Tea v2 component | Supported: mode toggle, zoom, pan, keyboard/mouse-wheel scrolling, and cleanup. Parent owns focus and quitting. |
| Multiple positioned Kitty widgets | Not exposed by the current nthtml wrapper |
| PNG export | Supported by `cmd/nthtml`; other image consumers can use `Result.Image` |
| Direct nttypeset / ntfonts / ntsvg integration | Not implemented; image and text-box outputs are available to host adapters |
| TinyGo / embedded devices | Unverified architectural goal; not a supported target yet |
| Full browser rendering | Only the demo's optional reference iframe, rendered by the host browser; not an nthtml capability |

To allow an image, decode it with `document.DecodeImage(reader)` or
`document.DecodeDataURL(src)` and insert it into `Options.Images[src]`. HTML never
opens a path or fetches a URL. The caller owns the lifetime and total memory of
its image map and must keep images immutable during rendering.

## Resource limits

Defaults: 1 MiB input, 10,000 nodes, depth 64, 256 CSS rules, 16 million output
pixels, height 16,384, width 32–4,096, and 100,000 paint operations. `Limits` may
lower these ceilings. Exceeding a budget returns `document.ErrLimit`. Font size,
spacing and borders are also capped; each declaration list uses its first 128
entries. TTY output is capped at 4 MiB and widths at 2–1,000 terminal cells.
Rendering checks context cancellation;
caller-provided readers and image implementations must themselves return promptly.
Decoded image helpers cap encoded input at 1 MiB and decoded output at 4 million
pixels. No goroutines, network fetches or unbounded browser tree repair are used
by the document engine.

## Browser demo

```sh
./scripts/build-web.sh
python3 -m http.server 8080 --directory web
# Open http://localhost:8080
```

The native and browser builds use the same `examples/demo/main.go`, embedded HTML,
public document API, layout and raster code. Only presentation differs: Bubble Tea
on native, canvas with scroll/zoom controls in the browser. `web/` is a static site;
the build copies the matching Go WASM runtime. Generated binaries are not committed.

```sh
CGO_ENABLED=0 go test ./...
go vet ./...
GOOS=js GOARCH=wasm go build ./...
go test ./document -fuzz FuzzRender -fuzztime 10s
```

A checked-in PNG golden verifies the shared renderer. Font antialiasing can differ
by one channel level across architectures; the golden allows a two-level
tolerance on at most 1% of pixels, while layout uses the same pixel coordinates.
TinyGo/embedded targets are an architectural goal, not verified support: font rasterization and the standard
HTML tokenizer may be too large for small devices. Use the isolated `document`
package and smaller budgets when evaluating such targets.

## License

MIT. Part of the Nimble Terminal family alongside ntpicture, ntsvg, ntpdf,
nttypeset and ntvideo.

### Bubble Tea on WASM

Like ntvideo, this repository pins the WASM-capable Bubble Tea fork. Go does not
inherit dependency replacements: applications importing the root widget package
on WASM must add this to their own `go.mod`:

```go
replace charm.land/bubbletea/v2 => github.com/neomantra/bubbletea/v2 v2.0.0-20261009201241-c397c32a5074
```

Consumers importing only `document` do not need this replacement.
