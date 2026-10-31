// Package nthtml provides a Bubble Tea document viewer backed by ntpicture.
package nthtml

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"image"
	"image/draw"
	"math"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
	"strconv"
	"strings"
)

const maxKittyBandRows = 283

// Four zoom steps double magnification, preserving the former 64x limit.
const zoomStepsPerDoubling = 4
const maxZoomSteps = 6 * zoomStepsPerDoubling

func zoomScale(steps int) float64 {
	return math.Exp2(float64(steps) / zoomStepsPerDoubling)
}

func cropBandImage(img image.Image, r image.Rectangle) image.Image {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	r = r.Intersect(b)
	if r == b || r.Empty() {
		return img
	}
	if sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return sub.SubImage(r)
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), img, r.Min, draw.Src)
	return dst
}

type focusOverlayKey struct {
	focusID                 int
	sourceID                string
	bandCrop                image.Rectangle
	cols, bandRows          int
	cellW, cellH            int
	match, selStart, selEnd int
}

// LayoutMode selects styled pixels or CSS-styled terminal-cell text.
// LayoutTypeset uses nthtml's rasterizer, not the separate nttypeset engine.
type LayoutMode int

const (
	LayoutTypeset LayoutMode = iota
	LayoutTTY
)

// TypesetFit selects a scrollable document viewport or a whole-page overview.
type TypesetFit uint8

const (
	// TypesetFitWidth uses the normal width-oriented viewport and scrolling.
	TypesetFitWidth TypesetFit = iota
	// TypesetFitContain fits the complete page inside the widget, with letterboxing.
	TypesetFitContain
)

// ModelOptions configures a Bubble Tea document viewer Model.
type ModelOptions struct {
	// Session shares terminal discovery, cell geometry, and transport settings
	// across multiple picture/document widgets.
	Session *ntpicture.TerminalSession
	// NextID allocates positive 24-bit Kitty image IDs across widgets.
	NextID func() (int, error)
	// Picture overrides underlying ntpicture.Options when nonzero.
	Picture ntpicture.Options
	// ReflowTypeset re-renders the underlying Document when terminal width
	// changes significantly on SetSize/SetBounds, matching browser reflow.
	ReflowTypeset bool
	// RenderOptions supplies images, fonts, or diagram settings used when
	// ReflowTypeset re-renders the Document.
	RenderOptions document.Options
}

// Model follows ntpicture's pointer-mutating component convention. Forward all
// messages and run returned commands. The host owns focus and quit handling.
type Model struct {
	index                         *document.TextIndex
	query, searchInput            string
	searching                     bool
	matches                       []document.Match
	match, selectStart, selectEnd int
	mouseDown, dragging           bool
	dragStart                     document.Match
	pic                           *ntpicture.Picture
	focusPic                      *ntpicture.Picture
	result                        *document.Result
	renderOpts                    document.Options
	reflowTypeset                 bool
	mode                          LayoutMode
	typesetFit                    TypesetFit
	ttyLines                      []string
	ttyResult                     *document.TTYResult
	focusID                       int
	focusSourceID                 string
	sourceFocusBounds             image.Rectangle
	origin, pressCell             image.Point
	pressedLink                   int
	ttyTop                        int
	err                           error
	source                        image.Image
	overlayImg                    image.Image
	overlayPainted                bool
	kittyBandActive               bool
	kittyCellSize                 image.Point
	bandCols, bandRows            int
	bandStartRow                  int
	bandCrop                      image.Rectangle
	focusCellRect                 image.Rectangle
	focusKey                      focusOverlayKey
	initial                       tea.Cmd
	zoom, x, y, cols, rows        int
}

func New(result *document.Result) *Model {
	return NewWithOptions(result, ModelOptions{})
}

