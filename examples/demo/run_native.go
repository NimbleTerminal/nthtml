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
	"strings"
	"time"
)

type app struct {
	viewer  *nthtml.Model
	cols    int
	notice  string
	closing bool
}
type openLinkResult struct{ err error }
type shutdownReadyMsg struct{}

// Keep input owned by Bubble Tea while the mouse-disable frame is flushed
// and in-flight terminal reports arrive. This is bounded, not a guarantee
// against arbitrary remote-terminal latency.
const shutdownGrace = 150 * time.Millisecond

func (a app) Init() tea.Cmd { return a.viewer.Init() }
func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.closing {
		switch msg.(type) {
		case shutdownReadyMsg:
			return a, tea.Quit
		case tea.KeyPressMsg, tea.KeyReleaseMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg, tea.WindowSizeMsg, nthtml.LinkMsg, openLinkResult:
			return a, nil
		default:
			// Graphics cleanup messages still need to reach the closed viewer.
			return a, a.viewer.Update(msg)
		}
	}
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
			a.closing = true
			return a, tea.Sequence(a.viewer.Close(), tea.Tick(shutdownGrace, func(time.Time) tea.Msg { return shutdownReadyMsg{} }))
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
	if !a.closing {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
func run(r *document.Result, _ string) {
	a := app{viewer: nthtml.New(r), cols: 80, notice: strings.Join(r.Warnings, "; ")}
	defer a.viewer.Close()
	if _, err := tea.NewProgram(a).Run(); err != nil {
		panic(err)
	}
}
