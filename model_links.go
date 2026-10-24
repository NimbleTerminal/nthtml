package nthtml

import (
	"image"
	"image/color"
	"image/draw"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml/document"
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
	links := m.links()
	if len(links) == 0 {
		return nil
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
		return nil
	}
	return m.showLinkFocus()
}
func (m *Model) showLinkFocus() tea.Cmd {
	if m.source == nil {
		return nil
	}
	link, ok := m.FocusedLink()
	if !ok {
		return m.viewport()
	}
	b := m.source.Bounds()
	img := image.NewRGBA(b)
	draw.Draw(img, b, m.source, b.Min, draw.Src)
	ink := image.NewUniform(color.RGBA{235, 155, 35, 255})
	for _, r := range link.Regions {
		r = r.Inset(-1)
		for _, edge := range []image.Rectangle{image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)} {
			draw.Draw(img, edge, ink, image.Point{}, draw.Over)
		}
	}
	return m.pic.SetImageCropped(img, m.cropRect())
}

// ActivateLink emits a LinkMsg for the host, or does nothing without a selection.
func (m *Model) ActivateLink() tea.Cmd {
	l, ok := m.FocusedLink()
	if !ok {
		return nil
	}
	return func() tea.Msg { return LinkMsg{Owner: m, Link: l} }
}
