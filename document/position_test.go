package document

import (
	"context"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestRelativePositionPreservesFlow(t *testing.T) {
	r := renderCSS(t, `<article style="height:30px;width:100px;position:relative;left:20px;top:-5px"><a href="#after">Move</a></article><section id="after" style="height:10px"></section>`)
	a, b := tagged(r, "article"), tagged(r, "section")
	if a.Bounds != image.Rect(20, -5, 120, 25) || b.Bounds.Min.Y != 30 {
		t.Fatal(a.Bounds, b.Bounds)
	}
	text := tagged(r, "text")
	if _, ok := r.LinkAt(text.Bounds.Intersect(r.Image.Bounds()).Min); !ok {
		t.Fatal("link regions did not move")
	}
	if r.Anchors["after"].Y != 30 {
		t.Fatal("normal-flow anchor displaced")
	}
}
func TestAbsoluteContainingBlockAndNestedOffsets(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative;left:10px;top:5px;width:200px;height:100px;padding:10px;border:2px solid blue"><section><aside id="badge" style="position:absolute;left:50%;top:25%;width:20px;height:10px;background:red"></aside></section></article><div style="height:10px"></div>`)
	a, b := tagged(r, "article"), tagged(r, "aside")
	if b.Bounds != image.Rect(122, 37, 142, 47) {
		t.Fatal("padding-box containing block", a.Bounds, b.Bounds)
	}
	if r.Anchors["badge"] != b.Bounds.Min {
		t.Fatal("absolute anchor lost", r.Anchors)
	}
	if tagged(r, "div").Bounds.Min.Y != 124 {
		t.Fatal("absolute or relative offsets altered normal flow")
	}
}
func TestAbsoluteRightBottomAndStretch(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative;width:200px;height:100px"><aside style="position:absolute;right:10%;bottom:10px;width:20px;height:15px"></aside><section style="position:absolute;left:10px;right:20px;top:5px;bottom:15px"></section></article>`)
	if b := tagged(r, "aside"); b.Bounds != image.Rect(160, 75, 180, 90) {
		t.Fatal(b.Bounds)
	}
	if b := tagged(r, "section"); b.Bounds != image.Rect(10, 5, 180, 85) {
		t.Fatal(b.Bounds)
	}
}
func TestAbsoluteShrinkToFitAndOverflow(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative"><aside style="position:absolute;left:0;top:0;padding:10px;border:2px solid blue"><div style="width:100px;height:30px"></div></aside></article><section style="height:1px"></section>`)
	if b := tagged(r, "aside"); b.Bounds != image.Rect(0, 0, 124, 54) {
		t.Fatal(b.Bounds)
	}
	if tagged(r, "section").Bounds.Min.Y != 0 || r.Image.Bounds().Dy() != 54 {
		t.Fatal("absolute content must extend canvas without taking flow space", r.Image.Bounds())
	}
}
func TestPositionedPaintOrder(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative;top:20px;height:20px;width:20px;background:red"></article><section style="height:30px;background:blue"></section><aside style="position:absolute;left:0;top:20px;width:10px;height:10px;background:green"></aside>`)
	if got := r.Image.RGBAAt(15, 25); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatal("relative box below normal flow", got)
	}
	if got := r.Image.RGBAAt(5, 25); got != (color.RGBA{0, 128, 0, 255}) {
		t.Fatal("positioned source order", got)
	}
}
func TestPositionedContentInsideCells(t *testing.T) {
	r := renderCSS(t, `<div style="height:20px"></div><table><tr><td style="height:60px;vertical-align:bottom"><article style="position:relative"><aside style="position:absolute;left:2px;top:3px;width:10px;height:5px"></aside>Cell</article></td></tr></table>`)
	a, b := tagged(r, "article"), tagged(r, "aside")
	if b.Bounds.Min != a.Bounds.Min.Add(image.Pt(2, 3)) {
		t.Fatal("table translation lost containing block", a.Bounds, b.Bounds)
	}
}
func TestAbsoluteInlineDescendantAndTTY(t *testing.T) {
	r := renderCSS(t, `<p>Before <span><a href="https://example.com" style="position:absolute;left:10px;top:2px">Badge</a></span> after</p>`)
	text, err := r.TTY(context.Background(), 40)
	if err != nil || !strings.Contains(text, "Badge") {
		t.Fatal("TTY must retain source text", text, err)
	}
	for _, b := range r.Boxes {
		if b.Text == "Badge" {
			if b.Bounds.Min != image.Pt(10, 2) {
				t.Fatal(b.Bounds)
			}
			if _, ok := r.LinkAt(b.Bounds.Min); !ok {
				t.Fatal("absolute link hit region")
			}
		}
	}
}
func TestPositionLimitsAndInvalidValues(t *testing.T) {
	for _, source := range []string{`<article style="position:absolute;top:16384px;height:10px"></article>`, `<article style="position:relative;top:-16384px"><section style="position:relative;top:-1px;height:10px"></section></article>`} {
		_, err := Render(context.Background(), strings.NewReader(source), Options{Width: 400})
		if !errors.Is(err, ErrLimit) {
			t.Fatal("expected bounded position", err)
		}
	}
	r := renderCSS(t, `<article style="position:relative;position:fixed;left:10px;left:NaN;top:auto;height:10px"></article>`)
	if b := tagged(r, "article"); b.Bounds.Min.X != 10 || b.Bounds.Min.Y != 0 {
		t.Fatal("invalid declaration fallback", b.Bounds)
	}
}

