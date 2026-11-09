// Copyright (c) 2026 Neomantra Corp. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0-only WITH LicenseRef-NimbleTerminal-Linking-Exception OR LicenseRef-Neomantra-Commercial

package document

import (
	"context"
	"image/color"
	"strconv"
	"strings"
	"testing"
)

func renderScheme(t *testing.T, src, scheme string) *Result {
	t.Helper()
	r, err := Render(context.Background(), strings.NewReader(src), Options{Width: 200, ColorScheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func pixel(t *testing.T, r *Result, x, y int) color.RGBA {
	t.Helper()
	img, err := r.Paint(context.Background(), r.Bounds(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return img.RGBAAt(x, y)
}

func boxFor(t *testing.T, r *Result, tag string) Box {
	t.Helper()
	for _, b := range r.Boxes {
		if b.Tag == tag {
			return b
		}
	}
	t.Fatalf("no %s box", tag)
	return Box{}
}

// A nested rule inside a declaration block (CSS Nesting) applies to the
// nested selector, and the declarations after it still belong to the parent:
// design tokens declared after a nested .dark { } block must survive.
func TestNestedRulesKeepSiblingDeclarations(t *testing.T) {
	r := render(t, `<style>
		:root { --a: #ff0000; .x { color: #0000ff; } --b: #00ff00; }
		body { margin: 0; background: var(--b); }
		.card { color: #000000; & .title { color: var(--a); } > p { color: #0000ff; } @media (min-width: 100px) { font-weight: bold; } }
	</style><body><div class="card"><span class="title">T</span><p>P</p></div><span class="x">x</span></body>`)
	if got := pixel(t, r, 190, r.Bounds().Dy()-1); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("token after nested block lost: background %v", got)
	}
	var title, p, x Box
	for _, b := range r.Boxes {
		switch {
		case b.Text == "T":
			title = b
		case b.Text == "P":
			p = b
		case b.Text == "x":
			x = b
		}
	}
	if title.Color != (color.RGBA{255, 0, 0, 255}) || !title.Bold {
		t.Fatalf("& nesting or nested @media not applied: %+v", title)
	}
	if p.Color != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("relative > nesting not applied: %+v", p)
	}
	if x.Color != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("rule nested in :root not applied: %+v", x)
	}
}

// Logical sizing, border and inset properties map onto their physical
// equivalents for horizontal-tb left-to-right writing.
func TestLogicalPropertiesAlias(t *testing.T) {
	r := render(t, `<style>body{margin:0;background:#fff}
		div { inline-size: 50px; block-size: 10px; border-block-end: 2px solid #ff0000; border-inline: 1px solid #0000ff; max-inline-size: 40px; min-block-size: 20px }
		span { position: absolute; inset-inline-start: 30px; inset-block: 5px 7px; display:block; width: 10px; height: 10px; background:#00ff00 }
	</style><body><div></div><span></span></body>`)
	d := boxFor(t, r, "div")
	if d.Bounds.Dx() != 42 || d.Bounds.Dy() != 22 {
		t.Fatalf("div %v, want 42x22 (40 + 2 inline borders, 20 + bottom border)", d.Bounds)
	}
	if got := pixel(t, r, 20, 21); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("border-block-end not painted at the bottom: %v", got)
	}
	if got := pixel(t, r, 0, 10); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("border-inline not painted at the left: %v", got)
	}
	s := boxFor(t, r, "span")
	if s.Bounds.Min.X != 30 || s.Bounds.Min.Y != 5 {
		t.Fatalf("inset-inline-start / inset-block: %v", s.Bounds)
	}
}

// light-dark() follows the rendering preference, and an element's own
// color-scheme pins it for its subtree.
func TestLightDarkFollowsScheme(t *testing.T) {
	src := `<style>:root{ --c: light-dark(#ffffff, #000000); color-scheme: light dark } body{margin:0;background:var(--c)}
		.pin { color-scheme: light; background: var(--c); padding-top: 10px; height: 10px } .pin div { background: light-dark(#ff0000, #0000ff); height: 10px }
		p { color: light-dark(#ff0000, #0000ff); margin:0 }</style><body><div class="pin"><div></div></div><p>t</p></body>`
	dark := renderScheme(t, src, "dark")
	if got := pixel(t, dark, 190, 30); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("dark scheme background %v", got)
	}
	if got := pixel(t, dark, 190, 2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("color-scheme: light did not pin the subtree: %v", got)
	}
	if got := pixel(t, dark, 190, 12); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("pinned scheme not inherited by the child: %v", got)
	}
	var text Box
	for _, b := range dark.Boxes {
		if b.Text == "t" {
			text = b
		}
	}
	if text.Color != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("literal light-dark() colour dropped: %+v", text.Color)
	}
	light := renderScheme(t, src, "light")
	if got := pixel(t, light, 190, 30); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("light scheme background %v", got)
	}
}

