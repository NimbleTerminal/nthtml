package nthtml

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

// LinkMsg asks the host to activate a link. The component never starts a browser
// or performs network I/O. Relative destinations must be resolved by the host.
type LinkMsg struct {
	Owner *Model
	Link  document.Link
}

func (m *Model) links() []document.Link {
	if m.mode == LayoutTTY && m.ttyResult != nil {
		return m.ttyResult.Links
	}
	if m.result != nil {
		return m.result.Links
	}
	return nil
}

// FocusedLink reports the keyboard-selected link, with independent region data.
func (m *Model) FocusedLink() (document.Link, bool) {
	for _, l := range m.links() {
		if l.ID == m.focusID {
			l.Regions = append([]image.Rectangle(nil), l.Regions...)
			return l, true
		}
	}
	return document.Link{}, false
}

// NextLink cycles through visible document links and scrolls the selection into view.
func (m *Model) NextLink(backward bool) tea.Cmd {
	m.pressedLink = 0
	m.clearSourceFocus()
	links := m.links()
	if len(links) == 0 {
		return m.refreshHighlights()
	}
	index := -1
	for i, l := range links {
		if l.ID == m.focusID {
			index = i
			break
		}
	}
	if backward {
		if index < 0 {
			index = 0
		}
		index = (index + len(links) - 1) % len(links)
	} else {
		index = (index + 1) % len(links)
	}
	m.focusID = links[index].ID
	if len(links[index].Regions) > 0 {
		p := links[index].Regions[0].Min
		if m.mode == LayoutTTY {
			m.ttyTop = p.Y
			m.scrollTTY(0)
		} else {
			m.x = p.X
			m.y = p.Y
		}
	}
	if m.mode == LayoutTTY {
		return m.refreshHighlights()
	}
	return m.showLinkFocus()
}
func (m *Model) hasSearchOrSelectionOverlay() bool {
	return (m.match >= 0 && m.match < len(m.matches)) || m.selectStart != m.selectEnd
}
func (m *Model) hasOverlay() bool {
	return len(m.focusRegions()) > 0 || m.hasSearchOrSelectionOverlay()
}
func (m *Model) paintSearchAndSelectionOverlay() *image.RGBA {
	b := m.source.Bounds()
	img := image.NewRGBA(b)
	draw.Draw(img, b, m.source, b.Min, draw.Src)
	if m.match >= 0 && m.match < len(m.matches) {
		hit := m.matches[m.match]
		for _, r := range m.textIndex().Regions(hit.Start, hit.End) {
			draw.Draw(img, r, image.NewUniform(color.NRGBA{255, 210, 35, 70}), image.Point{}, draw.Over)
		}
	}
	for _, r := range m.textIndex().Regions(m.selectStart, m.selectEnd) {
		draw.Draw(img, r, image.NewUniform(color.NRGBA{40, 120, 255, 110}), image.Point{}, draw.Over)
	}
	return img
}
func (m *Model) presentSource() tea.Cmd {
	m.overlayPainted = false
	m.overlayImg = nil
	m.focusKey = focusOverlayKey{}
	if m.source == nil {
		m.kittyBandActive = false
		m.focusCellRect = image.Rectangle{}
		return nil
	}
	if m.kittyViewportMode() {
		if m.hasSearchOrSelectionOverlay() {
			m.overlayImg = m.paintSearchAndSelectionOverlay()
			m.overlayPainted = true
		}
		return tea.Batch(m.syncKittyBand(true), m.updateKittyFocusOverlay())
	}
	m.kittyBandActive = false
	m.focusCellRect = image.Rectangle{}
	if m.hasOverlay() {
		return m.showLinkFocus()
	}
	return m.pic.SetImageCropped(m.source, m.cropRect())
}
func (m *Model) showLinkFocus() tea.Cmd {
	if m.source == nil {
		return nil
	}
	if m.kittyViewportMode() {
		var bandCmd tea.Cmd
		if m.hasSearchOrSelectionOverlay() {
			m.overlayImg = m.paintSearchAndSelectionOverlay()
			m.overlayPainted = true
			bandCmd = m.syncKittyBand(true)
		} else if m.overlayPainted {
			m.overlayPainted = false
			m.overlayImg = nil
			bandCmd = m.syncKittyBand(true)
		} else {
			bandCmd = m.syncKittyBand(false)
		}
		focusCmd := m.updateKittyFocusOverlay()
		return tea.Batch(bandCmd, focusCmd)
	}
	regions := m.focusRegions()
	if len(regions) == 0 && !m.hasSearchOrSelectionOverlay() {
		if m.overlayPainted {
			m.overlayPainted = false
			m.overlayImg = nil
			return m.pic.SetImageCropped(m.source, m.cropRect())
		}
		return m.viewport()
	}
	img := m.paintSearchAndSelectionOverlay()
	ink := image.NewUniform(color.RGBA{235, 155, 35, 255})
	for _, r := range regions {
		r = r.Inset(-1)
		for _, edge := range []image.Rectangle{image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)} {
			draw.Draw(img, edge, ink, image.Point{}, draw.Over)
		}
	}
	m.overlayPainted = true
	return m.pic.SetImageImmutableCropped(img, m.cropRect())
}

