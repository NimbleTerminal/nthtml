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
	if len(r.Warnings) != 1 {
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
