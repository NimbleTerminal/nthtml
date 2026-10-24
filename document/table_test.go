package document

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
)

func boxesWithTag(r *Result, tag string) []Box {
	var out []Box
	for _, b := range r.Boxes {
		if b.Tag == tag {
			out = append(out, b)
		}
	}
	return out
}
func TestTableSharedColumns(t *testing.T) {
	r := renderCSS(t, `<style>td,th{padding:4px;border:1px solid black}th{background:blue;color:white}</style><table><thead><tr><th>Description</th><th>Qty</th></tr></thead><tbody><tr><td>A considerably longer description here</td><td>2</td></tr><tr><td>Short</td><td>3</td></tr></tbody></table>`)
	heads, cells := boxesWithTag(r, "th"), boxesWithTag(r, "td")
	if heads[0].Bounds.Dx() <= heads[1].Bounds.Dx() {
		t.Fatal("description should receive more width", heads)
	}
	for i, c := range cells {
		h := heads[i%2]
		if c.Bounds.Min.X != h.Bounds.Min.X || c.Bounds.Max.X != h.Bounds.Max.X {
			t.Fatal("column mismatch", c, h)
		}
	}
	if heads[0].Color.B != 255 {
		t.Fatal("header background lost")
	}
	for _, b := range r.Boxes {
		if b.Text == "Description" && !b.Bold {
			t.Fatal("header should be bold")
		}
	}
}
func TestTableRowHeightAlignmentAndLinks(t *testing.T) {
	for _, align := range []string{"top", "middle", "bottom"} {
		r := renderCSS(t, `<style>td{padding:4px;border:1px solid blue}</style><table><tr><td style="height:100px">Tall</td><td style="vertical-align:`+align+`;text-align:right"><a href="https://example.com">Link</a></td></tr></table>`)
		cells := boxesWithTag(r, "td")
		if cells[0].Bounds.Dy() != cells[1].Bounds.Dy() {
			t.Fatal(cells)
		}
		var link Box
		for _, b := range r.Boxes {
			if b.Text == "Link" {
				link = b
			}
		}
		want := 5
		if align == "middle" {
			want += 39
		}
		if align == "bottom" {
			want += 78
		}
		if link.Bounds.Min.Y != want {
			t.Fatalf("%s: got %v want y=%d", align, link.Bounds, want)
		}
		if link.Bounds.Max.X != cells[1].Bounds.Max.X-5 {
			t.Fatal("right alignment", link.Bounds, cells[1].Bounds)
		}
		if len(r.Links) != 1 {
			t.Fatal(r.Links)
		}
		if hit, ok := r.LinkAt(image.Pt(link.Bounds.Min.X, link.Bounds.Min.Y)); !ok || hit.Href != "https://example.com" {
			t.Fatal("link hit region did not follow alignment")
		}
		borders := boxesWithTag(r, "border")
		if len(borders) != 8 || borders[5].Bounds.Max.Y != cells[1].Bounds.Max.Y || borders[6].Bounds.Max.Y != cells[1].Bounds.Max.Y {
			t.Fatal("borders do not stretch", borders)
		}
	}
}
func TestTableGroupsHiddenNestedAndRagged(t *testing.T) {
	r := renderCSS(t, `<table><caption>Caption</caption><thead style="background:blue"><tr><th>A</th><th>B</th></tr></thead><tbody><tr style="display:none"><td>Hidden</td></tr><tr><td><table><tr><td>Nested</td></tr></table></td><td>Outer</td></tr><tr><td>Ragged</td></tr></tbody></table>`)
	if len(boxesWithTag(r, "table")) != 2 {
		t.Fatal("nested table lost")
	}
	if boxesWithTag(r, "tr")[0].Color.B != 255 {
		t.Fatal("group background lost")
	}
	for _, b := range r.Boxes {
		if b.Text == "Hidden" {
			t.Fatal("hidden row painted")
		}
	}
	if boxesWithTag(r, "caption")[0].Bounds.Max.Y > boxesWithTag(r, "tr")[0].Bounds.Min.Y {
		t.Fatal("caption overlap")
	}
}
func TestTableBudgetsAndAllocation(t *testing.T) {
	_, err := Render(context.Background(), strings.NewReader("<table><tr>"+strings.Repeat("<td>x</td>", 257)+"</tr></table>"), Options{Width: 400})
	if !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	for width := 1; width <= 400; width++ {
		values := tableWidths([]int{20, 50, 10}, []int{30, 200, 10}, width)
		sum := 0
		for _, v := range values {
			if v < 0 {
				t.Fatal(values)
			}
			sum += v
		}
		if sum != width {
			t.Fatal(width, values)
		}
	}
}
