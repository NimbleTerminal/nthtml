package document

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
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

func TestNoImageLongPageAndRaisedLimits(t *testing.T) {
	ctx := context.Background()
	// 6,000 <p> tags + 6,000 text nodes = 12,000 nodes (> DefaultLimits.Nodes = 10,000),
	// and ~228,000 CSS px tall (> DefaultLimits.Height = 16,384 and > 16M pixels at width 800).
	src := strings.Repeat("<p>Long specification paragraph with enough text to occupy vertical flow.</p>", 6000)
	if _, err := Parse(ctx, strings.NewReader(src), Limits{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected default 10,000-node limit to reject 12,000-node page, got %v", err)
	}
	doc, err := Parse(ctx, strings.NewReader(src), Limits{Nodes: 50000, LayoutHeight: 500000})
	if err != nil {
		t.Fatalf("Parse with raised Nodes limit failed: %v", err)
	}
	// Whole-page image mode still enforces the picture height/pixel limit.
	if _, err := doc.Render(ctx, Options{Width: 800}); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected whole-page Render to enforce picture limit, got %v", err)
	}
	// NoImage mode lays out the full tall page and leaves the pixel limit to Paint.
	res, err := doc.Render(ctx, Options{Width: 800, NoImage: true})
	if err != nil {
		t.Fatalf("NoImage Render on long page failed: %v", err)
	}
	if res.Bounds().Dy() <= DefaultLimits.Height || res.Bounds().Dx()*res.Bounds().Dy() <= DefaultLimits.Pixels {
		t.Fatalf("expected page to exceed default picture height and pixels, got %v", res.Bounds())
	}
	band := image.Rect(0, res.Bounds().Dy()-200, 800, res.Bounds().Dy())
	tile, err := res.Paint(ctx, band, 1)
	if err != nil || tile.Bounds() != image.Rect(0, 0, 800, 200) {
		t.Fatalf("Paint of bottom band failed: tile=%v err=%v", tile, err)
	}
	// Painting the entire multi-hundred-thousand-pixel-tall page at once still hits Pixels limit.
	if _, err := res.Paint(ctx, res.Bounds(), 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected whole-page Paint to exceed pixel limit, got %v", err)
	}
}

func TestFontFamiliesAndDefaultColorsOptions(t *testing.T) {
	ctx := context.Background()
	html := `<p>Default body text</p><p style="font-family:serif">Serif</p><p style="font-family:sans-serif">Sans</p><code>Mono</code><p><a href="https://example.com">Link</a></p>`
	base, err := Render(ctx, strings.NewReader(html), Options{Width: 320})
	if err != nil {
		t.Fatal(err)
	}
	custom, err := Render(ctx, strings.NewReader(html), Options{
		Width: 320,
		FontFamilies: FontFamilies{
			Default:   "IBM Plex Mono",
			Serif:     "IBM Plex Sans",
			SansSerif: "Literata",
			Monospace: "JetBrains Mono",
		},
		Colors: Colors{
			Background: color.RGBA{20, 24, 32, 255},
			Text:       color.RGBA{230, 235, 240, 255},
			Link:       color.RGBA{120, 200, 255, 255},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(base.Image.Pix, custom.Image.Pix) {
		t.Fatal("expected custom FontFamilies and Colors to change rendered output")
	}
	if got := custom.Image.RGBAAt(0, 0); got != (color.RGBA{20, 24, 32, 255}) {
		t.Fatalf("expected custom canvas background color, got %v", got)
	}
	foundText, foundLink := false, false
	for _, b := range custom.Boxes {
		if b.Text == "Default" && b.Color == (color.RGBA{230, 235, 240, 255}) {
			foundText = true
		}
		if b.Text == "Link" && b.Color == (color.RGBA{120, 200, 255, 255}) {
			foundLink = true
		}
	}
	if !foundText || !foundLink {
		t.Fatalf("expected custom default text (%v) and link (%v) colors", foundText, foundLink)
	}

	// If a page sets any text or background color of its own (e.g. #666 note or
	// white background), fallback theme colors must not apply and clash with it.
	for _, partial := range []string{
		`<p>Body</p><p style="color:#666">Grey note</p>`,
		`<div style="background:#ffffff"><p>White card</p></div>`,
	} {
		res, err := Render(ctx, strings.NewReader(partial), Options{
			Width: 320,
			Colors: Colors{
				Background: color.RGBA{20, 24, 32, 255},
				Text:       color.RGBA{230, 235, 240, 255},
				Link:       color.RGBA{120, 200, 255, 255},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Image.RGBAAt(0, 0); got != (color.RGBA{255, 255, 255, 255}) {
			t.Fatalf("expected partially-styled page %q to keep white canvas, got %v", partial, got)
		}
		for _, b := range res.Boxes {
			if b.Text == "Body" || b.Text == "White" {
				if b.Color != (color.RGBA{0, 0, 0, 255}) {
					t.Fatalf("expected partially-styled page %q to keep black default text, got %v", partial, b.Color)
				}
			}
		}
	}
}

func TestCharacterSelectionHitAndRegions(t *testing.T) {
	ctx := context.Background()
	res, err := Render(ctx, strings.NewReader(`<p>Kingfisher</p>`), Options{Width: 300, NoImage: true})
	if err != nil {
		t.Fatal(err)
	}
	idx := res.TextIndex()
	if len(idx.Runs) != 1 || idx.Text != "Kingfisher" {
		t.Fatalf("unexpected text index: text=%q runs=%+v", idx.Text, idx.Runs)
	}
	run := idx.Runs[0]
	if len(run.CharX) != len("Kingfisher") {
		t.Fatalf("expected CharX for every rune, got %d", len(run.CharX))
	}
	firstHit, ok := idx.Hit(image.Rect(run.Bounds.Min.X, run.Bounds.Min.Y, run.Bounds.Min.X+1, run.Bounds.Max.Y))
	if !ok || idx.Selected(firstHit.Start, firstHit.End) != "K" {
		t.Fatalf("expected first character 'K', got %q (%+v)", idx.Selected(firstHit.Start, firstHit.End), firstHit)
	}
	fourthX := run.CharX[2] + 1
	fourthHit, ok := idx.Hit(image.Rect(fourthX, run.Bounds.Min.Y, fourthX+1, run.Bounds.Max.Y))
	if !ok || idx.Selected(fourthHit.Start, fourthHit.End) != "g" {
		t.Fatalf("expected fourth character 'g', got %q (%+v)", idx.Selected(fourthHit.Start, fourthHit.End), fourthHit)
	}
	subRegions := idx.Regions(1, 4) // "ing"
	if len(subRegions) != 1 {
		t.Fatalf("expected 1 region for substring, got %v", subRegions)
	}
	if subRegions[0].Min.X <= run.Bounds.Min.X || subRegions[0].Max.X >= run.Bounds.Max.X {
		t.Fatalf("expected substring region %v to be strictly inside word bounds %v", subRegions[0], run.Bounds)
	}
}
