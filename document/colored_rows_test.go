package document

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
)

func TestEscapedSelectorIdentifiers(t *testing.T) {
	for _, tc := range []struct{ selector, class string }{
		{`.bg-\[var\(--key\)\]`, `bg-[var(--key)]`},
		{`.a\,b`, `a,b`}, {`.a\{b`, `a{b`}, {`.a\[b`, `a[b`}, {`.a\]b`, `a]b`}, {`.hover\:x`, `hover:x`},
		{`.\31 23`, `123`}, {`.\00003123`, `123`}, {`.caf\e9`, `café`}, {`.café`, `café`},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			r := renderCSS(t, `<style>`+tc.selector+`{color:red}</style><p class="`+tc.class+`">Hello</p>`)
			if got := tagged(r, "text").Color; got != (color.RGBA{255, 0, 0, 255}) {
				t.Fatalf("escaped selector did not match: %v", got)
			}
		})
	}
	for _, s := range []string{".x\\", ".x\\\ny", ".123", ".-1", ".-"} {
		if _, _, ok := parseSelector(s); ok {
			t.Errorf("accepted %q", s)
		}
	}
	r := renderCSS(t, `<style>.a\,b, .other {color:red}</style><p class="a">Hello</p>`)
	if tagged(r, "text").Color == (color.RGBA{255, 0, 0, 255}) {
		t.Fatal("escaped comma broadened selector")
	}
}

func TestColoredRoundedFlexRows(t *testing.T) {
	src, err := os.ReadFile("testdata/colored-rows.html")
	if err != nil {
		t.Fatal(err)
	}
	r := renderCSS(t, string(src))
	var rows []Box
	for _, b := range r.Boxes {
		if b.Tag == "a" {
			rows = append(rows, b)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("rows: %d", len(rows))
	}
	for i, want := range []color.RGBA{{101, 91, 167, 255}, {167, 215, 177, 255}} {
		b := rows[i]
		if b.Color != want {
			t.Fatalf("row %d background: %v", i, b.Color)
		}
		if got := r.Image.RGBAAt(b.Bounds.Min.X, b.Bounds.Min.Y); got != r.Background() {
			t.Fatalf("square corner: %v", got)
		}
		if got := r.Image.RGBAAt(b.Bounds.Min.X+12, b.Bounds.Min.Y+2); got != want {
			t.Fatalf("missing background: %v", got)
		}
	}
	var label, links Box
	for _, b := range r.Boxes {
		if b.Text == "Media" {
			label = b
		}
		if strings.Contains(b.Text, "ntsvg") {
			links = b
		}
	}
	if links.Bounds.Min.X <= label.Bounds.Max.X+50 || links.Bounds.Min.Y != label.Bounds.Min.Y {
		t.Fatalf("nested flex alignment: label=%v links=%v", label.Bounds, links.Bounds)
	}
	// Cropped bands must be identical to whole-page pixels, including corners.
	band := image.Rect(0, 3, r.Bounds().Dx(), r.Bounds().Dy()-3)
	img, err := r.Paint(context.Background(), band, 1)
	if err != nil {
		t.Fatal(err)
	}
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if img.RGBAAt(x, y) != r.Image.RGBAAt(x, y+3) {
				t.Fatalf("band mismatch at %d,%d", x, y)
			}
		}
	}
}

