package typeset

import (
	"context"
	"image/color"
	"strings"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
)

func TestHeadingCSSColors(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<h1>plain</h1><div style="color:#123456"><h2>inherited<span style="color:#654321">child</span></h2></div><h3 style="color:#abcdef">explicit</h3>`)
	d, err := Build(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]color.RGBA{"plain": {}, "inherited": {0x12, 0x34, 0x56, 255}, "child": {0x65, 0x43, 0x21, 255}, "explicit": {0xab, 0xcd, 0xef, 255}}
	seen := map[string]bool{}
	for _, b := range d.Blocks {
		for _, s := range b.Spans {
			text := strings.TrimSpace(s.Text)
			if text == "" {
				continue
			}
			ink, ok := want[text]
			if !ok {
				t.Fatal(text)
			}
			if s.Role == nttypeset.RoleAccent || s.Ink != ink {
				t.Errorf("%s role=%v ink=%v want body role/ink=%v", text, s.Role, s.Ink, ink)
			}
			seen[text] = true
		}
	}
	if len(seen) != len(want) {
		t.Fatal("missing runs", seen)
	}
}

func TestCSSLineHeights(t *testing.T) {
	for _, tc := range []struct {
		name, html string
		height     int
	}{
		{"heading direct", `<h1 style="line-height:48px">MMMM<br>MMMM</h1>`, 48},
		{"heading unitless inherited", `<div style="line-height:2"><h1>MMMM<br>MMMM</h1></div>`, 64},
		{"heading length inherited", `<div style="line-height:48px"><h2>MMMM<br>MMMM</h2></div>`, 48},
		{"heading percent inherited", `<div style="font-size:32px;line-height:150%"><h3>MMMM<br>MMMM</h3></div>`, 48},
		{"heading explicit override", `<div style="line-height:48px"><h4 style="line-height:24px">MMMM<br>MMMM</h4></div>`, 24},
		{"pre direct", `<pre style="line-height:40px">MMMM` + "\n" + `MMMM</pre>`, 40},
		{"pre inherited", `<div style="line-height:40px"><pre>MMMM` + "\n" + `MMMM</pre></div>`, 40},
		{"paragraph inherited", `<div style="line-height:40px"><p>MMMM<br>MMMM</p></div>`, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			doc := parseChunks(t, tc.html)
			complete, err := Build(ctx, doc, Options{})
			if err != nil {
				t.Fatal(err)
			}
			source, err := BuildChunked(ctx, doc, ChunkOptions{})
			if err != nil {
				t.Fatal(err)
			}
			chunk, err := source.Chunk(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			for mode, d := range map[string]*nttypeset.Document{"complete": complete, "chunked": chunk} {
				if len(d.Blocks) != 1 {
					t.Fatalf("%s blocks=%d", mode, len(d.Blocks))
				}
				f := d.Blocks[0].Format
				if f == nil || f.LineHeight != tc.height {
					t.Errorf("%s format=%+v want line-height %d", mode, f, tc.height)
				}
				eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
				if err != nil {
					t.Fatal(err)
				}
				lay, err := eng.Layout(d, 400, nttypeset.LayoutOptions{})
				if err != nil {
					t.Fatal(err)
				}
				palette := nttypeset.LightPalette()
				pic, err := lay.Raster(ctx, nttypeset.View{Width: 400, Height: 400, Scale: 1}, nttypeset.RasterOptions{Palette: palette})
				if err != nil {
					t.Fatal(err)
				}
				var starts []int
				previous := false
				for y := 0; y < 400; y++ {
					ink := false
					for x := 0; x < 400; x++ {
						if pic.RGBAAt(x, y) != palette.Background {
							ink = true
							break
						}
					}
					if ink && !previous {
						starts = append(starts, y)
					}
					previous = ink
				}
				if len(starts) != 2 || starts[1]-starts[0] != tc.height {
					t.Errorf("%s painted line starts %v, want separation %d", mode, starts, tc.height)
				}

			}
		})
	}
}
