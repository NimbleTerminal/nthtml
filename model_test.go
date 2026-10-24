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
	defer m.Close()
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
	defer m.Close()
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