func NewWithOptions(result *document.Result, opts ModelOptions) *Model {
	picOpts := opts.Picture
	if !picOpts.HostGeometry {
		picOpts.AutoMode = true
	}
	if opts.Session != nil {
		picOpts.Session = opts.Session
	}
	if opts.NextID != nil {
		picOpts.NextID = opts.NextID
	}
	pic := ntpicture.New(picOpts)
	focusPic := ntpicture.New(ntpicture.Options{
		Session:      pic.Session(),
		HostGeometry: true,
		StableID:     true,
		RetireDelay:  -1,
		Render: ntpicture.Config{
			Fit:         ntpicture.FitFill,
			KittyFormat: picOpts.Render.KittyFormat,
			KittyMedium: picOpts.Render.KittyMedium,
			KittyZ:      picOpts.Render.KittyZ,
		},
	})
	m := &Model{
		pic:           pic,
		focusPic:      focusPic,
		cols:          80,
		rows:          24,
		match:         -1,
		reflowTypeset: opts.ReflowTypeset,
		renderOpts:    opts.RenderOptions,
	}
	if result != nil {
		if m.renderOpts.Width == 0 {
			m.renderOpts = result.Options()
		}
		m.result = result
		if result.Image != nil {
			m.source = result.Image
		} else if result.Document() != nil && !result.Bounds().Empty() {
			if img, err := result.Paint(context.Background(), result.Bounds(), 1); err == nil && img != nil {
				m.source = img
			}
		}
		if m.source != nil {
			m.initial = m.presentSource()
		}
	}
	return m
}

// Bounds returns the widget's terminal-cell rectangle (origin and size).
func (m *Model) Bounds() image.Rectangle {
	return image.Rect(m.origin.X, m.origin.Y, m.origin.X+m.cols, m.origin.Y+m.rows)
}

// Picture returns the underlying ntpicture.Picture for status, geometry, or
// multi-widget Kitty placement management.
func (m *Model) Picture() *ntpicture.Picture { return m.pic }

// SetVisible shows or hides the widget and its Kitty placements.
func (m *Model) SetVisible(visible bool) tea.Cmd {
	if m.focusPic != nil {
		return tea.Batch(m.pic.SetVisible(visible), m.focusPic.SetVisible(visible))
	}
	return m.pic.SetVisible(visible)
}

// Visible reports whether the widget's picture is visible.
func (m *Model) Visible() bool { return m.pic.Visible() }

// ReflowTypeset reports whether Typeset mode automatically re-renders on width changes.
func (m *Model) ReflowTypeset() bool { return m.reflowTypeset }

// SetReflowTypeset enables or disables responsive Typeset reflow on resize.
func (m *Model) SetReflowTypeset(enabled bool) tea.Cmd {
	m.reflowTypeset = enabled
	if enabled && m.mode == LayoutTypeset {
		return m.SetSize(m.cols, m.rows)
	}
	return nil
}

// Result returns the currently displayed document Result.
func (m *Model) Result() *document.Result { return m.result }

// SetResult replaces the displayed document Result and resets scroll/selection state.
func (m *Model) SetResult(result *document.Result) tea.Cmd {
	return m.setResult(result, false)
}

// SetResultKeepView replaces the displayed document Result while preserving
// scroll offset, zoom, search state, and the focused link (by Link.ID) when
// present in the new Result. Selection and an in-progress mouse drag are cleared:
// their text offsets may refer to different content in the replacement Result.
// Source focus is always cleared; hosts should reapply FocusSource after replacement.
func (m *Model) SetResultKeepView(result *document.Result) tea.Cmd {
	return m.setResult(result, true)
}