// A positive border width under a pixel is a hairline, never nothing.
func TestHairlineBorderPaints(t *testing.T) {
	r := render(t, `<style>body{margin:0;background:#fff} div{height:10px;border-bottom:0.0125em solid #ff0000}</style><body><div></div></body>`)
	if d := boxFor(t, r, "div"); d.Bounds.Dy() != 11 {
		t.Fatalf("div %v, want 11 rows", d.Bounds)
	}
	if got := pixel(t, r, 100, 10); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("hairline missing: %v", got)
	}
}

// justify-self and justify-items place a grid item at its own width in its
// slot instead of stretching it.
func TestGridJustifySelf(t *testing.T) {
	r := render(t, `<style>body{margin:0} .g{display:grid;grid-template-columns:100px 100px;justify-items:start}
		.a{width:20px;height:5px;justify-self:end} .b{width:20px;height:5px;justify-self:center} .c{width:20px;height:5px}</style>
		<body><div class="g"><i class="a"></i><i class="b"></i><i class="c"></i></div></body>`)
	var xs []int
	for _, b := range r.Boxes {
		if b.Tag == "i" {
			xs = append(xs, b.Bounds.Min.X)
		}
	}
	if len(xs) != 3 || xs[0] != 80 || xs[1] != 140 || xs[2] != 0 {
		t.Fatalf("item x positions %v, want [80 140 0]", xs)
	}
}

// summary { list-style: none } removes the disclosure triangle.
func TestSummaryMarkerHiddenByListStyle(t *testing.T) {
	r := render(t, `<details><summary style="list-style:none">Explore</summary>body</details><details><summary>Plain</summary>x</details>`)
	markers := 0
	for _, b := range r.Boxes {
		if strings.Contains(b.Text, "▸") {
			markers++
		}
	}
	if markers != 1 {
		t.Fatalf("%d markers, want only the plain summary's", markers)
	}
}

// A flexible track with a definite minimum keeps that minimum as its base size;
// a wide item does not widen it before the fr shares are dealt.
func TestGridDefiniteMinimumIgnoresContent(t *testing.T) {
	r := render(t, `<style>body{margin:0} .g{display:grid;grid-template-columns:minmax(0,1fr) minmax(20px,1fr)} img{display:block}</style>
		<body><div class="g"><p>a</p><img width="180" height="10"></div></body>`)
	if img := boxFor(t, r, "img"); img.Bounds.Min.X != 100 {
		t.Fatalf("second track starts at %d, want 100 (equal fr shares)", img.Bounds.Min.X)
	}
}

// A block-level start tag closes an open paragraph, as browsers do for the
// common unclosed <p>: the heading is a sibling, not padded inside it.
func TestBlockStartTagClosesOpenParagraph(t *testing.T) {
	r := render(t, `<style>body{margin:0} p{padding-left:50px;margin:0}</style><body><p>a<h2>b</h2><p>c<ul><li>d</li></ul><td-like><p>e</body>`)
	for _, b := range r.Boxes {
		switch b.Text {
		case "b", "d":
			if b.Bounds.Min.X >= 50 {
				t.Fatalf("%q still inside the paragraph: %v", b.Text, b.Bounds)
			}
		case "a", "c", "e":
			if b.Bounds.Min.X < 50 {
				t.Fatalf("%q lost its paragraph: %v", b.Text, b.Bounds)
			}
		}
	}
}

