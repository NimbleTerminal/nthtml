package document

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"os"
	"strings"
	"testing"
)

func render(t *testing.T, s string) *Result {
	t.Helper()
	r, e := Render(context.Background(), strings.NewReader(s), Options{Width: 200})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestGoldenDemo(t *testing.T) {
	src, e := os.ReadFile("../examples/demo/sample.html")
	if e != nil {
		t.Fatal(e)
	}
	r, e := Render(context.Background(), bytes.NewReader(src), Options{Width: 800})
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.Open("testdata/demo.png")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	gold, e := png.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	if r.Image.Bounds() != gold.Bounds() {
		t.Fatalf("bounds %v != %v", r.Image.Bounds(), gold.Bounds())
	}
	maxDelta, changed := 0, 0
	for y := 0; y < gold.Bounds().Dy(); y++ {
		for x := 0; x < gold.Bounds().Dx(); x++ {
			a, b, c, d := gold.At(x, y).RGBA()
			aa, bb, cc, dd := r.Image.At(x, y).RGBA()
			if a != aa || b != bb || c != cc || d != dd {
				changed++
				for _, pair := range [][2]uint32{{a, aa}, {b, bb}, {c, cc}, {d, dd}} {
					delta := int(pair[0]) - int(pair[1])
					if delta < 0 {
						delta = -delta
					}
					maxDelta = max(maxDelta, delta/257)
				}
			}
		}
	}
	t.Logf("golden: %d differing pixels, maximum channel delta %d", changed, maxDelta)
	if maxDelta > 2 || changed > gold.Bounds().Dx()*gold.Bounds().Dy()/100 {
		t.Fatalf("golden mismatch: %d pixels, delta %d", changed, maxDelta)
	}
}
func TestLimits(t *testing.T) {
	for _, s := range []string{strings.Repeat("x", (1<<20)+1), strings.Repeat("<div>", 65), strings.Repeat("<br>", 10001), "<style>" + strings.Repeat("p{color:red}", 257) + "</style>", "<div style='height:16384px;padding:10px'>x</div>"} {
		_, e := Render(context.Background(), strings.NewReader(s), Options{})
		if !errors.Is(e, ErrLimit) {
			t.Fatalf("expected resource limit, got %v", e)
		}
	}
	_, e := Render(context.Background(), strings.NewReader("<div style='height:10000px'></div>"), Options{Width: 4096})
	if !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
}
func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := Render(ctx, strings.NewReader("hello"), Options{})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestCascadeAndHiddenContent(t *testing.T) {
	r := render(t, "<style>p{color:red}.x{color:blue}#a{color:green}</style><script>bad()</script><p class='x' id='a'>safe</p><p style='color:#ff0000'>inline</p>")
	found := 0
	for _, b := range r.Boxes {
		if strings.Contains(b.Text, "bad") {
			t.Fatal("script rendered")
		}
		if b.Text == "safe" {
			found++
			if b.Color.G != 128 || b.Color.R != 0 {
				t.Fatal(b.Color)
			}
		}
		if b.Text == "inline" {
			found++
			if b.Color.R != 255 {
				t.Fatal(b.Color)
			}
		}
	}
	if found != 2 {
		t.Fatal(found)
	}
}
func TestWrappingAndTable(t *testing.T) {
	r := render(t, "<table><tr><td>left</td><td>right</td></tr></table><p>"+strings.Repeat("W", 80)+"</p>")
	var left, right int
	for _, b := range r.Boxes {
		if b.Text == "left" {
			left = b.Bounds.Min.X
		}
		if b.Text == "right" {
			right = b.Bounds.Min.X
		}
		if b.Text != "" && b.Bounds.Max.X > 200 {
			t.Fatal("text outside width", b)
		}
	}
	if right <= left {
		t.Fatal("table cells overlap")
	}
}
func TestImageBudget(t *testing.T) {
	if _, e := DecodeImage(strings.NewReader(strings.Repeat("x", (1<<20)+1))); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
	if _, e := DecodeDataURL("https://example.com/image.png"); e == nil {
		t.Fatal("accepted network URL")
	}
}
func FuzzRender(f *testing.F) {
	f.Add("<p>Hello <b>world</b></p>")
	f.Add(`<a href="#target">Jump</a><p id="target">Search <b>across</b> inline text</p>`)
	f.Add("<table><tr><td>x")
	f.Add(`<svg viewBox="0 0 100 50"><path d="M0 0L100 50" stroke="blue"/></svg>`)
	f.Add("<pre class=mermaid>flowchart LR\n A --> B</pre>")
	f.Add(`<table style="border-collapse:collapse"><tr><td style="border:3px solid red">long<span>word</span><br>line</td><td style="border:1px solid blue">two</td></tr></table>`)
	f.Add(`<table><thead><tr><th>Header</th><th>Qty</th></tr></thead><tr><td style="padding:4px;border:1px solid blue"><table><tr><td>Nested</td></tr></table></td><td style="vertical-align:bottom"><a href="https://example.com">2</a></td></tr></table>`)
	f.Add("<ul><li><a href=\"https://example.com\">linked item</a><pre>\n  code\n\tmore</pre></li></ul>")
	f.Add(spanExample)
	f.Add(`<style>table.report > tbody td.amount{color:red!important} .a .b{white-space:pre-line}</style><table class="report"><tbody><tr><td colspan="2" rowspan="0" class="amount">merged</td></tr><tr><td>x</td></tr></tbody></table>`)
	f.Add("<p>Before <a href='https://example.com' style='white-space:nowrap'>one two three</a> after</p><pre style='white-space:pre-line'>a\n\n b</pre>")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			t.Skip()
		}
		d, err := Parse(context.Background(), strings.NewReader(s), Limits{Bytes: 4096, Nodes: 100, Depth: 16, Rules: 16, Height: 1000, Pixels: 200000})
		if err != nil {
			return
		}
		if r, e := d.Render(context.Background(), Options{Width: 200}); e == nil {
			index := r.TextIndex()
			_ = index.Search("inline text")
			_, _ = r.Fragment("#target")
		}
		if r, e := d.TTYLayout(context.Background(), 20); e == nil {
			index := r.TextIndex()
			_ = r.ANSIHighlights(index.Search("text"))
			_, _ = r.Fragment("#target")
		}
	})
}
