package typeset

import (
	"context"
	"image"
	"image/color"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

func TestImageCSSMaximumDimensions(t *testing.T) {
	ctx := context.Background()
	red := color.RGBA{0xd0, 0x20, 0x30, 255}
	asset := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			asset.SetRGBA(x, y, red)
		}
	}
	for _, tc := range []struct {
		name, css     string
		width, height int
	}{
		{"width", "max-width:80px", 80, 40},
		{"height", "max-height:30px", 60, 30},
		{"both", "max-width:80px;max-height:30px", 60, 30},
		{"no enlargement", "max-width:400px;max-height:400px", 200, 100},
		{"em", "font-size:20px;max-width:4em", 80, 40},
		{"none", "max-width:none;max-height:none", 200, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseChunks(t, `<p><img src="x" style="`+tc.css+`"></p>`)
			opts := Options{Document: document.Options{Width: 800, Images: map[string]image.Image{"x": asset}}}
			full, err := Build(ctx, doc, opts)
			if err != nil {
				t.Fatal(err)
			}
			chunks, err := BuildChunked(ctx, doc, ChunkOptions{Options: opts})
			if err != nil {
				t.Fatal(err)
			}
			part, err := chunks.Chunk(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			for mode, d := range map[string]*nttypeset.Document{"complete": full, "chunked": part} {
				eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
				if err != nil {
					t.Fatal(err)
				}
				lay, err := eng.Layout(d, 800, nttypeset.LayoutOptions{})
				if err != nil {
					t.Fatal(err)
				}
				pic, err := lay.Raster(ctx, nttypeset.View{Width: 800, Height: 600, Scale: 1}, nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
				if err != nil {
					t.Fatal(err)
				}
				bounds := image.Rectangle{}
				for y := 0; y < 600; y++ {
					for x := 0; x < 800; x++ {
						if pic.RGBAAt(x, y) == red {
							bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
						}
					}
				}
				if bounds.Dx() != tc.width || bounds.Dy() != tc.height {
					t.Errorf("%s painted %v, want %dx%d", mode, bounds.Size(), tc.width, tc.height)
				}
			}
		})
	}
}
