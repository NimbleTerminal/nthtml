package conformance

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// With collapse prevented by parent padding and other margins zero, changing
// a vertical margin by 20px must move the following ink by exactly 20px.
// Compare deltas independently; default positions/fonts still differ.
func TestTypesetMarginDeltasChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium margin probe explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	files := fstest.MapFS{}
	suite := Suite{Manifest: Manifest{Viewport: Viewport{800, 600}, Files: map[string]string{}}, Files: files}
	elements := map[string]string{
		"pre": `<pre class="target">MMMM</pre>`,
		"ul":  `<ul class="target"><li>MMMM</li></ul>`,
		"li":  `<ul style="margin:0;padding:1px"><li class="target">MMMM</li></ul>`,
	}
	for name, body := range elements {
		for _, side := range []string{"top", "bottom"} {
			for _, margin := range []int{0, 20} {
				path := fmt.Sprintf("%s-%s-%d.html", name, side, margin)
				source := fmt.Sprintf(`<!doctype html><style>body{background:white}.target{color:#d02030;margin-top:0;margin-bottom:0;margin-%s:%dpx}.after{color:#2040d0;margin:0}</style><div style="padding:1px">%s<p class="after">MMMM</p></div>`, side, margin, body)
				files[path] = &fstest.MapFile{Data: []byte(source)}
				suite.Manifest.Files[path] = fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
			}
		}
	}
	session, err := startCDPSession(ctx, suite, browser)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	firstInk := func(pic *image.RGBA, ink color.RGBA) int {
		for y := 0; y < pic.Bounds().Dy(); y++ {
			for x := 0; x < pic.Bounds().Dx(); x++ {
				if pic.RGBAAt(x, y) == ink {
					return y
				}
			}
		}
		return -1
	}
	for name := range elements {
		for _, side := range []string{"top", "bottom"} {
			t.Run(name+"/"+side, func(t *testing.T) {
				positions := map[string][]int{"Chromium": {}, "typeset": {}}
				// Top margin moves the target; bottom margin moves its following sibling.
				ink := color.RGBA{0xd0, 0x20, 0x30, 255}
				if side == "bottom" {
					ink = color.RGBA{0x20, 0x40, 0xd0, 255}
				}
				for _, margin := range []int{0, 20} {
					c := Case{Path: fmt.Sprintf("%s-%s-%d.html", name, side, margin)}
					reference, _, err := session.renderCase(ctx, c, nil)
					if err != nil {
						t.Fatal(err)
					}
					actual, _, err := suite.renderTypesetWithBoxes(ctx, c)
					if err != nil {
						t.Fatal(err)
					}
					for backend, pic := range map[string]*image.RGBA{"Chromium": reference, "typeset": actual} {
						y := firstInk(pic, ink)
						if y < 0 {
							t.Fatal("missing ink", backend, c.Path)
						}
						positions[backend] = append(positions[backend], y)
					}
				}
				for backend, p := range positions {
					if p[1]-p[0] != 20 {
						t.Errorf("%s %s: ink moved %d pixels, want 20 (%v)", backend, strings.Join([]string{name, side}, "/"), p[1]-p[0], p)
					}
				}
			})
		}
	}
}
