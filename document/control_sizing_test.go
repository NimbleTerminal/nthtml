package document

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
)

func TestControlBoxSizingDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, html string
		height     int
	}{
		{"button", `<button style="height:30px">x</button>`, 30},
		{"select", `<select style="height:30px"><option>a</option></select>`, 30},
		{"textarea", `<textarea style="height:30px">x</textarea>`, 36},
		{"author override", `<input type=submit style="box-sizing:content-box;height:30px">`, 36},
	} {
		t.Run(tc.name, func(t *testing.T) { assertControlHeight(t, render(t, tc.html), tc.height) })
	}
	for _, typ := range strings.Fields("button submit reset image color file checkbox radio text search email url tel password number unknown") {
		t.Run(typ, func(t *testing.T) {
			want := 36
			switch typ {
			case "button", "submit", "reset", "image", "color", "file", "checkbox", "radio":
				want = 30
			}
			assertControlHeight(t, render(t, `<input type="`+typ+`" style="height:30px">`), want)
		})
	}
	t.Run("default text", func(t *testing.T) { assertControlHeight(t, render(t, `<input style="height:30px">`), 36) })
}

func assertControlHeight(t *testing.T, r *Result, want int) {
	t.Helper()
	for _, b := range r.Boxes {
		switch b.Tag {
		case "button", "select", "input", "textarea":
			if b.Bounds.Dy() != want {
				t.Fatalf("%s height = %d, want %d (%v)", b.Tag, b.Bounds.Dy(), want, b.Bounds)
			}
			return
		}
	}
	t.Fatal("missing control box")
}

func TestGoogleButtonSpriteDoesNotExposeStars(t *testing.T) {
	sprite := image.NewRGBA(image.Rect(0, 0, 167, 305))
	star := color.RGBA{255, 123, 17, 255}
	draw.Draw(sprite, sprite.Bounds(), image.NewUniform(color.RGBA{240, 240, 240, 255}), image.Point{}, draw.Src)
	draw.Draw(sprite, image.Rect(0, 291, 167, 305), image.NewUniform(star), image.Point{}, draw.Src)
	const html = `<style>
.ds{display:inline-block;margin:3px 0 4px;margin-left:4px}
.lsbb{background:#f3f5f6;border:solid 1px;border-color:#d2d2d2 #70757a #70757a #d2d2d2;height:30px;display:block}
.lsb{background:url(/images/nav_logo229.png) 0 -261px repeat-x;color:#1f1f1f;border:none;height:30px;margin:0;outline:0;font:15px sans-serif;vertical-align:top}
</style>
<span class=ds><span class=lsbb><input class=lsb value="Google Search" type=submit></span></span>`
	r, err := Render(context.Background(), strings.NewReader(html), Options{Width: 300, BackgroundImages: map[string]image.Image{"/images/nav_logo229.png": sprite}})
	if err != nil {
		t.Fatal(err)
	}
	assertControlHeight(t, r, 30)
	for y := r.Image.Rect.Min.Y; y < r.Image.Rect.Max.Y; y++ {
		for x := r.Image.Rect.Min.X; x < r.Image.Rect.Max.X; x++ {
			if r.Image.RGBAAt(x, y) == star {
				t.Fatalf("sprite star exposed at (%d,%d)", x, y)
			}
		}
	}
}