// A utility reset that declares many custom properties on every element stays
// within the declaration budget: identical inherited values share the parent's
// map and the budget scales with the node limit.
func TestUniversalCustomPropertiesFitTheDeclarationBudget(t *testing.T) {
	var css strings.Builder
	css.WriteString("*, ::before, ::after {")
	for i := 0; i < 100; i++ {
		css.WriteString("--tw-" + strings.Repeat("x", i%7+1) + strconv.Itoa(i) + ": 0;")
	}
	css.WriteString("}")
	var body strings.Builder
	for i := 0; i < 2000; i++ {
		body.WriteString("<span>s</span>")
	}
	r, err := Render(context.Background(), strings.NewReader("<style>"+css.String()+"</style><body>"+body.String()+"</body>"), Options{Width: 400})
	if err != nil {
		t.Fatalf("2000 elements under a 100-property reset: %v", err)
	}
	if len(r.Boxes) < 2000 {
		t.Fatalf("%d boxes", len(r.Boxes))
	}
}

// CSS fill and stroke on an inline SVG reach its root: a Tailwind fill-white
// icon declared with fill="none" paints white, and fill: currentColor takes the
// element's colour.
func TestCSSFillReachesInlineSVG(t *testing.T) {
	r := render(t, `<style>body{margin:0;background:#000} .w{fill:#ff0000} .c{fill:currentColor;color:#00ff00}</style>
		<body><div><svg class="w" width="20" height="20" fill="none" viewBox="0 0 20 20"><rect width="20" height="20"/></svg><svg class="c" width="20" height="20" viewBox="0 0 20 20"><rect width="20" height="20" fill="#123456"/></svg><svg class="c" width="20" height="20" viewBox="0 0 20 20"><rect width="20" height="20"/></svg></div></body>`)
	img, err := r.Paint(context.Background(), r.Bounds(), 1)
	if err != nil {
		t.Fatal(err)
	}
	var xs []int
	for _, b := range r.Boxes {
		if b.Tag == "svg" && (len(xs) == 0 || xs[len(xs)-1] != b.Bounds.Min.X) { // element and image boxes share a position
			xs = append(xs, b.Bounds.Min.X)
		}
	}
	if len(xs) < 3 {
		t.Fatalf("svg boxes %v", xs)
	}
	if got := img.RGBAAt(xs[0]+10, 10); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("CSS fill over fill=none: %v", got)
	}
	if got := img.RGBAAt(xs[1]+10, 10); got != (color.RGBA{0x12, 0x34, 0x56, 255}) {
		t.Fatalf("a descendant's own fill must survive: %v", got)
	}
	if got := img.RGBAAt(xs[2]+10, 10); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("fill: currentColor: %v", got)
	}
}

