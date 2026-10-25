package document

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"
)

func TestCSSLengthUnits(t *testing.T) {
	for _, s := range []string{"96px", "1in", "2.54cm", "25.4mm", "101.6q", "72pt", "6pc", "1IN"} {
		l, ok := parseLength(s, false, false)
		if !ok || l.resolve(0, 16384) != 96 {
			t.Errorf("%s = %+v", s, l)
		}
	}
	for _, s := range []string{"1ex", "1ch", "1vw", "1 rem", "NaNem", "Infpx", "0x1p2", "-1cm", "1e999in", "1."} {
		if _, ok := parseLength(s, false, false); ok {
			t.Errorf("accepted %q", s)
		}
	}
}
func TestRelativeLengthsAndRootFont(t *testing.T) {
	r := renderCSS(t, `<html style="font-size:20px"><body style="font-size:10px;margin:0"><article style="font-size:2em;width:10em;padding:.5em;border:solid .1em blue"><section style="font-size:.5em;width:10rem;height:2em"></section></article></body></html>`)
	a, b := tagged(r, "article"), tagged(r, "section")
	if a.Bounds.Dx() != 224 || b.Bounds.Dx() != 200 || b.Bounds.Dy() != 20 || b.Bounds.Min.X != 12 {
		t.Fatal(a.Bounds, b.Bounds)
	}
	r = renderCSS(t, `<html style="font-size:2rem"><article style="font-size:.5rem;width:2rem;height:1em"></article></html>`)
	if a := tagged(r, "article"); a.Bounds.Dx() != 64 || a.Bounds.Dy() != 16 {
		t.Fatal(a)
	}
	r = renderCSS(t, `<html style="font-size:8px"><article style="width:2rem;height:1rem"></article></html>`)
	if a := tagged(r, "article"); a.Bounds.Dx() != 16 || a.Bounds.Dy() != 8 {
		t.Fatal("small root size", a)
	}
}
func TestUnitLineHeightAndSpacing(t *testing.T) {
	r := renderCSS(t, `<article style="font-size:20px;line-height:2em"><span style="font-size:10px">Inherited</span></article><table style="font-size:20px;border-spacing:.5em .25em"><tr><td>A</td><td>B</td></tr></table>`)
	if b := tagged(r, "text"); b.Bounds.Dy() != 40 {
		t.Fatal("relative line-height must compute before inheritance", b)
	}
	cells := boxesWithTag(r, "td")
	if cells[1].Bounds.Min.X-cells[0].Bounds.Max.X != 10 {
		t.Fatal("relative spacing", cells)
	}
}
func TestMinMaxDimensions(t *testing.T) {
	r := renderCSS(t, `<article style="width:10px;max-width:20px;min-width:3em;font-size:20px;height:10px;max-height:20px;min-height:2em;padding:2px;border:1px solid black"><section style="height:50%"></section></article>`)
	a, b := tagged(r, "article"), tagged(r, "section")
	if a.Bounds.Dx() != 66 || a.Bounds.Dy() != 46 || b.Bounds.Dy() != 20 {
		t.Fatal(a.Bounds, b.Bounds)
	}
	r = renderCSS(t, `<article style="height:100px;max-height:20px;box-sizing:border-box;padding:2px;border:1px solid black">Overflow</article><section style="height:1px"></section>`)
	if tagged(r, "article").Bounds.Dy() != 20 || tagged(r, "section").Bounds.Min.Y != 20 {
		t.Fatal("max-height/box sizing")
	}
	r = renderCSS(t, `<article style="max-height:0">Overflow</article>`)
	if tagged(r, "article").Bounds.Dy() != 0 || r.Image.Bounds().Dy() < tagged(r, "text").Bounds.Max.Y {
		t.Fatal("auto max-height clips visible overflow")
	}
}
func TestCSSColors(t *testing.T) {
	for _, s := range []string{"red", "#f00", "#ff0000", "rgb(255, 0, 0)", "rgb(100% 0% 0%)", "hsl(0, 100%, 50%)", "hsl(1turn 100% 50%)"} {
		got, ok := cssColor(s, color.RGBA{})
		if !ok || got != (color.RGBA{255, 0, 0, 255}) {
			t.Errorf("%s: %v %v", s, got, ok)
		}
	}
	for _, s := range []string{"#ff000080", "rgba(255,0,0,.5)", "rgb(255 0 0 / 50%)", "hsla(0,100%,50%,.5)"} {
		got, ok := cssColor(s, color.RGBA{})
		if !ok || got != (color.RGBA{128, 0, 0, 128}) {
			t.Errorf("%s: %v", s, got)
		}
	}
	for _, s := range []string{"rgb(NaN 0 0)", "rgb(1 2)", "rgb(1,2,3 / .5)", "hsl(0 1 1)", "#fffff", "bogus", "rgb(1e999 0 0)"} {
		if _, ok := cssColor(s, color.RGBA{}); ok {
			t.Errorf("accepted %s", s)
		}
	}
	r := renderCSS(t, `<article style="color:rebeccapurple;color:invalid;background:currentColor;height:10px;border:solid 1px transparent"></article>`)
	if a := tagged(r, "article"); a.Color != (color.RGBA{102, 51, 153, 255}) {
		t.Fatal("currentColor or invalid fallback", a.Color)
	}
	if b := tagged(r, "border"); b.Color.A != 0 {
		t.Fatal("transparent border became text color")
	}
	r = renderCSS(t, `<article style="background:rgba(255,0,0,.5);height:10px"></article>`)
	if c := r.Image.RGBAAt(5, 5); c.R != 255 || c.G != 127 || c.B != 127 {
		t.Fatal("alpha compositing", c)
	}
}
func TestBorderOrderAndReset(t *testing.T) {
	for _, border := range []string{"blue solid 2px", "2px blue solid", "solid blue 2px"} {
		r := renderCSS(t, `<article style="border:`+border+`;height:1px"></article>`)
		if a := tagged(r, "article"); a.Bounds.Dy() != 5 {
			t.Fatal(border, a.Bounds)
		}
	}
	r := renderCSS(t, `<article style="border:solid thick red;border:none;height:1px"></article>`)
	if len(boxesWithTag(r, "border")) != 0 {
		t.Fatal("none did not reset border")
	}
	r = renderCSS(t, `<table><tr><td style="border:2px solid transparent;vertical-align:bottom">A</td><td style="height:50px">B</td></tr></table>`)
	if len(boxesWithTag(r, "td")) != 2 {
		t.Fatal("transparent table border layout")
	}
}
func TestUnitsRetainResourceLimits(t *testing.T) {
	for _, html := range []string{`<article style="height:16384in;padding:1px"></article>`, `<article style="font-size:128px;height:16384em;padding:1px"></article>`} {
		_, err := Render(context.Background(), strings.NewReader(html), Options{Width: 400})
		if !errors.Is(err, ErrLimit) {
			t.Fatalf("expected limit, got %v", err)
		}
	}
}