func TestRoundedBackgroundAndBorder(t *testing.T) {
	for _, radius := range []string{"20px", "50%", "40px / 20px", "var(--r)"} {
		r := renderCSS(t, `<style>body{margin:0}div{--r:20px;width:80px;height:40px;background:red;border:4px solid blue;border-radius:`+radius+`}</style><div></div>`)
		b := tagged(r, "div").Bounds
		if got := r.Image.RGBAAt(b.Min.X, b.Min.Y); got != r.Background() {
			t.Fatalf("%s corner=%v", radius, got)
		}
		if got := r.Image.RGBAAt(b.Min.X+b.Dx()/2, b.Min.Y+1); got != (color.RGBA{0, 0, 255, 255}) {
			t.Fatalf("%s border=%v", radius, got)
		}
		if got := r.Image.RGBAAt(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2); got != (color.RGBA{255, 0, 0, 255}) {
			t.Fatalf("%s center=%v", radius, got)
		}
		p, err := r.NewPainter(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, scale := range []float64{1.0 / 16, .5, 1, 2, 16} {
			area := b.Intersect(image.Rect(0, 3, 400, 100))
			a, err := r.Paint(context.Background(), area, scale)
			if err != nil {
				t.Fatal(err)
			}
			b, err := p.Paint(context.Background(), area, scale)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(a.Pix, b.Pix) {
				t.Fatalf("painter differs at scale %v", scale)
			}
		}
		p.Close()
	}
}

func TestRadiusCascadeAndOverlap(t *testing.T) {
	for _, tc := range []struct {
		css  string
		want [4][2]float64
	}{
		{"border-radius:10px 20px / 5px 15px", [4][2]float64{{10, 5}, {20, 15}, {10, 5}, {20, 15}}},
		{"border-radius:10px;border-top-left-radius:2px 3px;border-radius:-5px", [4][2]float64{{2, 3}, {10, 10}, {10, 10}, {10, 10}}},
		{"border-radius:10px;border-radius:initial", [4][2]float64{}},
		{"border-radius:1000px", [4][2]float64{{25, 25}, {25, 25}, {25, 25}, {25, 25}}},
	} {
		r := renderCSS(t, `<div style="width:100px;height:50px;`+tc.css+`"></div>`)
		if got := tagged(r, "div").radii; got != tc.want {
			t.Fatalf("%s: %v want %v", tc.css, got, tc.want)
		}
	}
}

func TestNestedGridPreservesFlex(t *testing.T) {
	r := renderCSS(t, `<div style="display:grid;grid-template-columns:1fr"><div style="display:flex;justify-content:space-between"><span>Left</span><span>Right</span></div></div>`)
	var left, right Box
	for _, b := range r.Boxes {
		if b.Text == "Left" {
			left = b
		}
		if b.Text == "Right" {
			right = b
		}
	}
	if right.Bounds.Min.X-left.Bounds.Max.X < 100 || right.Bounds.Min.Y != left.Bounds.Min.Y {
		t.Fatalf("nested layout: %v %v", left.Bounds, right.Bounds)
	}
}

func TestRoundedBackgroundImageAndTranslation(t *testing.T) {
	r := renderCSS(t, `<div style="position:relative;left:20px;top:10px;width:60px;height:40px;border-radius:12px;background:linear-gradient(red,blue)"></div>`)
	b := tagged(r, "div").Bounds
	if got := r.Image.RGBAAt(b.Min.X, b.Min.Y); got != r.Background() {
		t.Fatalf("image corner: %v", got)
	}
	if got := r.Image.RGBAAt(b.Min.X+30, b.Min.Y+20); got == r.Background() {
		t.Fatal("background image missing")
	}
	area := image.Rect(b.Min.X+2, b.Min.Y+2, b.Max.X-2, b.Max.Y-2)
	img, err := r.Paint(context.Background(), area, 1)
	if err != nil {
		t.Fatal(err)
	}
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if img.RGBAAt(x, y) != r.Image.RGBAAt(x+area.Min.X, y+area.Min.Y) {
				t.Fatalf("translated image band mismatch %d,%d", x, y)
			}
		}
	}
}

func TestEscapedTypeSelectorDoesNotBroaden(t *testing.T) {
	for _, sel := range []string{`\2a`, `\.x`, `\#x`} {
		r := renderCSS(t, `<style>`+sel+`{color:red}</style><p class="x" id="x">Hello</p>`)
		if tagged(r, "text").Color == (color.RGBA{255, 0, 0, 255}) {
			t.Fatalf("type selector broadened: %s", sel)
		}
	}
}
