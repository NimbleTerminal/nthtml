package document

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"
)

func columnWidth(t *testing.T, content string) int {
	t.Helper()
	r := renderCSS(t, "<table><tr><td>"+content+"</td><td>comparison text</td></tr></table>")
	return boxesWithTag(r, "td")[0].Bounds.Dx()
}
func TestIntrinsicLinesAndInlineWords(t *testing.T) {
	one := columnWidth(t, "Longword")
	for _, content := range []string{"Longword<br>Longword", "<div>Longword</div><div>Longword</div>", "Long<span>word</span>"} {
		if got := columnWidth(t, content); got != one {
			t.Fatalf("%s got %d want %d", content, got, one)
		}
	}
	if got := columnWidth(t, "Longword Longword"); got <= one {
		t.Fatalf("unbroken line should prefer more width: %d <= %d", got, one)
	}
	if a, b := columnWidth(t, " a   b "), columnWidth(t, "a b"); a != b {
		t.Fatalf("whitespace: %d != %d", a, b)
	}
}
func TestIntrinsicSurplusDistribution(t *testing.T) {
	// Below preferred width, only columns that can grow receive surplus.
	got := tableWidths([]int{20, 10}, []int{20, 50}, 50)
	if got[0] != 20 || got[1] != 30 {
		t.Fatal(got)
	}
	// Above preferred width, distribute by preferred size, not word count.
	got = tableWidths([]int{20, 10}, []int{20, 50}, 140)
	if got[0] != 40 || got[1] != 100 {
		t.Fatal(got)
	}
}
func TestCollapsedSharedEdges(t *testing.T) {
	source := `<style>table{border-collapse:collapse}td{padding:4px;border:1px solid blue;height:20px}</style><table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>`
	r := renderCSS(t, source)
	cells := boxesWithTag(r, "td")
	if len(boxesWithTag(r, "border")) != 0 {
		t.Fatal("separate cell borders were painted")
	}
	edges := boxesWithTag(r, "collapsed-border")
	if len(edges) != 12 {
		t.Fatalf("want 12 shared edge segments, got %d", len(edges))
	}
	x, y := cells[0].Bounds.Max.X, cells[0].Bounds.Min.Y+10
	blue := color.RGBA{0, 0, 255, 255}
	if got := r.Image.RGBAAt(x, y); got != blue {
		t.Fatal(got)
	}
	if r.Image.RGBAAt(x-1, y) == blue || r.Image.RGBAAt(x+1, y) == blue {
		t.Fatal("shared border is doubled")
	}
	// Touching rows also have one horizontal stroke.
	y = cells[0].Bounds.Max.Y
	x = cells[0].Bounds.Min.X + 20
	if r.Image.RGBAAt(x, y) != blue || r.Image.RGBAAt(x, y-1) == blue || r.Image.RGBAAt(x, y+1) == blue {
		t.Fatal("horizontal shared border is doubled")
	}
}
func TestCollapsedBorderConflict(t *testing.T) {
	for _, rightWidth := range []string{"2", "4"} {
		r := renderCSS(t, `<table style="border-collapse:collapse;border:1px solid black"><tr><td style="border:2px solid red;height:30px">A</td><td style="border:`+rightWidth+`px solid blue">B</td></tr></table>`)
		cells := boxesWithTag(r, "td")
		x := cells[0].Bounds.Max.X
		y := cells[0].Bounds.Min.Y + 25
		want := color.RGBA{255, 0, 0, 255}
		if rightWidth == "4" {
			want = color.RGBA{0, 0, 255, 255}
		}
		if got := r.Image.RGBAAt(x, y); got != want {
			t.Fatalf("width %s got %v want %v", rightWidth, got, want)
		}
	}
}
func TestCollapsedReflowAndNestedLinks(t *testing.T) {
	source := `<style>table{border-collapse:collapse}td{padding:3px;border:1px solid black}</style><table><tr><td>Long description with several words to wrap</td><td style="vertical-align:bottom"><a href="https://example.com">Link</a></td></tr><tr><td><table style="border-collapse:separate"><tr><td>Nested</td></tr></table></td></tr></table>`
	for _, width := range []int{64, 200, 400} {
		r, err := Render(context.Background(), strings.NewReader(source), Options{Width: width})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Links) != 1 {
			t.Fatal(r.Links)
		}
		for _, region := range r.Links[0].Regions {
			if hit, ok := r.LinkAt(region.Min); !ok || hit.Href != "https://example.com" {
				t.Fatal("bad link hit region")
			}
		}
		bounds := r.Image.Bounds()
		for _, edge := range boxesWithTag(r, "collapsed-border") {
			if !edge.Bounds.In(bounds) {
				t.Fatalf("border outside canvas: %v / %v", edge.Bounds, bounds)
			}
		}
		if len(boxesWithTag(r, "border")) != 4 {
			t.Fatal("nested separate override was lost")
		}
	}
}
func TestIntrinsicImageWidth(t *testing.T) {
	d, err := Parse(context.Background(), strings.NewReader(`<table><tr><td><img src="x"></td><td>short</td></tr></table>`), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := d.Render(context.Background(), Options{Width: 400, Images: map[string]image.Image{"x": image.NewRGBA(image.Rect(0, 0, 200, 20))}})
	if err != nil {
		t.Fatal(err)
	}
	cells := boxesWithTag(r, "td")
	if cells[0].Bounds.Dx() <= cells[1].Bounds.Dx() {
		t.Fatal("image intrinsic width ignored")
	}
}
