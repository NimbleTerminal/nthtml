package nthtml

import (
	"context"
	"github.com/charmbracelet/x/ansi"
	"image"
	"nimbleterminal.dev/nthtml/document"
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
