package document

import (
	"context"
	"image"
	"strings"
	"testing"
)

func TestPageBreakFallsBetweenLines(t *testing.T) {
	r, err := Render(context.Background(), strings.NewReader(strings.Repeat("<p>Line of text that wraps across the narrow page more than once.</p>", 40)), Options{Width: 200})
	if err != nil {
		t.Fatal(err)
	}
	h := r.Image.Bounds().Dy()
	if h < 400 {
		t.Fatalf("page only %d tall", h)
	}
	for limit := 100; limit < 300; limit += 7 {
		y := r.PageBreak(0, limit)
		if y > limit || y <= limit/2 {
			t.Fatalf("PageBreak(0, %d) = %d", limit, y)
		}
		for _, b := range r.Boxes {
			if b.Tag == "text" && b.Bounds.Min.Y < y && y < b.Bounds.Max.Y {
				t.Fatalf("PageBreak(0, %d) = %d cuts %q at %v", limit, y, b.Text, b.Bounds)
			}
		}
	}
	if y := r.PageBreak(0, h+500); y != h {
		t.Fatalf("past the end: %d, want %d", y, h)
	}
}

func TestImageIntrinsicSize(t *testing.T) {
	img := testImage(t, 40, 20)
	for _, c := range []struct {
		html string
		w, h int
	}{
		{`<img src="a">`, 40, 20},
		{`<img src="a" width="100">`, 100, 50},
		{`<img src="a" width="100" height="10">`, 100, 10},
		{`<img src="a" height="40">`, 80, 40},
		{`<img src="a" width="900">`, 200, 100},
		{`<img src="a" style="width: 50%">`, 100, 50},
	} {
		r, err := Render(context.Background(), strings.NewReader(c.html), Options{Width: 200, Images: map[string]image.Image{"a": img}})
		if err != nil {
			t.Fatal(err)
		}
		var got image.Rectangle
		for _, b := range r.Boxes {
			if b.Tag == "img" {
				got = b.Bounds
			}
		}
		if got.Dx() != c.w || got.Dy() != c.h {
			t.Errorf("%s: %v, want %d×%d", c.html, got, c.w, c.h)
		}
	}
}

func testImage(t *testing.T, w, h int) image.Image {
	t.Helper()
	return image.NewRGBA(image.Rect(0, 0, w, h))
}