func (m *Model) setResult(result *document.Result, keepView bool) tea.Cmd {
	m.result = result
	m.clearSourceFocus()
	m.index = nil
	m.pressedLink = 0
	m.clearSelection()
	if !keepView {
		m.focusID = 0
		m.zoom, m.x, m.y, m.ttyTop = 0, 0, 0, 0
		m.bandStartRow = 0
		m.kittyBandActive = false
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
		m.query, m.searchInput = "", ""
		m.searching = false
		m.matches = nil
		m.match = -1
	}
	if result == nil {
		m.focusID = 0
		m.matches = nil
		m.match = -1
		m.source = nil
		m.overlayImg = nil
		m.overlayPainted = false
		m.kittyBandActive = false
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
		return nil
	}
	m.renderOpts = result.Options()
	m.source = nil
	if result.Image != nil {
		m.source = result.Image
	} else if result.Document() != nil && !result.Bounds().Empty() {
		if img, err := result.Paint(context.Background(), result.Bounds(), 1); err == nil && img != nil {
			m.source = img
		}
	}
	if m.mode == LayoutTTY {
		m.reflowTTY()
		if _, ok := m.FocusedLink(); !ok {
			m.focusID = 0
		}
		return m.refreshHighlights()
	}
	m.resetTextLayout()
	if _, ok := m.FocusedLink(); !ok {
		m.focusID = 0
	}
	return m.presentSource()
}
func (m *Model) Init() tea.Cmd { return tea.Batch(m.pic.Init(), m.initial, m.SetSize(m.cols, m.rows)) }
func (m *Model) View() string {
	if m.mode == LayoutTTY {
		end := min(len(m.ttyLines), m.ttyTop+m.rows)
		text := strings.Join(m.ttyLines[m.ttyTop:end], "\n")
		if link, ok := m.FocusedLink(); ok {
			start := ansi.SetHyperlink(link.Href, "id=nthtml-"+strconv.Itoa(link.ID))
			text = strings.ReplaceAll(text, start, start+"\x1b[7m")
			text = strings.ReplaceAll(text, ansi.ResetHyperlink(), "\x1b[0m"+ansi.ResetHyperlink())
		}
		return text
	}
	base := m.pic.View()
	if m.kittyViewportMode() && !m.focusCellRect.Empty() && m.focusPic != nil {
		return m.spliceFocusOverlay(base)
	}
	return base
}
func (m *Model) LayoutMode() LayoutMode { return m.mode }
func (m *Model) Err() error             { return m.err }
func (m *Model) SetLayoutMode(mode LayoutMode) tea.Cmd {
	if mode != LayoutTypeset && mode != LayoutTTY {
		return nil
	}
	if m.mode == mode {
		return nil
	}
	m.pressedLink = 0
	m.mode = mode
	m.index = nil
	m.clearSelection()
	if mode == LayoutTTY {
		m.reflowTTY()
	}
	if mode == LayoutTypeset {
		m.resetTextLayout()
		return tea.Batch(m.SetVisible(true), m.showLinkFocus())
	}
	return tea.Batch(m.SetVisible(false), m.refreshHighlights())
}

// TypesetFit reports the current page fit policy.
func (m *Model) TypesetFit() TypesetFit { return m.typesetFit }

// SetTypesetFit changes the Typeset presentation without changing document text,
// queries, or selection. It resets pan/zoom; TTY layout is unaffected.
// Contain shows the complete page, so pan and zoom resume in Width mode.
func (m *Model) SetTypesetFit(fit TypesetFit) tea.Cmd {
	if fit != TypesetFitWidth && fit != TypesetFitContain || fit == m.typesetFit {
		return nil
	}
	m.typesetFit = fit
	m.zoom, m.x, m.y = 0, 0, 0
	m.mouseDown, m.dragging = false, false
	m.pressedLink = 0
	m.kittyBandActive = false
	m.focusCellRect = image.Rectangle{}
	m.focusKey = focusOverlayKey{}
	m.pic.ResetViewport()
	size := m.pic.SetSize(m.cols, m.rows)
	fitCmd := m.pic.SetFit(ntpicture.FitContain)
	if m.mode != LayoutTypeset {
		// The hidden picture may still contain only a previous Kitty band.
		// Restore the full source before returning to a contained Typeset view.
		var imageCmd tea.Cmd
		if m.source != nil {
			imageCmd = m.pic.SetImageCropped(m.source, m.cropRect())
		}
		return tea.Batch(size, fitCmd, imageCmd)
	}
	var focus tea.Cmd
	if m.focusPic != nil {
		focus = m.focusPic.SetVisible(fit == TypesetFitWidth)
	}
	return tea.Batch(size, fitCmd, focus, m.presentSource())
}

