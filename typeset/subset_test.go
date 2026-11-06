package typeset

import (
	"context"
	"fmt"
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

func TestSubsetImageOnlyDisplayNone(t *testing.T) {
	ctx := context.Background()
	asset := image.NewRGBA(image.Rect(0, 0, 12, 8))
	for _, tc := range []struct {
		name, body string
		images     int
	}{
		{"visible", `<p><a href="#target"><img src="x" alt="picture"></a></p>`, 1},
		{"hidden image", `<p><img style="display:none" src="x" alt="hidden"></p>`, 0},
		{"hidden link", `<p><a style="display:none" href="#target"><img src="x" alt="hidden"></a></p>`, 0},
		{"stylesheet", `<style>.hide {display:none}</style><p><a class="hide"><img src="x" alt="hidden"></a></p>`, 0},
		{"visible child of hidden link", `<p><a style="display:none"><img style="display:block" src="x" alt="hidden"></a></p>`, 0},
		{"hidden beside visible", `<p><img style="display:none" src="x" alt="hidden"><img src="x" alt="picture"></p>`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseChunks(t, `<body><p>before</p>`+tc.body+`<p>after</p></body>`)
			opts := Options{Document: document.Options{Images: map[string]image.Image{"x": asset}}}
			complete, err := Build(ctx, doc, opts)
			if err != nil {
				t.Fatal(err)
			}
			source, err := BuildChunked(ctx, doc, ChunkOptions{Options: opts})
			if err != nil {
				t.Fatal(err)
			}
			chunkDocs := []*nttypeset.Document{}
			for i := 0; i < source.Chunks(); i++ {
				d, _, err := source.ChunkWithSources(ctx, i)
				if err != nil {
					t.Fatal(err)
				}
				chunkDocs = append(chunkDocs, d)
			}
			for name, docs := range map[string][]*nttypeset.Document{"complete": {complete}, "chunked": chunkDocs} {
				count := 0
				for _, d := range docs {
					for _, b := range d.Blocks {
						if b.Kind == nttypeset.KindImage {
							count++
						}
						for _, span := range b.Spans {
							if strings.Contains(span.Text, "hidden") {
								t.Errorf("%s exposed hidden alt text", name)
							}
						}
					}
				}
				if count != tc.images {
					t.Errorf("%s images=%d, want %d", name, count, tc.images)
				}
			}
		})
	}
}

func TestSubsetImageFragmentIDs(t *testing.T) {
	ctx := context.Background()
	for _, asset := range []bool{true, false} {
		t.Run(fmt.Sprintf("asset=%v", asset), func(t *testing.T) {
			doc := parseChunks(t, `<body><p>before</p><p id="figure"><a id="link" href="#destination"><img id="photo" src="local.png" alt="picture"></a></p><p id="destination">after</p></body>`)
			opts := Options{Document: document.Options{Width: 400}}
			if asset {
				opts.Document.Images = map[string]image.Image{"local.png": image.NewRGBA(image.Rect(0, 0, 12, 8))}
			}
			eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
			if err != nil {
				t.Fatal(err)
			}
			check := func(d *nttypeset.Document, m *SourceMap) {
				t.Helper()
				start, end, ok := m.BlockRange("figure")
				if !ok || end != start+1 || d.Blocks[start].Kind != nttypeset.KindImage {
					t.Fatal("missing figure image")
				}
				for _, id := range []string{"link", "photo"} {
					a, b, ok := m.BlockRange("#" + id)
					if !ok || a != start || b != end {
						t.Errorf("fragment %s = [%d,%d), %v; want image [%d,%d)", id, a, b, ok, start, end)
					}
				}
				if m.ParentID("photo") != "link" || m.ParentID("link") != "figure" {
					t.Errorf("lost hierarchy: photo -> %q, link -> %q", m.ParentID("photo"), m.ParentID("link"))
				}
				for _, width := range []int{240, 600} {
					lay, err := eng.Layout(d, width, nttypeset.LayoutOptions{})
					if err != nil {
						t.Fatal(err)
					}
					for _, id := range []string{"figure", "link", "photo"} {
						anchor, ok, err := m.AnchorBounds(ctx, lay, "#"+id, true)
						if err != nil || !ok || anchor.Rect.Empty() {
							t.Errorf("width %d fragment %s: %+v, %v, %v", width, id, anchor, ok, err)
						}
					}
				}
				if d.Blocks[start].Href != "#destination" {
					t.Errorf("image link=%q", d.Blocks[start].Href)
				}
			}
			d, m, err := BuildWithSources(ctx, doc, opts)
			if err != nil {
				t.Fatal(err)
			}
			check(d, m)
			source, err := BuildChunked(ctx, doc, ChunkOptions{Options: opts, Limits: document.TypesetChunkLimits{Nodes: 6}})
			if err != nil {
				t.Fatal(err)
			}
			if source.Chunks() < 2 {
				t.Fatal("test did not split into chunks")
			}
			for _, id := range []string{"figure", "link", "photo"} {
				a, b, ok := source.ChunkRange("#" + id)
				if !ok {
					t.Errorf("missing chunk for %s", id)
					continue
				}
				found := false
				for i := a; i < b; i++ {
					part, sm, err := source.ChunkWithSources(ctx, i)
					if err != nil {
						t.Fatal(err)
					}
					if _, _, ok := sm.BlockRange("figure"); ok {
						check(part, sm)
						found = true
					}
				}
				if !found {
					t.Errorf("fragment %s does not reach image chunk", id)
				}
			}
		})
	}
}

func TestSubsetImageFragmentBoundaries(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<body><p id="same">first</p><p><a id="link"><img id="photo" src="x" alt="picture"></a></p><p><a id="same"><img id="other" src="x" alt="duplicate"></a></p><p><a style="display:none" id="hidden-link"><img id="hidden-image" src="x" alt="hidden"></a></p></body>`)
	d, m, err := BuildWithSources(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int{"same": 0, "link": 1, "photo": 1, "other": 2} {
		start, end, ok := m.BlockRange(id)
		if !ok || start != want || end != want+1 {
			t.Errorf("%s range [%d,%d), %v; want block %d", id, start, end, ok, want)
		}
	}
	if m.ParentID("photo") != "link" || m.ParentID("link") != "" || m.ParentID("other") != "" {
		t.Fatal("wrong image ancestry")
	}
	for _, id := range []string{"hidden-link", "hidden-image"} {
		if _, _, ok := m.BlockRange(id); ok {
			t.Errorf("hidden ID %s exposed", id)
		}
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Layout(d, 400, nttypeset.LayoutOptions{}); err != nil {
		t.Fatal(err)
	}
}
