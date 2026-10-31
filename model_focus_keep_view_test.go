package nthtml

import (
	"context"
	"image"
	"reflect"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
)

func tallFocusResult(t *testing.T) *document.Result {
	t.Helper()
	r, err := document.Render(context.Background(), strings.NewReader(`<p>first line</p><p><input id=q value="query"></p>`+strings.Repeat(`<p>filler text</p>`, 80)), document.Options{Width: 300})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestShowFocusSourceKeepsRestoredView(t *testing.T) {
	for _, kitty := range []bool{false, true} {
		r := tallFocusResult(t)
		m := New(r)
		t.Cleanup(func() { driveRaw(t, m, m.Close()) })
		driveRaw(t, m, m.initial, m.Picture().SetTerminal(kitty, 10, 20), m.SetSize(30, 8), m.Pan(0, 600))
		x, y := m.x, m.y
		driveRaw(t, m, m.SetResult(r))
		driveRaw(t, m, m.Pan(x, y))
		crop, visible := m.DocumentViewport()
		before := m.View()
		cmd, ok := m.ShowFocusSource("q")
		if !ok {
			t.Fatal("source missing")
		}
		driveRaw(t, m, cmd)
		after, afterVisible := m.DocumentViewport()
		if after != crop || visible != afterVisible || m.x != x || m.y != y {
			t.Fatalf("show changed viewport: %v -> %v", crop, after)
		}
		if len(m.focusRegions()) != 1 || !m.sourceFocusBounds.Intersect(crop).Empty() {
			t.Fatal("offscreen source focus missing or unexpectedly visible")
		}
		if kitty && m.View() != before {
			t.Fatal("offscreen ring changed visible Kitty rows")
		}
		bounds := m.sourceFocusBounds
		driveRaw(t, m, m.Pan(0, -y))
		if m.sourceFocusBounds != bounds {
			t.Fatal("scroll moved focus in document coordinates")
		}
		if kitty && m.focusCellRect.Empty() {
			t.Fatal("scrolling back did not show ring")
		}
	}
}

func TestShowFocusSourcePreservesSelectionAndSearch(t *testing.T) {
	for _, mode := range []LayoutMode{LayoutTypeset, LayoutTTY} {
		m := New(tallFocusResult(t))
		t.Cleanup(func() { m.Close() })
		m.SetSize(30, 8)
		m.SetLayoutMode(mode)
		m.Search("filler")
		m.SelectRange(0, 5)
		selected := m.SelectedText()
		matches := append([]document.Match(nil), m.matches...)
		query, match, x, y, top := m.query, m.match, m.x, m.y, m.ttyTop
		for _, id := range []string{"q", ""} {
			if _, ok := m.ShowFocusSource(id); !ok {
				t.Fatal("show rejected", id)
			}
			if m.SelectedText() != selected || m.query != query || m.match != match || !reflect.DeepEqual(m.matches, matches) || m.x != x || m.y != y || m.ttyTop != top {
				t.Fatal("show changed selection, search, or scroll")
			}
		}
		if len(m.focusRegions()) != 0 {
			t.Fatal("clear retained focus")
		}
		m.ShowFocusSource("q")
		before, bounds := m.View(), m.sourceFocusBounds
		if cmd, ok := m.ShowFocusSource("missing"); ok || cmd != nil {
			t.Fatal("unknown source accepted")
		}
		if m.View() != before || m.sourceFocusBounds != bounds || m.SelectedText() != selected {
			t.Fatal("unknown source changed state")
		}
	}
}

func TestFocusSourceKeepsVisibleRegion(t *testing.T) {
	for _, mode := range []LayoutMode{LayoutTypeset, LayoutTTY} {
		m := New(tallFocusResult(t))
		t.Cleanup(func() { m.Close() })
		m.SetSize(30, 8)
		m.SetLayoutMode(mode)
		// A small nonzero offset leaves the input visible, below the viewport top.
		m.Pan(0, 20)
		x, y, top := m.x, m.y, m.ttyTop
		var bounds, viewport image.Rectangle
		if mode == LayoutTTY {
			bounds, _ = m.ttyResult.AnchorBounds("q")
			viewport = image.Rect(0, top, m.cols, top+m.rows)
		} else {
			bounds, _ = m.result.AnchorBounds("q")
			viewport, _ = m.DocumentViewport()
		}
		if !bounds.In(viewport) || bounds.Min.Y == viewport.Min.Y {
			t.Fatalf("invalid visible-source fixture: %v in %v", bounds, viewport)
		}
		if _, ok := m.FocusSource("q"); !ok {
			t.Fatal("source missing")
		}
		if m.x != x || m.y != y || m.ttyTop != top {
			t.Fatal("visible source moved viewport")
		}
		m.Pan(0, 600)
		if _, ok := m.FocusSource("q"); !ok {
			t.Fatal("offscreen source missing")
		}
		if mode == LayoutTTY && m.ttyTop != bounds.Min.Y || mode == LayoutTypeset && m.y != bounds.Min.Y {
			t.Fatal("offscreen source did not scroll to bounds.Min")
		}
	}
}