func (m *Model) ToggleLayout() tea.Cmd {
	if m.mode == LayoutTypeset {
		return m.SetLayoutMode(LayoutTTY)
	}
	return m.SetLayoutMode(LayoutTypeset)
}
func (m *Model) reflowTTY() {
	result, err := m.result.TTYLayout(context.Background(), m.cols)
	m.err = err
	m.ttyResult = result
	text := ""
	if result != nil {
		text = result.ANSI()
	}
	m.ttyLines = strings.Split(text, "\n")
	m.scrollTTY(0)
	m.resetTextLayout()
	m.refreshHighlights()
}
func (m *Model) scrollTTY(delta int) {
	m.ttyTop = max(0, min(max(0, len(m.ttyLines)-m.rows), m.ttyTop+delta))
}
func (m *Model) Close() tea.Cmd {
	if m.focusPic != nil {
		return tea.Batch(m.pic.Close(), m.focusPic.Close())
	}
	return m.pic.Close()
}
func (m *Model) SetSize(cols, rows int) tea.Cmd {
	m.pressedLink = 0
	m.cols = max(1, min(cols, 1000))
	m.rows = max(1, min(rows, 1000))
	m.clearSelection()
	var cropCmd, sizeCmd tea.Cmd
	if !m.kittyViewportMode() {
		cropCmd = m.viewport()
		sizeCmd = m.pic.SetSize(m.cols, m.rows)
	}
	var reflowCmd tea.Cmd
	reflowed := false
	if m.reflowTypeset && m.result != nil && m.result.Document() != nil {
		targetW := m.pic.RasterGeometry().Pixels.X
		if m.pic.Mode() == ntpicture.PictureKitty {
			cw, _ := m.pic.CellPixelSize()
			if cw > 0 {
				targetW = max(1, int(math.Round(float64(m.cols)*float64(cw)*m.pic.Resolution())))
			}
		}
		if targetW <= 0 || targetW == m.cols {
			targetW = m.cols * 10
		}
		targetW = max(32, min(4096, targetW))
		curW := m.result.Bounds().Dx()
		if curW == 0 && m.source != nil {
			curW = m.source.Bounds().Dx()
		}
		if curW != targetW {
			opts := m.renderOpts
			opts.Width = targetW
			opts.NoImage = false
			if next, err := m.result.Document().Render(context.Background(), opts); err == nil {
				m.result = next
				m.source = next.Image
				m.resetTextLayout()
				reflowCmd = m.presentSource()
				reflowed = true
			} else {
				m.err = err
			}
		}
	}
	if m.mode == LayoutTTY {
		m.reflowTTY()
		return tea.Batch(cropCmd, sizeCmd, reflowCmd, m.refreshHighlights())
	}
	if reflowed {
		return tea.Batch(cropCmd, sizeCmd, reflowCmd)
	}
	return tea.Batch(cropCmd, sizeCmd, m.refreshHighlights())
}

