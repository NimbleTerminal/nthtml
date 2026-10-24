package document

import (
	"image/color"
	"testing"
)

func TestDefiniteHeightAndVisibleOverflow(t *testing.T) {
	for _, css := range []string{"height:0;height:-1px", "height:5px", "height:0;padding:3px;border:1px solid blue", "height:8px;box-sizing:border-box;padding:3px;border:1px solid blue"} {
		r := renderCSS(t, `<article style="background:red;`+css+`">Overflow</article><section style="height:1px"></section>`)
		box, next, text := tagged(r, "article"), tagged(r, "section"), tagged(r, "text")
		if next.Bounds.Min.Y != box.Bounds.Max.Y {
			t.Fatalf("%s: sibling displaced by overflow", css)
		}
		if text.Bounds.Max.Y <= box.Bounds.Max.Y {
			t.Fatalf("%s: expected overflowing text", css)
		}
		if r.Image.Bounds().Max.Y < text.Bounds.Max.Y {
			t.Fatalf("%s: canvas clips overflow", css)
		}
		if got := r.Image.RGBAAt(399, box.Bounds.Max.Y+1); got != (color.RGBA{255, 255, 255, 255}) {
			t.Fatalf("%s: background extends into overflow: %v", css, got)
		}
	}
}

func TestHeightBoxModel(t *testing.T) {
	for _, tc := range []struct {
		css    string
		height int
	}{
		{"height:0;height:-1px", 0}, {"height:5px", 5},
		{"height:0;padding:3px;border:1px solid blue", 8},
		{"height:8px;box-sizing:border-box;padding:3px;border:1px solid blue", 8},
	} {
		r := renderCSS(t, `<article style="`+tc.css+`">Text</article>`)
		if got := tagged(r, "article").Bounds.Dy(); got != tc.height {
			t.Errorf("%s: height %d, want %d", tc.css, got, tc.height)
		}
	}
	r := renderCSS(t, `<article style="height:10px"><section style="height:50%">Overflow</section></article>`)
	if got := tagged(r, "section").Bounds.Dy(); got != 5 {
		t.Fatalf("percentage height %d", got)
	}
}
