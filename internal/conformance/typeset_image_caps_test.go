package conformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
	"time"
)

// Compare painted image dimensions, not page positions or text metrics.
func TestTypesetImageCapsChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium image sizing explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	red := color.RGBA{R: 237, G: 11, B: 29, A: 255}
	asset := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			asset.SetRGBA(x, y, red)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, asset); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{"asset.png": &fstest.MapFile{Data: data.Bytes()}}
	cases := []struct {
		name, body    string
		width, height int
	}{
		{"width", `<p><img src="asset.png" style="max-width:80px"></p>`, 80, 40},
		{"height", `<p><img src="asset.png" style="max-height:30px"></p>`, 60, 30},
		{"both", `<p><img src="asset.png" style="max-width:80px;max-height:30px"></p>`, 60, 30},
		{"larger", `<p><img src="asset.png" style="max-width:400px;max-height:400px"></p>`, 200, 100},
		{"em", `<style>img{font-size:20px;max-width:4em}</style><p><img src="asset.png"></p>`, 80, 40},
		{"width-smaller", `<p><img src="asset.png" style="width:40px;max-width:80px"></p>`, 40, 20},
		{"non-inherited", `<p style="max-width:80px"><img src="asset.png"></p>`, 200, 100},
	}
	for _, tc := range cases {
		files[tc.name+".html"] = &fstest.MapFile{Data: []byte(`<!doctype html><body><p>before</p>` + tc.body + `<p>after</p></body>`)}
	}
	suite := Suite{Files: files, Manifest: Manifest{Viewport: Viewport{800, 600}, Files: map[string]string{}}}
	for name, f := range files {
		suite.Manifest.Files[name] = fmt.Sprintf("%x", sha256.Sum256(f.Data))
	}
	session, err := startCDPSession(ctx, suite, browser)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	size := func(pic *image.RGBA) image.Point {
		bounds := image.Rectangle{}
		for y := pic.Bounds().Min.Y; y < pic.Bounds().Max.Y; y++ {
			for x := pic.Bounds().Min.X; x < pic.Bounds().Max.X; x++ {
				if pic.RGBAAt(x, y) == red {
					bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return bounds.Size()
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Case{Path: tc.name + ".html", Assets: []string{"asset.png"}}
			reference, _, err := session.renderCase(ctx, c, nil)
			if err != nil {
				t.Fatal(err)
			}
			actual, _, err := suite.renderTypesetWithBoxes(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			browserSize, typesetSize := size(reference), size(actual)
			want := image.Pt(tc.width, tc.height)
			if browserSize != want || typesetSize != browserSize {
				t.Fatalf("painted image: Chromium=%v typeset=%v want=%v", browserSize, typesetSize, want)
			}
		})
	}
}
