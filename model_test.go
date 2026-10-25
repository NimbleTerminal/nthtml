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
