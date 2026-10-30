package nthtml

import (
	"context"
	"fmt"
	"image"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

func bandCoordinateModel(t *testing.T, result *document.Result, origin image.Point, cols, rows, cellW, cellH int) *Model {
	t.Helper()
	m := NewWithOptions(result, ModelOptions{Picture: ntpicture.Options{StableID: true, RetireDelay: -1, Render: ntpicture.Config{KittyFormat: ntpicture.KittyFormatZlib}}})
	t.Cleanup(func() { m.Close() })
	m.initial = tea.Batch(m.initial, m.Picture().SetTerminal(true, cellW, cellH), m.SetBounds(origin.X, origin.Y, cols, rows))
	return m
}

func TestKittyBandDocumentCoordinates(t *testing.T) {
	m := bandCoordinateModel(t, &document.Result{Image: image.NewRGBA(image.Rect(0, 0, 800, 20000))}, image.Pt(7, 4), 80, 22, 10, 20)
	// These are document offsets, independent of transport crop or cell mapping.
	// Cross the first 283-row band, then several more, in both directions.
	for _, top := range []int{0, 5000, 5200, 5220, 5660, 10800, 16000, 19560, 16000, 10800, 5660, 5200, 5000, 20, 0} {
		m.Pan(0, top-m.y)
		var center image.Rectangle
		for row := 0; row < 22; row++ {
			cell := m.origin.Add(image.Pt(40, row))
			got, ok := m.CellToDocument(cell)
			want := image.Rect(400, top+row*20, 410, top+(row+1)*20)
			if !ok || got != want {
				t.Fatalf("top=%d row=%d band=%v: got %v (%v), want %v", top, row, m.bandCrop, got, ok, want)
			}
			center = center.Union(got)
		}
		if got, ok := m.DocumentViewport(); !ok || got != image.Rect(0, top, 800, top+440) {
			t.Fatalf("top=%d viewport=%v (%v)", top, got, ok)
		}
		if center.Dy() != 440 {
			t.Fatalf("visible extent %v, want 440 pixels", center)
		}
	}
	if m.bandStartRow != 0 {
		t.Fatal("did not return to first band")
	}
}

func TestKittyBandParagraphReproduction(t *testing.T) {
	r, err := document.Render(context.Background(), strings.NewReader(strings.Repeat("<p>Paragraph for band coordinate regression.</p>", 400)), document.Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	m := bandCoordinateModel(t, r, image.Point{}, 80, 22, 10, 20)
	if m.bandRows != 283 {
		t.Fatalf("expected 283-row band, got %d", m.bandRows)
	}
	var visible image.Rectangle
	for row := 0; row < 22; row++ {
		area, ok := m.CellToDocument(image.Pt(40, row))
		if ok {
			visible = visible.Union(area)
		}
	}
	// Rounding the page height to complete terminal rows can add one pixel.
	if visible.Min.Y > 1 || visible.Dy() < 439 || visible.Dy() > 441 || visible.Dx() > 11 {
		t.Fatalf("incorrect visible center column: %v", visible)
	}
}

func TestKittyBandScaledCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		width, zoom, panX, top int
		want                   image.Rectangle
	}{
		{"downscaled", 1600, 0, 0, 12000, image.Rect(800, 12000, 820, 12040)},
		{"zoomed", 800, 1, 100, 6000, image.Rect(300, 6000, 305, 6010)},
		{"zoomed twice", 800, 2, 200, 3000, image.Rect(300, 3000, 303, 3005)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := bandCoordinateModel(t, &document.Result{Image: image.NewRGBA(image.Rect(0, 0, tc.width, 24000))}, image.Pt(11, 9), 80, 22, 10, 20)
			m.Zoom(tc.zoom * zoomStepsPerDoubling)
			m.Pan(tc.panX, tc.top)
			got, ok := m.CellToDocument(m.origin.Add(image.Pt(40, 0)))
			if !ok || got != tc.want {
				t.Fatalf("got %v (%v), want %v", got, ok, tc.want)
			}
			if _, ok := m.CellToDocument(m.origin.Sub(image.Pt(1, 0))); ok {
				t.Fatal("mapped outside widget")
			}
		})
	}
}

