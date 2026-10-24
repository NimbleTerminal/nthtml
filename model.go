// Package nthtml provides a Bubble Tea document viewer backed by ntpicture.
package nthtml

import (
	tea "charm.land/bubbletea/v2"
	"image"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

// Model follows ntpicture's pointer-mutating component convention. Forward all
// messages and run returned commands. The host owns focus and quit handling.
type Model struct {
	pic                    *ntpicture.Picture
	source                 image.Image
	initial                tea.Cmd
	zoom, x, y, cols, rows int
}

func New(result *document.Result) *Model {
	m := &Model{pic: ntpicture.New(ntpicture.Options{AutoMode: true}), cols: 80, rows: 24}
	if result != nil {
		m.source = result.Image
		m.initial = m.pic.SetImage(m.source)
	}
	return m
}
func (m *Model) Init() tea.Cmd  { return tea.Batch(m.pic.Init(), m.initial, m.SetSize(m.cols, m.rows)) }
func (m *Model) View() string   { return m.pic.View() }
func (m *Model) Close() tea.Cmd { return m.pic.Close() }
func (m *Model) SetSize(cols, rows int) tea.Cmd {
	m.cols = max(1, min(cols, 1000))
	m.rows = max(1, min(rows, 1000))
	return tea.Batch(m.pic.SetSize(m.cols, m.rows), m.viewport())
}
func (m *Model) Zoom(delta int) tea.Cmd { m.zoom = max(0, min(6, m.zoom+delta)); return m.viewport() }
func (m *Model) Pan(dx, dy int) tea.Cmd {
	m.x += max(-4096, min(4096, dx))
	m.y += max(-16384, min(16384, dy))
	return m.viewport()
}
func (m *Model) Reset() tea.Cmd { m.zoom, m.x, m.y = 0, 0, 0; return m.viewport() }
func (m *Model) viewport() tea.Cmd {
	if m.source == nil {
		return nil
	}
	b := m.source.Bounds()
	w := max(1, b.Dx()>>m.zoom)
	h := min(b.Dy(), max(1, w*m.rows*2/m.cols))
	m.x = max(0, min(m.x, b.Dx()-w))
	m.y = max(0, min(m.y, b.Dy()-h))
	return m.pic.SetCrop(image.Rect(b.Min.X+m.x, b.Min.Y+m.y, b.Min.X+m.x+w, b.Min.Y+m.y+h))
}

// Update handles arrows/hjkl, page up/down, +/- and 0, plus the mouse wheel.
// Only forward input while focused; always forward non-input messages.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		switch v.String() {
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
			cmd = m.Pan(0, -300)
		case "pgdown":
			cmd = m.Pan(0, 300)
		}
	case tea.MouseWheelMsg:
		switch v.Button {
		case tea.MouseWheelUp:
			cmd = m.Pan(0, -40)
		case tea.MouseWheelDown:
			cmd = m.Pan(0, 40)
		}
	}
	return tea.Batch(cmd, m.pic.Update(msg))
}
