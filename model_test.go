package nthtml

import (
	"image"
	"nimbleterminal.dev/nthtml/document"
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
