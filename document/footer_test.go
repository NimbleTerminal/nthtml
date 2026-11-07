package document

import (
	"context"
	"image/color"
	"os"
	"strings"
	"testing"
)

func TestFooterIntrinsicFlexAndSVG(t *testing.T) {
	src, err := os.ReadFile("testdata/footer.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{978, 390} {
		r, err := Render(context.Background(), strings.NewReader(string(src)), Options{Width: width})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Warnings) > 0 {
			t.Fatal(r.Warnings)
		}
		var github, copyright, built, with Box
		var svgs []Box
		for _, b := range r.Boxes {
			if b.Text == "GitHub" {
				github = b
			}
			if strings.Contains(b.Text, "2026") {
				copyright = b
			}
			if strings.Contains(b.Text, "Built") {
				built = b
			}
			if b.Text == "with" {
				with = b
			}

		}
		for _, op := range r.ops {
			if op.box.Tag == "svg" && op.source != nil {
				svgs = append(svgs, op.box)
			}
		}

		if github.Text == "" || built.Text == "" || with.Text == "" {
			t.Fatal("missing footer text")
		}
		if github.Bounds.Min.Y != copyright.Bounds.Min.Y {
			t.Fatalf("%d: GitHub wraps", width)
		}
		if with.Bounds.Min.Y != built.Bounds.Min.Y {
			t.Fatalf("%d: slogan wraps: %v %v", width, built.Bounds, with.Bounds)
		}
		if width >= 768 && built.Bounds.Min.X < width/2 {
			t.Fatalf("slogan not right-aligned: %v", built.Bounds)
		}
		if len(svgs) != 2 || svgs[0].Bounds.Dx() != 18 || svgs[1].Bounds.Dx() != 22 {
			t.Fatalf("icon sizes: %v", svgs)
		}
		heart := svgs[0].Bounds
		got := r.Image.RGBAAt(heart.Min.X+heart.Dx()/2, heart.Min.Y+heart.Dy()/2)
		if got.R < 240 || got.G > 80 || got.B > 60 {
			t.Fatalf("heart color: %v", got)
		}
	}
}

func TestSVGCurrentColorInheritance(t *testing.T) {
	r := renderCSS(t, `<div style="color:#fc3e1f"><svg width="30" height="10" viewBox="0 0 30 10"><rect width="10" height="10" fill="currentColor"/><g color="blue"><rect x="10" width="10" height="10" style="fill:currentColor"/></g><rect x="20" width="10" height="10" fill="green"/></svg></div>`)
	b := tagged(r, "svg").Bounds
	for i, want := range []color.RGBA{{252, 62, 31, 255}, {0, 0, 255, 255}, {0, 128, 0, 255}} {
		if got := r.Image.RGBAAt(b.Min.X+i*10+5, b.Min.Y+5); got != want {
			t.Fatalf("%d: %v want %v", i, got, want)
		}
	}
	src := `<svg xmlns="http://www.w3.org/2000/svg"><title>currentColor</title><path id="currentColor" fill="currentColor"/><use href="#currentColor"/></svg>`
	out, err := resolveSVGCurrentColor(context.Background(), src, color.RGBA{255, 0, 0, 255})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`id="currentColor"`, `href="#currentColor"`, `<title>currentColor</title>`, `fill="#ff0000"`} {
		if !strings.Contains(out, text) {
			t.Fatalf("unexpected rewrite: %s", out)
		}
	}
	// Color resolution must not turn external references into admitted resources.
	_, _, err = rewriteSVGWithImages(context.Background(), `<svg><path fill="url(https://example.com/x)"/></svg>`, nil)
	if err == nil {
		t.Fatal("external reference admitted")
	}
}
