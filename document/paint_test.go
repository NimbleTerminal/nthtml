package document

import (
	"bytes"
	"context"
	"image"
	"strings"
	"testing"
)

const paintPage = `<html><body style="background:#fafafa"><h1 style="color:#c00020">Paint</h1>` +
	`<p>Some <u>underlined</u> and <s>struck</s> text, with a <a href="https://example.com/">link</a>.</p>` +
	`<ul><li>one</li><li>two</li></ul><table border="1"><tr><td>a</td><td>b</td></tr></table></body></html>`

func renderPaintPage(t *testing.T, o Options) *Result {
	t.Helper()
	d, err := Parse(context.Background(), strings.NewReader(strings.Repeat(paintPage, 6)), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	o.Width = 360
	r, err := d.Render(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Paint at one device pixel to the CSS pixel is the page Render paints,
// whole or a band of it.
func TestPaintMatchesRender(t *testing.T) {
	r := renderPaintPage(t, Options{})
	if r.Bounds() != r.Image.Bounds() {
		t.Fatalf("bounds %v, image %v", r.Bounds(), r.Image.Bounds())
	}
	whole, err := r.Paint(context.Background(), r.Bounds(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(whole.Pix, r.Image.Pix) {
		t.Fatal("whole page differs from Render's")
	}
	band := image.Rect(0, 100, 360, 260)
	part, err := r.Paint(context.Background(), band, 1)
	if err != nil {
		t.Fatal(err)
	}
	if part.Bounds() != image.Rect(0, 0, 360, 160) {
		t.Fatalf("band %v", part.Bounds())
	}
	for yy := 0; yy < 160; yy++ {
		got := part.Pix[yy*part.Stride : yy*part.Stride+360*4]
		want := r.Image.Pix[(yy+100)*r.Image.Stride : (yy+100)*r.Image.Stride+360*4]
		if !bytes.Equal(got, want) {
			t.Fatalf("band row %d differs", yy)
		}
	}
}

// NoImage lays out without painting; Paint then draws at a device scale,
// with the same text in the same places, larger.
func TestPaintScaled(t *testing.T) {
	r := renderPaintPage(t, Options{NoImage: true})
	if r.Image != nil || r.Bounds().Dx() != 360 || r.Bounds().Dy() == 0 || len(r.Links) == 0 {
		t.Fatalf("image %v bounds %v links %d", r.Image != nil, r.Bounds(), len(r.Links))
	}
	if r.PageBreak(0, 100) > 100 {
		t.Fatal("page break past its limit")
	}
	if _, ok := r.LinkAt(r.Links[0].Regions[0].Min); !ok {
		t.Fatal("no link under its own region")
	}
	area := image.Rect(0, 0, 360, 120)
	big, err := r.Paint(context.Background(), area, 2)
	if err != nil {
		t.Fatal(err)
	}
	if big.Bounds() != image.Rect(0, 0, 720, 240) {
		t.Fatalf("scaled %v", big.Bounds())
	}
	red := func(img *image.RGBA) int {
		n := 0
		for i := 0; i < len(img.Pix); i += 4 {
			if img.Pix[i] > 150 && img.Pix[i+1] < 80 && img.Pix[i+2] < 100 {
				n++
			}
		}
		return n
	}
	one, err := r.Paint(context.Background(), area, 1)
	if err != nil {
		t.Fatal(err)
	}
	// The heading's ink grows with the square of the scale, give or take
	// its anti-aliased edges.
	if a, b := red(one), red(big); a == 0 || b < 3*a || b > 5*a {
		t.Fatalf("heading pixels %d at 1×, %d at 2×", a, b)
	}
}

func TestPaintRefuses(t *testing.T) {
	r := renderPaintPage(t, Options{NoImage: true})
	for _, c := range []struct {
		area  image.Rectangle
		scale float64
	}{
		{image.Rect(0, r.Bounds().Max.Y, 10, r.Bounds().Max.Y+10), 1},
		{image.Rect(0, 0, 10, 10), 0},
		{image.Rect(0, 0, 10, 10), 17},
		{r.Bounds(), 16},
	} {
		if _, err := r.Paint(context.Background(), c.area, c.scale); err == nil {
			t.Errorf("Paint(%v, %v) succeeded", c.area, c.scale)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Paint(ctx, image.Rect(0, 0, 10, 10), 1); err == nil {
		t.Error("cancelled Paint succeeded")
	}
}
