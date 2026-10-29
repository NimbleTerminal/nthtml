package main

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/nthtml/document"
)

func TestSharedSample(t *testing.T) {
	r, err := document.Render(context.Background(), strings.NewReader(sample), document.Options{Width: 550})
	if err != nil {
		t.Fatal(err)
	}
	tty, err := r.TTYLayout(context.Background(), 48)
	if err != nil {
		t.Fatal(err)
	}
	if len(tty.Styles) == 0 || len(tty.Spans) == 0 || len(r.Boxes) == 0 {
		t.Fatal("missing styled content")
	}
	if ansi.Strip(tty.ANSI()) != tty.Text {
		t.Fatal("plain text changed")
	}
	if len(tty.TextIndex().Search("Café")) == 0 {
		t.Fatal("missing searchable Unicode")
	}
}

func demoComparison(t *testing.T) comparison {
	t.Helper()
	r, err := document.Render(context.Background(), strings.NewReader(sample), document.Options{Width: 550})
	if err != nil {
		t.Fatal(err)
	}
	m := newComparison(r)
	m.Init()
	t.Cleanup(func() { m.left.Close(); m.right.Close() })
	return m
}

func TestComparisonSearchFeedback(t *testing.T) {
	m := demoComparison(t)
	send := func(key tea.KeyPressMsg) { next, _ := m.Update(key); m = next.(comparison) }
	send(tea.KeyPressMsg{Code: '/'})
	if !m.right.SearchInputActive() || !strings.Contains(ansi.Strip(m.View().Content), "TTY /▏") {
		t.Fatal("search prompt not visible")
	}
	for _, r := range "Café" {
		send(tea.KeyPressMsg{Code: r})
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "TTY /Café▏") {
		t.Fatal("typed query not visible")
	}
	send(tea.KeyPressMsg{Code: tea.KeyEnter})
	query, current, total := m.right.SearchStatus()
	if query != "Café" || current != 1 || total == 0 || !strings.Contains(m.status(), "1/") {
		t.Fatalf("search result not shown: %s", m.status())
	}
	send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(m.status(), "Café") {
		t.Fatal("search status not cleared")
	}
	m.right.Search("not-in-the-demo")
	if !strings.Contains(m.status(), "no matches") {
		t.Fatal(m.status())
	}
}

func TestComparisonTypesetReflowsAndFooterFits(t *testing.T) {
	m := demoComparison(t)
	if !m.left.ReflowTypeset() {
		t.Fatal("typeset reflow disabled")
	}
	m.left.Picture().SetTerminal(true, 10, 20)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 103, Height: 26})
	m = next.(comparison)
	if got := m.left.Result().Bounds().Dx(); got != 500 {
		t.Fatalf("typeset width %d, want pane width 50 * 10 pixels", got)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 83, Height: 26})
	m = next.(comparison)
	if got := m.left.Result().Bounds().Dx(); got != 400 {
		t.Fatalf("resize width %d, want 400", got)
	}
	// A shared terminal session may update geometry before the message reaches
	// this pane. Reflow must compare against the last observed geometry.
	m.left.Picture().SetTerminal(true, 12, 24)
	next, _ = m.Update(struct{}{})
	m = next.(comparison)
	if got := m.left.Result().Bounds().Dx(); got != 480 {
		t.Fatalf("discovered geometry width %d, want 480", got)
	}
	m.right.Update(tea.KeyPressMsg{Code: '/'})
	m.right.Update(tea.PasteMsg{Content: strings.Repeat("long query ", 20)})
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != 26 {
		t.Fatalf("view has %d lines, want 26", len(lines))
	}
	if width := ansi.StringWidth(lines[len(lines)-1]); width > m.cols {
		t.Fatalf("footer wraps: %d > %d", width, m.cols)
	}
}

func TestComparisonTypesetSearchAndPaneIsolation(t *testing.T) {
	m := demoComparison(t)
	send := func(msg tea.Msg) { next, _ := m.Update(msg); m = next.(comparison) }
	send(tea.KeyPressMsg{Code: tea.KeyF6})
	if !strings.Contains(m.status(), "Typeset:") || !strings.Contains(m.View().Content, "Typeset [active]") {
		t.Fatal("typeset focus not visible")
	}
	send(tea.KeyPressMsg{Code: '/'})
	send(tea.PasteMsg{Content: "Café"})
	if m.left.SearchInput() != "Café" || m.right.SearchInput() != "" {
		t.Fatal("paste reached wrong pane")
	}
	send(tea.KeyPressMsg{Code: tea.KeyEnter})
	query, current, total := m.left.SearchStatus()
	if query != "Café" || current != 1 || total == 0 {
		t.Fatalf("typeset search failed: %q %d/%d", query, current, total)
	}
	if query, _, _ := m.right.SearchStatus(); query != "" {
		t.Fatal("typeset search changed TTY search")
	}
	send(tea.KeyPressMsg{Code: 'v'})
	if m.left.SelectedText() != "Café" {
		t.Fatal("typeset match cannot be selected")
	}
	send(tea.KeyPressMsg{Code: tea.KeyF6})
	if !strings.HasPrefix(m.status(), "TTY:") {
		t.Fatal("TTY focus not restored")
	}
	send(tea.KeyPressMsg{Code: '/'})
	send(tea.PasteMsg{Content: "terminal"})
	send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if query, _, _ := m.left.SearchStatus(); query != "Café" {
		t.Fatal("TTY search changed typeset search")
	}
	send(tea.KeyPressMsg{Code: tea.KeyF6})
	if !strings.Contains(m.status(), "Café") {
		t.Fatal("typeset search state lost")
	}
}
