package document

import (
	"context"
	"testing"
)

func TestMediaWidthRanges(t *testing.T) {
	for _, tt := range []struct {
		query string
		width int
		want  bool
	}{
		{"(width>=1012px)", 1012, true}, {"(width>=1012px)", 1011, false},
		{"(width<=1011.98px)", 1011, true}, {"(width<=1011.98px)", 1012, false},
		{"(1012px <= width)", 1200, true}, {"(1200px > width)", 1200, false},
		{"(400px <= width < 800px)", 400, true}, {"(400px <= width < 800px)", 800, false},
		{"(800px >= width > 400px)", 800, true}, {"(800px >= width > 400px)", 400, false},
		{"(width = 800px)", 800, true}, {"(width > -1px)", 800, true},
		{"not (width > 800px)", 800, true}, {"screen and (width>=800px)", 800, true},
		{"not (width >< 800px)", 800, false}, {"not (400px < width > 800px)", 800, false},
		{"not (height > 800px)", 800, false}, {"not (width == 800px)", 800, false},
		{"not (width >=)", 800, false}, {"not (width < width)", 800, false},
		{"(width<=calc(48rem - .02px))", 767, true}, {"(width<=calc(48rem - .02px))", 768, false},
		{"(width >= calc(48rem))", 768, true}, {"(min-width: calc(40rem + 128px))", 768, true},
		{"(width <= calc(max(48rem, 800px)))", 800, true}, {"not (width <= calc(min(max(48rem, (800px)))))", 800, false},
		{"not (width <= calc(100% - 8px))", 800, false}, {"(width <= calc(48rem - .02px)", 700, false},
	} {
		if got := evalMediaQueryList(tt.query, tt.width, "light"); got != tt.want {
			t.Errorf("%s at %d = %v want %v", tt.query, tt.width, got, tt.want)
		}
	}
}

func TestResponsiveHeaderRangeQueries(t *testing.T) {
	d := parseFeature(t, `<style>.menu{display:block}@media(width<=1011.98px){.menu{position:absolute;visibility:hidden}}@media(width>=1012px){.menu{display:flex;gap:12px}}</style><nav class="menu"><a href="/products">Products</a><a href="/developers">Developers</a></nav><p>Repository</p>`)
	for _, width := range []int{800, 1200, 800} {
		r, err := d.Render(context.Background(), Options{Width: width, NoImage: true})
		if err != nil {
			t.Fatal(err)
		}
		if width < 1012 {
			if len(r.Links) != 0 {
				t.Fatal("closed mobile menu remains interactive")
			}
			continue
		}
		if len(r.Links) != 2 {
			t.Fatalf("desktop links: %+v", r.Links)
		}
		a, b := r.Links[0].Regions[0], r.Links[1].Regions[0]
		if b.Min.X <= a.Min.X || b.Min.Y != a.Min.Y {
			t.Fatalf("desktop header not horizontal: %v %v", a, b)
		}
	}
}
