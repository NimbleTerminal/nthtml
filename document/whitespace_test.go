package document

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestWhiteSpaceModes(t *testing.T) {
	for _, tc := range []struct{ mode, input, want string }{
		{"normal", " a  b\nc ", "a b c"},
		{"nowrap", " a  b\nc ", "a b c"},
		{"pre", " a  b\nc ", " a  b\nc "},
		{"pre-wrap", " a  b\nc ", " a  b\nc "},
		{"break-spaces", " a  b\nc ", " a  b\nc "},
		{"pre-line", " a  b\nc ", "a b\nc"},
	} {
		r := renderCSS(t, `<div style="white-space:`+tc.mode+`">`+tc.input+`</div>`)
		got, err := r.TTY(context.Background(), 80)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("TTY %s: %q want %q", tc.mode, got, tc.want)
		}
		// Every explicit newline in preserving modes must move raster text down.
		if tc.mode != "normal" && tc.mode != "nowrap" {
			boxes := boxesWithTag(r, "text")
			if boxes[len(boxes)-1].Bounds.Min.Y <= boxes[0].Bounds.Min.Y {
				t.Errorf("%s newline lost", tc.mode)
			}
		}
	}
}
func TestOverflowWrapping(t *testing.T) {
	for _, tc := range []struct {
		css  string
		wrap bool
	}{
		{"white-space:nowrap", false}, {"white-space:pre", false},
		{"overflow-wrap:normal", false}, {"overflow-wrap:break-word", true}, {"overflow-wrap:anywhere", true},
		{"white-space:nowrap;overflow-wrap:anywhere", false},
	} {
		source := `<div style="width:40px;` + tc.css + `">abcdefghijklmnop</div>`
		r := renderCSS(t, source)
		boxes := boxesWithTag(r, "text")
		wrapped := boxes[len(boxes)-1].Bounds.Min.Y > boxes[0].Bounds.Min.Y
		if wrapped != tc.wrap {
			t.Errorf("raster %s: wrapped=%v", tc.css, wrapped)
		}
		text, err := r.TTY(context.Background(), 4)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "\n") != tc.wrap {
			t.Errorf("TTY %s: %q", tc.css, text)
		}
	}
}

func TestInlineWhiteSpaceAndLinks(t *testing.T) {
	r := renderCSS(t, `<p>Before <a href="https://example.com" style="white-space:nowrap">one two three</a> after</p><div style="white-space:pre">  <span>kept  spaces</span>
next</div>`)
	layout, err := r.document.TTYLayout(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(layout.Text, "one two three") || !strings.Contains(layout.Text, "  kept  spaces\nnext") {
		t.Fatal(layout.Text)
	}
	if len(layout.Spans) == 0 {
		t.Fatal("link spans lost")
	}
}

func TestPreLineAndInvalidDeclarations(t *testing.T) {
	r := renderCSS(t, "<div style='white-space:pre-line;white-space:invalid'>a\n\n\nb</div>")
	text, err := r.TTY(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if text != "a\n\n\nb" {
		t.Fatalf("blank lines: %q", text)
	}
	r = renderCSS(t, "<div style='white-space:pre'>\nfirst</div>")
	if tagged(r, "text").Bounds.Min.Y == 0 {
		t.Fatal("CSS pre stripped a non-HTML-pre newline")
	}
}

func TestAnchoredPreInitialNewline(t *testing.T) {
	r := renderCSS(t, "<pre id='code'>\n  code</pre>")
	layout, err := r.document.TTYLayout(context.Background(), 40)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Text != "  code" {
		t.Fatalf("%q", layout.Text)
	}
	if _, ok := layout.Anchors["code"]; !ok {
		t.Fatal("anchor lost")
	}
}

func TestAdjacentNowrapSpanWrapping(t *testing.T) {
	r := renderCSS(t, `<div style="width:90px;white-space:normal"><span style="white-space:nowrap">alpha </span> <span style="white-space:nowrap">bravo </span> <span style="white-space:nowrap">charlie </span></div>`)
	boxes := boxesWithTag(r, "text")
	if len(boxes) < 3 || boxes[len(boxes)-1].Bounds.Min.Y <= boxes[0].Bounds.Min.Y {
		t.Fatalf("expected adjacent nowrap spans separated by normal spaces to wrap: %v", boxes)
	}
	text, err := r.TTY(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "\n") {
		t.Fatalf("expected TTY wrapping between nowrap spans: %q", text)
	}
}

func TestInlineBlockFlowAndShrinkToFit(t *testing.T) {
	r := renderCSS(t, `<div style="margin:0"><div style="display:inline-block;width:30px;height:16px;background:red"></div><div style="display:inline-block;width:15px;height:16px"></div><div style="display:inline-block;width:30px;height:16px;background:blue"></div></div>`)
	if got := r.Image.RGBAAt(10, 8); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("expected first inline-block red at (10,8), got %v", got)
	}
	if got := r.Image.RGBAAt(35, 8); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("expected middle inline-block gap white at (35,8), got %v", got)
	}
	if got := r.Image.RGBAAt(50, 8); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("expected third inline-block blue at (50,8), got %v", got)
	}
}

func TestInlineImageFlow(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 15))
	d, err := Parse(context.Background(), strings.NewReader(`<div style="margin:0"><img src="a"><img src="a"></div>`), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := d.Render(context.Background(), Options{Width: 200, Images: map[string]image.Image{"a": img}})
	if err != nil {
		t.Fatal(err)
	}
	imgs := boxesWithTag(r, "img")
	if len(imgs) != 2 {
		t.Fatalf("expected 2 inline images, got %d", len(imgs))
	}
	if imgs[0].Bounds != image.Rect(0, 0, 20, 15) || imgs[1].Bounds != image.Rect(20, 0, 40, 15) {
		t.Fatalf("unexpected inline image bounds: %v %v", imgs[0].Bounds, imgs[1].Bounds)
	}
}

func TestInlineBackgroundsMarginsAndZeroWidthSpace(t *testing.T) {
	r := renderCSS(t, `<div style="margin:0;font-size:20px;line-height:20px"><span>X</span><span style="margin-left:40px;background:lime">&#x200B; &#x200B;</span><span>X</span></div>`)
	texts := boxesWithTag(r, "text")
	if len(texts) < 3 {
		t.Fatalf("expected text boxes, got %v", texts)
	}
	if texts[1].Bounds.Min.X < texts[0].Bounds.Max.X+40 {
		t.Fatalf("expected inline margin-left of 40px: first=%v second=%v", texts[0].Bounds, texts[1].Bounds)
	}
	// Check that inline background painted across the space between zero-width spaces.
	if got := r.Image.RGBAAt(texts[1].Bounds.Max.X+2, 10); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("expected lime inline background across preserved space, got %v", got)
	}
}

func TestCSSWideKeywords(t *testing.T) {
	r := renderCSS(t, `<div style="color:green;white-space:pre"><div style="color:red;color:inherit;white-space:pre;white-space:initial;background:red;background:unset">A   B</div></div>`)
	txt := tagged(r, "text")
	if txt.Color != (color.RGBA{0, 128, 0, 255}) {
		t.Fatalf("expected color:inherit to resolve to green, got %v", txt.Color)
	}
	got, err := r.TTY(context.Background(), 80)
	if err != nil {
		t.Fatal(err)
	}
	if got != "A B" {
		t.Fatalf("expected white-space:initial to collapse spaces, got %q", got)
	}
	if bg := r.Image.RGBAAt(200, 5); bg != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("expected background:unset to clear background to transparent, got %v", bg)
	}
}
