package document

import (
	"context"
	"image/color"
	"strings"
	"testing"
)

func TestHoverCascadeIsImmutableAndIncludesAncestors(t *testing.T) {
	d := parseFeature(t, `<style>a{color:black}a:hover{color:red}nav:hover a{background:blue}nav:has(a:hover){padding:10px}</style><nav id="nav"><a id="link" href="/x">link</a></nav>`)
	for _, id := range []string{"link", "", "link", "missing", ""} {
		r, err := d.Render(context.Background(), Options{Width: 400, NoImage: true, HoverSource: id})
		if err != nil {
			t.Fatal(err)
		}
		want := color.RGBA{0, 0, 0, 255}
		if id == "link" {
			want = color.RGBA{255, 0, 0, 255}
		}
		found := false
		for _, b := range r.Boxes {
			if b.Tag == "text" && strings.Contains(b.Text, "link") {
				found = true
				if b.Color != want {
					t.Fatalf("hover %q: %v want %v", id, b.Color, want)
				}
			}
		}
		if !found {
			t.Fatal("missing text")
		}
		if id == "link" && r.Links[0].Regions[0].Min.X < 10 {
			t.Fatal("ancestor :has(:hover) did not alter layout")
		}
	}
}
