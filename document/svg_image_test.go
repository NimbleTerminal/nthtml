package document

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
)

func TestSVGImageAllowlist(t *testing.T) {
	img := image.NewRGBA(image.Rect(5, 7, 7, 9))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
	for _, attr := range []string{`href="asset.png"`, `xlink:href="asset.png"`, `src="asset.png"`} {
		src := `<svg width="20" height="20" viewBox="0 0 20 20"><rect width="20" height="20" fill="blue"/><g transform="translate(2 3)"><image ` + attr + ` width="10" height="10"/></g><rect x="8" y="9" width="4" height="4" fill="lime"/></svg>`
		r, err := Render(context.Background(), strings.NewReader(src), Options{Width: 40, SVGImages: map[string]image.Image{"asset.png": img}})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Warnings) > 0 {
			t.Fatalf("%s: %v, %v", attr, err, r.Warnings)
		}
		for _, point := range []struct {
			x, y int
			want color.RGBA
		}{{0, 0, color.RGBA{B: 255, A: 255}}, {4, 5, color.RGBA{R: 255, A: 255}}, {9, 10, color.RGBA{G: 255, A: 255}}, {15, 15, color.RGBA{B: 255, A: 255}}} {
			if got := r.Image.RGBAAt(point.x, point.y); got != point.want {
				t.Fatalf("%s at %d,%d: got %v want %v", attr, point.x, point.y, got, point.want)
			}
		}
		copy := r.Options()
		delete(copy.SVGImages, "asset.png")
		if r.Options().SVGImages["asset.png"] == nil {
			t.Fatal("Options leaked map")
		}
	}
}

func TestSVGImagePolicy(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	images := map[string]image.Image{"allowed": img}
	for _, content := range []string{
		`<image href="https://example.test/not-allowed" width="10" height="10"/>`,
		`<image href="allowed" onload="bad()"/>`,
		`<use href="allowed"/>`,
		`<defs><image href="allowed"/></defs>`,
		`<unknown><image href="allowed"/></unknown>`,
		`<image href="allowed" fill="url(https://example.test/x)"/>`,
	} {
		if _, _, err := rewriteSVGWithImages(context.Background(), `<svg>`+content+`</svg>`, images); err == nil {
			t.Fatalf("accepted %s", content)
		}
	}
	if _, _, err := rewriteSVGWithImages(context.Background(), `<svg><image href="allowed"/></svg>`, nil); err == nil {
		t.Fatal("default permitted image href")
	}
}

func TestSVGImageSizingVisibilityAndOpacity(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
	for _, tc := range []struct {
		attrs, parent string
		x, y          int
		want          color.RGBA
	}{
		{``, ``, 5, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{``, ``, 5, 5, color.RGBA{R: 255, A: 255}},
		{`preserveAspectRatio="none"`, ``, 5, 1, color.RGBA{R: 255, A: 255}},
		{`style="display:none"`, ``, 5, 5, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{``, `style="display:none"`, 5, 5, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{`opacity="0"`, ``, 5, 5, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
	} {
		r, err := Render(context.Background(), strings.NewReader(`<svg width="10" height="10"><g `+tc.parent+`><image href="allowed" width="10" height="10" `+tc.attrs+`/></g></svg>`), Options{Width: 40, SVGImages: map[string]image.Image{"allowed": img}})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Warnings) > 0 {
			t.Fatalf("%+v: %v %v", tc, err, r.Warnings)
		}
		if got := r.Image.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Fatalf("%+v got %v", tc, got)
		}
	}
}

func TestSVGImageLimits(t *testing.T) {
	images := map[string]image.Image{"huge": image.NewUniform(color.Black)}
	if _, _, err := rewriteSVGWithImages(context.Background(), `<svg><image href="huge" width="1" height="1"/></svg>`, images); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected image dimension limit, got %v", err)
	}
	images["huge"] = image.NewRGBA(image.Rect(0, 0, 1, 1))
	source := `<svg>` + strings.Repeat(`<image href="huge"/>`, 2048) + `</svg>`
	if _, _, err := rewriteSVGWithImages(context.Background(), source, images); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected element limit, got %v", err)
	}
}

func TestSVGImageSources(t *testing.T) {
	d, err := Parse(context.Background(), strings.NewReader(`<svg><image href="a"/><image xlink:href="b"/><image src="a"/></svg>`), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	sources := d.ImageSourceList()
	if len(sources) != 2 || sources[0] != (ImageSource{Src: "a", Kind: "svg-image"}) || sources[1] != (ImageSource{Src: "b", Kind: "svg-image"}) {
		t.Fatal(sources)
	}
}
