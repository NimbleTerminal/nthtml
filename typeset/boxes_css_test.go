package typeset

import (
	"bytes"
	"context"
	"image/color"
	"os"
	"strings"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

func TestCalloutBoxStyles(t *testing.T) {
	ctx := context.Background()
	source, err := os.ReadFile("../internal/conformance/testdata/authored/callout-boxes.html")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	for _, chunked := range []bool{false, true} {
		t.Run(fmtMode(chunked), func(t *testing.T) {
			var original [][]byte
			for variant, html := range []string{string(source), strings.Replace(string(source), "/* explicit-zero-padding */", "h2, p {padding:0}", 1)} {
				doc := parseChunks(t, html)
				var docs []*nttypeset.Document
				if chunked {
					chunks, err := BuildChunked(ctx, doc, ChunkOptions{Limits: document.TypesetChunkLimits{Nodes: 10}})
					if err != nil {
						t.Fatal(err)
					}
					if chunks.Chunks() < 2 {
						t.Fatal("test did not split")
					}
					for i := 0; i < chunks.Chunks(); i++ {
						d, err := chunks.Chunk(ctx, i)
						if err != nil {
							t.Fatal(err)
						}
						docs = append(docs, d)
					}
				} else {
					d, err := Build(ctx, doc, Options{})
					if err != nil {
						t.Fatal(err)
					}
					docs = append(docs, d)
				}
				if variant == 1 && len(docs) != len(original) {
					t.Fatal("padding:0 changed chunk count")
				}
				for i, d := range docs {
					var outer *nttypeset.Section
					for j := range d.Sections {
						if d.Sections[j].SourceID == "callout" {
							outer = &d.Sections[j]
						}
					}
					for _, section := range d.Sections {
						switch section.SourceID {
						case "heading", "paragraph":
							if section.Background.A != 0 || section.Padding != (nttypeset.Insets{}) {
								t.Fatalf("child inherited non-inherited box styles: %+v", section)
							}
						case "code":
							if section.Background != (color.RGBA{0xcc, 0xdd, 0xee, 255}) || section.Padding != (nttypeset.Insets{Top: 8, Right: 10, Bottom: 8, Left: 10}) {
								t.Fatalf("code box lost own styles: %+v", section)
							}
						}
					}

					if outer != nil {
						if outer.Padding != (nttypeset.Insets{Top: 12, Right: 18, Bottom: 16, Left: 20}) || outer.Background != (color.RGBA{0xe8, 0xf0, 0xf8, 255}) {
							t.Fatalf("outer box: %+v", outer)
						}
						if outer.Top.Width != 3 || outer.Right.Width != 5 || outer.Bottom.Width != 7 || outer.Left.Width != 9 {
							t.Fatalf("outer borders: %+v", outer)
						}
					}
					for _, b := range d.Blocks {
						if b.Format == nil {
							t.Fatal("bordered block retained reading defaults")
						}
						if b.Format.Padding != (nttypeset.Insets{}) || b.Format.Background.A != 0 || b.Format.SpaceBefore != 0 || b.Format.SpaceAfter != 0 {
							t.Fatalf("decoration duplicated inside section: %+v", b.Format)
						}
					}
					lay, err := eng.Layout(d, 800, nttypeset.LayoutOptions{})
					if err != nil {
						t.Fatal(err)
					}
					pic, err := lay.Raster(ctx, nttypeset.View{Width: 800, Height: 600, Scale: 1}, nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
					if err != nil {
						t.Fatal(err)
					}
					if variant == 0 {
						original = append(original, append([]byte(nil), pic.Pix...))
					} else if !bytes.Equal(original[i], pic.Pix) {
						t.Errorf("chunk %d: padding:0 changed pixels", i)
					}
					if outer != nil {
						a, ok, err := lay.AnchorBounds(ctx, "callout", true)
						if err != nil || !ok {
							t.Fatal("missing callout anchor", err)
						}
						r := a.Rect
						for _, edge := range []struct {
							x, y int
							ink  color.RGBA
						}{
							{r.Min.X + r.Dx()/2, r.Min.Y + 1, outer.Top.Color},
							{r.Max.X - 2, r.Min.Y + r.Dy()/2, outer.Right.Color},
							{r.Min.X + r.Dx()/2, r.Max.Y - 2, outer.Bottom.Color},
							{r.Min.X + 2, r.Min.Y + r.Dy()/2, outer.Left.Color},
							{r.Min.X + outer.Left.Width + 1, r.Min.Y + outer.Top.Width + 1, outer.Background},
						} {
							if got := pic.RGBAAt(edge.x, edge.y); got != edge.ink {
								t.Errorf("paint (%d,%d)=%v want %v", edge.x, edge.y, got, edge.ink)
							}
						}
					}
				}
			}
		})
	}
}

func fmtMode(chunked bool) string {
	if chunked {
		return "chunked"
	}
	return "complete"
}