func TestAbsoluteAutoMarginsAndRelativePrecedence(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative;width:200px;height:100px"><aside style="position:absolute;left:10px;right:10px;width:40px;height:10px;margin:0 auto"></aside></article><section style="position:relative;left:2.54cm;right:3px;bottom:5px;height:10px"></section>`)
	if b := tagged(r, "aside"); b.Bounds != image.Rect(80, 0, 120, 10) {
		t.Fatal("absolute auto margins", b.Bounds)
	}
	if b := tagged(r, "section"); b.Bounds.Min != image.Pt(96, 95) {
		t.Fatal("relative offset precedence/physical units", b.Bounds)
	}
}
func TestOverlappingPositionedLinks(t *testing.T) {
	r := renderCSS(t, `<a href="https://example.com/first" style="position:absolute;left:0;top:0">Link</a><a href="https://example.com/last" style="position:absolute;left:0;top:0">Link</a>`)
	hit, ok := r.LinkAt(image.Pt(1, 1))
	if !ok || hit.Href != "https://example.com/last" {
		t.Fatal("wrong paint-order link", hit, ok)
	}
}
func TestAbsoluteNestedFrameAndPercentageSpacing(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative;left:10px;width:200px;height:100px"><aside style="position:absolute;left:10%;top:10px;width:50%;height:50%;padding:5%"><section style="position:absolute;right:0;bottom:0;width:10px;height:10px"></section></aside></article>`)
	a, b := tagged(r, "aside"), tagged(r, "section")
	if a.Bounds != image.Rect(30, 10, 150, 80) || b.Bounds != image.Rect(140, 70, 150, 80) {
		t.Fatal("nested absolute frames", a.Bounds, b.Bounds)
	}
}

func TestInvalidSignedInsets(t *testing.T) {
	for _, value := range []string{"-+2px", "- 2px", "--2px", "-auto"} {
		r := renderCSS(t, `<article style="position:relative;left:10px;left:`+value+`;height:10px"></article>`)
		if b := tagged(r, "article"); b.Bounds.Min.X != 10 {
			t.Fatal("invalid offset replaced valid declaration", value, b.Bounds)
		}
	}
}

func TestZIndexStackingOrder(t *testing.T) {
	r := renderCSS(t, `<div style="position:relative;width:40px;height:40px">`+
		`<div style="background:lime;width:40px;height:40px"></div>`+
		`<div style="position:absolute;left:0;top:0;width:40px;height:40px;background:red;z-index:-1"></div>`+
		`</div>`+
		`<div style="position:relative;width:40px;height:40px">`+
		`<div style="position:absolute;left:0;top:0;width:40px;height:40px;background:blue;z-index:2"></div>`+
		`<div style="position:absolute;left:0;top:0;width:40px;height:40px;background:red;z-index:1"></div>`+
		`</div>`)
	if got := r.Image.RGBAAt(10, 10); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("expected negative z-index behind normal flow (lime), got %v", got)
	}
	if got := r.Image.RGBAAt(10, 50); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("expected higher positive z-index on top (blue), got %v", got)
	}
}
