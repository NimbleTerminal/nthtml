package nthtml

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	tea "charm.land/bubbletea/v2"
	xdraw "golang.org/x/image/draw"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

// PaintViewport describes a desired CSS viewport and its terminal geometry.
// Zero cell dimensions use glyph geometry (one by two pixels). Scale optionally
// requests an initial paint density; zero chooses it from geometry and zoom.
type PaintViewport struct {
	Cols, Rows            int
	CellWidth, CellHeight int
	Origin                image.Point
	ZoomSteps             int
	MaxRasterPixels       int
	Scale                 float64
}

// PaintPlan is immutable once submitted. Area is the resident CSS band;
// Viewport is the visible part. Pixels are painted at Scale, while transport
// performs remaining resampling. The row fields describe the band cell grid.
type PaintPlan struct {
	Area, Viewport                  image.Rectangle
	Scale                           float64
	Cols, Rows, BandRows, RowOffset int
	ZoomSteps                       int
	CellWidth, CellHeight           int
}

// PlanPaint chooses a bounded band using the same 283-row ceiling as the native
// viewer. It accounts for the document allocation and transport raster budgets.
// It performs no layout, painting, or terminal I/O and is safe on a worker.
func PlanPaint(result *document.Result, v PaintViewport) (PaintPlan, error) {
	if result == nil || result.Bounds().Empty() {
		return PaintPlan{}, fmt.Errorf("nthtml: empty paint layout")
	}
	cols, rows := max(1, min(v.Cols, 1000)), max(1, min(v.Rows, 1000))
	cw, ch := v.CellWidth, v.CellHeight
	if cw <= 0 {
		cw = 1
	}
	if ch <= 0 {
		ch = 2
	}
	if cw > 4096 || ch > 4096 {
		return PaintPlan{}, fmt.Errorf("nthtml: invalid cell size")
	}
	zoom := max(0, min(maxZoomSteps, v.ZoomSteps))
	b := result.Bounds()
	width := max(1, int(float64(b.Dx())/zoomScale(zoom)))
	rowHeight := float64(width) * float64(ch) / float64(cols*cw)
	scale := v.Scale
	if scale == 0 {
		scale = float64(cols*cw) / float64(width)
	}
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return PaintPlan{}, fmt.Errorf("nthtml: invalid paint scale")
	}
	scale = max(1.0/16, min(16, scale))
	pixelCap := min(result.PixelLimit(), ntpicture.MaxImagePixels)
	rasterCap := ntpicture.MaxImagePixels
	if v.MaxRasterPixels > 0 {
		rasterCap = min(rasterCap, v.MaxRasterPixels)
	}
	bandRows := min(maxKittyBandRows, rasterCap/max(1, cols*cw*ch))
	// Include rounding at both band edges in the source allocation bound.
	rasterWidth := int(math.Ceil(float64(width) * scale))
	for bandRows > 0 && rasterWidth > pixelCap/max(1, int(math.Ceil((float64(bandRows)*rowHeight+1)*scale))) {
		bandRows--
	}
	if bandRows < 1 {
		return PaintPlan{}, fmt.Errorf("%w: viewport band", document.ErrLimit)
	}
	pageRows := max(1, int(math.Ceil(float64(b.Dy())/rowHeight)))
	bandRows = min(bandRows, pageRows)
	visibleRows := min(rows, bandRows)
	topRow := max(0, min(pageRows-visibleRows, int(math.Floor(float64(v.Origin.Y-b.Min.Y)/rowHeight))))
	margin := min(rows, max(0, (bandRows-visibleRows)/2))
	startRow := max(0, min(topRow-margin, pageRows-bandRows))
	x := max(b.Min.X, min(v.Origin.X, b.Max.X-width))
	area := image.Rect(x, b.Min.Y+int(math.Floor(float64(startRow)*rowHeight)), x+width, min(b.Max.Y, b.Min.Y+int(math.Ceil(float64(startRow+bandRows)*rowHeight))))
	plan := PaintPlan{Area: area, Scale: scale, Cols: cols, Rows: visibleRows, BandRows: bandRows, RowOffset: topRow - startRow, ZoomSteps: zoom, CellWidth: cw, CellHeight: ch}
	plan.Viewport = plan.viewport()
	return plan, nil
}
func (p PaintPlan) viewport() image.Rectangle {
	return p.cellArea(image.Rect(0, p.RowOffset, p.Cols, p.RowOffset+p.Rows))
}

