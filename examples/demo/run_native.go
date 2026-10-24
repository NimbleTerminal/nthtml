//go:build !js

package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"
	"os/exec"
	"runtime"
)

type app struct {
	viewer *nthtml.Model
	cols   int
	notice string
}
type openLinkResult struct{ err error }

func (a app) Init() tea.Cmd { return a.viewer.Init() }
func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case nthtml.LinkMsg:
		if v.Owner != a.viewer {
			return a, nil
		}
		href := document.ResolveURL("", v.Link.Href)
		if href == "" {
			a.notice = "Host must resolve: " + v.Link.Href
			return a, nil
		}
		return a, func() tea.Msg {
			var command *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				command = exec.Command("open", href)
			case "linux":
				command = exec.Command("xdg-open", href)
			default:
				return openLinkResult{fmt.Errorf("open %s in your browser", href)}
			}
			return openLinkResult{command.Run()}
		}
	case openLinkResult:
		if v.err != nil {
			a.notice = v.err.Error()
		}

	case tea.KeyPressMsg:
		if v.String() == "q" || v.String() == "ctrl+c" {
			return a, tea.Sequence(a.viewer.Close(), tea.Quit)
		}
	case tea.WindowSizeMsg:
		a.cols = v.Width
		return a, tea.Batch(a.viewer.SetBounds(0, 0, v.Width, max(1, v.Height-2)), a.viewer.Update(msg))
	}
	return a, a.viewer.Update(msg)
}
func (a app) View() tea.View {
	mode := "Typeset"
	if a.viewer.LayoutMode() == nthtml.LayoutTTY {
		mode = "TTY"
	}
	target := a.notice
	if link, ok := a.viewer.FocusedLink(); ok {
		target = "Enter to open: " + link.Href
	}
	v := tea.NewView(a.viewer.View() + "\n" + ansi.Truncate(target, max(1, a.cols), "…") + "\n nthtml · " + mode + " · t layout · Tab links · click/Enter open · arrows scroll · +/- zoom · q quit")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
func run(r *document.Result, _ string) {
	a := app{viewer: nthtml.New(r), cols: 80}
	defer a.viewer.Close()
	if _, err := tea.NewProgram(a).Run(); err != nil {
		panic(err)
	}
}
