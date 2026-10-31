package nthtml

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"testing"

	xdraw "golang.org/x/image/draw"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

func TestFocusPatchLetterboxGeometry(t *testing.T) {
	for _, tc := range []struct {
		name               string
		crop, region, page image.Rectangle
	}{
		{"top", image.Rect(0, 0, 100, 70), image.Rect(20, 0, 40, 5), image.Rect(0, 15, 100, 85)},
		{"bottom", image.Rect(0, 0, 100, 70), image.Rect(20, 65, 40, 70), image.Rect(0, 15, 100, 85)},
		{"left", image.Rect(0, 0, 70, 100), image.Rect(0, 20, 5, 40), image.Rect(15, 0, 85, 100)},
		{"right", image.Rect(0, 0, 70, 100), image.Rect(65, 20, 70, 40), image.Rect(15, 0, 85, 100)},
		{"middle", image.Rect(0, 0, 100, 70), image.Rect(20, 25, 40, 35), image.Rect(0, 15, 100, 85)},
		{"scaled", image.Rect(10, 20, 210, 160), image.Rect(50, 20, 90, 30), image.Rect(0, 15, 100, 85)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cells, pixels, cellSize := image.Pt(10, 5), image.Pt(100, 100), image.Pt(10, 20)
			cr, page := focusBandCellsAndPage([]image.Rectangle{tc.region}, cells, pixels, tc.crop)
			if page != tc.page || cr.Empty() {
				t.Fatalf("cells %v page %v, want page %v", cr, page, tc.page)
			}
			src := image.NewRGBA(tc.crop)
			for y := tc.crop.Min.Y; y < tc.crop.Max.Y; y++ {
				for x := tc.crop.Min.X; x < tc.crop.Max.X; x++ {
					src.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 123, 255})
				}
			}
			bg := color.RGBA{13, 21, 34, 255}
			patch := paintFocusPatch(src, []image.Rectangle{tc.region}, tc.crop, page, cr, cells, pixels, cellSize, bg)
			if patch.Bounds().Dx() != cr.Dx()*10 || patch.Bounds().Dy() != cr.Dy()*20 {
				t.Fatal("patch does not cover full cells at base scale")
			}
			// Independent full-page rendering with the old source-pixel ring provides
			// the reference. Cropping the output must match, including scaler samples.
			decorated := image.NewRGBA(tc.crop)
			draw.Draw(decorated, tc.crop, src, tc.crop.Min, draw.Src)
			r := tc.region.Inset(-1)
			ink := image.NewUniform(color.RGBA{235, 155, 35, 255})
			for _, edge := range []image.Rectangle{image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)} {
				draw.Draw(decorated, edge, ink, image.Point{}, draw.Src)
			}
			full := image.NewRGBA(image.Rect(0, 0, 100, 100))
			draw.Draw(full, full.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
			xdraw.CatmullRom.Scale(full, page, decorated, tc.crop, draw.Over, nil)
			for y := patch.Bounds().Min.Y; y < patch.Bounds().Max.Y; y++ {
				for x := patch.Bounds().Min.X; x < patch.Bounds().Max.X; x++ {
					if got, want := patch.RGBAAt(x, y), full.RGBAAt(x, y); got != want {
						t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
					}
				}
			}
		})
	}
}

func decodeFocusPNG(t *testing.T, raw string) image.Image {
	t.Helper()
	var payload strings.Builder
	for _, chunk := range strings.Split(raw, "\x1b_G")[1:] {
		control, data, ok := strings.Cut(chunk, ";")
		if !ok {
			continue
		}
		if strings.Contains(control, "a=d") {
			continue
		}
		data, _, _ = strings.Cut(data, "\x1b\\")
		payload.WriteString(data)
	}
	data, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode Kitty PNG: %v", err)
	}
	return img
}

func TestKittyFocusPreservesLetterboxPixels(t *testing.T) {
	for _, bg := range []color.Color{color.RGBA{13, 21, 34, 255}, color.Transparent} {
		src := image.NewRGBA(image.Rect(0, 0, 300, 170))
		draw.Draw(src, src.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		// A distinctive stripe makes any vertical stretch visible in the assertion.
		draw.Draw(src, image.Rect(0, 5, 300, 7), image.NewUniform(color.RGBA{0, 0, 255, 255}), image.Point{}, draw.Src)
		region := image.Rect(30, 0, 70, 10)
		m := NewWithOptions(&document.Result{Image: src, Boxes: []document.Box{{SourceID: "top", Bounds: region}}}, ModelOptions{Picture: ntpicture.Options{StableID: true, RetireDelay: -1, Render: ntpicture.Config{Background: bg, KittyFormat: ntpicture.KittyFormatPNG}}})
		t.Cleanup(func() { driveRaw(t, m, m.Close()) })
		driveRaw(t, m, m.initial, m.Picture().SetTerminal(true, 10, 20), m.SetSize(30, 10))
		base := decodeFocusPNG(t, driveRaw(t, m, m.presentSource()))
		cmd, ok := m.ShowFocusSource("top")
		if !ok {
			t.Fatal("focus source missing")
		}
		overlay := decodeFocusPNG(t, driveRaw(t, m, cmd))
		origin := image.Pt(m.focusCellRect.Min.X*10, m.focusCellRect.Min.Y*20)
		if overlay.Bounds().Size() != image.Pt(m.focusCellRect.Dx()*10, m.focusCellRect.Dy()*20) {
			t.Fatal("wrong overlay pixel size")
		}
		for y := 0; y < overlay.Bounds().Dy(); y++ {
			for x := 0; x < overlay.Bounds().Dx(); x++ {
				p := origin.Add(image.Pt(x, y))
				got := color.RGBAModel.Convert(overlay.At(x, y)).(color.RGBA)
				if got == (color.RGBA{235, 155, 35, 255}) {
					continue
				}
				want := color.RGBAModel.Convert(base.At(p.X, p.Y)).(color.RGBA)
				if got != want {
					t.Fatalf("focus changed base pixel at %v: %v != %v", p, got, want)
				}
			}
		}
	}
}