// The host-painted transport fills the planned cell grid. This absorbs the
// subpixel padding from ceil(Area.Size*Scale) without introducing letterboxes
// whose shape changes with paint density.
func (p PaintPlan) cellArea(cells image.Rectangle) image.Rectangle {
	cells = cells.Intersect(image.Rect(0, 0, p.Cols, p.BandRows))
	if cells.Empty() {
		return image.Rectangle{}
	}
	return image.Rect(p.Area.Min.X+cells.Min.X*p.Area.Dx()/p.Cols,
		p.Area.Min.Y+cells.Min.Y*p.Area.Dy()/p.BandRows,
		p.Area.Min.X+(cells.Max.X*p.Area.Dx()+p.Cols-1)/p.Cols,
		p.Area.Min.Y+(cells.Max.Y*p.Area.Dy()+p.BandRows-1)/p.BandRows).Intersect(p.Area)
}

// PaintRequest asks the host to paint on a worker. Context is cancelled when
// superseded, on layout replacement, or on Close. Hosts add their own deadline.
// A serial document.Painter per layout can service these requests efficiently.
type PaintRequest struct {
	Owner              *Model
	Result             *document.Result
	Revision, Sequence uint64
	Plan               PaintPlan
	Context            context.Context
}

// PaintReady is returned to Update after painting a request. Image must have
// origin (0,0), size ceil(Area.Size*Scale), and remain immutable after delivery.
// The viewer ignores stale completions, including errors.
type PaintReady struct {
	Request PaintRequest
	Image   *image.RGBA
	Err     error
}

// PaintedResult installs a layout and its already-painted initial band together.
// Revision must increase on navigation/reflow. Plan records the initial view;
// use PlanPaint on the layout worker, then paint Plan.Area at Plan.Scale.
type PaintedResult struct {
	Result   *document.Result
	Revision uint64
	Plan     PaintPlan
	Image    *image.RGBA
}

type hostPaintState struct {
	revision, sequence uint64
	plan               PaintPlan
	presentedPlan      PaintPlan
	presentedRevision  uint64
	pending            *PaintRequest
	cancel             context.CancelFunc
	closed             bool
}

// SetPaintedResult atomically replaces layout and initial raster. Validation
// happens before any state change. No painting occurs on the caller's goroutine.
// keepView retains search choices; the supplied plan owns scroll and zoom.
func (m *Model) SetPaintedResult(frame PaintedResult, keepView bool) (tea.Cmd, error) {
	if m.hostPaint == nil || m.hostPaint.closed {
		return nil, fmt.Errorf("nthtml: host-painted mode required")
	}
	if frame.Revision <= m.hostPaint.revision {
		return nil, fmt.Errorf("nthtml: stale layout revision")
	}
	if err := validatePaint(frame.Result, frame.Plan, frame.Image); err != nil {
		return nil, err
	}
	m.cancelPaint()
	h := m.hostPaint
	h.revision, h.plan = frame.Revision, frame.Plan
	m.result, m.renderOpts = frame.Result, frame.Result.Options()
	m.source = frame.Image
	m.index = nil
	m.clearSelection()
	m.clearSourceFocus()
	m.pressedLink = 0
	m.hoverID = 0
	m.focusID = 0
	if !keepView {
		m.query, m.searchInput = "", ""
		m.searching = false
		m.matches = nil
		m.match = -1
		m.ttyTop = 0
	}
	m.zoom = frame.Plan.ZoomSteps
	m.x, m.y = frame.Plan.Viewport.Min.X, frame.Plan.Viewport.Min.Y
	m.err = nil
	m.resetTextLayout()
	m.overlayImg = nil
	m.overlayPainted = false
	m.focusKey = focusOverlayKey{}
	if m.mode == LayoutTTY {
		m.reflowTTY()
		return nil, nil
	}
	present := m.presentPainted(true, true)
	v := m.PaintViewport()
	p := frame.Plan
	if p.Cols != v.Cols || p.Rows != min(v.Rows, p.BandRows) || p.CellWidth != v.CellWidth || p.CellHeight != v.CellHeight {
		return tea.Batch(present, m.paintViewport(true)), nil
	}
	return present, nil
}

func validatePaint(result *document.Result, p PaintPlan, img *image.RGBA) error {
	if result == nil || p.Area.Empty() || !p.Area.In(result.Bounds()) || !p.Viewport.In(p.Area) || p.Viewport.Empty() || !(p.Scale >= 1.0/16 && p.Scale <= 16) {
		return fmt.Errorf("nthtml: invalid painted area")
	}
	if p.Cols < 1 || p.Cols > 1000 || p.Rows < 1 || p.Rows > p.BandRows || p.BandRows > maxKittyBandRows || p.RowOffset < 0 || p.RowOffset+p.Rows > p.BandRows || p.CellWidth < 1 || p.CellWidth > 4096 || p.CellHeight < 1 || p.CellHeight > 4096 || p.ZoomSteps < 0 || p.ZoomSteps > maxZoomSteps || p.Viewport != p.viewport() {
		return fmt.Errorf("nthtml: invalid painted geometry")
	}
	size := image.Pt(int(math.Ceil(float64(p.Area.Dx())*p.Scale)), int(math.Ceil(float64(p.Area.Dy())*p.Scale)))
	if size.Y < 1 || size.X > min(result.PixelLimit(), ntpicture.MaxImagePixels)/size.Y || img == nil || img.Bounds() != (image.Rectangle{Max: size}) {
		return fmt.Errorf("nthtml: invalid painted raster")
	}
	return nil
}

