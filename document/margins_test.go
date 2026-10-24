package document

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
)

func TestParagraphAndBodyDefaultMargins(t *testing.T) {
	r := renderCSS(t, `<html><body><p>First</p><p style="font-size:20px">Second</p></body></html>`)
	ps := boxesWithTag(r, "p")
	body := tagged(r, "body")
	if ps[0].Bounds.Min != (image.Point{8, 16}) || body.Bounds.Min != (image.Point{8, 16}) {
		t.Fatal("body/paragraph defaults", body.Bounds, ps)
	}
	if gap := ps[1].Bounds.Min.Y - ps[0].Bounds.Max.Y; gap != 20 {
		t.Fatal("1em margins should collapse to the larger font-relative value", gap)
	}
	r = renderCSS(t, `<p>Fragment</p>`)
	if p := tagged(r, "p"); p.Bounds.Min != (image.Point{0, 16}) {
		t.Fatal("fragment paragraph", p.Bounds)
	}
}
func TestSiblingMarginsAndEmptyChains(t *testing.T) {
	for _, tc := range []struct {
		middle string
		gap    int
	}{
		{"", 30},
		{`<div style="margin:5px 0 50px"></div>`, 50},
		{`<div style="margin:5px 0 50px"><section style="margin:10px 0 40px"></section></div>`, 50},
		{`<div style="display:none;margin:100px"></div>`, 30},
		{` <span> </span> `, 30},
	} {
		r := renderCSS(t, `<article style="height:10px;margin-bottom:20px"></article>`+tc.middle+`<aside style="height:10px;margin-top:30px"></aside>`)
		a, b := tagged(r, "article"), tagged(r, "aside")
		if gap := b.Bounds.Min.Y - a.Bounds.Max.Y; gap != tc.gap {
			t.Fatalf("%s: gap %d want %d", tc.middle, gap, tc.gap)
		}
	}
}
func TestParentChildMarginCollapse(t *testing.T) {
	r := renderCSS(t, `<article style="margin:10px 0 12px;background:blue"><section style="height:10px;margin:30px 0 40px"></section></article><aside style="height:10px;margin-top:20px"></aside>`)
	a, b, c := tagged(r, "article"), tagged(r, "section"), tagged(r, "aside")
	if a.Bounds.Min.Y != 30 || a.Bounds != b.Bounds || c.Bounds.Min.Y-a.Bounds.Max.Y != 40 {
		t.Fatal(a.Bounds, b.Bounds, c.Bounds)
	}
	// Escaped child margins must not be included in the parent's background.
	if pixel := r.Image.RGBAAt(1, a.Bounds.Max.Y+1); pixel.B != 255 || pixel.R != 255 {
		t.Fatal("background paints collapsed margin", pixel)
	}
}
func TestMarginBarriers(t *testing.T) {
	for _, tc := range []struct {
		css   string
		inset int
	}{
		{"padding:1px", 1}, {"border:1px solid blue", 1}, {"padding:2px;border:1px solid blue", 3},
	} {
		r := renderCSS(t, `<article style="`+tc.css+`"><section style="height:10px;margin:30px 0 40px"></section></article>`)
		a, b := tagged(r, "article"), tagged(r, "section")
		if b.Bounds.Min.Y-a.Bounds.Min.Y != 30+tc.inset || a.Bounds.Max.Y-b.Bounds.Max.Y != 40+tc.inset {
			t.Fatal(tc.css, a.Bounds, b.Bounds)
		}
	}
	for _, css := range []string{"height:100px", "height:0", "min-height:1px"} {
		r := renderCSS(t, `<article style="`+css+`;margin-bottom:12px"><section style="height:10px;margin-bottom:40px"></section></article><aside style="height:10px;margin-top:20px"></aside>`)
		a, c := tagged(r, "article"), tagged(r, "aside")
		if c.Bounds.Min.Y-a.Bounds.Max.Y != 20 {
			t.Fatal("bottom margin escaped a height constraint", css, a.Bounds, c.Bounds)
		}
	}
	r := renderCSS(t, `<table><tr><td><p>Cell</p></td></tr></table>`)
	cell, p := tagged(r, "td"), tagged(r, "p")
	if p.Bounds.Min.Y-cell.Bounds.Min.Y != 16 || cell.Bounds.Max.Y-p.Bounds.Max.Y != 16 {
		t.Fatal("cell boundary", cell.Bounds, p.Bounds)
	}
}
func TestInlineLinesSeparateMargins(t *testing.T) {
	r := renderCSS(t, `<article><section style="height:10px;margin-bottom:20px"></section>Text<aside style="height:10px;margin-top:30px"></aside></article>`)
	a, b, c := tagged(r, "section"), tagged(r, "text"), tagged(r, "aside")
	if b.Bounds.Min.Y-a.Bounds.Max.Y != 20 || c.Bounds.Min.Y-b.Bounds.Max.Y != 30 {
		t.Fatal(a.Bounds, b.Bounds, c.Bounds)
	}
	r = renderCSS(t, "<article style='white-space:pre-line'>\n</article><aside style='height:10px;margin-top:30px'></aside>")
	if tagged(r, "article").Bounds.Dy() == 0 {
		t.Fatal("preserved newline collapsed through")
	}
}
func TestMarginCollapseBudget(t *testing.T) {
	source := strings.Repeat("<article>", 50) + strings.Repeat("<div><p>x</p></div>", 2000) + strings.Repeat("</article>", 50)
	d, err := Parse(context.Background(), strings.NewReader(source), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Render(context.Background(), Options{Width: 400})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("expected bounded layout work/output, got %v", err)
	}
}

func TestConstrainedHeightsAndThroughMargins(t *testing.T) {
	r := renderCSS(t, `<article style="height:100px;max-height:0;margin:10px 0 20px"></article><aside style="height:10px;margin-top:30px"></aside>`)
	if got := tagged(r, "aside").Bounds.Min.Y; got != 40 {
		t.Fatalf("positive specified height must prevent own-margin collapse: %d", got)
	}
	r = renderCSS(t, `<article style="height:100px;max-height:0"><section style="height:50%;margin:10px 0 20px"></section></article>`)
	if got := tagged(r, "article").Bounds.Min.Y; got != 20 {
		t.Fatalf("child percentages must use constrained height: %d", got)
	}
}
