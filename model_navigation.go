package nthtml

import (
	tea "charm.land/bubbletea/v2"
	"image"
	"nimbleterminal.dev/nthtml/document"
	"strings"
	"unicode/utf8"
)

// CopyMsg requests that the host copy selected text. The widget does not read
// or write the system clipboard itself.
type CopyMsg struct {
	Owner *Model
	Text  string
}

func (m *Model) textIndex() *document.TextIndex {
	if m.index == nil {
		if m.mode == LayoutTTY {
			m.index = m.ttyResult.TextIndex()
		} else {
			m.index = m.result.TextIndex()
		}
	}
	return m.index
}

// NavigateFragment scrolls local #id links without emitting a LinkMsg.
func (m *Model) NavigateFragment(href string) (tea.Cmd, bool) {
	var p image.Point
	var ok bool
	if m.mode == LayoutTTY && m.ttyResult != nil {
		p, ok = m.ttyResult.Fragment(href)
	} else if m.result != nil {
		p, ok = m.result.Fragment(href)
	}
	if !ok {
		return nil, false
	}
	m.focusID = 0
	m.clearSelection()
	cmd := m.scrollTo(p)
	if m.mode == LayoutTTY {
		cmd = m.refreshHighlights()
	}
	return cmd, true
}

// NavigateSource scrolls to the HTML element with id (or #fragment) in the
// active layout mode.
func (m *Model) NavigateSource(id string) (tea.Cmd, bool) {
	if id == "" {
		return nil, false
	}
	if strings.HasPrefix(id, "#") {
		return m.NavigateFragment(id)
	}
	var rect image.Rectangle
	var ok bool
	if m.mode == LayoutTTY && m.ttyResult != nil {
		rect, ok = m.ttyResult.AnchorBounds(id)
	} else if m.result != nil {
		rect, ok = m.result.AnchorBounds(id)
	}
	if !ok {
		return nil, false
	}
	m.focusID = 0
	m.clearSelection()
	cmd := m.scrollTo(rect.Min)
	if m.mode == LayoutTTY {
		cmd = m.refreshHighlights()
	}
	return cmd, true
}
func (m *Model) scrollTo(p image.Point) tea.Cmd {
	if m.mode == LayoutTTY {
		m.ttyTop = p.Y
		m.scrollTTY(0)
		return nil
	}
	m.x, m.y = p.X, p.Y
	return m.showLinkFocus()
}

// Search sets a literal case-insensitive query and selects its first match.
func (m *Model) Search(query string) tea.Cmd {
	m.query = query
	m.matches = m.textIndex().Search(query)
	m.match = -1
	m.clearSelection()
	return m.NextMatch(false)
}
func (m *Model) SearchStatus() (query string, current, total int) {
	return m.query, m.match + 1, len(m.matches)
}
func (m *Model) SearchInputActive() bool { return m.searching }
func (m *Model) SearchInput() string     { return m.searchInput }
func (m *Model) NextMatch(backward bool) tea.Cmd {
	if len(m.matches) == 0 {
		return m.refreshHighlights()
	}
	if backward {
		if m.match < 0 {
			m.match = 0
		}
		m.match = (m.match + len(m.matches) - 1) % len(m.matches)
	} else {
		m.match = (m.match + 1) % len(m.matches)
	}
	m.focusID = 0
	hit := m.matches[m.match]
	regions := m.textIndex().Regions(hit.Start, hit.End)
	if len(regions) > 0 {
		cmd := m.scrollTo(regions[0].Min)
		if m.mode == LayoutTTY {
			return m.refreshHighlights()
		}
		return cmd
	}
	return m.refreshHighlights()
}

// SelectRange selects UTF-8 offsets from the current layout's TextIndex.
func (m *Model) SelectRange(start, end int) tea.Cmd {
	m.selectStart, m.selectEnd = m.textIndex().Range(start, end)
	return m.refreshHighlights()
}
func (m *Model) SelectedText() string { return m.textIndex().Selected(m.selectStart, m.selectEnd) }

// SelectedSourceID returns the HTML element SourceID owning the current
// selection or active search match.
func (m *Model) SelectedSourceID() (string, bool) {
	if m.selectStart != m.selectEnd {
		return m.textIndex().SourceAt(m.selectStart)
	}
	if m.match >= 0 && m.match < len(m.matches) {
		return m.textIndex().SourceAt(m.matches[m.match].Start)
	}
	return "", false
}

