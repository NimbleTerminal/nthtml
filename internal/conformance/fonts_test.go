package conformance

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
)

func TestPinnedAhem(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := s.fontSource(true)
	if err != nil {
		t.Fatal(err)
	}
	// At 20px Ahem's 1em square has an 80% ascent and 20% descent.
	// A 1em line box therefore produces exactly the CSS rectangle below.
	for _, tc := range []struct {
		text, css string
		width     int
	}{{"XX", "", 40}, {"X X", "word-spacing:20px;", 80}, {"X&nbsp;X", "word-spacing:20px;", 80}, {"X X", "word-spacing:-20px;", 40}} {
		html := `<div style="font:20px/1 Ahem;color:black;` + tc.css + `">` + tc.text + `</div>`
		r, err := document.Render(context.Background(), strings.NewReader(html), document.Options{Width: 100, Fonts: fonts})
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 20; y++ {
			for x := 0; x < 100; x++ {
				want := color.RGBA{255, 255, 255, 255}
				if x < 20 || x >= tc.width-20 && x < tc.width || tc.text == "XX" && x < 40 {
					want = color.RGBA{0, 0, 0, 255}
				}
				if got := r.Image.RGBAAt(x, y); got != want {
					t.Fatalf("%+v at %d,%d: %v want %v", tc, x, y, got, want)
				}
			}
		}
	}
	data, err := fs.ReadFile(s.Files, ahemTTF)
	if err != nil || !bytes.Equal(data, fonts.ahem) {
		t.Fatal("font was not pinned asset", err)
	}
	delete(s.Manifest.Files, ahemTTF)
	if _, err := s.fontSource(true); err == nil {
		t.Fatal("unpinned font accepted")
	}
}

func TestAhemStylesheetOptIn(t *testing.T) {
	html := []byte(`<link rel="stylesheet" href="/fonts/ahem.css"><link rel="match" href="ref.html">`)
	if _, err := referencesWith("css/test.html", html, nil, false); err == nil {
		t.Fatal("implicit font access")
	}
	if _, err := referencesWith("css/test.html", html, nil, true); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"https://example.com/fonts/ahem.css", "/fonts/other.css"} {
		if _, err := referencesWith("css/test.html", []byte(`<link rel="stylesheet" href="`+url+`">`), nil, true); err == nil {
			t.Fatal("unreviewed stylesheet", url)
		}
	}
}

func TestAhemLeadingAndOverflow(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := s.fontSource(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []int{0, 10, 20, 40} {
		source := fmt.Sprintf(`<div style="padding-top:40px;font:20px/%dpx Ahem;color:black">X</div>`, line)
		r, err := document.Render(context.Background(), strings.NewReader(source), document.Options{Width: 100, Fonts: fonts})
		if err != nil {
			t.Fatal(err)
		}
		var text document.Box
		for _, b := range r.Boxes {
			if b.Tag == "text" {
				text = b
			}
		}
		top := 40 + (line-20)/2
		if text.Bounds.Dy() != line || text.Baseline != 16+(line-20)/2 || text.InkBounds != image.Rect(0, top, 20, top+20) {
			t.Fatal(line, text)
		}
		if r.Image.Bounds().Max.Y < top+20 {
			t.Fatal("overflow was clipped", line)
		}
		for y := 0; y < r.Image.Bounds().Max.Y; y++ {
			want := color.RGBA{255, 255, 255, 255}
			if y >= top && y < top+20 {
				want = color.RGBA{0, 0, 0, 255}
			}
			if got := r.Image.RGBAAt(10, y); got != want {
				t.Fatal(line, y, got, want)
			}
		}
	}
}
