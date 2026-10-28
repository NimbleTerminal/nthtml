package document

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
)

func TestEmbeddedSVG(t *testing.T) {
	source := `<p>Before</p><svg viewBox="0 0 100 50" aria-label="Red chart"><title>Chart</title><rect width="100" height="50" fill="#ff0000"/></svg><p>After</p>`
	r := renderCSS(t, source)
	if len(r.Warnings) > 0 {
		t.Fatal(r.Warnings)
	}
	var found bool
	for _, b := range r.Boxes {
		if b.Tag == "svg" && b.Bounds.Dy() == 200 {
			found = true
			if c := r.Image.RGBAAt(b.Bounds.Min.X+10, b.Bounds.Min.Y+10); c.R != 255 || c.G != 0 {
				t.Fatal(c)
			}
		}
	}
	if !found {
		t.Fatal("SVG did not preserve its viewBox aspect ratio")
	}
	tty, err := r.TTY(context.Background(), 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tty, "[SVG: Red chart]") || strings.Contains(tty, "rect") {
		t.Fatal(tty)
	}
}
func TestMermaidBlocks(t *testing.T) {
	samples := []string{
		"flowchart LR\n A[HTML] --> B[SVG]\n B --> C[Pixels]",
		"sequenceDiagram\n Alice->>Bob: Hello\n Bob-->>Alice: Hi",
		"pie title Traffic\n \"Terminal\" : 60\n \"Browser\" : 40",
		"stateDiagram-v2\n [*] --> Ready\n Ready --> Done",
		"classDiagram\n class Renderer\n class Document\n Document --> Renderer",
	}
	for _, source := range samples {
		for _, markup := range []string{"<pre class=mermaid>" + source + "</pre>", "<pre><code class=language-mermaid>" + source + "</code></pre>", "<div class=mermaid>" + source + "</div>"} {
			r := renderCSS(t, markup)
			if len(r.Warnings) > 0 {
				t.Fatalf("%s: %v", source, r.Warnings)
			}
			if len(boxesWithTag(r, "mermaid")) != 1 {
				t.Fatal("missing diagram")
			}
			tty, e := r.TTY(context.Background(), 80)
			if e != nil || !strings.Contains(tty, strings.Split(source, "\n")[0]) {
				t.Fatal(tty, e)
			}
		}
	}
}
func TestSVGIsolationAndFallback(t *testing.T) {
	r := renderCSS(t, `<svg><style>p{display:none}</style><script>bad()</script></svg><p>Visible</p>`)
	if len(r.Warnings) != 2 {
		t.Fatal(r.Warnings)
	}
	seen := false
	for _, b := range r.Boxes {
		if b.Text == "Visible" {
			seen = true
		}
		if b.Text == "bad()" {
			t.Fatal("SVG script leaked")
		}
	}
	if !seen {
		t.Fatal("SVG CSS leaked into HTML")
	}
	for _, src := range []string{
		`<svg><use href="#loop"/></svg>`,
		`<svg><image href="https://example.com/x.png"/></svg>`,
		`<svg><foreignObject>HTML</foreignObject></svg>`,
		`<svg><defs><marker id="x"><path d="M0 0L10 10" marker-end="url(#x)"/></marker></defs></svg>`,
	} {
		if len(renderCSS(t, src).Warnings) != 1 {
			t.Fatal(src)
		}
	}
	r = renderCSS(t, "<pre class=mermaid>not a diagram</pre>")
	if len(r.Warnings) != 1 {
		t.Fatal("invalid Mermaid should degrade")
	}
}
func TestDiagramLimitsAndCancellation(t *testing.T) {
	for _, src := range []string{
		"<svg><path d='M1e99 0L1 1'/></svg>",
		"<svg><path style='stroke-width:1e99' d='M0 0L1 1'/></svg>",
		"<pre class=mermaid>flowchart LR;" + strings.Repeat("A-->B;", 129) + "</pre>",
		"<pre class=mermaid>flowchart LR\n" + strings.Repeat("A --> B\n", 65) + "</pre>",
		strings.Repeat("<svg viewBox='0 0 10 1'/>", 33),
	} {
		_, e := Render(context.Background(), strings.NewReader(src), Options{Width: 400})
		if !errors.Is(e, ErrLimit) {
			t.Fatalf("want limit, got %v", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := (DefaultDiagramRenderer{}).RenderDiagram(ctx, "svg", "<svg/>", 100, 100)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

type testDiagramRenderer struct{ calls int }

func (r *testDiagramRenderer) RenderDiagram(ctx context.Context, kind, source string, w, h int) (image.Image, error) {
	r.calls++
	return image.NewRGBA(image.Rect(0, 0, min(20, w), min(10, h))), nil
}
func TestDiagramRendererAndDisabledMode(t *testing.T) {
	renderer := &testDiagramRenderer{}
	for _, disabled := range []bool{true, false} {
		r, e := Render(context.Background(), strings.NewReader(`<svg aria-label="Custom"/>`), Options{Width: 400, Diagrams: renderer, DisableDiagrams: disabled})
		if e != nil {
			t.Fatal(e)
		}
		if disabled && renderer.calls != 0 {
			t.Fatal("disabled renderer called")
		}
		if !disabled && renderer.calls != 1 {
			t.Fatal("custom renderer unused")
		}
		if len(r.Warnings) > 0 {
			t.Fatal(r.Warnings)
		}
	}
}

func TestSVGInLinkAndExplicitSize(t *testing.T) {
	r := renderCSS(t, `<p>Before <a href="https://example.com"><svg width="120" height="60" viewBox="0 0 20 10" aria-label="Linked diagram"><rect width="20" height="10" fill="blue"/></svg></a> After</p>`)
	if len(r.Warnings) > 0 || len(r.Links) != 1 {
		t.Fatal(r.Warnings, r.Links)
	}
	if r.Links[0].Text != "Linked diagram" {
		t.Fatal(r.Links)
	}
	if r.Links[0].Regions[0].Dx() != 120 || r.Links[0].Regions[0].Dy() != 60 {
		t.Fatal(r.Links)
	}
	tty, e := r.TTYLayout(context.Background(), 80)
	if e != nil {
		t.Fatal(e)
	}
	if len(tty.Links) != 1 || len(tty.Spans) == 0 {
		t.Fatal("TTY diagram label lost its anchor")
	}
}
func TestSVGParseBudgets(t *testing.T) {
	for _, src := range []string{
		"<svg>" + strings.Repeat("<g>", 65) + strings.Repeat("</g>", 65) + "</svg>",
		"<svg>" + strings.Repeat("<rect/>", 10001) + "</svg>",
	} {
		_, e := Parse(context.Background(), strings.NewReader(src), Limits{})
		if !errors.Is(e, ErrLimit) {
			t.Fatal(e)
		}
	}
}

func TestSVGUnsupportedSubtrees(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		red, blue     uint8
		warnings      string
	}{
		{"foreignObject", `<foreignObject><div>x</div><rect width="10" height="10" fill="green"/></foreignObject><rect width="10" height="10" fill="red"/>`, 255, 0, "foreignObject,div"},
		{"use", `<use/><rect width="10" height="10" fill="blue"/>`, 0, 255, "use"},
		{"metadata", `<metadata>m</metadata>`, 255, 255, "metadata"},
		{"repeated", `<image/><image/><rect width="10" height="10" fill="red"/>`, 255, 0, "image"},
		{"nested drawable", `<foreignObject><rect width="10" height="10" fill="red"/></foreignObject>`, 255, 255, "foreignObject"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := renderCSS(t, `<svg width="10" height="10">`+tc.content+`</svg>`)
			boxes := boxesWithTag(r, "svg")
			if len(boxes) == 0 || boxes[0].Bounds.Size() != image.Pt(10, 10) {
				t.Fatalf("expected 10×10 SVG box: %v; warnings: %v", boxes, r.Warnings)
			}
			b := boxes[len(boxes)-1].Bounds
			if c := r.Image.RGBAAt(b.Min.X+5, b.Min.Y+5); c.R != tc.red || c.B != tc.blue {
				t.Fatalf("unexpected pixel: %v", c)
			}
			for _, box := range r.Boxes {
				if box.Text != "" {
					t.Fatalf("SVG text leaked or fell back: %q", box.Text)
				}
			}
			names := strings.Split(tc.warnings, ",")
			if len(r.Warnings) != len(names) {
				t.Fatal(r.Warnings)
			}
			for i, name := range names {
				if r.Warnings[i] != "svg: skipped unsupported element "+name {
					t.Fatal(r.Warnings)
				}
			}
		})
	}
}

func TestSVGGoogleAppsIcon(t *testing.T) {
	r := renderCSS(t, `<svg class="gb_H" aria-hidden="true" focusable="false" viewbox="0 0 24 24"><path d="M6,8c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2zM12,20c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2zM6,20c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2zM6,14c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2zM12,14c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2zM16,6c0,1.1 0.9,2 2,2s2,-0.9 2,-2 -0.9,-2 -2,-2 -2,0.9 -2,2zM12,8c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2zM18,14c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2zM18,20c1.1,0 2,-0.9 2,-2s-0.9,-2 -2,-2 -2,0.9 -2,2 0.9,2 2,2z"></path><image src="https://ssl.gstatic.com/gb/images/bar/al-icon.png" alt="" height="24" width="24" style="border:none;display:none \9"></image></svg>`)
	boxes := boxesWithTag(r, "svg")
	if len(boxes) == 0 {
		t.Fatalf("missing SVG: %v", r.Warnings)
	}
	if len(r.Warnings) != 1 || r.Warnings[0] != "svg: skipped unsupported element image" {
		t.Fatal(r.Warnings)
	}
	b := boxes[len(boxes)-1].Bounds
	// Locate the ink independently of the renderer's intrinsic sizing fallback
	// for the lowercase viewbox attribute in the original Google markup.
	var ink image.Rectangle
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := r.Image.RGBAAt(x, y)
			if c.R != 255 || c.G != 255 || c.B != 255 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if ink.Empty() {
		t.Fatal("SVG has no visible dots")
	}
	for y := 2; y <= 14; y += 3 {
		for x := 2; x <= 14; x += 3 {
			c := r.Image.RGBAAt(ink.Min.X+x*ink.Dx()/16, ink.Min.Y+y*ink.Dy()/16)
			white := c.R == 255 && c.G == 255 && c.B == 255
			dot := x%6 == 2 && y%6 == 2
			if white == dot {
				t.Fatalf("grid point (%d,%d): %v, want dot=%v", x, y, c, dot)
			}
		}
	}
	for _, box := range r.Boxes {
		if strings.Contains(box.Text, "SVG:") {
			t.Fatal(box.Text)
		}
	}
}

func TestSVGSkippedElementsSafety(t *testing.T) {
	for _, content := range []string{
		`<image href="https://example.test/a.png"/>`,
		`<use href="#a"/><rect width="10" height="10" fill="blue"/>`,
		`<unknown><image xlink:href="#a"/></unknown>`,
		`<unknown onclick="bad()"/>`,
		`<unknown fill="url(https://example.test/a)"/>`,
		`<unknown><defs><rect fill="url(#a)"/></defs></unknown>`,
	} {
		r := renderCSS(t, `<svg width="10" height="10">`+content+`</svg>`)
		var text strings.Builder
		for _, box := range r.Boxes {
			text.WriteString(box.Text)
		}
		if !strings.Contains(text.String(), "SVG:") || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "references are unsupported") && !strings.Contains(r.Warnings[0], "references inside definitions") {
			t.Fatalf("unsafe SVG accepted: %s; warnings %v", content, r.Warnings)
		}
	}
	for _, content := range []string{
		strings.Repeat(`<unknown/>`, 3000),
		strings.Repeat(`<unknown>`, 33) + strings.Repeat(`</unknown>`, 33),
	} {
		_, err := Render(context.Background(), strings.NewReader(`<svg>`+content+`</svg>`), Options{Width: 400})
		if !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "SVG complexity") {
			t.Fatalf("expected SVG complexity limit, got %v", err)
		}
	}
}

func TestSVGRewritePreservesSource(t *testing.T) {
	const start = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:custom="urn:test" custom:attr="a &amp; b">`
	const kept = `<g mystery='value'><rect width="10" height="10" custom:flag="yes"/></g>`
	got, skipped, err := rewriteSVG(context.Background(), start+`<unknown><rect/></unknown>`+kept+`<unknown/>`+`</svg>`)
	if err != nil || got != start+kept+`</svg>` || len(skipped) != 1 || skipped[0] != "unknown" {
		t.Fatalf("rewrite: %q, %v, %v", got, skipped, err)
	}
	for _, source := range []string{
		`<g><rect/></g>`,
		`<!DOCTYPE svg><svg/>`,
		`<svg><unknown><?test value?></unknown></svg>`,
		`<svg><unknown></svg>`,
	} {
		if _, _, err := rewriteSVG(context.Background(), source); err == nil {
			t.Fatalf("accepted invalid SVG: %s", source)
		}
	}
}
