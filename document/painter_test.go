package document

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
)

func painterDocument(tb testing.TB, n int) *Result {
	tb.Helper()
	var src strings.Builder
	src.WriteString(`<style>body{background:#e4e0d0}p{margin:2px;padding:2px;border:1px solid #785} .abs{position:absolute;left:15px;top:200px;background:#fe8}</style><body><div class="abs">positioned</div>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&src, `<p id="p%d">Paragraph %d with <b>bold</b> and <i>italic</i> text.</p>`, i, i)
	}
	r, err := Render(context.Background(), strings.NewReader(src.String()), Options{Width: 400, NoImage: true, Fonts: MinimalFonts})
	if err != nil {
		tb.Fatal(err)
	}
	return r
}

func TestPainterMatchesPaintAndBoundsIndex(t *testing.T) {
	ctx := context.Background()
	r := painterDocument(t, 1000)
	p, err := r.NewPainter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	refs := len(p.broad)
	for _, ids := range p.buckets {
		refs += len(ids)
	}
	if refs > len(r.ops)*paintMaxBucketsPerOp {
		t.Fatalf("unbounded index: %d", refs)
	}
	for _, scale := range []float64{1, 0.0625, 2, 16, 1} {
		for _, y := range []int{0, 180, 10000, r.Bounds().Max.Y - 60} {
			area := image.Rect(0, y, 160, y+32)
			want, err := r.Paint(ctx, area, scale)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Paint(ctx, area, scale)
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("paint differs at y=%d scale=%g", y, scale)
			}
			if len(p.candidates) > len(r.ops)/4 {
				t.Fatalf("band scans too much: %d/%d", len(p.candidates), len(r.ops))
			}
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Paint(cancelled, image.Rect(0, 0, 400, 400), 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := p.Paint(ctx, r.Bounds(), 16); !errors.Is(err, ErrLimit) {
		t.Fatalf("pixel limit: %v", err)
	}
	p.Close()
	if _, err := p.Paint(ctx, image.Rect(0, 0, 1, 1), 1); err == nil {
		t.Fatal("closed painter accepted work")
	}
}

func TestPainterCSSBackgroundImage(t *testing.T) {
	ctx := context.Background()
	tile := image.NewRGBA(image.Rect(0, 0, 4, 4))
	tile.SetRGBA(1, 1, color.RGBA{255, 0, 0, 255})
	r, err := Render(ctx, strings.NewReader(`<body style="background-image:url(tile)">`+strings.Repeat(`<div style="height:1000px;background-image:url(tile)">text</div>`, 20)+`</body>`), Options{Width: 200, NoImage: true, Fonts: MinimalFonts, BackgroundImages: map[string]image.Image{"tile": tile}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.NewPainter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	area := image.Rect(0, 15000, 200, 15100)
	want, err := r.Paint(ctx, area, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Paint(ctx, area, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("background image changed")
	}
}

type countedFonts struct{ calls int }

func (f *countedFonts) Face(family, variant string) ([]byte, error) {
	f.calls++
	return MinimalFonts.Face(family, variant)
}
func TestPainterReusesFonts(t *testing.T) {
	ctx := context.Background()
	fonts := &countedFonts{}
	r, err := Render(ctx, strings.NewReader(`<p>text <b>bold</b></p><p>more text</p>`), Options{Width: 400, NoImage: true, Fonts: fonts})
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.NewPainter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err = p.Paint(ctx, r.Bounds(), 1); err != nil {
		t.Fatal(err)
	}
	calls := fonts.calls
	if _, err = p.Paint(ctx, r.Bounds(), 1); err != nil {
		t.Fatal(err)
	}
	if fonts.calls != calls {
		t.Fatal("warm paint reloaded fonts")
	}
	if _, err = p.Paint(ctx, r.Bounds(), 2); err != nil {
		t.Fatal(err)
	}
	if fonts.calls != calls {
		t.Fatal("scale change reparsed font source")
	}
}

func BenchmarkBandPaint(b *testing.B) {
	for _, n := range []int{100, 1100} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			r := painterDocument(b, n)
			ctx := context.Background()
			area := image.Rect(0, r.Bounds().Dy()/2, 400, r.Bounds().Dy()/2+200)
			b.Run("uncached", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := r.Paint(ctx, area, 1); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("cold", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					p, err := r.NewPainter(ctx)
					if err != nil {
						b.Fatal(err)
					}
					_, err = p.Paint(ctx, area, 1)
					p.Close()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("warm", func(b *testing.B) {
				p, err := r.NewPainter(ctx)
				if err != nil {
					b.Fatal(err)
				}
				defer p.Close()
				b.ReportAllocs()
				for b.Loop() {
					if _, err := p.Paint(ctx, area, 1); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func TestPainterHostFallbackCache(t *testing.T) {
	data := coverageFont(t, '漢')
	calls := 0
	host := fontSource(func(family, variant string) ([]byte, error) { calls++; return data, nil })
	ctx := context.Background()
	r, err := Render(ctx, strings.NewReader(strings.Repeat(`<p>Latin <b>漢漢</b> text</p>`, 100)), Options{Width: 400, NoImage: true, Fonts: MinimalFonts, FallbackFaces: []FallbackFace{{Family: "CJK", Source: host}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.NewPainter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	area := image.Rect(0, 1000, 400, 1200)
	want, err := r.Paint(ctx, area, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Paint(ctx, area, 1)
	if err != nil || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatalf("fallback repaint mismatch: %v", err)
	}
	warmCalls := calls
	if _, err = p.Paint(ctx, image.Rect(0, 1200, 400, 1400), 1); err != nil {
		t.Fatal(err)
	}
	if calls != warmCalls {
		t.Fatalf("fallback fonts reloaded: %d -> %d", warmCalls, calls)
	}
}
