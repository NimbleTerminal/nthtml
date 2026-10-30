package nthtml

import (
	"context"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml/document"
)

// Run component commands without a terminal or URL opener. Glyph preparation
// and its presentation messages are processed as they would be by Bubble Tea.
func mouseCommands(t *testing.T, m *Model, cmds ...tea.Cmd) []LinkMsg {
	t.Helper()
	queue := append([]tea.Cmd(nil), cmds...)
	var links []LinkMsg
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 1000 {
			t.Fatal("command loop did not settle")
		}
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		switch v := msg.(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, v...)
		case LinkMsg:
			links = append(links, v)
		default:
			queue = append(queue, m.Update(msg))
		}
	}
	return links
}
func clickCell(t *testing.T, m *Model, p image.Point, button tea.MouseButton) []LinkMsg {
	t.Helper()
	links := mouseCommands(t, m, m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: button}))
	if len(links) > 0 {
		t.Fatal("link activated before release")
	}
	return mouseCommands(t, m, m.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: button}))
}
func TestMouseTTYLinksWithOriginAndScroll(t *testing.T) {
	src := strings.Repeat("<p>before</p>", 8) + `<p><a href="https://example.com">click this wrapped link</a></p>` + strings.Repeat("<p>after</p>", 8)
	r, e := document.Render(context.Background(), strings.NewReader(src), document.Options{Width: 400})
	if e != nil {
		t.Fatal(e)
	}
	m := New(r)
	defer func() { _ = m.Close() }()
	m.SetBounds(7, 4, 20, 4)
	m.SetLayoutMode(LayoutTTY)
	region := m.ttyResult.Links[0].Regions[0]
	m.ttyTop = region.Min.Y - 1
	p := image.Pt(7+region.Min.X, 4+region.Min.Y-m.ttyTop)
	if l, ok := m.LinkAtCell(p); !ok || l.Href != "https://example.com" {
		t.Fatal("TTY hit missed", l)
	}
	links := clickCell(t, m, p, tea.MouseLeft)
	if len(links) != 1 || links[0].Owner != m || links[0].Link.Href != "https://example.com" {
		t.Fatal(links)
	}
	if len(clickCell(t, m, p, tea.MouseRight)) != 0 {
		t.Fatal("right click activated")
	}
	if _, ok := m.LinkAtCell(image.Pt(6, 4)); ok {
		t.Fatal("hit outside origin")
	}
	if len(clickCell(t, m, image.Pt(7, 8), tea.MouseLeft)) != 0 {
		t.Fatal("footer click activated")
	}
	m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: p.X + 1, Y: p.Y, Button: tea.MouseLeft})
	if len(mouseCommands(t, m, m.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft}))) != 0 {
		t.Fatal("drag activated")
	}
	m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.Pan(0, 40)
	if len(mouseCommands(t, m, m.Update(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft}))) != 0 {
		t.Fatal("scroll during press activated")
	}
}
func mouseRasterResult(height int, region image.Rectangle) *document.Result {
	return &document.Result{Image: image.NewRGBA(image.Rect(0, 0, 800, height)), Links: []document.Link{{ID: 1, Href: "https://example.com", Text: "link", Regions: []image.Rectangle{region}}}}
}
func TestMouseRasterLinksAndZoom(t *testing.T) {
	m := New(mouseRasterResult(1200, image.Rect(240, 100, 320, 122)))
	defer func() { _ = m.Close() }()
	size := m.SetBounds(10, 5, 80, 24)
	// No click may target an image that has not reached the displayed generation.
	p := image.Pt(34, 10)
	if m.mouseReady() {
		t.Fatal("unpresented image accepted clicks")
	}
	m.Update(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	if m.pressedLink != 0 {
		t.Fatal("captured click during pending render")
	}
	mouseCommands(t, m, m.initial, size)
	if !m.mouseReady() {
		t.Fatalf("glyph frame not ready: %+v", m.pic.Status())
	}
	if got := clickCell(t, m, p, tea.MouseLeft); len(got) != 1 {
		t.Fatal("raster click failed", got)
	}
	if m.x != 0 || m.y != 0 {
		t.Fatal("click unexpectedly scrolled viewport")
	}
	mouseCommands(t, m, m.Zoom(zoomStepsPerDoubling), m.Pan(200, 80))
	if got := clickCell(t, m, image.Pt(18, 7), tea.MouseLeft); len(got) != 1 {
		t.Fatal("zoomed/panned click failed", got)
	}
}
func TestMouseRasterLetterboxAndKittyGeometry(t *testing.T) {
	m := New(mouseRasterResult(100, image.Rect(240, 22, 320, 24)))
	defer func() { _ = m.Close() }()
	m.SetSize(80, 24)
	if _, ok := m.LinkAtCell(image.Pt(24, 0)); ok {
		t.Fatal("top letterbox activated")
	}
	if _, ok := m.LinkAtCell(image.Pt(24, 23)); ok {
		t.Fatal("bottom letterbox activated")
	}
	if _, ok := m.LinkAtCell(image.Pt(24, 10)); !ok {
		t.Fatal("glyph fitted position missed")
	}
	if _, ok := m.LinkAtCell(image.Pt(24, 11)); ok {
		t.Fatal("glyph mapping ignored vertical fit")
	}
	// Tall physical cells move the fitted image within the terminal grid.
	m.pic.SetTerminal(true, 10, 40)
	if _, ok := m.LinkAtCell(image.Pt(24, 10)); ok {
		t.Fatal("Kitty used glyph pixel geometry")
	}
	if _, ok := m.LinkAtCell(image.Pt(24, 11)); !ok {
		t.Fatal("Kitty fitted position missed")
	}
}

func TestCellToDocumentAgreesWithLinkAtCell(t *testing.T) {
	const page = `<!doctype html><html><body style="font-size:16px">
<p><a id="first" href="/one">first link on the first line</a></p>
<p style="text-align:right"><a id="second" href="/two">right-aligned</a></p>
<div style="height:600px"></div>
<p><a id="third" href="/three">a link far below the fold</a> plain text after it</p>
<div style="height:600px"></div>
</body></html>`
	res, err := document.Render(context.Background(), strings.NewReader(page), document.Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	m := New(res)
	defer func() { _ = m.Close() }()
	_ = m.Init()
	_ = m.SetBounds(3, 2, 80, 24)

	overlaps := func(area image.Rectangle, l document.Link) bool {
		for _, r := range l.Regions {
			if !r.Intersect(area).Empty() {
				return true
			}
		}
		return false
	}
	check := func(label string) (hits int) {
		for y := 0; y < 30; y++ {
			for x := 0; x < 90; x++ {
				p := image.Pt(x, y)
				link, hasLink := m.LinkAtCell(p)
				area, ok := m.CellToDocument(p)
				if hasLink {
					hits++
					if !ok || !overlaps(area, link) {
						t.Fatalf("%s: cell %v: LinkAtCell sees %q, CellToDocument %v (ok=%v) misses it", label, p, link.Href, area, ok)
					}
					continue
				}
				if !ok {
					continue
				}
				for _, l := range res.Links {
					if overlaps(area, l) {
						t.Fatalf("%s: cell %v: CellToDocument %v overlaps %q but LinkAtCell sees no link", label, p, area, l.Href)
					}
				}
			}
		}
		return hits
	}
	if n := check("top"); n == 0 {
		t.Fatal("no link cells at top of page")
	}
	_ = m.Pan(0, 700)
	if n := check("scrolled"); n == 0 {
		t.Fatal("no link cells after scrolling")
	}
	if _, ok := m.CellToDocument(image.Pt(0, 0)); ok {
		t.Fatal("cell outside widget origin mapped to document area")
	}
	m.SetLayoutMode(LayoutTTY)
	if _, ok := m.CellToDocument(image.Pt(5, 5)); ok {
		t.Fatal("CellToDocument should return false in LayoutTTY mode")
	}
}