func (m *Model) cancelPaint() {
	h := m.hostPaint
	if h == nil {
		return
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	h.pending = nil
}

// PaintViewport returns the desired view for a host's next reflow snapshot.
// DocumentViewport remains the currently presented view while a paint is pending.
func (m *Model) PaintViewport() PaintViewport {
	cw, ch := 1, 2
	if m.pic.Mode() == ntpicture.PictureKitty {
		cw, ch = m.pic.CellPixelSize()
		if cw <= 0 {
			cw = 10
		}
		if ch <= 0 {
			ch = 20
		}
	}
	return PaintViewport{Cols: m.cols, Rows: m.rows, CellWidth: cw, CellHeight: ch, Origin: image.Pt(m.x, m.y), ZoomSteps: m.zoom, MaxRasterPixels: m.pic.MaxRasterPixels()}
}

// PaintedRevision is the installed layout revision. Hosts must additionally
// check InputReady before dispatching actions against it.
func (m *Model) PaintedRevision() uint64 {
	if m.hostPaint == nil {
		return 0
	}
	return m.hostPaint.revision
}

// InputReady reports that the current raster has completed presentation. A host
// should pair this with its own DOM/frame revision check before dispatching hits.
func (m *Model) InputReady() bool {
	return m.mouseReady() && (m.hostPaint == nil || !m.hostPaint.closed)
}

func (m *Model) paintViewport(overlays bool) tea.Cmd {
	h := m.hostPaint
	if h == nil || h.closed || m.result == nil || m.source == nil || m.mode == LayoutTTY {
		return nil
	}
	plan, err := PlanPaint(m.result, m.PaintViewport())
	if err != nil {
		m.err = err
		return nil
	}
	// Keep sub-row input in the desired origin; only presentation snaps to rows.
	b := m.result.Bounds()
	m.x = max(b.Min.X, min(m.x, b.Max.X-plan.Area.Dx()))
	rowHeight := float64(plan.Area.Dx()) * float64(plan.CellHeight) / float64(plan.Cols*plan.CellWidth)
	lastRow := max(0, int(math.Ceil(float64(b.Dy())/rowHeight))-plan.Rows)
	lastOrigin := b.Min.Y + int(math.Ceil(float64(lastRow)*rowHeight))
	m.y = max(b.Min.Y, min(m.y, lastOrigin))
	current := h.plan
	if plan.Viewport.In(current.Area) && plan.Area.Dx() == current.Area.Dx() && plan.Area.Min.X == current.Area.Min.X && plan.Scale <= current.Scale && plan.Cols == current.Cols && plan.CellWidth == current.CellWidth && plan.CellHeight == current.CellHeight && plan.ZoomSteps == current.ZoomSteps {
		m.cancelPaint()
		current.Rows = min(plan.Rows, current.BandRows)
		current.RowOffset = max(0, min(current.BandRows-current.Rows, int(math.Round(float64(plan.Viewport.Min.Y-current.Area.Min.Y)*float64(current.BandRows)/float64(current.Area.Dy())))))
		current.Viewport = current.viewport()
		h.plan = current
		return m.presentPainted(false, overlays)
	}
	if h.pending != nil && h.pending.Plan == plan {
		return nil
	}
	m.cancelPaint()
	h.sequence++
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	request := PaintRequest{Owner: m, Result: m.result, Revision: h.revision, Sequence: h.sequence, Plan: plan, Context: ctx}
	h.pending = &request
	return func() tea.Msg { return request }
}

func (m *Model) applyPaint(ready PaintReady) tea.Cmd {
	h := m.hostPaint
	if h == nil || h.closed || h.pending == nil {
		return nil
	}
	request := ready.Request
	expected := h.pending
	if request.Owner != m || request.Revision != h.revision || request.Sequence != expected.Sequence || request.Result != m.result || request.Plan != expected.Plan {
		return nil
	}
	m.cancelPaint()
	if ready.Err != nil {
		m.err = ready.Err
		return nil
	}
	if err := validatePaint(m.result, request.Plan, ready.Image); err != nil {
		m.err = err
		return nil
	}
	h.plan = request.Plan
	m.source = ready.Image
	m.err = nil
	m.focusKey = focusOverlayKey{}
	return m.presentPainted(true, true)
}

func (m *Model) hostKitty() bool {
	return m.hostPaint != nil && m.mode == LayoutTypeset && m.pic.Mode() == ntpicture.PictureKitty && m.source != nil
}

func (m *Model) paintRect(rect image.Rectangle) image.Rectangle {
	p := m.hostPaint.plan
	return image.Rect(int(math.Floor(float64(rect.Min.X-p.Area.Min.X)*p.Scale)), int(math.Floor(float64(rect.Min.Y-p.Area.Min.Y)*p.Scale)), int(math.Ceil(float64(rect.Max.X-p.Area.Min.X)*p.Scale)), int(math.Ceil(float64(rect.Max.Y-p.Area.Min.Y)*p.Scale)))
}

func (m *Model) paintedOverlay(includeFocus bool) image.Image {
	img := image.NewRGBA(m.source.Bounds())
	draw.Draw(img, img.Bounds(), m.source, m.source.Bounds().Min, draw.Src)
	shade := func(regions []image.Rectangle, ink color.Color) {
		for _, r := range regions {
			draw.Draw(img, m.paintRect(r), image.NewUniform(ink), image.Point{}, draw.Over)
		}
	}
	if m.match >= 0 && m.match < len(m.matches) {
		hit := m.matches[m.match]
		shade(m.textIndex().Regions(hit.Start, hit.End), color.NRGBA{255, 210, 35, 70})
	}
	shade(m.textIndex().Regions(m.selectStart, m.selectEnd), color.NRGBA{40, 120, 255, 110})
	if includeFocus {
		for _, r := range m.focusRegions() {
			r = r.Inset(-1)
			shade([]image.Rectangle{image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)}, m.linkFeedbackColor())
		}
	}
	return img
}

