package nthtml

import (
	"context"
	"image"
	"reflect"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

func focusTestResult(t *testing.T) *document.Result {
	t.Helper()
	r, err := document.Render(context.Background(), strings.NewReader(`<p><input id=q value="query"></p><p><a id=link href="https://example.com">Inline link text</a></p>`), document.Options{Width: 300})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFocusSourceView(t *testing.T) {
	for _, name := range []string{"fallback", "kitty", "kitty-contain"} {
		kitty := name != "fallback"
		t.Run(name, func(t *testing.T) {
			r := focusTestResult(t)
			m := NewWithOptions(r, ModelOptions{Picture: ntpicture.Options{StableID: true, RetireDelay: -1}})
			t.Cleanup(func() { driveRaw(t, m, m.Close()) })
			driveRaw(t, m, m.initial, m.Picture().SetTerminal(kitty, 10, 20), m.SetSize(30, 15))
			if name == "kitty-contain" {
				driveRaw(t, m, m.SetTypesetFit(TypesetFitContain))
			}
			before := m.View()
			cmd, ok := m.FocusSource("q")
			if !ok {
				t.Fatal("input not found")
			}
			paint := driveRaw(t, m, cmd)
			if name == "kitty-contain" && (paint == "" || !m.overlayPainted) {
				t.Fatal("contain focus did not repaint picture")
			}
			if name != "kitty-contain" && m.View() == before {
				t.Fatal("focus did not change view")
			}
			for _, b := range r.Boxes {
				if b.Tag == "input" && b.SourceID == "q" && !b.Bounds.In(m.sourceFocusBounds) {
					t.Fatalf("focus %v does not contain %v", m.sourceFocusBounds, b.Bounds)
				}
			}
			if name == "kitty" && m.focusCellRect.Empty() {
				t.Fatal("missing Kitty focus overlay")
			}
			focused, bounds, x, y := m.View(), m.sourceFocusBounds, m.x, m.y
			if cmd, ok := m.FocusSource("missing"); ok || cmd != nil {
				t.Fatal("missing source accepted")
			}
			if m.View() != focused || m.sourceFocusBounds != bounds || m.x != x || m.y != y {
				t.Fatal("missing source changed focus/view")
			}
			cmd, ok = m.FocusSource("")
			if !ok {
				t.Fatal("clear rejected")
			}
			paint = driveRaw(t, m, cmd)
			if name == "kitty-contain" && (paint == "" || m.overlayPainted) {
				t.Fatal("contain clear did not restore picture")
			}
			if m.View() != before {
				t.Fatal("clear did not restore unfocused view")
			}
		})
	}
}

func TestFocusSourceInlineAndLinkOwnership(t *testing.T) {
	m := New(focusTestResult(t))
	defer m.Close()
	if _, ok := m.FocusSource("link"); !ok {
		t.Fatal("link source not found")
	}
	var textBounds image.Rectangle
	for _, run := range m.result.TextIndex().Runs {
		if run.SourceID == "link" {
			textBounds = textBounds.Union(run.Bounds)
		}
	}
	if textBounds.Empty() || textBounds.Dx() <= 1 || !textBounds.In(m.sourceFocusBounds) {
		t.Fatalf("focus %v does not cover inline runs %v", m.sourceFocusBounds, textBounds)
	}
	if _, ok := m.FocusedLink(); ok || m.ActivateLink() != nil {
		t.Fatal("source focus activated link navigation")
	}
	m.NextLink(false)
	link, ok := m.FocusedLink()
	if !ok || m.focusSourceID != "" || !reflect.DeepEqual(m.focusRegions(), link.Regions) {
		t.Fatal("NextLink did not replace source focus")
	}
	m.FocusSource("q")
	if _, ok := m.FocusedLink(); ok || m.focusSourceID != "q" {
		t.Fatal("source focus did not replace link focus")
	}
	for _, keep := range []bool{false, true} {
		m.FocusSource("q")
		if keep {
			m.SetResultKeepView(m.result)
		} else {
			m.SetResult(m.result)
		}
		if m.focusSourceID != "" || len(m.focusRegions()) != 0 {
			t.Fatal("result replacement retained source focus")
		}
	}
}

func TestFocusSourceTTY(t *testing.T) {
	m := New(focusTestResult(t))
	defer m.Close()
	m.SetSize(40, 10)
	m.SetLayoutMode(LayoutTTY)
	before := m.View()
	if _, ok := m.FocusSource("q"); !ok {
		t.Fatal("TTY source not found")
	}
	if m.View() == before || !strings.Contains(m.View(), "\x1b[7m") {
		t.Fatalf("TTY source not highlighted: %q", m.View())
	}
	m.SetSize(25, 10)
	if !strings.Contains(m.View(), "\x1b[7m") {
		t.Fatal("TTY reflow lost source highlight")
	}
	m.NextLink(false)
	if m.focusSourceID != "" {
		t.Fatal("TTY NextLink retained source focus")
	}
	m.FocusSource("q")
	m.FocusSource("")
	if strings.Contains(m.View(), "\x1b[7m") {
		t.Fatal("TTY clear retained highlight")
	}
}

func TestFocusSourceInkBoundsAndMissingResult(t *testing.T) {
	r := &document.Result{Image: image.NewRGBA(image.Rect(0, 0, 100, 100)), Boxes: []document.Box{{Anchor: "ink", InkBounds: image.Rect(10, 10, 30, 30)}}}
	m := New(r)
	defer m.Close()
	if _, ok := m.FocusSource("ink"); !ok || m.sourceFocusBounds != r.Boxes[0].InkBounds {
		t.Fatal("ink bounds ignored")
	}
	m.NextLink(false)
	if m.focusSourceID != "" {
		t.Fatal("NextLink without links retained source focus")
	}
	m.SetResult(nil)
	if _, ok := m.FocusSource("ink"); ok {
		t.Fatal("nil result accepted focus")
	}
	if _, ok := m.FocusSource(""); !ok {
		t.Fatal("nil result rejected clear")
	}
}

func TestFocusSourceScrollsIntoView(t *testing.T) {
	r, err := document.Render(context.Background(), strings.NewReader(strings.Repeat("<p>filler text</p>", 40)+`<p><input id=target value="target"></p>`), document.Options{Width: 300})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []LayoutMode{LayoutTypeset, LayoutTTY} {
		m := New(r)
		m.SetSize(30, 5)
		m.SetLayoutMode(mode)
		if _, ok := m.FocusSource("target"); !ok {
			t.Fatal("target missing")
		}
		if mode == LayoutTypeset && m.y == 0 || mode == LayoutTTY && m.ttyTop == 0 {
			t.Fatal("did not scroll to source")
		}
		m.Close()
	}
}
