// Package nthtml provides a Bubble Tea document viewer backed by ntpicture.
package nthtml

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"image"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
	"strconv"
	"strings"
)

// LayoutMode selects styled pixels or plain terminal-cell text.
// LayoutTypeset uses nthtml's rasterizer, not the separate nttypeset engine.
type LayoutMode int

const (
	LayoutTypeset LayoutMode = iota
	LayoutTTY
)

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
	result                        *document.Result
	mode                          LayoutMode
	ttyLines                      []string
	ttyResult                     *document.TTYResult
	focusID                       int
	origin, pressCell             image.Point
	pressedLink                   int
	ttyTop                        int
	err                           error
	source                        image.Image
	initial                       tea.Cmd
	zoom, x, y, cols, rows        int
}

func New(result *document.Result) *Model {
	m := &Model{pic: ntpicture.New(ntpicture.Options{AutoMode: true}), cols: 80, rows: 24, match: -1}
	if result != nil {
		m.result = result
		m.source = result.Image
		m.initial = m.pic.SetImage(m.source)
	}
	return m
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
	return m.pic.View()
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
		return tea.Batch(m.pic.SetVisible(true), m.showLinkFocus())
	}
	return tea.Batch(m.pic.SetVisible(false), m.refreshHighlights())
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
}
func (m *Model) scrollTTY(delta int) {
	m.ttyTop = max(0, min(max(0, len(m.ttyLines)-m.rows), m.ttyTop+delta))
}
func (m *Model) Close() tea.Cmd { return m.pic.Close() }
func (m *Model) SetSize(cols, rows int) tea.Cmd {
	m.pressedLink = 0
	m.cols = max(1, min(cols, 1000))
	m.rows = max(1, min(rows, 1000))
	m.clearSelection()
	if m.mode == LayoutTTY {
		m.reflowTTY()
	}
	return tea.Batch(m.pic.SetSize(m.cols, m.rows), m.refreshHighlights(), m.viewport())
}
func (m *Model) Zoom(delta int) tea.Cmd {
	m.pressedLink = 0
	m.clearSelection()
	if m.mode == LayoutTTY {
		return nil
	}
	m.zoom = max(0, min(6, m.zoom+delta))
	return m.refreshHighlights()
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
func (m *Model) viewport() tea.Cmd {
	if m.source == nil {
		return nil
	}
	return m.pic.SetCrop(m.cropRect())
}
func (m *Model) cropRect() image.Rectangle {
	if m.source == nil {
		return image.Rectangle{}
	}
	b := m.source.Bounds()
	w := max(1, b.Dx()>>m.zoom)
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
	return tea.Batch(cmd, m.pic.Update(msg))
}
