package conformance

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"testing"
	"testing/fstest"
	"time"
)

//go:embed testdata/authored/table-cells.html
var tableCells []byte

// A fixed fixture compares cell paint dimensions, not overall table placement
// or the general CSS column-sizing algorithm. Text uses different fonts.
func TestTypesetCellPaintChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium cell paint explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suite := Suite{Manifest: Manifest{Viewport: Viewport{800, 600}, Files: map[string]string{"cells.html": fmt.Sprintf("%x", sha256.Sum256(tableCells))}}, Files: fstest.MapFS{"cells.html": &fstest.MapFile{Data: tableCells}}}
	session, err := startCDPSession(ctx, suite, browser)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	c := Case{Path: "cells.html"}
	reference, _, err := session.renderCase(ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	actual, _, err := suite.renderTypesetWithBoxes(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	bounds := func(pic *image.RGBA, ink color.RGBA) image.Rectangle {
		var r image.Rectangle
		for y := 0; y < pic.Bounds().Dy(); y++ {
			for x := 0; x < pic.Bounds().Dx(); x++ {
				if pic.RGBAAt(x, y) == ink {
					r = r.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return r
	}
	for _, tc := range []struct {
		ink  color.RGBA
		size image.Point
	}{
		{color.RGBA{0xd0, 0x20, 0x30, 255}, image.Pt(338, 56)},
		{color.RGBA{0x20, 0xb0, 0x50, 255}, image.Pt(338, 56)},
		{color.RGBA{0x30, 0x50, 0xd0, 255}, image.Pt(344, 50)},
		{color.RGBA{0xd0, 0x90, 0x20, 255}, image.Pt(344, 50)},
	} {
		want, got := bounds(reference, tc.ink), bounds(actual, tc.ink)
		if want.Size() != tc.size || got.Size() != want.Size() {
			t.Errorf("color %v: Chromium=%v typeset=%v expected size=%v", tc.ink, want, got, tc.size)
		}
	}
}
