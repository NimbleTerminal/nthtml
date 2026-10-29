// tty-styles displays the same HTML/CSS as raster and styled TTY side by side.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

//go:embed sample.html
var sample string

type comparison struct {
	left, right  *nthtml.Model
	cols, rows   int
	cellW, cellH int
	typesetFocus bool
	pictureMode  ntpicture.PictureMode
	resolution   float64
}

func newComparison(result *document.Result) comparison {
	return comparison{
		left:  nthtml.NewWithOptions(result, nthtml.ModelOptions{ReflowTypeset: true}),
		right: nthtml.New(result), cols: 100, rows: 28,
	}
}

func (m comparison) Init() tea.Cmd {
	// Establish pane geometry and mode before Init schedules image preparation.
	bounds := m.setPaneBounds()
	mode := m.right.SetLayoutMode(nthtml.LayoutTTY)
	return tea.Batch(bounds, mode, m.left.Init(), m.right.Init())
}
func (m comparison) setPaneBounds() tea.Cmd {
	left := max(1, (m.cols-3)/2)
	return tea.Batch(m.left.SetBounds(0, 1, left, m.rows), m.right.SetBounds(left+3, 1, max(1, m.cols-left-3), m.rows))
}

func (m comparison) focused() (*nthtml.Model, string) {
	if m.typesetFocus {
		return m.left, "Typeset"
	}
	return m.right, "TTY"
}

func (m comparison) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	focused, _ := m.focused()
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.cols, m.rows = v.Width, max(1, v.Height-2)
		return m, tea.Batch(m.setPaneBounds(), m.left.Update(msg), m.right.Update(msg))
	case tea.PasteMsg:
		return m, focused.Update(msg)
	case tea.KeyPressMsg:
		if v.String() == "f6" {
			m.typesetFocus = !m.typesetFocus
			return m, nil
		}
		if v.String() == "ctrl+c" || v.String() == "q" && !focused.SearchInputActive() {
			return m, tea.Sequence(tea.Batch(m.left.Close(), m.right.Close()), tea.Quit)
		}
		// Keep the panes in their named modes; input goes to the focused pane.
		if v.String() == "t" && !focused.SearchInputActive() {
			return m, nil
		}
		return m, focused.Update(msg)
	}
	// Discovery can change the rendering mode without changing cell dimensions.
	// Both mode and resolution affect the document width used by reflow.
	leftCmd, rightCmd := m.left.Update(msg), m.right.Update(msg)
	afterW, afterH := m.left.Picture().CellPixelSize()
	mode, resolution := m.left.Picture().Mode(), m.left.Picture().Resolution()
	var reflow tea.Cmd
	if m.cellW != afterW || m.cellH != afterH || m.pictureMode != mode || m.resolution != resolution {
		m.cellW, m.cellH = afterW, afterH
		m.pictureMode, m.resolution = mode, resolution
		bounds := m.left.Bounds()
		reflow = m.left.SetBounds(bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy())
	}
	return m, tea.Batch(leftCmd, rightCmd, reflow)
}
func (m comparison) View() tea.View {
	width := max(1, (m.cols-3)/2)
	left, right := strings.Split(m.left.View(), "\n"), strings.Split(m.right.View(), "\n")
	var out strings.Builder
	leftTitle, rightTitle := "Typeset", "TTY [active]"
	if m.typesetFocus {
		leftTitle, rightTitle = "Typeset [active]", "TTY"
	}
	out.WriteString(ansi.Truncate(leftTitle+strings.Repeat(" ", max(0, width-len(leftTitle)))+" │ "+rightTitle, max(1, m.cols), "") + "\n")
	for i := 0; i < m.rows; i++ {
		a, b := "", ""
		if i < len(left) {
			a = left[i]
		}
		if i < len(right) {
			b = right[i]
		}
		out.WriteString(a + strings.Repeat(" ", max(0, width-ansi.StringWidth(a))) + " │ " + b + "\n")
	}
	out.WriteString(ansi.Truncate(m.status(), max(1, m.cols), "…"))
	view := tea.NewView(out.String())
	view.AltScreen = true
	return view
}
func (m comparison) status() string {
	focused, name := m.focused()
	if focused.SearchInputActive() {
		return name + " /" + focused.SearchInput() + "▏ · Enter search · Esc cancel · F6 pane"
	}
	if query, current, total := focused.SearchStatus(); query != "" {
		if total == 0 {
			return fmt.Sprintf("%s search %q · no matches · / edit · Esc clear · F6 pane", name, query)
		}
		return fmt.Sprintf("%s search %q · %d/%d · n/N next/previous · v select · F6 pane", name, query, current, total)
	}
	return name + ": F6 switch pane · / search · n next · v select · arrows scroll · q quit"
}

func main() {
	result, err := document.Render(context.Background(), strings.NewReader(sample), document.Options{Width: 550})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	app := newComparison(result)
	defer app.left.Close()
	defer app.right.Close()
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