// SourceAtCell resolves the innermost HTML element SourceID at terminal cell p.
func (m *Model) SourceAtCell(p image.Point) (string, bool) {
	area := m.cellArea(p)
	if area.Empty() {
		return "", false
	}
	if m.mode == LayoutTTY && m.ttyResult != nil {
		return m.ttyResult.SourceAt(area.Min)
	}
	if m.result != nil {
		return m.result.SourceAt(area.Min)
	}
	return m.textIndex().HitSource(area)
}

// ActiveSourceID returns the SourceID of the current selection/search match if
// active, or the SourceID at the top of the current viewport.
func (m *Model) ActiveSourceID() (string, bool) {
	if id, ok := m.SelectedSourceID(); ok {
		return id, true
	}
	if m.mode == LayoutTTY && m.ttyResult != nil {
		return m.ttyResult.SourceAt(image.Pt(0, m.ttyTop))
	}
	if m.result != nil {
		crop := m.cropRect()
		if !crop.Empty() {
			if id, ok := m.result.SourceAt(crop.Min); ok {
				return id, true
			}
			return m.textIndex().HitSource(image.Rect(crop.Min.X, crop.Min.Y, crop.Max.X, crop.Min.Y+32))
		}
	}
	return "", false
}
func (m *Model) CopySelection() tea.Cmd {
	text := m.SelectedText()
	if text == "" || len(text) > 1<<20 {
		return nil
	}
	return func() tea.Msg { return CopyMsg{m, text} }
}
func (m *Model) clearSelection() {
	m.selectStart, m.selectEnd = 0, 0
	m.mouseDown, m.dragging = false, false
}
func (m *Model) refreshHighlights() tea.Cmd {
	if m.mode == LayoutTTY {
		spans := append([]document.Match(nil), m.matches...)
		if m.selectStart != m.selectEnd {
			spans = append(spans, document.Match{Start: m.selectStart, End: m.selectEnd})
		}
		m.ttyLines = strings.Split(m.ttyResult.ANSIHighlights(spans), "\n")
		return nil
	}
	return m.showLinkFocus()
}
func (m *Model) resetTextLayout() {
	prevMatch := m.match
	m.index = nil
	m.clearSelection()
	if m.query == "" {
		m.matches = nil
		m.match = -1
		return
	}
	m.matches = m.textIndex().Search(m.query)
	if prevMatch >= 0 && len(m.matches) > 0 {
		m.match = min(prevMatch, len(m.matches)-1)
	} else {
		m.match = -1
	}
}
func (m *Model) cellArea(p image.Point) image.Rectangle {
	p = p.Sub(m.origin)
	if p.X < 0 || p.Y < 0 || p.X >= m.cols || p.Y >= m.rows {
		return image.Rectangle{}
	}
	if m.mode == LayoutTTY {
		return image.Rect(p.X, p.Y+m.ttyTop, p.X+1, p.Y+m.ttyTop+1)
	}
	return rasterCellSource(p, image.Pt(m.cols, m.rows), m.pic.RasterGeometry().Pixels, m.cropRect())
}
func (m *Model) selectMotion(p image.Point) tea.Cmd {
	if !m.mouseDown || p == m.pressCell {
		return nil
	}
	if hit, ok := m.textIndex().Hit(m.cellArea(p)); ok {
		m.dragging = true
		m.pressedLink = 0
		start, end := m.dragStart.Start, hit.End
		if hit.Start < m.dragStart.Start {
			start, end = hit.Start, m.dragStart.End
		}
		return m.SelectRange(start, end)
	}
	return nil
}
func (m *Model) searchKey(v tea.KeyPressMsg) tea.Cmd {
	switch v.String() {
	case "esc":
		m.searching = false
		return nil
	case "enter":
		m.searching = false
		return m.Search(m.searchInput)
	case "backspace":
		_, size := utf8.DecodeLastRuneInString(m.searchInput)
		m.searchInput = m.searchInput[:len(m.searchInput)-size]
	default:
		text := v.Text
		if text == "" && v.Code >= 32 && v.Code <= utf8.MaxRune && v.Mod == 0 {
			text = string(v.Code)
		}
		if utf8.RuneCountInString(m.searchInput+text) <= 256 {
			m.searchInput += text
		}
	}
	return nil
}