// fitBand returns the band at exactly the Kitty raster's size. The host paints
// ceil(Area.Size*Scale) pixels, a pixel or two more than the cell grid they
// fill, and the page's last band falls short of the grid by under a row.
// ntpicture uses an equal-sized opaque image as it is, but resamples any other
// size with a kernel scaler whose scratch is 32 bytes per destination column
// per source row: hundreds of MiB and two filter passes for a full-screen band,
// per frame. Rounding excess is cropped without a copy; a shortfall is
// stretched once with a bilinear filter, the same mapping the transport's fill
// would apply, so cell coordinates still land where the plan says.
func fitBand(band image.Image, target image.Point) image.Image {
	if band == nil || target.X <= 0 || target.Y <= 0 {
		return band
	}
	b := band.Bounds()
	if b.Dx() == target.X && b.Dy() == target.Y {
		return band
	}
	if b.Dx() >= target.X && b.Dy() >= target.Y {
		return cropBandImage(band, image.Rect(b.Min.X, b.Min.Y, b.Min.X+target.X, b.Min.Y+target.Y))
	}
	out := image.NewRGBA(image.Rect(0, 0, target.X, target.Y))
	xdraw.ApproxBiLinear.Scale(out, out.Bounds(), band, b, draw.Src, nil)
	return out
}

func (m *Model) presentPainted(force, overlays bool) tea.Cmd {
	h := m.hostPaint
	p := h.plan
	// Resize the retained band immediately, even while a new paint is pending.
	// Keep its CSS extent and map hits through this actual presentation grid,
	// which never exceeds the Kitty placeholder grid: a wider band is cropped
	// to the columns the terminal can address rather than refused whole.
	p.Cols = m.kittyCols()
	p.Viewport = p.viewport()
	force = force || h.presentedRevision != h.revision
	resize := h.presentedPlan.Cols != p.Cols || h.presentedPlan.BandRows != p.BandRows
	h.presentedRevision, h.presentedPlan = h.revision, p
	m.bandCols, m.bandRows = p.Cols, p.BandRows
	m.pic.SetViewport(ntpicture.Viewport{X: 0, Y: p.RowOffset, Width: p.Cols, Height: p.Rows})
	var imageCmd, sizeCmd tea.Cmd
	if force || resize {
		sizeCmd = m.pic.SetSize(p.Cols, p.BandRows)
	}
	wantOverlay := m.hasSearchOrSelectionOverlay() || !m.hostKitty() && len(m.focusRegions()) > 0
	if force || overlays && (wantOverlay || m.overlayPainted) {
		m.overlayImg = nil
		m.overlayPainted = wantOverlay
		source := m.source
		if wantOverlay {
			m.overlayImg = m.paintedOverlay(!m.hostKitty())
			source = m.overlayImg
		}
		if m.hostKitty() {
			source = fitBand(source, m.pic.RasterGeometry().Pixels)
		}
		imageCmd = m.pic.SetImageImmutableCropped(source, image.Rectangle{})
	}
	var focus tea.Cmd
	if m.hostKitty() {
		focus = m.updateKittyFocusOverlay()
	} else {
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
	}
	return tea.Batch(sizeCmd, imageCmd, focus)
}
