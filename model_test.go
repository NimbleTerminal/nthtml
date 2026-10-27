package nthtml

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"image"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
	"reflect"
	"strings"
	"testing"
)

func TestNavigationClamps(t *testing.T) {
	m := New(&document.Result{Image: image.NewRGBA(image.Rect(0, 0, 800, 2000))})
	defer func() { _ = m.Close() }()
	m.Zoom(100)
	m.Pan(100000, 100000)
	if m.zoom != 6 || m.x < 0 || m.x >= 800 || m.y < 0 || m.y >= 2000 {
		t.Fatalf("invalid viewport: %+v", m)
	}
	m.Reset()
	if m.zoom != 0 || m.x != 0 || m.y != 0 {
		t.Fatal("reset failed")
	}
}

func TestLayoutToggleAndResize(t *testing.T) {
	r, e := document.Render(context.Background(), strings.NewReader("<h1>Readable heading</h1><p>"+strings.Repeat("Hello world. ", 100)+"</p>"), document.Options{Width: 400})
	if e != nil {
		t.Fatal(e)
	}
	m := New(r)
	defer func() { _ = m.Close() }()
	m.SetSize(40, 5)
	m.Zoom(1)
	m.Pan(20, 40)
	zoom, x, y := m.zoom, m.x, m.y
	m.ToggleLayout()
	if m.LayoutMode() != LayoutTTY || !strings.Contains(m.View(), "Readable heading") {
		t.Fatal("did not switch to text")
	}
	before := len(m.ttyLines)
	m.Pan(0, 100)
	top := m.ttyTop
	if top == 0 {
		t.Fatal("TTY did not scroll")
	}
	m.ToggleLayout()
	if m.zoom != zoom || m.x != x || m.y != y {
		t.Fatal("raster viewport lost")
	}
	m.ToggleLayout()
	if m.ttyTop != top {
		t.Fatal("TTY position lost")
	}
	m.SetSize(20, 5)
	if len(m.ttyLines) <= before {
		t.Fatal("TTY failed to reflow")
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(line) > 20 {
			t.Fatal("TTY line overflow", line)
		}
	}
	m.Reset()
	if m.ttyTop != 0 {
		t.Fatal("TTY reset failed")
	}
	if m.Err() != nil {
		t.Fatal(m.Err())
	}
}

func TestKeyboardLinkNavigation(t *testing.T) {
	r, e := document.Render(context.Background(), strings.NewReader(`<p><a href="https://one.example">One</a></p><p><a href="https://two.example">Two</a></p>`), document.Options{Width: 300})
	if e != nil {
		t.Fatal(e)
	}
	m := New(r)
	defer func() { _ = m.Close() }()
	if m.ActivateLink() != nil {
		t.Fatal("activated without selection")
	}
	m.NextLink(false)
	link, ok := m.FocusedLink()
	if !ok || link.Href != "https://one.example" {
		t.Fatal(link)
	}
	msg := m.ActivateLink()().(LinkMsg)
	if msg.Owner != m || msg.Link.ID != link.ID {
		t.Fatal(msg)
	}
	m.NextLink(false)
	link, _ = m.FocusedLink()
	if link.Href != "https://two.example" {
		t.Fatal(link)
	}
	m.NextLink(false)
	link, _ = m.FocusedLink()
	if link.Href != "https://one.example" {
		t.Fatal("did not wrap")
	}
	m.SetLayoutMode(LayoutTTY)
	if !strings.Contains(m.View(), "\x1b]8;") {
		t.Fatal("TTY links missing")
	}
	m.NextLink(true)
	link, _ = m.FocusedLink()
	if link.Href != "https://two.example" {
		t.Fatal("backward navigation failed")
	}
	m.SetSize(12, 4)
	link, _ = m.FocusedLink()
	if link.Href != "https://two.example" {
		t.Fatal("selection lost on reflow")
	}
}