// Kitty presentation commands can include both batches and sequences. Drain
// them fully before the next mouse event, while retaining activation messages.
func bandMouseCommands(t *testing.T, m *Model, cmd tea.Cmd) []LinkMsg {
	t.Helper()
	var links []LinkMsg
	steps := 0
	var run func(tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		steps++
		if steps > 1000 {
			t.Fatal("command loop did not settle")
		}
		msg := cmd()
		if msg == nil {
			return
		}
		if link, ok := msg.(LinkMsg); ok {
			links = append(links, link)
			return
		}
		v := reflect.ValueOf(msg)
		if v.Kind() == reflect.Slice {
			for i := 0; i < v.Len(); i++ {
				run(v.Index(i).Interface().(tea.Cmd))
			}
			return
		}
		run(m.Update(msg))
	}
	run(cmd)
	return links
}

func TestKittyBandLinksSelectionAndReplacement(t *testing.T) {
	var page strings.Builder
	page.WriteString(`<style>body {margin:0} p {margin:0;height:40px;line-height:20px}</style>`)
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&page, `<p><a href="/row/%d">row%03d selectable text</a></p>`, i, i)
	}
	render := func() *document.Result {
		r, err := document.Render(context.Background(), strings.NewReader(page.String()), document.Options{Width: 800})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	m := bandCoordinateModel(t, render(), image.Pt(3, 2), 80, 22, 10, 20)
	driveRaw(t, m, m.initial)
	for _, top := range []int{6000, 12000, 6000} {
		driveRaw(t, m, m.Pan(0, top-m.y))
		p := m.origin.Add(image.Pt(1, 0))
		want := fmt.Sprintf("/row/%d", top/40)
		if link, ok := m.LinkAtCell(p); !ok || link.Href != want {
			t.Fatalf("top=%d hit %q (%v), want %q", top, link.Href, ok, want)
		}
		if links := bandMouseCommands(t, m, m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})); len(links) > 0 {
			t.Fatal("activated before release")
		}
		if links := bandMouseCommands(t, m, m.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})); len(links) != 1 || links[0].Link.Href != want {
			t.Fatalf("click: %v ready=%v status=%+v pressed=%d selected=%q", links, m.mouseReady(), m.pic.Status(), m.pressedLink, m.SelectedText())
		}
		// Clicking focuses a link but must not change the mapped viewport.
		driveRaw(t, m, m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft}))
		end := p.Add(image.Pt(12, 0))
		driveRaw(t, m, m.Update(tea.MouseMotionMsg{X: end.X, Y: end.Y, Button: tea.MouseLeft}))
		if links := bandMouseCommands(t, m, m.Update(tea.MouseReleaseMsg{X: end.X, Y: end.Y, Button: tea.MouseLeft})); len(links) != 0 {
			t.Fatal("drag activated link")
		}
		if text := m.SelectedText(); !strings.Contains(text, fmt.Sprintf("%03d", top/40)) {
			t.Fatalf("wrong drag selection at %d: %q", top, text)
		}
		driveRaw(t, m, m.SetResultKeepView(render()))
		if m.SelectedText() != "" {
			t.Fatal("replacement retained stale selection offsets")
		}
		if link, ok := m.LinkAtCell(p); !ok || link.Href != want {
			t.Fatalf("replacement hit: %v %v", link, ok)
		}
		driveRaw(t, m, m.Pan(0, 40))
		if link, ok := m.LinkAtCell(p); !ok || link.Href != fmt.Sprintf("/row/%d", top/40+1) {
			t.Fatalf("post-replacement scroll hit: %v %v", link, ok)
		}
	}
}

