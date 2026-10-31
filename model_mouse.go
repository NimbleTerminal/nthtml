package nthtml

import (
	"image"
	"math"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml/document"
)

// SetBounds sets the widget's zero-based terminal-cell origin and size. The host
// must compose View at this origin. SetSize keeps the existing origin.
func (m *Model) SetBounds(x, y, cols, rows int) tea.Cmd {
	m.origin = image.Pt(max(0, min(x, 1<<20)), max(0, min(y, 1<<20)))
	return m.SetSize(cols, rows)
}

// CellToDocument maps a terminal screen cell to the document-pixel rectangle it
// displays in LayoutTypeset mode, accounting for widget origin, scroll/crop,
// zoom, scaling, and letterboxing. It returns false in LayoutTTY mode, outside
// the widget or letterbox bounds, or when no raster image is loaded.
func (m *Model) CellToDocument(p image.Point) (image.Rectangle, bool) {
	if m == nil || m.mode != LayoutTypeset {
		return image.Rectangle{}, false
	}
	area := m.cellArea(p)
	return area, !area.Empty()
}

// DocumentViewport reports the visible document-pixel bounds in LayoutTypeset
// mode, excluding letterboxing and accounting for the actual band, cell scroll,
// zoom, and raster scaling. Min is the displayed scroll origin; Size is the
// visible extent. Fractional pixel edges are rounded outward once for the whole
// viewport. It returns false in LayoutTTY mode or when no document pixels are
// visible. Like CellToDocument, it describes the current target geometry, which
// may be pending presentation; it neither scrolls nor changes selection.
func (m *Model) DocumentViewport() (image.Rectangle, bool) {
	cells, pixels, crop, view := m.rasterMapping()
	area := rasterAreaSource(view, cells, pixels, crop)
	return area, !area.Empty()
}

// LinkAtCell hit-tests a terminal cell using the current layout, scroll/crop,
// scaling, and letterboxing. Unlike Result.LinkAt, p is in terminal screen cells.
// A coarse raster cell can cover several pixels; the greatest visible link
// overlap wins. Callers should not activate links while a new raster is pending.
func (m *Model) LinkAtCell(p image.Point) (document.Link, bool) {
	area := m.cellArea(p)
	if area.Empty() {
		return document.Link{}, false
	}
	var best document.Link
	bestArea := 0
	for _, link := range m.links() {
		overlap := 0
		for _, r := range link.Regions {
			hit := r.Intersect(area)
			overlap += hit.Dx() * hit.Dy()
		}
		if overlap > bestArea {
			best = link
			bestArea = overlap
		}
	}
	if bestArea == 0 {
		return document.Link{}, false
	}
	best.Regions = append([]image.Rectangle(nil), best.Regions...)
	return best, true
}

// Match ntpicture's centered FitContain, including its integer rounding. The
// glyph transport uses cols × 2*rows pixels; Kitty uses its raster target size.
func rasterCellSource(p, cells, pixels image.Point, crop image.Rectangle) image.Rectangle {
	return rasterAreaSource(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))}, cells, pixels, crop)
}

// rasterAreaSource maps a rectangle of cells in the full picture grid to its
// source pixels, clipping letterboxing before rounding the outside edges.
func rasterAreaSource(area image.Rectangle, cells, pixels image.Point, crop image.Rectangle) image.Rectangle {
	area = area.Intersect(image.Rectangle{Max: cells})
	if area.Empty() {
		return image.Rectangle{}
	}
	if cells.X <= 0 || cells.Y <= 0 || pixels.X <= 0 || pixels.Y <= 0 || crop.Empty() {
		return image.Rectangle{}
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
	left := max(float64(ox), float64(area.Min.X)*float64(tw)/float64(cells.X))
	top := max(float64(oy), float64(area.Min.Y)*float64(th)/float64(cells.Y))
	right := min(float64(ox+iw), float64(area.Max.X)*float64(tw)/float64(cells.X))
	bottom := min(float64(oy+ih), float64(area.Max.Y)*float64(th)/float64(cells.Y))
	if right <= left || bottom <= top {
		return image.Rectangle{}
	}
	return image.Rect(crop.Min.X+int(math.Floor((left-float64(ox))*float64(sw)/float64(iw))), crop.Min.Y+int(math.Floor((top-float64(oy))*float64(sh)/float64(ih))), crop.Min.X+int(math.Ceil((right-float64(ox))*float64(sw)/float64(iw))), crop.Min.Y+int(math.Ceil((bottom-float64(oy))*float64(sh)/float64(ih)))).Intersect(crop)
}
func (m *Model) mouseReady() bool {
	if m.pic.Closed() {
		return false
	}
	if m.mode == LayoutTTY {
		return true
	}
	s := m.pic.Status()
	return m.pic.Visible() && !m.pic.Closed() && s.Err == nil && s.Presented > 0 && s.Presented == s.Generation
}
func (m *Model) mousePress(v tea.MouseClickMsg) tea.Cmd {
	m.pressedLink = 0
	m.mouseDown, m.dragging = false, false
	if v.Button != tea.MouseLeft || !m.mouseReady() {
		return nil
	}
	m.pressCell = image.Pt(v.X, v.Y)
	if hit, ok := m.textIndex().Hit(m.cellArea(m.pressCell)); ok {
		m.mouseDown = true
		m.dragStart = hit
	}
	if link, ok := m.LinkAtCell(image.Pt(v.X, v.Y)); ok {
		m.pressedLink = link.ID
		m.pressCell = image.Pt(v.X, v.Y)
	}
	return nil
}
func (m *Model) mouseRelease(v tea.MouseReleaseMsg) tea.Cmd {
	if m.mouseDown {
		cmd := m.selectMotion(image.Pt(v.X, v.Y))
		dragged := m.dragging
		m.mouseDown, m.dragging = false, false
		if dragged {
			m.pressedLink = 0
			return cmd
		}
	}
	pressed := m.pressedLink
	m.pressedLink = 0
	if pressed == 0 || (v.Button != tea.MouseLeft && v.Button != tea.MouseNone) || !m.mouseReady() {
		return nil
	}
	if image.Pt(v.X, v.Y) != m.pressCell {
		return nil
	}
	link, ok := m.LinkAtCell(image.Pt(v.X, v.Y))
	if !ok || link.ID != pressed {
		return nil
	}
	m.clearSourceFocus()
	m.focusID = link.ID
	if m.mode == LayoutTTY {
		return tea.Batch(m.refreshHighlights(), m.ActivateLink())
	}
	return tea.Batch(m.showLinkFocus(), m.ActivateLink())
}
