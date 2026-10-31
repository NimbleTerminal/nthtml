package nthtml

import (
	"image"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml/document"
)

// FocusSource highlights the element whose Anchor or SourceID is id and scrolls
// it into view. An empty id clears focus. An unknown id returns false without
// changing the view. SetResult and SetResultKeepView clear source focus.
// In TTY mode, mapped source text runs use reverse video; elements without TTY
// text have no visible highlight. Source focus does not activate links.
func (m *Model) FocusSource(id string) (tea.Cmd, bool) {
	if id == "" {
		m.focusID = 0
		m.clearSourceFocus()
		return m.refreshHighlights(), true
	}
	if m.result == nil {
		return nil, false
	}
	var bounds image.Rectangle
	for _, b := range m.result.Boxes {
		if b.Anchor == id || b.SourceID == id {
			r := b.Bounds
			if r.Empty() {
				r = b.InkBounds
			}
			bounds = bounds.Union(r)
		}
	}
	for _, run := range m.result.TextIndex().Runs {
		if run.SourceID == id {
			bounds = bounds.Union(run.Bounds)
		}
	}
	if bounds.Empty() {
		return nil, false
	}
	m.focusID = 0
	m.pressedLink = 0
	m.clearSelection()
	m.focusSourceID = id
	m.sourceFocusBounds = bounds
	if m.mode == LayoutTTY {
		if r, ok := m.ttyResult.AnchorBounds(id); ok {
			m.ttyTop = r.Min.Y
			m.scrollTTY(0)
		}
		return m.refreshHighlights(), true
	}
	return m.scrollTo(bounds.Min), true
}

func (m *Model) clearSourceFocus() {
	m.focusSourceID = ""
	m.sourceFocusBounds = image.Rectangle{}
}

// Both focus owners feed the same raster overlay; link regions retain their
// individual line rectangles, while source elements use their full bounds.
func (m *Model) focusRegions() []image.Rectangle {
	if m.focusSourceID != "" {
		return []image.Rectangle{m.sourceFocusBounds}
	}
	if link, ok := m.FocusedLink(); ok {
		return link.Regions
	}
	return nil
}

func (m *Model) sourceFocusMatches() []document.Match {
	if m.focusSourceID == "" || m.ttyResult == nil {
		return nil
	}
	var spans []document.Match
	for _, run := range m.ttyResult.TextIndex().Runs {
		if run.SourceID == m.focusSourceID {
			spans = append(spans, document.Match{Start: run.Start, End: run.End})
		}
	}
	return spans
}
