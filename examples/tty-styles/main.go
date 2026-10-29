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
	left, right *nthtml.Model
	cols, rows  int
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
	return m, tea.Batch(m.left.Update(msg), m.right.Update(msg))
}
func (m comparison) View() tea.View {
	width := max(1, (m.cols-3)/2)
	left, right := strings.Split(m.left.View(), "\n"), strings.Split(m.right.View(), "\n")
	var out strings.Builder
	out.WriteString("Typeset" + strings.Repeat(" ", max(0, width-len("Typeset"))) + " │ TTY (same HTML/CSS)\n")
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
	out.WriteString("TTY: / search · n next · v select match · Esc clear · arrows scroll · q quit")
	view := tea.NewView(out.String())
	view.AltScreen = true
	return view
}
func main() {
	result, err := document.Render(context.Background(), strings.NewReader(sample), document.Options{Width: 550})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	app := comparison{left: nthtml.New(result), right: nthtml.New(result), cols: 100, rows: 28}
	defer app.left.Close()
	defer app.right.Close()
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
