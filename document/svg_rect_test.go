package document

import (
	"bytes"
	"image/color"
	"testing"
)

func TestInlineSVGRectAutoRadius(t *testing.T) {
	rect := func(attrs string) *Result {
		return render(t, `<svg width="40" height="40" viewBox="0 0 40 40"><rect width="40" height="40" fill="white"/><rect width="40" height="40" fill="red" `+attrs+`/></svg>`)
	}
	rounded := rect(`rx="15" ry="15"`)
	for _, attr := range []string{`rx="15"`, `ry="15"`} {
		t.Run(attr, func(t *testing.T) {
			r := rect(attr)
			if !bytes.Equal(r.Image.Pix, rounded.Image.Pix) {
				t.Fatal("single radius differs from explicit rx and ry")
			}
			if got := r.Image.RGBAAt(1, 1); got != (color.RGBA{255, 255, 255, 255}) {
				t.Fatalf("corner = %v, want white", got)
			}
			if got := r.Image.RGBAAt(20, 20); got != (color.RGBA{255, 0, 0, 255}) {
				t.Fatalf("center = %v, want red", got)
			}
		})
	}
}