func (m *Model) updateKittyFocusOverlay() tea.Cmd {
	if !m.kittyViewportMode() || m.focusPic == nil {
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
		return nil
	}
	regions := m.focusRegions()
	if len(regions) == 0 {
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
		return nil
	}
	cells := image.Pt(m.bandCols, m.bandRows)
	pixels := m.pic.RasterGeometry().Pixels
	crop := m.bandCrop
	if cells.X <= 0 || cells.Y <= 0 || pixels.X <= 0 || pixels.Y <= 0 || crop.Empty() {
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
		return nil
	}
	cellRect, pagePixels := focusBandCellsAndPage(regions, cells, pixels, crop)
	if cellRect.Empty() || pagePixels.Empty() {
		m.focusCellRect = image.Rectangle{}
		m.focusKey = focusOverlayKey{}
		return nil
	}
	m.focusCellRect = cellRect
	cellW, cellH := m.pic.CellPixelSize()
	if cellW <= 0 {
		cellW = 10
	}
	if cellH <= 0 {
		cellH = 20
	}
	key := focusOverlayKey{
		focusID:    m.focusID,
		sourceID:   m.focusSourceID,
		pixels:     pixels,
		background: color.RGBA64Model.Convert(m.pic.Background()).(color.RGBA64),
		bandCrop:   crop,
		cols:       cells.X,
		bandRows:   cells.Y,
		cellW:      cellW,
		cellH:      cellH,
		match:      m.match,
		selStart:   m.selectStart,
		selEnd:     m.selectEnd,
	}
	if m.focusKey == key {
		return nil
	}
	m.focusKey = key
	base := m.source
	if m.overlayImg != nil {
		base = m.overlayImg
	}
	patch := paintFocusPatch(base, regions, crop, pagePixels, cellRect, cells, pixels, image.Pt(cellW, cellH), m.pic.Background())
	return m.focusPic.Submit(ntpicture.Frame{
		Image:      patch,
		Cols:       cellRect.Dx(),
		Rows:       cellRect.Dy(),
		Mode:       ntpicture.PictureKitty,
		CellWidth:  cellW,
		CellHeight: cellH,
		Immutable:  true,
	})
}