func TestResponsiveTypesetReflowAndMultiWidgetOptions(t *testing.T) {
	ctx := context.Background()
	doc, err := document.Parse(ctx, strings.NewReader("<h1>Responsive</h1><p>"+strings.Repeat("Reflowing paragraph text across widths. ", 20)+"</p>"), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := doc.Render(ctx, document.Options{Width: 800, NoImage: true})
	if err != nil {
		t.Fatal(err)
	}
	allocCalls := 0
	nextID := func() (int, error) {
		allocCalls++
		return 1000 + allocCalls, nil
	}
	m1 := NewWithOptions(r, ModelOptions{
		ReflowTypeset: true,
		NextID:        nextID,
	})
	defer func() { _ = m1.Close() }()
	m2 := NewWithOptions(r, ModelOptions{
		ReflowTypeset: true,
		NextID:        nextID,
	})
	defer func() { _ = m2.Close() }()

	m1.SetBounds(0, 0, 40, 20)
	m2.SetBounds(40, 0, 60, 20)

	if m1.Bounds() != image.Rect(0, 0, 40, 20) || m2.Bounds() != image.Rect(40, 0, 100, 20) {
		t.Fatalf("unexpected widget cell bounds: m1=%v m2=%v", m1.Bounds(), m2.Bounds())
	}
	if m1.Result().Bounds().Dx() == 800 || m1.Result().Bounds().Dx() == m2.Result().Bounds().Dx() {
		t.Fatalf("expected responsive Typeset reflow to update document widths: m1=%v m2=%v", m1.Result().Bounds(), m2.Result().Bounds())
	}
	if m1.Picture() == nil || !m1.Visible() {
		t.Fatal("expected underlying picture to be accessible and visible")
	}
	m1.SetVisible(false)
	if m1.Visible() {
		t.Fatal("expected m1 to be hidden after SetVisible(false)")
	}
}

func TestSetResultKeepViewAndSetResultReset(t *testing.T) {
	ctx := context.Background()
	renderPage := func(body string) *document.Result {
		r, err := document.Render(ctx, strings.NewReader(`<!doctype html><html><body>`+body+`<div style="height:2400px"></div></body></html>`), document.Options{Width: 400})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	first := renderPage(`<p>needle one</p><p><a id="l1" href="https://example.com/1">First</a></p><p>needle two</p>`)
	m := New(first)
	defer func() { _ = m.Close() }()
	m.SetSize(80, 24)
	m.Search("needle")
	m.NextMatch(false) // second match (index 1, current=2)
	m.NextLink(false)
	m.Zoom(1)
	m.Pan(40, 600)
	wantX, wantY := m.x, m.y

	focusedBefore, ok := m.FocusedLink()
	if !ok || focusedBefore.Anchor != "l1" {
		t.Fatalf("expected focused link l1, got %+v (ok=%v)", focusedBefore, ok)
	}
	q, cur, tot := m.SearchStatus()
	if q != "needle" || cur != 2 || tot != 2 {
		t.Fatalf("unexpected search status before reflow: q=%q cur=%d tot=%d", q, cur, tot)
	}

	// Reflow with same structure keeps zoom, scroll offset, search query/match, and focused link.
	second := renderPage(`<p>needle one</p><p><a id="l1" href="https://example.com/1-updated">First updated</a></p><p>needle two</p>`)
	m.SetResultKeepView(second)
	if m.zoom != 1 || m.x != wantX || m.y != wantY {
		t.Fatalf("SetResultKeepView lost viewport: zoom=%d x=%d y=%d (want x=%d y=%d)", m.zoom, m.x, m.y, wantX, wantY)
	}
	if crop := m.Picture().Crop(); crop.Min.X != wantX || crop.Min.Y != wantY {
		t.Fatalf("SetResultKeepView did not apply crop: %v (want min=(%d,%d))", crop, wantX, wantY)
	}
	q, cur, tot = m.SearchStatus()
	if q != "needle" || cur != 2 || tot != 2 {
		t.Fatalf("SetResultKeepView lost search state: q=%q cur=%d tot=%d", q, cur, tot)
	}
	focusedAfter, ok := m.FocusedLink()
	if !ok || focusedAfter.ID != focusedBefore.ID || focusedAfter.Href != "https://example.com/1-updated" {
		t.Fatalf("SetResultKeepView lost focused link: %+v (ok=%v)", focusedAfter, ok)
	}

	// If a reflow removes the focused link, focus clears cleanly while keeping scroll/zoom.
	noLinks := renderPage(`<p>needle only once</p>`)
	m.SetResultKeepView(noLinks)
	if _, ok := m.FocusedLink(); ok {
		t.Fatal("expected focus to clear when focused link ID is absent in new Result")
	}
	q, cur, tot = m.SearchStatus()
	if q != "needle" || cur != 1 || tot != 1 {
		t.Fatalf("expected search match to clamp to remaining match, got q=%q cur=%d tot=%d", q, cur, tot)
	}

	// SetResult (new page) resets scroll, zoom, search, and focus.
	m.SetResult(first)
	if m.zoom != 0 || m.x != 0 || m.y != 0 {
		t.Fatalf("SetResult did not reset viewport: zoom=%d x=%d y=%d", m.zoom, m.x, m.y)
	}
	if q, cur, tot := m.SearchStatus(); q != "" || cur != 0 || tot != 0 {
		t.Fatalf("SetResult did not reset search: q=%q cur=%d tot=%d", q, cur, tot)
	}
	if _, ok := m.FocusedLink(); ok {
		t.Fatal("SetResult did not reset link focus")
	}
}

func driveRaw(t *testing.T, m *Model, cmds ...tea.Cmd) string {
	t.Helper()
	var out strings.Builder
	var run func(tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		msg := cmd()
		if msg == nil {
			return
		}
		v := reflect.ValueOf(msg)
		if v.Kind() == reflect.Slice {
			for i := 0; i < v.Len(); i++ {
				run(v.Index(i).Interface().(tea.Cmd))
			}
			return
		}
		if raw, ok := msg.(tea.RawMsg); ok {
			if s, ok := raw.Msg.(string); ok {
				out.WriteString(s)
			}
		}
		run(m.Update(msg))
	}
	for _, cmd := range cmds {
		run(cmd)
	}
	return out.String()
}

func renderHNTestPage(t *testing.T, extraTitle string) *document.Result {
	t.Helper()
	var rows strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&rows, `<tr class="athing" id="story-%d">
			<td align="right" valign="top" class="title">%d.</td>
			<td class="title"><a href="https://example.com/%d">Show HN: High-density terminal document rendering %s #%d</a> <span class="comhead">(example.com)</span></td>
		</tr>
		<tr>
			<td></td>
			<td class="subtext">%d points by <a href="user?id=user%d">user%d</a> 3 hours ago | <a href="item?id=%d">%d comments</a></td>
		</tr>
		<tr style="height:5px"></tr>`, i, i, i, extraTitle, i, 40+i*7, i, i, 1000+i, 12+i*3)
	}
	html := `<!doctype html><html><head><style>
		body { font-family: Verdana, Geneva, sans-serif; font-size: 10pt; color: #828282; background: #ffffff; margin: 8px; }
		table { border-collapse: collapse; }
		.title { font-size: 10pt; color: #000000; }
		.subtext { font-size: 7pt; color: #828282; }
		.comhead { font-size: 8pt; color: #828282; }
		a:link { color: #000000; text-decoration: none; }
		.subtext a:link { color: #828282; }
	</style></head><body>
		<center>
			<table id="hnmain" border="0" cellpadding="0" cellspacing="0" width="85%" bgcolor="#f6f6ef">
				<tr><td bgcolor="#ff6600" style="padding:2px"><b><a href="news">Hacker News</a></b> new | past | comments | ask | show | jobs | submit</td></tr>
				<tr style="height:10px"></tr>
				<tr><td><table border="0" cellpadding="0" cellspacing="0">` + rows.String() + `</table></td></tr>
			</table>
		</center>
	</body></html>`
	r, err := document.Render(context.Background(), strings.NewReader(html), document.Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestKittyViewportScrollBandCrossingFocusAndKeepView(t *testing.T) {
	first := renderHNTestPage(t, "v1")
	var idCalls int
	m := NewWithOptions(first, ModelOptions{
		NextID: func() (int, error) {
			idCalls++
			return 40 + idCalls, nil
		},
		Picture: ntpicture.Options{
			StableID:    true,
			RetireDelay: -1,
			Render:      ntpicture.Config{KittyFormat: ntpicture.KittyFormatZlib},
		},
	})
	defer func() { driveRaw(t, m, m.Close()) }()

	// 120x40 terminal with 11x21 px cells.
	initOut := driveRaw(t, m, m.Picture().SetTerminal(true, 11, 21), m.SetSize(120, 40))
	if strings.Count(initOut, "a=T") != 1 {
		t.Fatalf("expected 1 initial band upload (a=T), got %d in %d bytes", strings.Count(initOut, "a=T"), len(initOut))
	}
	vp0, ok := m.Picture().Viewport()
	if !ok || vp0 != (ntpicture.Viewport{X: 0, Y: 0, Width: 120, Height: 40}) {
		t.Fatalf("initial Viewport() = %+v (ok=%v), want {0 0 120 40}", vp0, ok)
	}
	viewBefore := m.View()
	if !strings.Contains(viewBefore, "\U0010EEEE") {
		t.Fatalf("expected Kitty Unicode placeholder view, got %q", viewBefore)
	}
	linesBefore := strings.Split(viewBefore, "\n")
	if len(linesBefore) != 40 {
		t.Fatalf("expected 40 view lines, got %d", len(linesBefore))
	}

	// 1. A Down key in Kitty typeset mode emits no Kitty command and changes View() by the scrolled rows only.
	downOut := driveRaw(t, m, m.Update(tea.KeyPressMsg{Code: tea.KeyDown}))
	if downOut != "" {
		t.Fatalf("Down key emitted %d bytes of Kitty output (%q), want 0 bytes", len(downOut), downOut)
	}
	vp1, ok := m.Picture().Viewport()
	if !ok || vp1.Y <= vp0.Y || vp1.Width != 120 || vp1.Height != 40 {
		t.Fatalf("Viewport() after Down = %+v (ok=%v), expected Y > %d", vp1, ok, vp0.Y)
	}
	scrolledRows := vp1.Y - vp0.Y
	viewAfterDown := m.View()
	linesAfterDown := strings.Split(viewAfterDown, "\n")
	if len(linesAfterDown) != 40 {
		t.Fatalf("expected 40 view lines after Down, got %d", len(linesAfterDown))
	}
	for i := 0; i < 40-scrolledRows; i++ {
		if linesAfterDown[i] != linesBefore[i+scrolledRows] {
			t.Fatalf("row %d after Down does not match row %d before Down", i, i+scrolledRows)
		}
	}
	if linesAfterDown[39] == linesBefore[39] {
		t.Fatal("expected bottom row of View() to advance to a new placement row after Down")
	}

	// 2. Hit testing (CellToDocument and LinkAtCell) adds the viewport offset.
	topCellBeforeScroll := image.Pt(60, 0)
	docRectScrolled, ok := m.CellToDocument(topCellBeforeScroll)
	if !ok || docRectScrolled.Min.Y <= 0 {
		t.Fatalf("CellToDocument(60, 0) after scroll = %v (ok=%v), expected Min.Y > 0", docRectScrolled, ok)
	}
	// Find a link visible after scrolling and verify LinkAtCell hits it at its scrolled screen row.
	foundScrolledLink := false
	for y := 0; y < 40 && !foundScrolledLink; y++ {
		for x := 0; x < 120; x++ {
			if link, ok := m.LinkAtCell(image.Pt(x, y)); ok {
				area, ok2 := m.CellToDocument(image.Pt(x, y))
				if !ok2 {
					t.Fatalf("LinkAtCell hit %+v at (%d,%d) but CellToDocument returned false", link, x, y)
				}
				overlaps := false
				for _, r := range link.Regions {
					if !r.Intersect(area).Empty() {
						overlaps = true
						break
					}
				}
				if !overlaps {
					t.Fatalf("LinkAtCell(%d,%d) = %+v does not overlap CellToDocument %v", x, y, link.Regions, area)
				}
				foundScrolledLink = true
				break
			}
		}
	}
	if !foundScrolledLink {
		t.Fatal("expected LinkAtCell to find a link in the scrolled viewport")
	}

	// 3. SetResultKeepView keeps the viewport row and re-transmits once under StableID.
	idBeforeReflow := idCalls
	second := renderHNTestPage(t, "v2")
	reflowOut := driveRaw(t, m, m.SetResultKeepView(second))
	if idCalls != idBeforeReflow {
		t.Fatalf("SetResultKeepView allocated a new id under StableID: idCalls=%d, want %d", idCalls, idBeforeReflow)
	}
	vpReflow, ok := m.Picture().Viewport()
	if !ok || vpReflow.Y != vp1.Y {
		t.Fatalf("SetResultKeepView viewport = %+v (ok=%v), want Y=%d", vpReflow, ok, vp1.Y)
	}
	if strings.Count(reflowOut, "a=T") != 1 || strings.Contains(reflowOut, "a=d") {
		t.Fatalf("SetResultKeepView expected 1 a=T and no a=d, got a=T=%d a=d=%v", strings.Count(reflowOut, "a=T"), strings.Contains(reflowOut, "a=d"))
	}

	// 4. Tab (NextLink) paints the link-focus ring without re-uploading the full page band.
	idBeforeTab := idCalls
	viewBeforeTab := m.View()
	tabOut := driveRaw(t, m, m.Update(tea.KeyPressMsg{Code: tea.KeyTab}))
	if idCalls != idBeforeTab {
		t.Fatalf("Tab re-uploaded or allocated a new page band ID: idCalls=%d, want %d", idCalls, idBeforeTab)
	}
	if strings.Count(tabOut, "a=T") != 1 || len(tabOut) > 8192 {
		t.Fatalf("Tab should upload only the small focus ring overlay (< 8 KB, 1 a=T), got %d bytes and %d a=T", len(tabOut), strings.Count(tabOut, "a=T"))
	}
	if m.View() == viewBeforeTab {
		t.Fatal("expected View() to splice the focused link overlay after Tab")
	}

	// 5. Zoom re-sizes and re-uploads the band, then subsequent Down scrolls with 0 Kitty bytes.
	zoomOut := driveRaw(t, m, m.Zoom(1))
	if strings.Count(zoomOut, "a=T") < 1 {
		t.Fatalf("expected Zoom(1) to re-upload band, got %q", zoomOut)
	}
	downAfterZoom := driveRaw(t, m, m.Update(tea.KeyPressMsg{Code: tea.KeyDown}))
	if downAfterZoom != "" {
		t.Fatalf("expected Down after Zoom(1) to emit 0 Kitty bytes, got %d bytes", len(downAfterZoom))
	}

	// 6. Band crossing on a tall page (> 283 rows) uploads exactly one new image.
	tallImg := image.NewRGBA(image.Rect(0, 0, 800, 9000)) // 9000 / 21 = 429 rows (> 283)
	if first.Image != nil {
		srcB := first.Image.Bounds()
		for y := 0; y < 9000; y++ {
			srcY := srcB.Min.Y + (y % srcB.Dy())
			for x := 0; x < 800; x++ {
				tallImg.Set(x, y, first.Image.At(srcB.Min.X+x, srcY))
			}
		}
	}
	mTall := NewWithOptions(&document.Result{Image: tallImg}, ModelOptions{
		Picture: ntpicture.Options{
			StableID:    true,
			RetireDelay: -1,
			Render:      ntpicture.Config{KittyFormat: ntpicture.KittyFormatZlib},
		},
	})
	defer func() { driveRaw(t, mTall, mTall.Close()) }()
	tallInit := driveRaw(t, mTall, mTall.Picture().SetTerminal(true, 11, 21), mTall.SetSize(120, 40))
	if strings.Count(tallInit, "a=T") != 1 {
		t.Fatalf("expected 1 initial band upload for tall page, got %d", strings.Count(tallInit, "a=T"))
	}
	if _, rows := mTall.Picture().Size(); rows != maxKittyBandRows {
		t.Fatalf("tall page band rows = %d, want %d", rows, maxKittyBandRows)
	}
	// Scroll within the first band: 0 Kitty bytes.
	inBandOut := driveRaw(t, mTall, mTall.Pan(0, 200))
	if inBandOut != "" {
		t.Fatalf("expected 0 Kitty bytes scrolling within band 0, got %d bytes", len(inBandOut))
	}
	// Cross into the last screen of band 0 (row >= 243 -> y >= 243*21 = 5103): uploads exactly 1 new band image.
	crossOut := driveRaw(t, mTall, mTall.Pan(0, 5200))
	if strings.Count(crossOut, "a=T") != 1 {
		t.Fatalf("expected band crossing to upload exactly 1 new image (a=T), got %d in %d bytes", strings.Count(crossOut, "a=T"), len(crossOut))
	}
	// Subsequent scroll inside the new band emits 0 Kitty bytes.
	afterCrossDown := driveRaw(t, mTall, mTall.Update(tea.KeyPressMsg{Code: tea.KeyDown}))
	if afterCrossDown != "" {
		t.Fatalf("expected 0 Kitty bytes after band crossing, got %d bytes", len(afterCrossDown))
	}

	// 7. Also verify AutoMode discovery via Update(KittyCapabilityMsg) after SetSize(120, 40).
	var autoIDCalls int
	mAuto := NewWithOptions(first, ModelOptions{
		NextID: func() (int, error) {
			autoIDCalls++
			return 100 + autoIDCalls, nil
		},
		Picture: ntpicture.Options{
			StableID:    true,
			RetireDelay: -1,
			Render: ntpicture.Config{
				KittyFormat:     ntpicture.KittyFormatZlib,
				CellPixelWidth:  11,
				CellPixelHeight: 21,
			},
		},
	})
	defer func() { driveRaw(t, mAuto, mAuto.Close()) }()
	autoInit := driveRaw(t, mAuto, mAuto.SetSize(120, 40), mAuto.Update(ntpicture.KittyCapabilityMsg{Capability: ntpicture.KittyCapabilitySupported}))
	if autoIDCalls != 1 || strings.Count(autoInit, "a=T") != 1 {
		t.Fatalf("AutoMode transition allocated %d IDs and sent %d a=T commands (want 1 each)", autoIDCalls, strings.Count(autoInit, "a=T"))
	}
	if autoDown := driveRaw(t, mAuto, mAuto.Update(tea.KeyPressMsg{Code: tea.KeyDown})); autoDown != "" {
		t.Fatalf("expected 0 Kitty bytes on Down after AutoMode transition, got %d", len(autoDown))
	}

	t.Logf("HN page (120x40 cells, 11x21 px): initial band upload=%d bytes (%d chunks), Down keypress=%d Kitty bytes (%d scrolled rows in View()), Tab link focus=%d Kitty bytes (%d chunks)",
		len(initOut), strings.Count(initOut, "\x1b_G"), len(downOut), scrolledRows, len(tabOut), strings.Count(tabOut, "\x1b_G"))
}