// Zoom changes magnification by delta steps; four steps double the scale.
func (m *Model) Zoom(delta int) tea.Cmd {
	if m.mode == LayoutTypeset && m.typesetFit == TypesetFitContain {
		return nil
	}
	m.pressedLink = 0
	hadSelection := m.selectStart != m.selectEnd
	m.clearSelection()
	if m.mode == LayoutTTY {
		return nil
	}
	m.zoom = max(0, min(maxZoomSteps, m.zoom+delta))
	if hadSelection {
		return m.refreshHighlights()
	}
	return m.viewport()
}
func (m *Model) Pan(dx, dy int) tea.Cmd {
	m.mouseDown, m.dragging = false, false
	m.pressedLink = 0
	if m.mode == LayoutTTY {
		step := dy / 20
		if step == 0 && dy != 0 {
			if dy > 0 {
				step = 1
			} else {
				step = -1
			}
		}
		m.scrollTTY(step)
		return nil
	}
	if m.typesetFit == TypesetFitContain {
		return nil
	}
	m.x += max(-4096, min(4096, dx))
	m.y += max(-16384, min(16384, dy))
	return m.viewport()
}
func (m *Model) Reset() tea.Cmd {
	m.pressedLink = 0
	if m.mode == LayoutTTY {
		m.ttyTop = 0
		return nil
	}
	m.zoom, m.x, m.y = 0, 0, 0
	return m.viewport()
}
func (m *Model) kittyViewportMode() bool {
	return m.mode == LayoutTypeset && m.typesetFit == TypesetFitWidth && m.pic != nil && m.pic.Mode() == ntpicture.PictureKitty && m.source != nil
}
func (m *Model) syncKittyBand(forceUpload bool) tea.Cmd {
	if !m.kittyViewportMode() {
		return nil
	}
	b := m.source.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}
	cellW, cellH := m.pic.CellPixelSize()
	if cellW <= 0 {
		cellW = 10
	}
	if cellH <= 0 {
		cellH = 20
	}
	visW := max(1, int(float64(srcW)/zoomScale(m.zoom)))
	tw := max(1, m.cols*cellW)
	scaledH := max(1, int(math.Ceil(float64(srcH)*zoomScale(m.zoom))))
	if visW > tw {
		scaledH = max(1, int((int64(srcH)*int64(tw)+int64(visW)-1)/int64(visW)))
	}
	pageRows := max(1, (scaledH+cellH-1)/cellH)
	maxBand := min(maxKittyBandRows, ntpicture.MaxImagePixels/max(1, m.cols*cellW*cellH))
	if maxRaster := m.pic.MaxRasterPixels(); maxRaster > 0 {
		maxBand = min(maxBand, maxRaster/max(1, m.cols*cellW*cellH))
	}
	maxBand = max(1, maxBand)
	bandRows := min(pageRows, maxBand)

	viewSrcH := min(srcH, max(1, int((int64(srcH)*int64(min(m.rows, pageRows))+int64(pageRows)-1)/int64(pageRows))))
	maxY := max(0, srcH-viewSrcH)
	m.x = max(0, min(m.x, srcW-visW))
	m.y = max(0, min(m.y, maxY))

	maxTopRow := max(0, pageRows-m.rows)
	topRow := 0
	if maxY > 0 && maxTopRow > 0 {
		topRow = min(maxTopRow, int((int64(m.y)*int64(maxTopRow)+int64(maxY)/2)/int64(maxY)))
		if m.y > 0 && topRow == 0 {
			topRow = 1
		}
	}

	if pageRows <= bandRows {
		m.bandStartRow = 0
	} else {
		needNewBand := !m.kittyBandActive ||
			topRow < m.bandStartRow ||
			topRow+min(m.rows, bandRows) > m.bandStartRow+bandRows ||
			(m.bandStartRow+bandRows < pageRows && topRow >= m.bandStartRow+bandRows-min(m.rows, bandRows/2))
		if needNewBand {
			margin := min(m.rows, max(0, (bandRows-m.rows)/2))
			m.bandStartRow = max(0, min(topRow-margin, pageRows-bandRows))
		}
	}

	y0 := b.Min.Y
	y1 := b.Max.Y
	if pageRows > bandRows {
		y0 = b.Min.Y + int((int64(m.bandStartRow)*int64(srcH))/int64(pageRows))
		y1 = b.Min.Y + int((int64(m.bandStartRow+bandRows)*int64(srcH)+int64(pageRows)-1)/int64(pageRows))
		y1 = min(b.Max.Y, max(y0+1, y1))
	}
	bandCrop := image.Rect(b.Min.X+m.x, y0, b.Min.X+m.x+visW, y1)
	rowOffset := max(0, min(topRow-m.bandStartRow, max(0, bandRows-min(m.rows, bandRows))))
	m.pic.SetViewport(ntpicture.Viewport{X: 0, Y: rowOffset, Width: m.cols, Height: m.rows})

	cellSize := image.Pt(cellW, cellH)
	needUpload := forceUpload ||
		!m.kittyBandActive ||
		m.kittyCellSize != cellSize ||
		m.bandCols != m.cols ||
		m.bandRows != bandRows ||
		m.bandCrop != bandCrop
	m.kittyBandActive = true
	m.kittyCellSize = cellSize
	m.bandCols = m.cols
	m.bandRows = bandRows
	m.bandCrop = bandCrop
	if !needUpload {
		return nil
	}
	sizeCmd := m.pic.SetSize(m.cols, bandRows)
	if m.overlayImg != nil {
		return tea.Batch(sizeCmd, m.pic.SetImageImmutableCropped(cropBandImage(m.overlayImg, bandCrop), image.Rectangle{}))
	}
	return tea.Batch(sizeCmd, m.pic.SetImageCropped(cropBandImage(m.source, bandCrop), image.Rectangle{}))
}
func (m *Model) viewport() tea.Cmd {
	if m.source == nil {
		return nil
	}
	if m.kittyViewportMode() {
		return tea.Batch(m.syncKittyBand(false), m.updateKittyFocusOverlay())
	}
	return m.pic.SetCrop(m.cropRect())
}
func (m *Model) cropRect() image.Rectangle {
	if m.source == nil {
		return image.Rectangle{}
	}
	b := m.source.Bounds()
	if m.typesetFit == TypesetFitContain {
		return b
	}
	w := max(1, int(float64(b.Dx())/zoomScale(m.zoom)))
	h := min(b.Dy(), max(1, w*m.rows*2/m.cols))
	m.x = max(0, min(m.x, b.Dx()-w))
	m.y = max(0, min(m.y, b.Dy()-h))
	return image.Rect(b.Min.X+m.x, b.Min.Y+m.y, b.Min.X+m.x+w, b.Min.Y+m.y+h)
}