func focusBandCellsAndPage(regions []image.Rectangle, cells, pixels image.Point, crop image.Rectangle) (image.Rectangle, image.Rectangle) {
	if cells.X <= 0 || cells.Y <= 0 || pixels.X <= 0 || pixels.Y <= 0 || crop.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	tw, th := pixels.X, pixels.Y
	sw, sh := crop.Dx(), crop.Dy()
	iw, ih := tw, th
	if int64(sw)*int64(th) >= int64(sh)*int64(tw) {
		ih = max(1, int(int64(sh)*int64(tw)/int64(sw)))
	} else {
		iw = max(1, int(int64(sw)*int64(th)/int64(sh)))
	}
	ox, oy := (tw-iw)/2, (th-ih)/2
	var cellUnion image.Rectangle
	for _, r := range regions {
		ring := r.Inset(-1).Intersect(crop)
		if ring.Empty() {
			continue
		}
		rx0 := float64(ox) + float64(ring.Min.X-crop.Min.X)*float64(iw)/float64(sw)
		ry0 := float64(oy) + float64(ring.Min.Y-crop.Min.Y)*float64(ih)/float64(sh)
		rx1 := float64(ox) + float64(ring.Max.X-crop.Min.X)*float64(iw)/float64(sw)
		ry1 := float64(oy) + float64(ring.Max.Y-crop.Min.Y)*float64(ih)/float64(sh)
		cx0 := max(0, min(cells.X-1, int(math.Floor(rx0*float64(cells.X)/float64(tw)))))
		cy0 := max(0, min(cells.Y-1, int(math.Floor(ry0*float64(cells.Y)/float64(th)))))
		cx1 := max(cx0+1, min(cells.X, int(math.Ceil(rx1*float64(cells.X)/float64(tw)))))
		cy1 := max(cy0+1, min(cells.Y, int(math.Ceil(ry1*float64(cells.Y)/float64(th)))))
		cr := image.Rect(cx0, cy0, cx1, cy1)
		if cellUnion.Empty() {
			cellUnion = cr
		} else {
			cellUnion = cellUnion.Union(cr)
		}
	}
	if cellUnion.Empty() || cellUnion.Dx() > maxKittyBandRows || cellUnion.Dy() > maxKittyBandRows {
		return image.Rectangle{}, image.Rectangle{}
	}
	return cellUnion, image.Rect(ox, oy, ox+iw, oy+ih)
}

// focusRingImage adds the same source-pixel ring as the fallback picture path.
// Keeping the entire crop as the scaler's source preserves sampling at patch
// edges; only the destination is clipped to the selected whole cells.
type focusRingImage struct {
	image.Image
	edges []image.Rectangle
}

func (im focusRingImage) At(x, y int) color.Color {
	for _, edge := range im.edges {
		if image.Pt(x, y).In(edge) {
			return color.RGBA{235, 155, 35, 255}
		}
	}
	return im.Image.At(x, y)
}

func paintFocusPatch(base image.Image, regions []image.Rectangle, crop, pagePixels, cellRect image.Rectangle, cells, pixels, cellSize image.Point, bg color.Color) *image.RGBA {
	patchRect := image.Rect(cellRect.Min.X*cellSize.X, cellRect.Min.Y*cellSize.Y, cellRect.Max.X*cellSize.X, cellRect.Max.Y*cellSize.Y)
	patch := image.NewRGBA(patchRect)
	draw.Draw(patch, patchRect, image.NewUniform(bg), image.Point{}, draw.Src)
	ring := focusRingImage{Image: base}
	for _, r := range regions {
		r = r.Inset(-1)
		ring.edges = append(ring.edges,
			image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1),
			image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y),
			image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y),
			image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y))
	}
	// Usually raster pixels equal terminal pixels. Use the base raster's actual
	// letterbox proportions when its resolution has been reduced.
	target := image.Rect(
		pagePixels.Min.X*cells.X*cellSize.X/pixels.X,
		pagePixels.Min.Y*cells.Y*cellSize.Y/pixels.Y,
		pagePixels.Max.X*cells.X*cellSize.X/pixels.X,
		pagePixels.Max.Y*cells.Y*cellSize.Y/pixels.Y)
	if target.Size() == crop.Size() {
		draw.Draw(patch, target, ring, crop.Min, draw.Over)
	} else {
		xdraw.CatmullRom.Scale(patch, target, ring, crop, draw.Over, nil)
	}
	return patch
}

