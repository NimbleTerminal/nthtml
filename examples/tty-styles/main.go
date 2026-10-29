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
)

//go:embed sample.html
var sample string

type comparison struct {
	left, right  *nthtml.Model
	cols, rows   int
	cellW, cellH int
}

func newComparison(result *document.Result) comparison {
	return comparison{
		left:  nthtml.NewWithOptions(result, nthtml.ModelOptions{ReflowTypeset: true}),
		right: nthtml.New(result), cols: 100, rows: 28,
	}
}

func (m comparison) Init() tea.Cmd {
	return tea.Batch(m.left.Init(), m.right.Init(), m.right.SetLayoutMode(nthtml.LayoutTTY))
}
func (m comparison) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.cols, m.rows = v.Width, max(1, v.Height-2)
		left := max(1, (v.Width-3)/2)
		return m, tea.Batch(m.left.SetBounds(0, 1, left, m.rows), m.right.SetBounds(left+3, 1, max(1, v.Width-left-3), m.rows), m.left.Update(msg), m.right.Update(msg))
	case tea.KeyPressMsg:
		if v.String() == "ctrl+c" || v.String() == "q" && !m.right.SearchInputActive() {
			return m, tea.Sequence(tea.Batch(m.left.Close(), m.right.Close()), tea.Quit)
		}
		// Keep the panes in their named modes. Navigation/search affects TTY only.
		if v.String() == "t" && !m.right.SearchInputActive() {
			return m, nil
		}
		return m, m.right.Update(msg)
	}
	// Terminal discovery can change pixel geometry after WindowSizeMsg.
	leftCmd, rightCmd := m.left.Update(msg), m.right.Update(msg)
	afterW, afterH := m.left.Picture().CellPixelSize()
	var reflow tea.Cmd
	if m.cellW != afterW || m.cellH != afterH {
		m.cellW, m.cellH = afterW, afterH
		bounds := m.left.Bounds()
		reflow = m.left.SetBounds(bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy())
	}
	return m, tea.Batch(leftCmd, rightCmd, reflow)
}
func (m comparison) View() tea.View {
	width := max(1, (m.cols-3)/2)
	left, right := strings.Split(m.left.View(), "\n"), strings.Split(m.right.View(), "\n")
	var out strings.Builder
	out.WriteString(ansi.Truncate("Typeset"+strings.Repeat(" ", max(0, width-len("Typeset")))+" │ TTY (same HTML/CSS)", max(1, m.cols), "") + "\n")
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
	if m.right.SearchInputActive() {
		return "TTY /" + m.right.SearchInput() + "▏ · Enter search · Esc cancel"
	}
	if query, current, total := m.right.SearchStatus(); query != "" {
		if total == 0 {
			return fmt.Sprintf("TTY search %q · no matches · / edit · Esc clear", query)
		}
		return fmt.Sprintf("TTY search %q · %d/%d · n/N next/previous · v select · Esc clear", query, current, total)
	}
	return "TTY: / search · n next · v select match · Esc clear · arrows scroll · q quit"
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
