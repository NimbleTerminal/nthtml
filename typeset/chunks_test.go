package typeset

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

type chunkDiagrams struct {
	calls  int
	cancel context.CancelFunc
}

func (d *chunkDiagrams) RenderDiagram(ctx context.Context, kind, source string, w, h int) (image.Image, error) {
	d.calls++
	if d.cancel != nil {
		d.cancel()
		return nil, ctx.Err()
	}
	return image.NewRGBA(image.Rect(0, 0, 2, 2)), nil
}

func parseChunks(t *testing.T, html string) *document.Document {
	t.Helper()
	d, err := document.Parse(context.Background(), strings.NewReader(html), document.Limits{Bytes: 16 << 20, Nodes: 200000})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestChunkedConversation(t *testing.T) {
	ctx := context.Background()
	var html strings.Builder
	html.WriteString(`<body style="color:#123456;font-size:21px"><main id="history">`)
	for i := 0; i < 1500; i++ {
		fmt.Fprintf(&html, `<article id="message-%d" style="padding:4px;border:1px solid red"><h2>Message %d</h2><p id="text-%d">Hello <b>world</b> <a href="#message-1499">last</a></p><pre><code class="language-go">var x = %d</code></pre><svg width="2" height="2"><rect width="2" height="2"/></svg></article>`, i, i, i, i)
	}
	html.WriteString(`</main></body>`)
	renderer := &chunkDiagrams{}
	source, err := BuildChunked(ctx, parseChunks(t, html.String()), ChunkOptions{Options: Options{Syntax: styles.Get("github"), Document: document.Options{Diagrams: renderer}}})
	if err != nil {
		t.Fatal(err)
	}
	if renderer.calls != 0 || source.Chunks() != 1500 {
		t.Fatalf("eager work or unexpected partitions: calls=%d chunks=%d", renderer.calls, source.Chunks())
	}
	a, b, ok := source.ChunkRange("#message%2D1499")
	if !ok || a != 1499 || b != 1500 {
		t.Fatal(a, b, ok)
	}
	a, b, ok = source.ChunkRange("history")
	if !ok || a != 0 || b != 1500 {
		t.Fatal(a, b, ok)
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := eng.NewChunked(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 1499, 750, 1} {
		chunk, err := chunks.At(ctx, i, 600, nttypeset.LayoutOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk.Document.Blocks) != 4 {
			t.Fatalf("chunk %d: %d blocks", i, len(chunk.Document.Blocks))
		}
		p := chunk.Document.Blocks[1]
		if p.Size != 21 || p.Spans[0].Ink != (color.RGBA{0x12, 0x34, 0x56, 255}) {
			t.Fatalf("inheritance lost: %+v", p)
		}
		if p.Spans[len(p.Spans)-1].Href != "#message-1499" {
			t.Fatal("lost link")
		}
		anchor, ok, err := chunk.Layout.AnchorBounds(ctx, fmt.Sprintf("message-%d", i), true)
		if err != nil || !ok || anchor.Rect.Empty() {
			t.Fatal(anchor, ok, err)
		}
	}
	if renderer.calls != 4 {
		t.Fatalf("loaded intermediate chunks: %d", renderer.calls)
	}
	// Eviction must release the AST: reopening regenerates its diagram, while
	// a second access to the resident layout reuses it.
	for range 2 {
		if _, err := chunks.At(ctx, 1499, 600, nttypeset.LayoutOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if renderer.calls != 5 {
		t.Fatalf("unexpected AST residency: %d", renderer.calls)
	}
	_, sm, err := source.ChunkWithSources(ctx, 1499)
	if err != nil {
		t.Fatal(err)
	}
	if sm.ParentID("message-1499") != "history" || sm.RootID("message-1499") != "history" {
		t.Fatal("lost ancestry", sm)
	}
	start, end, ok := sm.BlockRange("message-1499")
	if !ok || start != 0 || end != 4 {
		t.Fatal("not chunk local", start, end, ok)
	}
}

func TestChunkedCompatibility(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<body><section id="s" style="padding:3px"><h1 id="h">Title</h1><p>Hello <a href="#h">link</a></p><ol start="3"><li>three</li><li>four</li></ol><table><tr><th>A</th><th>B</th></tr><tr><td>C</td><td>D</td></tr></table><img src="pic" alt="test"><pre><code class="language-go">package main</code></pre></section></body>`)
	opts := Options{Syntax: styles.Get("github"), Document: document.Options{Images: map[string]image.Image{"pic": image.NewRGBA(image.Rect(0, 0, 2, 2))}}}
	want, sm, err := BuildWithSources(ctx, doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	source, err := BuildChunked(ctx, doc, ChunkOptions{Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	got, gm, err := source.ChunkWithSources(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if source.Chunks() != 1 || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gm, sm) {
		t.Fatal("single chunk changed existing conversion")
	}
	plain, err := Build(ctx, doc, opts)
	if err != nil || !reflect.DeepEqual(plain, want) {
		t.Fatal("Build changed", err)
	}
}

func TestChunkedOversizedAndCancellation(t *testing.T) {
	ctx := context.Background()
	for _, unit := range []string{`<pre><span style="display:none">` + strings.Repeat("x", 65537) + `</span></pre>`, `<pre>` + strings.Repeat("x", 65537) + `</pre>`, `<ul>` + strings.Repeat(`<li>item</li>`, 1100) + `</ul>`, `<table>` + strings.Repeat(`<tr><td>cell</td></tr>`, 700) + `</table>`, `<div>inline ` + strings.Repeat(`<span>x</span>`, 1100) + `</div>`} {
		src, err := BuildChunked(ctx, parseChunks(t, `<body><p>first</p>`+unit+`<p id="last">last</p></body>`), ChunkOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if src.Chunks() != 3 {
			t.Fatalf("atomic unit split: %d", src.Chunks())
		}
		if _, err := src.Chunk(ctx, 0); err != nil {
			t.Fatal(err)
		}
		if d, err := src.Chunk(ctx, 1); !errors.Is(err, document.ErrLimit) || d != nil {
			t.Fatal("oversized content not rejected", err)
		}
		if _, err := src.Chunk(ctx, 2); err != nil {
			t.Fatal("oversized unit blocked later content", err)
		}
	}
	doc := parseChunks(t, `<p>test</p>`)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := BuildChunked(cancelled, doc, ChunkOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	source, err := BuildChunked(ctx, doc, ChunkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Chunk(cancelled, 0); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, i := range []int{-1, source.Chunks()} {
		if _, err := source.Chunk(ctx, i); err == nil {
			t.Fatal("bad index accepted")
		}
	}
	mid, cancel := context.WithCancel(ctx)
	renderer := &chunkDiagrams{cancel: cancel}
	source, err = BuildChunked(mid, parseChunks(t, `<svg width="2" height="2"><rect width="2" height="2"/></svg>`), ChunkOptions{Options: Options{Document: document.Options{Diagrams: renderer}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Chunk(mid, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("mid extraction cancellation", err)
	}
}

func TestChunkedDuplicateIDsAndAssets(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<body><section id="same"><p>first</p></section><section id="same"><p>second</p></section><p id="hidden" style="display:none">hidden</p></body>`)
	source, err := BuildChunked(ctx, doc, ChunkOptions{Limits: document.TypesetChunkLimits{Nodes: 6}})
	if err != nil {
		t.Fatal(err)
	}
	a, b, ok := source.ChunkRange("same")
	if !ok || a != 0 || b != 1 {
		t.Fatal(a, b, ok)
	}
	if _, _, ok := source.ChunkRange("#hidden"); ok {
		t.Fatal("hidden ID indexed")
	}
	_, sm, err := source.ChunkWithSources(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := sm.BlockRange("same"); ok {
		t.Fatal("duplicate ID retained")
	}
	for _, href := range []string{"https://example.com/#same", "#%zz", "missing"} {
		if _, _, ok := source.ChunkRange(href); ok {
			t.Fatal(href)
		}
	}
	assets := map[string]image.Image{}
	for i := 0; i < 257; i++ {
		assets[fmt.Sprint(i)] = nil
	}
	if _, err := BuildChunked(ctx, doc, ChunkOptions{Options: Options{Document: document.Options{Images: assets}}}); !errors.Is(err, document.ErrLimit) {
		t.Fatal("asset count", err)
	}
}

// A deterministic context exercises cancellation inside indexing, not only its
// entry guard, without a timing-dependent goroutine.
type indexCancelContext struct {
	context.Context
	checks int
}

func (c *indexCancelContext) Err() error {
	c.checks++
	if c.checks > 50 {
		return context.Canceled
	}
	return nil
}

func TestChunkedIndexCancellationAndLayoutLimits(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<body>`+strings.Repeat(`<p>message</p>`, 100)+`</body>`)
	cancelled := &indexCancelContext{Context: ctx}
	if source, err := BuildChunked(cancelled, doc, ChunkOptions{}); !errors.Is(err, context.Canceled) || source != nil {
		t.Fatal(source, err)
	}
	source, err := BuildChunked(ctx, parseChunks(t, `<p>`+strings.Repeat("x", 16385)+`</p>`), ChunkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := eng.NewChunked(source)
	if err != nil {
		t.Fatal(err)
	}
	part, err := chunks.At(ctx, 0, 600, nttypeset.LayoutOptions{})
	if err == nil {
		err = part.Layout.Complete(ctx)
	}
	if !errors.Is(err, nttypeset.ErrLimit) {
		t.Fatal("layout rune limit bypassed", err)
	}
	deep := strings.Repeat(`<section id="deep">`, 65) + `<p>text</p>` + strings.Repeat(`</section>`, 65)
	d, err := document.Parse(ctx, strings.NewReader(deep), document.Limits{Depth: 128})
	if err != nil {
		t.Fatal(err)
	}
	source, err = BuildChunked(ctx, d, ChunkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Chunk(ctx, 0); !errors.Is(err, document.ErrLimit) {
		t.Fatal("section depth silently clipped", err)
	}
	if _, err = BuildChunked(ctx, doc, ChunkOptions{Options: Options{Document: document.Options{Images: map[string]image.Image{"huge": image.NewUniform(color.Black)}}}}); !errors.Is(err, document.ErrLimit) {
		t.Fatal("asset pixels", err)
	}
}

func TestChunkedOrderAndLocalSourceAt(t *testing.T) {
	ctx := context.Background()
	html := `<body><section id="outer" style="font-style:italic"><p id="first">one <a href="#last">link</a></p><ol start="8"><li>eight</li><li>nine</li></ol><blockquote><p>quote</p></blockquote><p id="last">last</p></section></body>`
	doc := parseChunks(t, html)
	want, err := Build(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := BuildChunked(ctx, doc, ChunkOptions{Limits: document.TypesetChunkLimits{Nodes: 12}})
	if err != nil {
		t.Fatal(err)
	}
	if source.Chunks() < 2 {
		t.Fatal("test did not split")
	}
	var got []nttypeset.Block
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < source.Chunks(); i++ {
		part, sm, err := source.ChunkWithSources(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, part.Blocks...)
		lay, err := eng.Layout(part, 400, nttypeset.LayoutOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range sm.IDs() {
			start, _, ok := sm.BlockRange(id)
			if !ok {
				t.Fatal(id)
			}
			actual, ok, err := sm.SourceAt(ctx, lay, nttypeset.Place{Block: start, Placed: true})
			if err != nil || !ok || actual == "" {
				t.Fatal(actual, ok, err)
			}
		}
	}
	if !reflect.DeepEqual(got, want.Blocks) {
		t.Fatal("splitting changed block order/content/styles")
	}
	start, _, ok := source.ChunkRange("#last")
	if !ok || start != source.Chunks()-1 {
		t.Fatal(start, ok)
	}
}

func TestChunkedBeyondWholeASTBlockLimit(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<body>`+strings.Repeat(`<p>message</p>`, 33000)+`</body>`)
	if _, err := Build(ctx, doc, Options{}); !errors.Is(err, document.ErrLimit) {
		t.Fatal("fixture must exceed complete AST limit", err)
	}
	source, err := BuildChunked(ctx, doc, ChunkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if source.Chunks() != 33000 {
		t.Fatal("history shortened", source.Chunks())
	}
	for _, i := range []int{0, 32999} {
		part, err := source.Chunk(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		if len(part.Blocks) != 1 || flattenBlockText(part.Blocks[0]) != "message" {
			t.Fatal("lost message", i)
		}
	}
}
