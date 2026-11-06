package typeset

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

// This checks preservation, not CSS/browser geometry conformance. In particular,
// inline replaced-element baselines and mixed text/image lines remain separate gaps.
func TestSubsetStandaloneImageBetweenBlocks(t *testing.T) {
	ctx := context.Background()
	asset := image.NewRGBA(image.Rect(0, 0, 12, 8))
	red := color.RGBA{R: 237, G: 11, B: 29, A: 255}
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			asset.SetRGBA(x, y, red)
		}
	}
	for _, chunked := range []bool{false, true} {
		name := "complete"
		if chunked {
			name = "chunked"
		}
		t.Run(name, func(t *testing.T) {
			doc := parseChunks(t, `<body><p>before</p>
<img id="photo" src="local.png" width="12" height="8" alt="fallback">
<p>after</p></body>`)
			opts := Options{Document: document.Options{Width: 400, Images: map[string]image.Image{"local.png": asset}}}
			var docs []*nttypeset.Document
			if chunked {
				source, err := BuildChunked(ctx, doc, ChunkOptions{Options: opts})
				if err != nil {
					t.Fatal(err)
				}
				if _, _, ok := source.ChunkRange("#photo"); !ok {
					t.Fatal("lost image fragment")
				}
				for i := 0; i < source.Chunks(); i++ {
					d, _, err := source.ChunkWithSources(ctx, i)
					if err != nil {
						t.Fatal(err)
					}
					docs = append(docs, d)
				}
			} else {
				d, m, err := BuildWithSources(ctx, doc, opts)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, ok := m.BlockRange("photo"); !ok {
					t.Fatal("lost image fragment")
				}
				docs = append(docs, d)
			}
			order := ""
			images := 0
			painted := false
			eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range docs {
				for _, b := range d.Blocks {
					if b.Kind == nttypeset.KindImage {
						images++
						order += "image "
						if b.Image == nil {
							t.Fatal("missing image asset")
						}
					}
					for _, s := range b.Spans {
						if strings.Contains(s.Text, "before") {
							order += "before "
						}
						if strings.Contains(s.Text, "after") {
							order += "after "
						}
					}
				}
				lay, err := eng.Layout(d, 400, nttypeset.LayoutOptions{})
				if err != nil {
					t.Fatal(err)
				}
				pic, err := lay.Raster(ctx, nttypeset.View{Width: 400, Height: 400, Scale: 1}, nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
				if err != nil {
					t.Fatal(err)
				}
				for y := 0; y < pic.Bounds().Dy(); y++ {
					for x := 0; x < pic.Bounds().Dx(); x++ {
						if pic.RGBAAt(x, y) == red {
							painted = true
						}
					}
				}
			}
			if images != 1 || order != "before image after " || !painted {
				t.Fatalf("images=%d order=%q painted=%v", images, order, painted)
			}
		})
	}
}

func TestSubsetInlineImageBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		images     int
	}{
		{"mixed text", `<p>before</p>left<img src="x" alt="picture">right<p>after</p>`, 0},
		{"adjacent images", `<p>before</p><img src="x" alt="one"><img src="x" alt="two"><p>after</p>`, 0},
		{"hidden image", `<p>before</p><img style="display:none" src="x" alt="hidden"><p>after</p>`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ast, err := Build(context.Background(), parseChunks(t, tc.body), Options{Document: document.Options{Images: map[string]image.Image{"x": image.NewRGBA(image.Rect(0, 0, 2, 2))}}})
			if err != nil {
				t.Fatal(err)
			}
			images := 0
			for _, b := range ast.Blocks {
				if b.Kind == nttypeset.KindImage {
					images++
				}
				for _, s := range b.Spans {
					if strings.Contains(s.Text, "hidden") {
						t.Fatal("hidden image exposed")
					}
				}
			}
			if images != tc.images {
				t.Fatalf("images=%d", images)
			}
		})
	}
}
