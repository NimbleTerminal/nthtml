package nthtml

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"image"
	"image/color"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
	"strings"
	"testing"
)

func TestPointerFeedbackPreservesFocusAndActivation(t *testing.T) {
	m := New(mouseRasterResult(1200, image.Rect(240, 100, 320, 122)))
	defer func() { _ = m.Close() }()
	mouseCommands(t, m, m.initial, m.SetBounds(10, 5, 80, 24))
	p := image.Pt(34, 10)
	original := m.source
	if links := mouseCommands(t, m, m.Update(tea.MouseMotionMsg{X: p.X, Y: p.Y, Button: tea.MouseNone})); len(links) != 0 {
		t.Fatal("hover activated link")
	}
	if m.hoverID != 1 || m.focusID != 0 {
		t.Fatal("hover changed keyboard focus", m.hoverID, m.focusID)
	}
	if m.linkFeedbackColor() != (color.RGBA{30, 190, 230, 255}) {
		t.Fatal("hover color")
	}
	// Repeated motion within the link does not resubmit its raster.
	generation := m.pic.Status().Generation
	mouseCommands(t, m, m.Update(tea.MouseMotionMsg{X: p.X, Y: p.Y, Button: tea.MouseNone}))
	if m.pic.Status().Generation != generation {
		t.Fatal("redundant hover repaint")
	}
	mouseCommands(t, m, m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft}))
	if m.linkFeedbackColor() != (color.RGBA{235, 155, 35, 255}) {
		t.Fatal("pressed color")
	}
	links := mouseCommands(t, m, m.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft}))
	if len(links) != 1 || m.pressedLink != 0 {
		t.Fatal("release activation", links)
	}
	mouseCommands(t, m, m.Update(tea.MouseMotionMsg{X: 0, Y: 0, Button: tea.MouseNone}))
	if m.hoverID != 0 || m.focusID != 1 {
		t.Fatal("leaving link lost keyboard focus")
	}
	if m.source != original {
		t.Fatal("feedback replaced source raster")
	}
}

func TestPointerFeedbackClearsOnReplacementAndCancel(t *testing.T) {
	m := New(mouseRasterResult(1200, image.Rect(240, 100, 320, 122)))
	defer func() { _ = m.Close() }()
	mouseCommands(t, m, m.initial, m.SetBounds(10, 5, 80, 24))
	p := image.Pt(34, 10)
	mouseCommands(t, m, m.Update(tea.MouseMotionMsg{X: p.X, Y: p.Y, Button: tea.MouseNone}))
	mouseCommands(t, m, m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft}))
	if m.pressedLink != 1 {
		t.Fatal("press not captured")
	}
	// ntbrowser consumes activation and forwards a right-button release.
	if links := mouseCommands(t, m, m.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseRight})); len(links) != 0 {
		t.Fatal("duplicate activation")
	}
	if m.pressedLink != 0 || m.hoverID != 1 {
		t.Fatal("canceled press feedback")
	}
	mouseCommands(t, m, m.SetResult(mouseRasterResult(1200, image.Rect(240, 100, 320, 122))))
	if m.hoverID != 0 || m.pressedLink != 0 {
		t.Fatal("stale pointer state after replacement")
	}
}

func TestHostKittyPointerFeedbackKeepsBaseBandReady(t *testing.T) {
	r := paintedPage(t)
	m := NewWithOptions(nil, ModelOptions{HostPainted: true, Picture: ntpicture.Options{StableID: true, RetireDelay: -1}})
	defer m.Close()
	driveRaw(t, m, m.Picture().SetTerminal(true, 10, 20), m.SetSize(40, 12))
	cmd, err := m.SetPaintedResult(paintInitial(t, r, PaintViewport{Cols: 40, Rows: 12, CellWidth: 10, CellHeight: 20}, 1), false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	var point image.Point
	found := false
	for y := 0; y < 12 && !found; y++ {
		for x := 0; x < 40; x++ {
			if _, ok := m.LinkAtCell(image.Pt(x, y)); ok {
				point = image.Pt(x, y)
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("no link cell")
	}
	generation := m.pic.Status().Generation
	driveRaw(t, m, m.Update(tea.MouseMotionMsg{X: point.X, Y: point.Y, Button: tea.MouseNone}))
	press := m.Update(tea.MouseClickMsg{X: point.X, Y: point.Y, Button: tea.MouseLeft})
	if !m.InputReady() || m.pic.Status().Generation != generation {
		t.Fatal("feedback invalidated base-band input")
	}
	release := m.Update(tea.MouseReleaseMsg{X: point.X, Y: point.Y, Button: tea.MouseRight})
	driveRaw(t, m, press, release)
	if m.hostPaint.pending != nil || m.pic.Status().Generation != generation {
		t.Fatal("feedback repainted document band")
	}
}

func TestTTYHoverClearsWhenScrolling(t *testing.T) {
	r, err := document.Render(context.Background(), strings.NewReader(`<a href="/link">link</a><p>more</p>`), document.Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	m := New(r)
	defer m.Close()
	m.SetSize(40, 12)
	m.SetLayoutMode(LayoutTTY)
	before := strings.Join(m.ttyLines, "\n")
	p := m.ttyResult.Links[0].Regions[0].Min
	m.Update(tea.MouseMotionMsg{X: p.X, Y: p.Y, Button: tea.MouseNone})
	if strings.Join(m.ttyLines, "\n") == before {
		t.Fatal("TTY hover not visible")
	}
	m.Pan(0, 40)
	if m.hoverID != 0 || strings.Join(m.ttyLines, "\n") != before {
		t.Fatal("TTY hover remained after scrolling")
	}
}
