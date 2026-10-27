package document

import (
	"image"
	"strings"
	"testing"
)

func TestFontShorthandCascade(t *testing.T) {
	for _, value := range []string{`italic 700 20px/1.5 "IBM Plex Sans", sans-serif`, `700 italic 20px / 1.5 "IBM Plex Sans", sans-serif`} {
		r := renderCSS(t, `<div style='font:`+value+`'>Hello</div>`)
		b := tagged(r, "text")
		if b.FontSize != 20 || !b.Bold || !b.Italic || b.Bounds.Dy() != 30 || b.FontFamily != `"IBM Plex Sans", sans-serif` {
			t.Fatalf("%s: %+v", value, b)
		}
	}
	d := declarations(`font-weight:bold;font-style:italic;line-height:3;font:20px serif;font-size:12px`)
	if d["font-weight"] != "normal" || d["font-style"] != "normal" || d["line-height"] != "normal" || d["font-size"] != "12px" {
		t.Fatal(d)
	}
	d = declarations(`font-size:12px!important;font:20px serif;font-weight:bold!important;font:14px monospace!important`)
	if d["font-size"] != "14px !important" || d["font-weight"] != "normal !important" {
		t.Fatal(d)
	}
	for _, bad := range []string{"serif", "italic serif", "20px", "20px/ serif", "-2px serif", "20px/-1 serif", "20px a,", `20px "unclosed`, "bold bold 20px serif", "small-caps small-caps 20px serif", strings.Repeat("normal ", 65) + "20px serif"} {
		if _, ok := fontValues(bad); ok {
			t.Errorf("accepted %q", bad)
		}
		if d := declarations("font-size:12px;font:" + bad); d["font-size"] != "12px" {
			t.Fatal(bad, d)
		}
	}
}

func TestExplicitLineHeightInheritance(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{{"1", 10}, {".5", 5}, {"0", 0}, {"0px", 0}, {"5px", 5}, {"50%", 10}, {"normal", 13}} {
		r := renderCSS(t, `<article style="font-size:20px;line-height:`+tc.line+`"><span style="font-size:10px">X</span></article>`)
		b := tagged(r, "text")
		if b.Bounds.Dy() != tc.want {
			t.Errorf("%s: %v", tc.line, b.Bounds)
		}
	}
	r := renderCSS(t, `<div style="padding-top:40px;font:20px/0 serif"><a href="#x">X</a></div>`)
	b := tagged(r, "text")
	if b.Bounds.Dy() != 0 || b.InkBounds.Empty() || b.InkBounds.Max.Y > r.Image.Bounds().Max.Y {
		t.Fatal("zero-height ink clipped", b, r.Image.Bounds())
	}
	if len(r.Links) != 1 || len(r.Links[0].Regions) == 0 {
		t.Fatal("zero-height link has no hit region", r.Links)
	}
	before := b.InkBounds
	b.translate(image.Pt(5, 7))
	if b.InkBounds != before.Add(image.Pt(5, 7)) {
		t.Fatal("ink not translated")
	}
	r = renderCSS(t, `<div style="font-size:0">Invisible</div><section style="height:1px"></section>`)
	if tagged(r, "section").Bounds.Min.Y != 0 || !tagged(r, "text").InkBounds.Empty() {
		t.Fatal("zero font creates ink or flow height", r.Boxes)
	}
}

func TestWordSpacingLayout(t *testing.T) {
	for _, spacing := range []string{"5px", ".25em", "3.75pt"} {
		plain := renderCSS(t, `<div style="font:20px/1 monospace">a b</div>`)
		spaced := renderCSS(t, `<div style="font:20px/1 monospace;word-spacing:`+spacing+`">a b</div>`)
		a, b := boxesWithTag(plain, "text"), boxesWithTag(spaced, "text")
		if b[1].Bounds.Min.X-a[1].Bounds.Min.X != 5 {
			t.Fatal(spacing, a, b)
		}
	}
	r := renderCSS(t, `<div style="font-size:20px;word-spacing:.5em"><span style="font-size:10px">a&nbsp;b</span></div>`)
	b := tagged(r, "text")
	if b.WordSpacing != 10 {
		t.Fatal("inherited spacing must be computed on parent", b)
	}
	a := renderCSS(t, `<div style="font:20px/1 monospace">a b</div>`)
	b0 := boxesWithTag(a, "text")
	neg := renderCSS(t, `<div style="font:20px/1 monospace;word-spacing:-256px">a b</div>`)
	bn := boxesWithTag(neg, "text")
	if bn[1].Bounds.Min.X != bn[0].Bounds.Max.X || bn[1].Bounds.Min.X >= b0[1].Bounds.Min.X {
		t.Fatal("negative space clamp", bn)
	}
	for _, bad := range []string{"10%", "NaNpx", "1ex", "calc(10%)"} {
		if _, ok := wordSpacingValue(bad); ok {
			t.Fatal(bad)
		}
	}
}

func TestWordSpacingChangesWrapping(t *testing.T) {
	plain := renderCSS(t, `<div style="width:40px;font:20px/1 monospace">a b</div>`)
	spaced := renderCSS(t, `<div style="width:40px;font:20px/1 monospace;word-spacing:20px">a b</div>`)
	a, b := boxesWithTag(plain, "text"), boxesWithTag(spaced, "text")
	if a[0].Bounds.Min.Y != a[1].Bounds.Min.Y || b[1].Bounds.Min.Y-b[0].Bounds.Min.Y != 20 {
		t.Fatal("spacing did not affect wrap", a, b)
	}
	// A nonbreaking space stays inside the word but still receives word spacing.
	nbsp := renderCSS(t, `<div style="font:20px/1 monospace;word-spacing:20px">a&nbsp;b</div>`)
	normal := renderCSS(t, `<div style="font:20px/1 monospace">a&nbsp;b</div>`)
	if tagged(nbsp, "text").Bounds.Dx()-tagged(normal, "text").Bounds.Dx() != 20 {
		t.Fatal("NBSP advance")
	}
}

func TestZeroRootFontUnits(t *testing.T) {
	r := renderCSS(t, `<html style="font-size:0"><div style="font-size:20px;width:5rem;height:1rem;padding:1rem">X</div></html>`)
	b := tagged(r, "div")
	if b.Bounds.Dx() != 0 || b.Bounds.Dy() != 0 {
		t.Fatal("zero root must stay zero through descendant rem units", b)
	}
	r = renderCSS(t, `<div style="width:1rem;height:1rem"></div>`)
	if b := tagged(r, "div"); b.Bounds.Dx() != 16 || b.Bounds.Dy() != 16 {
		t.Fatal("fragment initial root", b)
	}
}
