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

// Authored visibility probe, separate from the pinned WPT status baseline.
// Compare only the image's painted pixels: text metrics/box layout still differ.
func TestTypesetImageVisibilityChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium visibility explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	red := color.RGBA{R: 237, G: 11, B: 29, A: 255}
	asset := image.NewRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			asset.SetRGBA(x, y, red)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, asset); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{"asset.png": &fstest.MapFile{Data: data.Bytes()}}
	cases := []struct {
		name, body string
		pixels     int
	}{
		{"visible", `<p><a href="#target"><img src="asset.png" width="12" height="8"></a></p>`, 96},
		{"image-hidden", `<p><img style="display:none" src="asset.png" width="12" height="8"></p>`, 0},
		{"link-hidden", `<style>.hidden{display:none}</style><p><a class="hidden"><img style="display:block" src="asset.png" width="12" height="8"></a></p>`, 0},
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
	count := func(pic *image.RGBA) int {
		n := 0
		for y := pic.Bounds().Min.Y; y < pic.Bounds().Max.Y; y++ {
			for x := pic.Bounds().Min.X; x < pic.Bounds().Max.X; x++ {
				if pic.RGBAAt(x, y) == red {
					n++
				}
			}
		}
		return n
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
			browserPixels, typesetPixels := count(reference), count(actual)
			if browserPixels != tc.pixels || typesetPixels != browserPixels {
				t.Fatalf("image pixels: Chromium=%d typeset=%d want=%d", browserPixels, typesetPixels, tc.pixels)
			}
		})
	}
}
