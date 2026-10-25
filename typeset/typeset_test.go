package typeset

import (
	"context"
	"strings"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

func TestBuildAndRasterWithNTTypeset(t *testing.T) {
	const src = `<html><body>
		<h1>Typeset Bridge</h1>
		<p>Hello <b>bold</b>, <i>italic</i>, <code>mono</code>, <s>strike</s>, and <a href="https://nimbleterminal.dev">link</a>.</p>
		<blockquote><p>A quoted observation.</p></blockquote>
		<ol start="3"><li>Third item</li><li>Fourth item</li></ol>
		<pre><code class="language-go">package main
func main() {}</code></pre>
		<hr>
		<table><thead><tr><th>Name</th><th>Value</th></tr></thead><tbody><tr><td>Alpha</td><td>42</td></tr></tbody></table>
	</body></html>`

	ctx := context.Background()
	doc, err := document.Parse(ctx, strings.NewReader(src), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	tdoc, err := Build(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tdoc.Blocks) < 7 {
		t.Fatalf("expected at least 7 blocks, got %d", len(tdoc.Blocks))
	}
	if tdoc.Blocks[0].Kind != nttypeset.KindHeading || tdoc.Blocks[0].Size != 50 {
		t.Fatalf("expected H1 heading block, got %+v", tdoc.Blocks[0])
	}

	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	img, lay, err := Raster(ctx, eng, doc, 600, nttypeset.View{Width: 600, Height: 400, Scale: 1}, nttypeset.RasterOptions{Palette: nttypeset.LightPalette()}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 400 {
		t.Fatalf("unexpected raster bounds %v", img.Bounds())
	}
	links := lay.Links(nttypeset.View{Width: 600, Height: 400, Scale: 1})
	if len(links) == 0 || links[0].URL != "https://nimbleterminal.dev" {
		t.Fatalf("expected link hit area from nttypeset layout, got %+v", links)
	}
}

func TestFromResultBoxes(t *testing.T) {
	ctx := context.Background()
	res, err := document.Render(ctx, strings.NewReader(`<h1>Heading</h1><p>Paragraph with <a href="#sec">anchor</a>.</p><hr>`), document.Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	tdoc := FromResult(res)
	if len(tdoc.Blocks) < 3 {
		t.Fatalf("expected heading, paragraph, and rule blocks, got %d", len(tdoc.Blocks))
	}
	if tdoc.Blocks[0].Kind != nttypeset.KindHeading {
		t.Fatalf("expected first block to be heading, got %v", tdoc.Blocks[0].Kind)
	}
	if tdoc.Blocks[len(tdoc.Blocks)-1].Kind != nttypeset.KindRule {
		t.Fatalf("expected last block to be rule, got %v", tdoc.Blocks[len(tdoc.Blocks)-1].Kind)
	}
}
