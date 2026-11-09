// Copyright (c) 2026 Neomantra Corp. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0-only WITH LicenseRef-NimbleTerminal-Linking-Exception OR LicenseRef-Neomantra-Commercial

package document

import (
	"context"
	"image"
	"strings"
	"testing"
)

// inkRight is the rightmost column of img whose pixels differ from the canvas
// colour at x=0,y=0, or -1.
func inkRight(img *image.RGBA) int {
	bg := img.RGBAAt(0, 0)
	right := -1
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Max.X - 1; x > right; x-- {
			if img.RGBAAt(x, y) != bg {
				right = x
				break
			}
		}
	}
	return right
}

// Text painted at a device scale keeps the layout's glyph positions: words end
// where the 1x layout put them, times the scale, instead of following the
// scaled face's own hinted advances, which ran words together at 2x.
func TestPaintAtScaleKeepsLayoutAdvances(t *testing.T) {
	for _, html := range []string{
		`<p style="font:13px Verdana, Geneva, sans-serif;margin:0;white-space:nowrap">Humanswillneveragaincreateaprojectlike</p>`,
		`<p style="font:10pt Verdana, Geneva, sans-serif;margin:0;white-space:nowrap">RiverSourceControlisbacktojoinKrisandNick</p>`,
		`<p style="font:bold 11px sans-serif;margin:0;white-space:nowrap;letter-spacing:1px">RiverSourceControlisbacktojoinKris</p>`,
		`<p style="font:15px monospace;margin:0;white-space:nowrap;font-variant:small-caps">TheGoodtheBadandtheSloppy</p>`,
		`<p style="font:17px serif;margin:0;white-space:nowrap">Fallthroughisalongformconversationaboutsoftware</p>`,
	} {
		r, err := Render(context.Background(), strings.NewReader(html), Options{Width: 900, NoImage: true})
		if err != nil {
			t.Fatal(err)
		}
		area := r.Bounds()
		one, err := r.Paint(context.Background(), area, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, scale := range []float64{2, 3, 1.5} {
			big, err := r.Paint(context.Background(), area, scale)
			if err != nil {
				t.Fatal(err)
			}
			want := float64(inkRight(one)+1) * scale
			got := float64(inkRight(big) + 1)
			t.Logf("%s at %gx: ink %g, want %g", html[:40], scale, got, want)
			if got < want-scale || got > want+scale {
				t.Errorf("%s at %gx: ink ends at %g px, 1x layout ends at %g px (×%g = %g)", html[:60], scale, got, float64(inkRight(one)+1), scale, want)
			}
		}
	}
}