func TestDocumentViewportLetterboxingAndGeometry(t *testing.T) {
	for _, tc := range []struct {
		name               string
		bounds             image.Rectangle
		zoom, panX, panY   int
		cell               image.Point
		wantCell, wantView image.Rectangle
		outside            image.Point
	}{
		{"horizontal letterbox", image.Rect(0, 0, 200, 12000), 0, 0, 6000, image.Pt(30, 0), image.Rect(0, 6000, 10, 6020), image.Rect(0, 6000, 200, 6440), image.Pt(29, 0)},
		{"zoomed letterbox", image.Rect(0, 0, 200, 12000), 1, 40, 6000, image.Pt(30, 0), image.Rect(40, 6000, 45, 6010), image.Rect(40, 6000, 140, 6220), image.Pt(50, 0)},
		{"short page", image.Rect(0, 0, 800, 100), 0, 0, 0, image.Pt(40, 4), image.Rect(400, 80, 410, 100), image.Rect(0, 0, 800, 100), image.Pt(40, 5)},
		{"partial row", image.Rect(0, 0, 800, 101), 0, 0, 0, image.Pt(40, 0), image.Rect(400, 0, 410, 11), image.Rect(0, 0, 800, 101), image.Pt(40, 6)},
		{"nonzero source", image.Rect(13, 27, 813, 20027), 0, 0, 6000, image.Pt(40, 0), image.Rect(413, 6027, 423, 6047), image.Rect(13, 6027, 813, 6467), image.Pt(80, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := bandCoordinateModel(t, &document.Result{Image: image.NewRGBA(tc.bounds)}, image.Pt(9, 6), 80, 22, 10, 20)
			m.Zoom(tc.zoom * zoomStepsPerDoubling)
			m.Pan(tc.panX, tc.panY)
			beforeX, beforeY := m.x, m.y
			got, ok := m.DocumentViewport()
			if !ok || got != tc.wantView {
				t.Fatalf("viewport %v (%v), want %v", got, ok, tc.wantView)
			}
			if m.x != beforeX || m.y != beforeY {
				t.Fatal("viewport accessor changed scroll")
			}
			if got, ok := m.CellToDocument(m.origin.Add(tc.cell)); !ok || got != tc.wantCell {
				t.Fatalf("cell %v (%v), want %v", got, ok, tc.wantCell)
			}
			if got, ok := m.CellToDocument(m.origin.Add(tc.outside)); ok {
				t.Fatalf("letterbox/outside mapped to %v", got)
			}
		})
	}
}

func TestDocumentViewportResolutionAndFractionalScroll(t *testing.T) {
	m := bandCoordinateModel(t, &document.Result{Image: image.NewRGBA(image.Rect(0, 0, 800, 20000))}, image.Point{}, 80, 22, 10, 20)
	for _, factor := range []float64{0.5, 1, 1.5, 2} {
		m.Picture().SetResolution(factor)
		m.Pan(0, 6003-m.y)
		// Transport scrolling snaps to cells. Report the displayed origin, not the
		// requested pixel offset (6003), and do not mutate that requested offset.
		want := image.Rect(0, 6000, 800, 6440)
		if got, ok := m.DocumentViewport(); !ok || got != want {
			t.Fatalf("factor %g: %v (%v), want %v", factor, got, ok, want)
		}
		if m.y != 6003 {
			t.Fatalf("accessor changed requested y to %d", m.y)
		}
	}
	// A raster pixel cap may shorten bands but must not change document mapping.
	m.Picture().SetMaxRasterPixels(800 * 20 * 100)
	m.Pan(0, 0)
	if m.bandRows != 100 {
		t.Fatalf("capped band rows %d", m.bandRows)
	}
	if got, ok := m.DocumentViewport(); !ok || got != image.Rect(0, 6000, 800, 6440) {
		t.Fatalf("capped viewport %v (%v)", got, ok)
	}
}

func TestDocumentViewportGlyphAndUnavailable(t *testing.T) {
	var nilModel *Model
	if _, ok := nilModel.DocumentViewport(); ok {
		t.Fatal("nil model has viewport")
	}
	empty := New(nil)
	defer empty.Close()
	if _, ok := empty.DocumentViewport(); ok {
		t.Fatal("empty model has viewport")
	}
	m := New(&document.Result{Image: image.NewRGBA(image.Rect(0, 0, 800, 100))})
	defer m.Close()
	m.SetBounds(4, 3, 80, 24)
	if got, ok := m.DocumentViewport(); !ok || got != image.Rect(0, 0, 800, 100) {
		t.Fatalf("glyph viewport %v (%v)", got, ok)
	}
	if _, ok := m.CellToDocument(m.origin); ok {
		t.Fatal("glyph letterbox mapped")
	}
	tty := navModel(t)
	defer tty.Close()
	tty.SetLayoutMode(LayoutTTY)
	if _, ok := tty.DocumentViewport(); ok {
		t.Fatal("TTY has document-pixel viewport")
	}
}
