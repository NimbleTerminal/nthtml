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

func TestInlineLinkFollowedByPunctuation(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		html     string
		wantText string
		wantLink string
	}{
		{`<p>by <a href="x">W</a>.</p>`, "by W.", "W"},
		{`<p style="font-family:serif;font-size:15px">The weather turned by <a href="#wednesday">Wednesday</a>.</p>`, "The weather turned by Wednesday.", "Wednesday"},
		{`<p><a href="x">the end</a>.</p>`, "the end.", "the end"},
	} {
		r := renderCSS(t, tc.html)
		idx := r.TextIndex()
		if idx.Text != tc.wantText {
			t.Fatalf("%s: TextIndex().Text = %q, want %q", tc.html, idx.Text, tc.wantText)
		}
		if len(r.Links) != 1 {
			t.Fatalf("%s: Links = %d, want 1", tc.html, len(r.Links))
		}
		link := r.Links[0]
		if len(link.Regions) != 1 {
			t.Fatalf("%s: expected 1 unified link region on one line, got %v", tc.html, link.Regions)
		}
		var dotBox *Box
		for i := range r.Boxes {
			if r.Boxes[i].Tag == "text" && r.Boxes[i].Text == "." {
				dotBox = &r.Boxes[i]
				break
			}
		}
		if dotBox == nil {
			t.Fatalf("%s: missing '.' text box in %+v", tc.html, r.Boxes)
		}
		if link.Regions[0].Max.X != dotBox.Bounds.Min.X {
			t.Fatalf("%s: link region Max.X = %d, want '.' Min.X = %d", tc.html, link.Regions[0].Max.X, dotBox.Bounds.Min.X)
		}
		ttyRes, err := r.TTYLayout(ctx, 80)
		if err != nil {
			t.Fatal(err)
		}
		if ttyRes.Text != tc.wantText {
			t.Fatalf("%s: TTYLayout.Text = %q, want %q", tc.html, ttyRes.Text, tc.wantText)
		}
	}
}

func TestInlineBoundaryWhitespaceCollapsing(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		html string
		want string
	}{
		{`<p><em>a</em> <em>b</em></p>`, "a b"},
		{`<p><em>a </em> b</p>`, "a b"},
		{`<p><em>a </em> <em> b</em></p>`, "a b"},
		{`<p><a href="x">a </a> b</p>`, "a b"},
		{`<p><a href="x">a </a>b</p>`, "a b"},
		{`<p>by <a href="x"> W</a>.</p>`, "by W."},
		{`<p><em>a</em>, <code>b</code>)</p>`, "a, b)"},
	} {
		r := renderCSS(t, tc.html)
		if got := r.TextIndex().Text; got != tc.want {
			t.Fatalf("%s: TextIndex().Text = %q, want %q", tc.html, got, tc.want)
		}
		ttyRes, err := r.TTYLayout(ctx, 80)
		if err != nil {
			t.Fatal(err)
		}
		if ttyRes.Text != tc.want {
			t.Fatalf("%s: TTYLayout.Text = %q, want %q", tc.html, ttyRes.Text, tc.want)
		}
		blocks, err := r.Document().Blocks(ctx, Options{Width: 400})
		if err != nil {
			t.Fatal(err)
		}
		if len(blocks) != 1 {
			t.Fatalf("%s: len(Blocks) = %d, want 1", tc.html, len(blocks))
		}
		var sb strings.Builder
		for _, sp := range blocks[0].Spans {
			sb.WriteString(sp.Text)
		}
		if sb.String() != tc.want {
			t.Fatalf("%s: typeset spans = %q (%+v), want %q", tc.html, sb.String(), blocks[0].Spans, tc.want)
		}
	}

	// Verify trailing space inside <a> does not extend link region or underline in native or TTY.
	r := renderCSS(t, `<p><a href="x">a </a> b</p>`)
	var aBox, bBox *Box
	for i := range r.Boxes {
		if r.Boxes[i].Tag == "text" {
			switch r.Boxes[i].Text {
			case "a":
				aBox = &r.Boxes[i]
			case "b":
				bBox = &r.Boxes[i]
			}
		}
	}
	if aBox == nil || bBox == nil {
		t.Fatalf("missing 'a' or 'b' text boxes: %+v", r.Boxes)
	}
	if len(r.Links) != 1 || len(r.Links[0].Regions) != 1 {
		t.Fatalf("unexpected links: %+v", r.Links)
	}
	if r.Links[0].Regions[0].Max.X != aBox.Bounds.Max.X || aBox.Bounds.Max.X >= bBox.Bounds.Min.X {
		t.Fatalf("trailing space inside <a> leaked into link region: region=%v a=%v b=%v", r.Links[0].Regions[0], aBox.Bounds, bBox.Bounds)
	}
	ttyRes, err := r.TTYLayout(ctx, 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(ttyRes.Spans) != 1 || ttyRes.Text[ttyRes.Spans[0].Start:ttyRes.Spans[0].End] != "a" {
		t.Fatalf("expected TTY link span to cover only 'a', got spans=%+v text=%q", ttyRes.Spans, ttyRes.Text)
	}
}

func TestSerifInterWordSpacingConsistency(t *testing.T) {
	r := renderCSS(t, `<p style="font-family:serif;font-size:17px;width:600px">A light westerly. The slipway was repainted</p>`)
	var words []Box
	for _, b := range r.Boxes {
		if b.Tag == "text" && strings.TrimSpace(b.Text) != "" {
			words = append(words, b)
		}
	}
	if len(words) < 6 {
		t.Fatalf("expected at least 6 word boxes, got %d", len(words))
	}
	wantGap := words[1].Bounds.Min.X - words[0].Bounds.Max.X
	if wantGap <= 0 {
		t.Fatalf("invalid word gap %d", wantGap)
	}
	for i := 1; i < len(words)-1; i++ {
		if words[i+1].Bounds.Min.Y != words[i].Bounds.Min.Y {
			continue
		}
		gap := words[i+1].Bounds.Min.X - words[i].Bounds.Max.X
		if gap != wantGap {
			t.Fatalf("gap between %q and %q = %dpx, want %dpx", words[i].Text, words[i+1].Text, gap, wantGap)
		}
	}
}
