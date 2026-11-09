// Copyright (c) 2026 Neomantra Corp. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0-only WITH LicenseRef-NimbleTerminal-Linking-Exception OR LicenseRef-Neomantra-Commercial

package nthtml

import (
	"image"
	"image/color"
	"testing"
)

// The transport gets a band of exactly the raster's size: an equal band as is,
// rounding excess cropped without a copy, and a short last band stretched once
// instead of being handed to the transport's kernel scaler.
func TestFitBandMatchesRasterSize(t *testing.T) {
	band := image.NewRGBA(image.Rect(0, 0, 40, 22))
	for y := 0; y < 22; y++ {
		for x := 0; x < 40; x++ {
			band.Set(x, y, color.RGBA{R: uint8(y * 10), A: 255})
		}
	}
	if got := fitBand(band, image.Pt(40, 22)); got != image.Image(band) {
		t.Fatal("equal-sized band was not passed through")
	}
	if got := fitBand(band, image.Point{}); got != image.Image(band) {
		t.Fatal("unknown raster must leave the band alone")
	}
	cropped, ok := fitBand(band, image.Pt(40, 20)).(*image.RGBA)
	if !ok || cropped.Bounds() != image.Rect(0, 0, 40, 20) || &cropped.Pix[0] != &band.Pix[0] {
		t.Fatalf("excess rows should be cropped as a sub-image: %v", cropped)
	}
	stretched, ok := fitBand(band, image.Pt(40, 24)).(*image.RGBA)
	if !ok || stretched.Bounds() != image.Rect(0, 0, 40, 24) {
		t.Fatalf("short band should be stretched to the raster: %v", stretched)
	}
	top, bottom := stretched.RGBAAt(0, 0), stretched.RGBAAt(0, 23)
	if top.R > 10 || bottom.R < 200 || stretched.Opaque() != true {
		t.Fatalf("stretch lost the gradient or opacity: top %v bottom %v", top, bottom)
	}
}
