package nthtml

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"image"
	"nimbleterminal.dev/nthtml/document"
	"strings"
	"testing"
)

func navModel(t *testing.T) *Model {
	t.Helper()
	r, e := document.Render(context.Background(), strings.NewReader(`<a href="#end">End</a><p>`+strings.Repeat("filler ", 100)+`</p><h2 id="end">Find café here</h2>`), document.Options{Width: 200})
	if e != nil {
		t.Fatal(e)
	}
	return New(r)
}
func TestNavigationSearchCopy(t *testing.T) {
	for _, mode := range []LayoutMode{LayoutTypeset, LayoutTTY} {
		m := navModel(t)
		defer func() { _ = m.Close() }()
		m.SetLayoutMode(mode)
		m.SetSize(30, 5)
		if _, ok := m.NavigateFragment("#end"); !ok {
			t.Fatal("fragment")
		}
		if mode == LayoutTTY && m.ttyTop == 0 || mode == LayoutTypeset && m.y == 0 {
			t.Fatal("did not scroll")
		}
		if _, ok := m.NavigateFragment("#missing"); ok {
			t.Fatal("missing")
		}
		m.Search("CAFÉ")
		_, current, total := m.SearchStatus()
		if current != 1 || total != 1 {
			t.Fatal(current, total)
		}
		m.Update(tea.KeyPressMsg{Code: 'v'})
		if m.SelectedText() != "café" {
			t.Fatal(m.SelectedText())
		}
		cmd := m.CopySelection()
		if cmd == nil {
			t.Fatal("copy")
		}
		copy, ok := cmd().(CopyMsg)
		if !ok || copy.Owner != m || copy.Text != "café" {
			t.Fatal(copy)
		}
		m.SetSize(25, 5)
		if m.SelectedText() != "" {
			t.Fatal("selection survived reflow")
		}
	}
}
func TestSearchInputAndDrag(t *testing.T) {
	m := navModel(t)
	defer func() { _ = m.Close() }()
	m.SetLayoutMode(LayoutTTY)
	m.SetSize(30, 8)
	m.Update(tea.KeyPressMsg{Code: '/'})
	m.Update(tea.KeyPressMsg{Code: 'q'})
	if !m.SearchInputActive() || m.SearchInput() != "q" {
		t.Fatal("search entry")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.mousePress(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	m.selectMotion(image.Pt(2, 0))
	m.mouseRelease(tea.MouseReleaseMsg{X: 2, Y: 0, Button: tea.MouseLeft})
	if m.SelectedText() != "End" {
		t.Fatal("drag selection", m.SelectedText())
	}
}
