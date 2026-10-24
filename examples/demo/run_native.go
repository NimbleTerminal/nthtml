//go:build !js

package main

import (
	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"
)

type app struct{ viewer *nthtml.Model }

func (a app) Init() tea.Cmd { return a.viewer.Init() }
func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		if v.String() == "q" || v.String() == "ctrl+c" {
			return a, tea.Sequence(a.viewer.Close(), tea.Quit)
		}
	case tea.WindowSizeMsg:
		return a, tea.Batch(a.viewer.SetSize(v.Width, max(1, v.Height-1)), a.viewer.Update(msg))
	}
	return a, a.viewer.Update(msg)
}
func (a app) View() tea.View {
	mode := "Typeset"
	if a.viewer.LayoutMode() == nthtml.LayoutTTY {
		mode = "TTY"
	}
	v := tea.NewView(a.viewer.View() + "\n nthtml · " + mode + " · t layout · arrows scroll · +/- zoom · 0 reset · q quit")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
func run(r *document.Result, _ string) {
	a := app{nthtml.New(r)}
	defer a.viewer.Close()
	if _, err := tea.NewProgram(a).Run(); err != nil {
		panic(err)
	}
}