func TestZeroWidthDoesNotPaintBackground(t *testing.T) {
	r := renderCSS(t, `<article style="width:0;min-width:0pt;height:1in;background:red"></article>`)
	if b := tagged(r, "article"); b.Bounds.Dx() != 0 {
		t.Fatal(b.Bounds)
	}
	for _, c := range r.Image.Pix {
		if c != 255 {
			t.Fatal("zero-width box painted")
		}
	}
}

func TestTranslucentTableBackgroundPaintsOnce(t *testing.T) {
	r := renderCSS(t, `<table style="background:rgba(255,0,0,.5)"><tr><td style="width:20px;height:20px"></td></tr></table>`)
	if c := r.Image.RGBAAt(5, 5); c != (color.RGBA{255, 127, 127, 255}) {
		t.Fatal("table background applied again on row", c)
	}
}
func TestPhysicalAndPixelLayoutEquivalent(t *testing.T) {
	for _, length := range []string{"1in", "2.54cm", "25.4mm", "101.6q", "72pt", "6pc", "6em", "6rem"} {
		r := renderCSS(t, `<article style="width:`+length+`;height:`+length+`;font-size:16px;background:steelblue"></article>`)
		if box := tagged(r, "article"); box.Bounds.Dx() != 96 || box.Bounds.Dy() != 96 {
			t.Errorf("%s: %v", length, box.Bounds)
		}
	}
}

func TestAlphaBorderCornersPaintOnce(t *testing.T) {
	for _, source := range []string{`<article style="width:20px;height:20px;border:rgba(255,0,0,.5) solid 2px"></article>`, `<table style="border-spacing:0"><tr><td style="width:20px;height:20px;border:rgba(255,0,0,.5) solid 2px"></td></tr></table>`} {
		r := renderCSS(t, source)
		corner, edge := r.Image.RGBAAt(0, 0), r.Image.RGBAAt(5, 0)
		if corner != edge || corner != (color.RGBA{255, 127, 127, 255}) {
			t.Fatal("overlapping border corners", corner, edge)
		}
	}
}

func TestDefaultForegroundIsCSSBlack(t *testing.T) {
	r := renderCSS(t, `<div style="width:20px;height:20px;border:2px solid">text</div>`)
	black := color.RGBA{0, 0, 0, 255}
	if got := r.Image.RGBAAt(0, 0); got != black {
		t.Fatalf("expected default border currentColor black, got %v", got)
	}
	if got := tagged(r, "text").Color; got != black {
		t.Fatalf("expected default text color black, got %v", got)
	}
}