func (m *Model) spliceFocusOverlay(base string) string {
	if base == "" || m.focusPic == nil || m.focusCellRect.Empty() {
		return base
	}
	vp, ok := m.pic.Viewport()
	if !ok {
		return base
	}
	screenRect := m.focusCellRect.Sub(image.Pt(vp.X, vp.Y))
	visRect := screenRect.Intersect(image.Rect(0, 0, m.cols, m.rows))
	if visRect.Empty() {
		return base
	}
	focusView := m.focusPic.View()
	if focusView == "" || !strings.ContainsRune(focusView, '\U0010EEEE') {
		return base
	}
	baseLines := strings.Split(base, "\n")
	focusLines := strings.Split(focusView, "\n")
	changed := false
	for sy := visRect.Min.Y; sy < visRect.Max.Y && sy < len(baseLines); sy++ {
		fy := sy - screenRect.Min.Y
		if fy < 0 || fy >= len(focusLines) {
			continue
		}
		fx0 := visRect.Min.X - screenRect.Min.X
		fx1 := visRect.Max.X - screenRect.Min.X
		spliced := spliceKittyRow(baseLines[sy], focusLines[fy], visRect.Min.X, visRect.Max.X, fx0, fx1)
		if spliced != baseLines[sy] {
			baseLines[sy] = spliced
			changed = true
		}
	}
	if !changed {
		return base
	}
	return strings.Join(baseLines, "\n")
}

func parseKittyRowCells(row string) (sgr string, cells []string, ok bool) {
	first := strings.IndexRune(row, '\U0010EEEE')
	if first < 0 {
		return "", nil, false
	}
	end := strings.LastIndex(row, "\x1b[39m")
	if end < first {
		return "", nil, false
	}
	sgr = row[:first]
	body := row[first:end]
	var starts []int
	for i, r := range body {
		if r == '\U0010EEEE' {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return "", nil, false
	}
	cells = make([]string, len(starts))
	for i, st := range starts {
		if i+1 < len(starts) {
			cells[i] = body[st:starts[i+1]]
		} else {
			cells[i] = body[st:]
		}
	}
	return sgr, cells, true
}

func spliceKittyRow(baseRow, focusRow string, bx0, bx1, fx0, fx1 int) string {
	baseSGR, baseCells, ok1 := parseKittyRowCells(baseRow)
	focusSGR, focusCells, ok2 := parseKittyRowCells(focusRow)
	if !ok1 || !ok2 || bx0 < 0 || bx1 > len(baseCells) || bx0 >= bx1 || fx0 < 0 || fx1 > len(focusCells) || fx1-fx0 != bx1-bx0 {
		return baseRow
	}
	var sb strings.Builder
	sb.Grow(len(baseRow) + len(focusSGR) + len(baseSGR))
	if bx0 > 0 {
		sb.WriteString(baseSGR)
		for _, c := range baseCells[:bx0] {
			sb.WriteString(c)
		}
	}
	sb.WriteString(focusSGR)
	for _, c := range focusCells[fx0:fx1] {
		sb.WriteString(c)
	}
	if bx1 < len(baseCells) {
		sb.WriteString(baseSGR)
		for _, c := range baseCells[bx1:] {
			sb.WriteString(c)
		}
	}
	sb.WriteString("\x1b[39m")
	return sb.String()
}

// ActivateLink emits a LinkMsg for the host, or does nothing without a selection.
func (m *Model) ActivateLink() tea.Cmd {
	l, ok := m.FocusedLink()
	if !ok {
		return nil
	}
	if strings.HasPrefix(l.Href, "#") {
		cmd, _ := m.NavigateFragment(l.Href)
		return cmd
	}
	return func() tea.Msg { return LinkMsg{Owner: m, Link: l} }
}