// Update handles keyboard navigation, link clicks, and mouse-wheel scrolling.
// Only forward input while focused; always forward non-input messages.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch v := msg.(type) {
	case tea.PasteMsg:
		if m.searching {
			for _, r := range v.Content {
				if r >= 32 && len([]rune(m.searchInput)) < 256 {
					m.searchInput += string(r)
				}
			}
			return nil
		}
	case tea.MouseClickMsg:
		cmd = m.mousePress(v)
	case tea.MouseReleaseMsg:
		cmd = m.mouseRelease(v)
	case tea.MouseMotionMsg:
		cmd = m.selectMotion(image.Pt(v.X, v.Y))
		if image.Pt(v.X, v.Y) != m.pressCell {
			m.pressedLink = 0
		}
	case tea.KeyPressMsg:
		if m.searching {
			return m.searchKey(v)
		}
		m.pressedLink = 0
		switch v.String() {
		case "/":
			m.searching = true
			m.searchInput = m.query
		case "n", "N":
			cmd = m.NextMatch(v.String() == "N")
		case "esc":
			m.query = ""
			m.matches = nil
			m.match = -1
			m.focusID = 0
			m.clearSourceFocus()
			m.clearSelection()
			cmd = m.refreshHighlights()
		case "ctrl+a":
			cmd = m.SelectRange(0, len(m.textIndex().Text))
		case "v":
			if m.match >= 0 && m.match < len(m.matches) {
				hit := m.matches[m.match]
				cmd = m.SelectRange(hit.Start, hit.End)
			}
		case "y":
			cmd = m.CopySelection()
		case "tab":
			cmd = m.NextLink(false)
		case "shift+tab":
			cmd = m.NextLink(true)
		case "enter":
			cmd = m.ActivateLink()
		case "t":
			cmd = m.ToggleLayout()
		case "+", "=":
			cmd = m.Zoom(1)
		case "-":
			cmd = m.Zoom(-1)
		case "0", "home":
			cmd = m.Reset()
		case "left", "h":
			cmd = m.Pan(-40, 0)
		case "right", "l":
			cmd = m.Pan(40, 0)
		case "up", "k":
			cmd = m.Pan(0, -40)
		case "down", "j":
			cmd = m.Pan(0, 40)
		case "pgup":
			if m.mode == LayoutTTY {
				m.scrollTTY(-m.rows)
			} else {
				cmd = m.Pan(0, -300)
			}
		case "pgdown":
			if m.mode == LayoutTTY {
				m.scrollTTY(m.rows)
			} else {
				cmd = m.Pan(0, 300)
			}
		}
	case tea.MouseWheelMsg:
		switch v.Button {
		case tea.MouseWheelUp:
			cmd = m.Pan(0, -40)
		case tea.MouseWheelDown:
			cmd = m.Pan(0, 40)
		}
	}
	wasKitty := m.kittyViewportMode()
	prevCW, prevCH := m.pic.CellPixelSize()
	var preBandCmd tea.Cmd
	if wasKitty && (!m.kittyBandActive || m.kittyCellSize != image.Pt(prevCW, prevCH)) {
		preBandCmd = tea.Batch(m.syncKittyBand(false), m.updateKittyFocusOverlay())
	}
	picCmd := m.pic.Update(msg)
	var focusCmd tea.Cmd
	if m.focusPic != nil {
		focusCmd = m.focusPic.Update(msg)
	}
	nowKitty := m.kittyViewportMode()
	nowCW, nowCH := m.pic.CellPixelSize()
	var postBandCmd tea.Cmd
	if nowKitty && (!wasKitty || !m.kittyBandActive || m.kittyCellSize != image.Pt(nowCW, nowCH)) {
		postBandCmd = tea.Batch(m.syncKittyBand(false), m.updateKittyFocusOverlay())
	} else if !nowKitty && m.kittyBandActive {
		m.kittyBandActive = false
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
		m.pic.ResetViewport()
		postBandCmd = tea.Batch(m.pic.SetCrop(m.cropRect()), m.pic.SetSize(m.cols, m.rows), m.presentSource())
	}
	return tea.Batch(cmd, preBandCmd, picCmd, focusCmd, postBandCmd)
}
