package nthtml

import (
	"context"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
)

func TestTypesetFitContainAndRestore(t *testing.T) {
	result, err := document.Render(context.Background(), strings.NewReader("<p>searchable page</p><div style='height:1600px'>bottom</div>"), document.Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	for _, kitty := range []bool{false, true} {
		m := New(result)
		m.Picture().SetTerminal(kitty, 10, 20)
		m.SetBounds(4, 2, 40, 20)
		m.Search("searchable")
		m.SelectRange(0, 10)
		m.SetTypesetFit(TypesetFitContain)
		if m.kittyViewportMode() {
			t.Fatal("contain still uses a cropped Kitty band")
		}
		if _, enabled := m.Picture().Viewport(); enabled {
			t.Fatal("contain retained band viewport")
		}
		if viewport, ok := m.DocumentViewport(); !ok || viewport != result.Bounds() {
			t.Fatalf("contain viewport %v, want full page %v", viewport, result.Bounds())
		}
		if query, _, total := m.SearchStatus(); query != "searchable" || total != 1 || m.SelectedText() != "searchable" {
			t.Fatal("fit discarded search/selection")
		}
		m.Pan(0, 100)
		m.Zoom(1)
		if m.x != 0 || m.y != 0 || m.zoom != 0 {
			t.Fatal("overview accumulated invisible pan/zoom")
		}
		m.SetTypesetFit(TypesetFitWidth)
		if viewport, ok := m.DocumentViewport(); !ok || viewport.Dy() >= result.Bounds().Dy() {
			t.Fatalf("width did not restore viewport: %v", viewport)
		}
		if kitty && !m.kittyViewportMode() {
			t.Fatal("width did not restore Kitty bands")
		}
		m.Close()
	}
}

func TestTypesetFitChangedWhileTTY(t *testing.T) {
	result, err := document.Render(context.Background(), strings.NewReader("<p>text</p><div style='height:1600px'>bottom</div>"), document.Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	m := New(result)
	defer m.Close()
	m.Picture().SetTerminal(true, 10, 20)
	m.SetBounds(0, 0, 40, 20)
	m.SetLayoutMode(LayoutTTY)
	before := m.View()
	m.SetTypesetFit(TypesetFitContain)
	if m.View() != before {
		t.Fatal("fit changed TTY text")
	}
	m.SetLayoutMode(LayoutTypeset)
	if viewport, ok := m.DocumentViewport(); !ok || viewport != result.Bounds() {
		t.Fatalf("stale band viewport: %v", viewport)
	}
}
