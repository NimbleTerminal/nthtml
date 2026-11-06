package conformance

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

//go:embed testdata/authored/callout-boxes.html
var calloutBoxes string

// Adding explicit default padding must be a no-op. This tests equivalence
// within each renderer, not absolute browser geometry/font conformance.
func TestTypesetCalloutDefaultPaddingChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium callout explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	files := fstest.MapFS{}
	suite := Suite{Manifest: Manifest{Viewport: Viewport{800, 600}, Files: map[string]string{}}, Files: files}
	for name, html := range map[string]string{"implicit.html": calloutBoxes, "explicit.html": strings.Replace(calloutBoxes, "/* explicit-zero-padding */", "h2, p { padding: 0; }", 1)} {
		files[name] = &fstest.MapFile{Data: []byte(html)}
		suite.Manifest.Files[name] = fmt.Sprintf("%x", sha256.Sum256([]byte(html)))
	}
	session, err := startCDPSession(ctx, suite, browser)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	var chromium, typeset []*image.RGBA
	for _, path := range []string{"implicit.html", "explicit.html"} {
		c := Case{Path: path}
		pic, _, err := session.renderCase(ctx, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		chromium = append(chromium, pic)
		pic, _, err = suite.renderTypesetWithBoxes(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		typeset = append(typeset, pic)
	}
	for backend, pics := range map[string][]*image.RGBA{"Chromium": chromium, "typeset": typeset} {
		pixels, _, _ := Compare(pics[0], pics[1])
		if pixels != 0 {
			t.Errorf("%s: explicit default padding changed %d pixels", backend, pixels)
		}
	}
}
