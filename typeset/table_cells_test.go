package typeset

import (
	"context"
	"image/color"
	"os"
	"strings"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
)

func TestCSSCellStyles(t *testing.T) {
	ctx := context.Background()
	html, err := os.ReadFile("../internal/conformance/testdata/authored/table-cells.html")
	if err != nil {
		t.Fatal(err)
	}
	doc := parseChunks(t, string(html))
	full, err := Build(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := BuildChunked(ctx, doc, ChunkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	part, err := chunks.Chunk(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for mode, d := range map[string]*nttypeset.Document{"complete": full, "chunked": part} {
		if len(d.Blocks) != 1 || d.Blocks[0].Table == nil {
			t.Fatal(mode, "missing table styles")
		}
		cells := d.Blocks[0].Table.Cells
		if len(cells) != 2 || len(cells[0]) != 2 {
			t.Fatal("lost grid")
		}
		if c := cells[0][0]; c.Padding != (nttypeset.Insets{Top: 6, Right: 12, Bottom: 10, Left: 16}) || c.Top.Width != 2 || c.Right.Width != 4 || c.Bottom.Width != 6 || c.Left.Width != 8 || c.Background != (color.RGBA{0xd0, 0x20, 0x30, 255}) {
			t.Fatalf("bad cell styles %+v", c)
		}
		if cells[0][1].TextAlign != "right" || cells[1][0].TextAlign != "center" {
			t.Fatal("lost alignment")
		}
		eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
		if err != nil {
			t.Fatal(err)
		}
		lay, err := eng.Layout(d, 800, nttypeset.LayoutOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := lay.Complete(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCSSCellStyleEligibility(t *testing.T) {
	base := `<table style="border-collapse:separate;border-spacing:0"><tr><td style="vertical-align:top">one</td><td style="vertical-align:top"></td></tr></table>`
	for _, tc := range []struct {
		name, html string
		styled     bool
	}{
		{"empty cell", base, true},
		{"collapsed", strings.Replace(base, "separate", "collapse", 1), false},
		{"spacing", strings.Replace(base, "border-spacing:0", "border-spacing:2px", 1), false},
		{"vertical", strings.ReplaceAll(base, "vertical-align:top", "vertical-align:middle"), false},
		{"spanning", strings.Replace(base, "<td ", `<td colspan="2" `, 1), false},
		{"dashed", strings.Replace(base, "vertical-align:top", "vertical-align:top;border:2px dashed red", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Build(context.Background(), parseChunks(t, tc.html), Options{})
			if err != nil {
				t.Fatal(err)
			}
			styled := len(d.Blocks) == 1 && d.Blocks[0].Table != nil
			if styled != tc.styled {
				t.Errorf("styled=%v want %v", styled, tc.styled)
			}
			if styled && (len(d.Blocks[0].Rows) != 1 || len(d.Blocks[0].Rows[0]) != 2) {
				t.Fatal("empty column or single row lost")
			}
		})
	}
}
