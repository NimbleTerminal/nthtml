# Host-painted CSS viewports

`ModelOptions.HostPainted` opts into a viewer that never calls `Result.Paint`.
Existing constructors and result setters retain their existing behavior when
this option is false. Host-painted mode uses width fit; `TypesetFitContain` is
unsupported. TTY still uses the whole document's text layout.

## Atomic initial frame

Create the viewer with `NewWithOptions(nil, ModelOptions{HostPainted: true, ...})`.
On a worker, in one pass:

1. Render with `document.Options.NoImage: true` and explicit resource limits.
2. Create `result.NewPainter(ctx)`.
3. Call `PlanPaint(result, viewport)` using the host's current geometry and
   desired scroll/zoom. For initial navigation use origin zero, zoom zero, and
   `Scale: 0` (the default, derived from geometry and zoom).
   `Model.PaintViewport()` snapshots geometry for later reflows;
   snapshot on the UI goroutine before starting work.
4. Paint `plan.Area` at `plan.Scale` with that painter.
5. Deliver `PaintedResult{Result: result, Revision: revision, Plan: plan, Image: band}`
   to the UI, rejecting superseded navigation/reflow jobs in the host.
6. Call `SetPaintedResult(frame, keepView)` and return its command through Bubble
   Tea. Validation occurs before installation. A revision must strictly increase.

The compiled `ExamplePlanPaint` demonstrates this pass. No layout is installed
without a matching first band. `keepView` retains search state; the supplied plan
owns scroll and zoom. Source/link focus resets; hosts can reapply source focus.
Installing during TTY defers picture submission until Typeset is selected. The
viewer tracks the submitted revision separately, so it uploads the new band
before accepting raster input. On installation it reconciles stale viewport
geometry; a height-only change can reuse the existing band without painting.

## Later paint requests

Forward normal messages to `viewer.Update`. Intercept `nthtml.PaintRequest`
messages and enqueue them on a **single serial worker** for this viewer. Keep at
most one active paint and one latest queued request. The viewer cancels request
contexts when work is superseded, the layout changes, TTY is selected, or the
viewer closes. Discard queued requests with cancelled contexts. Add a host
painting deadline, e.g. `context.WithTimeout(request.Context, 2*time.Second)`.
Do not spawn an unbounded goroutine per request.

On that worker, verify the request's `Result` and `Revision` refer to its current
painter, then call `painter.Paint(ctx, request.Plan.Area, request.Plan.Scale)`.
Deliver `PaintReady{Request: request, Image: img, Err: err}` to the viewer's
`Update`. Do not reconstruct or modify the request. Owner, revision, sequence,
result identity, and plan must all match pending work before a completion is
accepted. Close the old painter on the same worker when replacing a layout.
Host cancellation should also interrupt layout creation and initial painting.

Only the latest requested viewport is painted. Until completion, the old band
and its displayed viewport remain together. Failures leave them intact and set
`Model.Err()`; a later pan/resize can retry. `DocumentViewport` and
`CellToDocument` describe the displayed band, while `PaintViewport` describes
the desired view. Sub-row pan input accumulates in that desired origin.
A column resize immediately resizes the retained picture grid, preserving its
CSS extent and updating hit-test mapping while replacement painting is pending.
If painting fails, that resized old band remains usable. Dispatch actions only when `InputReady()` and the host's
DOM/frame revision checks both pass. A paint in flight does not invalidate hits
on an otherwise current, fully presented old band.

## Bounds, scaling, and ownership

Bands use the existing 283-row ceiling and respect the document pixel limit,
`ntpicture.MaxImagePixels`, and transport `MaxRasterPixels`. If a viewport is too
large for its budget, only the bounded number of rows is presented. Paint scale
is clamped to 1/16..16, and text painted at any scale keeps the 1x layout's
glyph advances (scaled), so word boxes and their glyphs agree; transport resampling handles remaining glyph reduction
or zoom up to 64x. The host-painted picture fills the planned cell grid, absorbing
subpixel raster rounding without scale-dependent letterboxes; the viewer hands it
a band of exactly the raster's size (rounding excess cropped, a short last band
stretched once) so the transport copies rather than resamples. The presented
grid never exceeds `MaxKittyPlaceholderCells` columns (the protocol's 297
row/column diacritics); a host that lays out at the terminal's width should
size the viewer no wider, so the band matches the raster cell for cell. A page
shorter than the bounds is followed by rows of the canvas colour in `View`,
never by a shorter view. Source pixels, transport pixels, and CSS hit-test
coordinates remain distinct.

Search and selection copy only the resident band. Kitty focus uses a separate
band-local patch. Cached scrolling reuses the image and stable Kitty ID.
Images and result assets must remain immutable after submission. An active paint,
the presented band, its optional overlay, and asynchronous transport buffers can
coexist; the per-image cap is not a total-process memory cap.

A `document.Painter` caches parsed fonts and faces under the renderer's existing
font budgets. A scale change discards faces and retains parsed fonts. It indexes
operations into 512-CSS-pixel y buckets, with at most eight references per
operation; tall operations use a shared list. Candidates are painted in original
order. `Paint` and `Close` must be serialized. The painter retains no output raster.
Layout and DOM remain eager and resource-bounded; this is not lazy CSS layout.

## Validation

`go test ./...` covers atomic installation, stale completion/cancellation,
error recovery, scale limits, TTY transitions, global coordinates, compressed
Kitty transport and stable IDs, bounded overlays/focus, and pixel equality with
`Result.Paint`, including background images and positioned content.

Run `go test ./document -run '^$' -bench BenchmarkBandPaint -benchmem` to compare
uncached, cold-index, and warm paints for 100 and 1,100 paragraphs (the larger
fixture approaches the default 10,000-node budget). Cold indexing is explicit
and can cost more than one uncached paint. Warm paints avoid full-list scans and
font reparsing; actual glyph rasterization and band allocation remain costs.

Measured on Apple M5 Max / Go 1.26.8, 400×200 bands at scale 1,
`-benchtime=200ms` (one local sample, not a latency guarantee):

| Paragraphs | Painter | Time/paint | Bytes/paint | Allocs/paint |
| --- | --- | --- | --- | --- |
| 100 | Uncached | 0.535 ms | 359,227 | 248 |
| 100 | Cold | 0.546 ms | 777,257 | 329 |
| 100 | Warm | 0.493 ms | 330,324 | 174 |
| 1,100 | Uncached | 0.609 ms | 358,853 | 248 |
| 1,100 | Cold | 0.683 ms | 884,678 | 867 |
| 1,100 | Warm | 0.491 ms | 329,933 | 174 |

The fixtures use regular, bold, and italic Latin fonts. Host-supplied CJK
fallback cache correctness is covered with a synthetic coverage font; throughput
with a production CJK font remains a host benchmark to run before rollout.

## ntbrowser adoption

Publish this API before changing ntbrowser's dependency: ntbrowser requires
published versions and forbids sibling-library replacements. Its integration
must move CSS rendering to `NoImage: true`, send layout plus first band atomically
from its serialized loop worker, route subsequent paint requests through that
worker, and retain DOM revision checks at action dispatch. Update its `AGENTS.md`,
architecture §3/decision 19, loop frame payload, app viewer options, and
`TestViewerKeepsKittyIDAndSendsZlib`. The opt-in semantic chunk mode remains a
separate rendering choice.