// An SVG that starts with the XML declaration and a plain DOCTYPE, as
// exported files do, renders; a DOCTYPE with an internal subset or another
// processing instruction is still refused.
func TestSVGPrologIsTolerated(t *testing.T) {
	prolog := `<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">\n`
	body := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="#ff0000"/></svg>`
	out, _, err := rewriteSVG(context.Background(), prolog+body)
	if err != nil || strings.Contains(out, "<?xml") || strings.Contains(out, "DOCTYPE") || !strings.Contains(out, "<rect") {
		t.Fatalf("prolog: %v %q", err, out)
	}
	for _, bad := range []string{
		`<!DOCTYPE svg [<!ENTITY a "aaaa">]>` + body,
		`<?xml-stylesheet href="x.css" type="text/css"?>` + body,
	} {
		if _, _, err := rewriteSVG(context.Background(), bad); err == nil {
			t.Fatalf("accepted %q", bad[:30])
		}
	}
}

// A modular type scale built from nested custom-property calc() chains
// resolves inside clamp(): --s4 is four calc() levels deep once substituted.
func TestNestedCalcScaleInsideClamp(t *testing.T) {
	r := render(t, `<style>:root { --ratio: 1.25; --s0: 1rem; --s1: calc(var(--s0) * var(--ratio)); --s2: calc(var(--s1) * var(--ratio)); --s3: calc(var(--s2) * var(--ratio)); --s4: calc(var(--s3) * var(--ratio)); --s5: calc(var(--s4) * var(--ratio)) }
		h1 { font-size: clamp(var(--s2), 50vw, var(--s5)); margin: 0 } h2 { font-size: var(--s4); margin: 0 }</style><body><h1>A</h1><h2>B</h2></body>`)
	sizes := map[string]int{}
	for _, b := range r.Boxes {
		if b.Text != "" {
			sizes[b.Text] = b.FontSize
		}
	}
	if sizes["A"] != 48 || sizes["B"] != 39 {
		t.Fatalf("font sizes %v, want A=48 (s5, 48.8px) and B=39 (s4)", sizes)
	}
}

// line-height: normal follows a font size the cascade sets, instead of keeping
// the body's absolute line as a floor: 7pt table text sits in a short line.
func TestNormalLineHeightFollowsCSSFontSize(t *testing.T) {
	r := render(t, `<style>body{margin:0} .small{font-size:9px} .big{font-size:30px}</style><body><p class="small">s</p><p class="big">b</p><p>n</p></body>`)
	heights := map[string]int{}
	for _, b := range r.Boxes {
		if b.Text != "" {
			heights[b.Text] = b.Bounds.Dy()
		}
	}
	if heights["s"] != 12 || heights["b"] != 41 || heights["n"] != 22 {
		t.Fatalf("line boxes %v, want s=12 (9px), b=41 (30px), n=22 (default 16px)", heights)
	}
}

// -webkit-line-clamp cuts a block after N lines, and a positioned element
// translated fully off-canvas is not painted; an unpositioned translate is.
func TestLineClampAndOffCanvasTransform(t *testing.T) {
	r := render(t, `<style>body{margin:0;font-size:16px} .c{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;overflow:hidden;width:60px}
		.drawer{position:fixed;top:0;right:0;width:100px;transform:translate(100%)} .shift{transform:translateX(100%)}</style>
		<body><p class="c">one two three four five six seven eight nine ten</p><nav class="drawer">menu</nav><p class="shift">shown</p></body>`)
	var clamp, drawer, shown int
	for _, b := range r.Boxes {
		switch b.Text {
		case "one":
			clamp = 1
		case "menu":
			drawer++
		case "shown":
			shown++
		}
		if b.Tag == "p" && b.Bounds.Dx() == 60 && b.Bounds.Dy() != 44 {
			t.Fatalf("clamped block %v, want two 22px lines", b.Bounds)
		}
	}
	if clamp == 0 || drawer != 0 || shown == 0 {
		t.Fatalf("clamp=%d drawer=%d shown=%d", clamp, drawer, shown)
	}
}

// A mask-image icon paints no background box: the renderer cannot shape the
// colour by the mask, and a blank beats a solid square.
func TestMaskedBackgroundIsNotPainted(t *testing.T) {
	r := render(t, `<style>body{margin:0;background:#fff} span{display:inline-block;width:20px;height:20px;background-color:#000}
		.m{mask-image:url(/icon.svg)} .w{-webkit-mask-image:url(/icon.svg)}</style><body><span class="m"></span><span class="w"></span><span></span></body>`)
	img, err := r.Paint(context.Background(), r.Bounds(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(10, 10); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("masked span painted %v", got)
	}
	if got := img.RGBAAt(30, 10); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("-webkit-masked span painted %v", got)
	}
	if got := img.RGBAAt(50, 10); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("plain span lost its background: %v", got)
	}
}

// The font shorthand takes a math-function size, a unitless line height and
// any weight from 1 to 1000, as variable fonts use (fallthrough.fm's
// headings are `font: 560 clamp(2.8rem, 6vw, 5.75rem)/.91 serif`).
func TestFontShorthandWithClampAndVariableWeight(t *testing.T) {
	r, err := Render(context.Background(), strings.NewReader(`<style>h1{font: 560 clamp(2.8rem, 6vw, 5.75rem)/.91 Georgia, serif; margin:0} p{font: 300 clamp(1.06rem, 1.35vw, 1.25rem)/1.55 serif; margin:0}</style><body><h1>H</h1><p>P</p></body>`), Options{Width: 1280})
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[string]int{}
	bold := map[string]bool{}
	for _, b := range r.Boxes {
		if b.Text != "" {
			sizes[b.Text], bold[b.Text] = b.FontSize, b.Bold
		}
	}
	if sizes["H"] != 76 || sizes["P"] != 17 || bold["H"] || bold["P"] {
		t.Fatalf("sizes %v bold %v, want H=76 (6vw) and P=17 (1.35vw), both under the bold threshold", sizes, bold)
	}
}
