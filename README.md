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

## Demo gallery

Four embedded documents run through the same renderer on native Go and WASM:

| Demo | What it shows | Terminal command |
| --- | --- | --- |
| Platform overview | Cards, inline emphasis, feature table | `go run ./examples/demo` |
| Studio invoice | Nested blocks, itemized table, right-aligned totals | `go run ./examples/demo -demo invoice` |
| Field notes | Editorial typography, pull quote, long-page scrolling | `go run ./examples/demo -demo field-notes` |
| Delivery status | Dark palette, metric cards, service report | `go run ./examples/demo -demo status-report` |

The browser gallery has a document picker and shareable URLs such as
`http://localhost:8080/?demo=invoice`. All business names and operational data are
fictional. HTML sources live in [examples/demo/documents](examples/demo/documents).

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

Arrows / hjkl pan, Page Up / Down scroll, + / − zoom, and 0 / Home reset.
Kitty support is detected by ntpicture, with half-block glyph fallback.
Zoom crops the raster; it does not reflow or regenerate higher-resolution text.
The example uses a full-screen component; multiple positioned Kitty widgets are
not exposed by this first wrapper.

## Supported subset

| Area | First version |
| --- | --- |
| HTML | Bounded, forgiving token-stack parser; block elements, inline spans, headings, paragraphs, br |
| CSS | Embedded style and style attributes; tag, class, ID and universal selectors; specificity and source order |
| Boxes | Uniform pixel margin/padding/borders, width, minimum height, solid backgrounds |
| Text | Bundled Go fonts, pixel sizes/line heights, regular/bold, hex/basic named colors, left/center/right alignment |
| Tables | Equal-width cells per row, nested block content |
| Images | Caller-supplied `Options.Images` allowlist; bounded PNG/JPEG/GIF and base64 data URL decoding helpers |
| Output | RGBA image and structured paint boxes in document pixel coordinates |

Whitespace collapses. Long words break at rune boundaries. Unknown elements
flow as blocks. Unsupported properties/selectors are ignored. CSS lengths are
nonnegative pixels (unitless values also treated as pixels); spacing does not
collapse and sizing is constrained to the available width. Inline spans carry
text styles, not independent box decoration. Tables require explicit rows/cells;
no colspan, rowspan, intrinsic sizing, or browser HTML tree repair.

Deferred: flex/grid, floats, absolute/fixed positioning, percentages, individual
box sides, advanced selectors, `!important`, external stylesheets, SVG, custom
ntfonts providers, complex-script shaping, list markers, links/forms and scripting.
This is an experimental first version, not a standards-conformant HTML engine.

To allow an image, decode it with `document.DecodeImage(reader)` or
`document.DecodeDataURL(src)` and insert it into `Options.Images[src]`. HTML never
opens a path or fetches a URL. Missing images render their alt text. The caller
owns the lifetime and total memory of its image map and must keep images immutable
during rendering.

## Resource limits

Defaults: 1 MiB input, 10,000 nodes, depth 64, 256 CSS rules, 16 million output
pixels, height 16,384, width 32–4,096, and 100,000 paint operations. `Limits` may
lower these ceilings. Exceeding a budget returns `document.ErrLimit`. Font size,
spacing and borders are also capped; each declaration list uses its first 128 entries. Rendering checks context cancellation;
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
tolerance on at most 1% of pixels, while layout uses the same pixel coordinates. TinyGo/embedded targets are
an architectural goal, not verified support: font rasterization and the standard
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
