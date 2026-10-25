package document

import (
	"context"
	"image"
	"strings"
	"testing"
)

func TestMinimalFontsRender(t *testing.T) {
	const src = `<html><body>
		<h1>Minimal Font Bundle</h1>
		<p>Regular, <b>bold</b>, <i>italic</i>, <b><i>bold italic</i></b>, and <code>monospace</code>.</p>
		<table><tr><th>Key</th><th>Value</th></tr><tr><td>Mode</td><td>Embedded/Constrained</td></tr></table>
	</body></html>`

	ctx := context.Background()
	doc, err := Parse(ctx, strings.NewReader(src), Limits{
		Bytes:  64 << 10,
		Nodes:  512,
		Depth:  16,
		Rules:  32,
		Pixels: 1 << 20,
		Height: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.Render(ctx, Options{
		Width: 400,
		Fonts: MinimalFonts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Image == nil || res.Bounds().Dx() != 400 || res.Bounds().Dy() < 60 {
		t.Fatalf("unexpected minimal-fonts render bounds: %v", res.Bounds())
	}
	blocks, err := doc.Blocks(ctx, Options{Width: 400, Fonts: MinimalFonts})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 3 {
		t.Fatalf("expected 3 styled blocks (heading, paragraph, table), got %d", len(blocks))
	}
}

func BenchmarkRenderMinimalFonts(b *testing.B) {
	ctx := context.Background()
	src := strings.Repeat(paintPage, 4)
	doc, err := Parse(ctx, strings.NewReader(src), Limits{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := doc.Render(ctx, Options{Width: 600, Fonts: MinimalFonts}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderMinimalFontsNoImagePaintBand(b *testing.B) {
	ctx := context.Background()
	src := strings.Repeat(paintPage, 4)
	doc, err := Parse(ctx, strings.NewReader(src), Limits{})
	if err != nil {
		b.Fatal(err)
	}
	band := image.Rect(0, 0, 600, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := doc.Render(ctx, Options{Width: 600, Fonts: MinimalFonts, NoImage: true})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := res.Paint(ctx, band, 1); err != nil {
			b.Fatal(err)
		}
	}
}
