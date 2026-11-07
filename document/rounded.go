package document

import (
	"image"
	"image/color"
	"math"
	"strings"
)

var radiusNames = [4]string{"border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"}

func radiusValues(v string, shorthand bool) ([4][2]cssLength, bool) {
	var out [4][2]cssLength
	parts := strings.Split(v, "/")
	if len(parts) > 2 || (!shorthand && len(parts) > 1) {
		return out, false
	}
	if !shorthand {
		t, ok := cssTokens(v)
		if !ok || len(t) < 1 || len(t) > 2 {
			return out, false
		}
		if len(t) == 1 {
			t = append(t, t[0])
		}
		for j := range 2 {
			x, ok := parseLength(t[j], false, false)
			if !ok || x.value < 0 {
				return out, false
			}
			out[0][j] = x
		}
		return out, true
	}
	for axis := range 2 {
		part := parts[0]
		if axis == 1 && len(parts) == 2 {
			part = parts[1]
		}
		t, ok := cssTokens(part)
		if !ok || len(t) < 1 || len(t) > 4 {
			return out, false
		}
		var vals [4]cssLength
		for i, v := range t {
			x, ok := parseLength(v, false, false)
			if !ok || x.value < 0 {
				return out, false
			}
			vals[i] = x
		}
		switch len(t) {
		case 1:
			vals[1], vals[2], vals[3] = vals[0], vals[0], vals[0]
		case 2:
			vals[2], vals[3] = vals[0], vals[1]
		case 3:
			vals[3] = vals[1]
		}
		for i := range 4 {
			out[i][axis] = vals[i]
		}
	}
	return out, true
}

// Radii are resolved against the border box and reduced together when adjacent
// corners overlap, as required by CSS Backgrounds and Borders.
func (s style) rounded(r image.Rectangle) [4][2]float64 {
	var v [4][2]float64
	for i := range 4 {
		v[i][0] = float64(max(0, s.radii[i][0].resolve(r.Dx(), 1<<30)))
		v[i][1] = float64(max(0, s.radii[i][1].resolve(r.Dy(), 1<<30)))
	}
	f := 1.0
	for _, a := range [][3]float64{{float64(r.Dx()), v[0][0], v[1][0]}, {float64(r.Dx()), v[3][0], v[2][0]}, {float64(r.Dy()), v[0][1], v[3][1]}, {float64(r.Dy()), v[1][1], v[2][1]}} {
		if a[1]+a[2] > 0 {
			f = min(f, a[0]/(a[1]+a[2]))
		}
	}
	for i := range 4 {
		for j := range 2 {
			v[i][j] *= f
		}
	}
	return v
}

func roundContains(r image.Rectangle, radii [4][2]float64, x, y float64) bool {
	if x < float64(r.Min.X) || y < float64(r.Min.Y) || x >= float64(r.Max.X) || y >= float64(r.Max.Y) {
		return false
	}
	for i, v := range radii {
		rx, ry := v[0], v[1]
		if rx == 0 || ry == 0 {
			continue
		}
		cx, cy := float64(r.Min.X)+rx, float64(r.Min.Y)+ry
		right, bottom := i == 1 || i == 2, i >= 2
		if right {
			cx = float64(r.Max.X) - rx
		}
		if bottom {
			cy = float64(r.Max.Y) - ry
		}
		if (right && x > cx || !right && x < cx) && (bottom && y > cy || !bottom && y < cy) {
			dx, dy := (x-cx)/rx, (y-cy)/ry
			return dx*dx+dy*dy <= 1
		}
	}
	return true
}

// A procedural, band-local mask: no page-sized raster or mask allocation.
type roundedMask struct {
	b      Box
	area   image.Rectangle
	scale  float64
	bounds image.Rectangle
}

func (m roundedMask) ColorModel() color.Model { return color.AlphaModel }
func (m roundedMask) Bounds() image.Rectangle { return m.bounds }
func (m roundedMask) At(x, y int) color.Color {
	n := 0
	for _, dy := range []float64{.25, .75} {
		for _, dx := range []float64{.25, .75} {
			px, py := float64(m.area.Min.X)+(float64(x)+dx)/m.scale, float64(m.area.Min.Y)+(float64(y)+dy)/m.scale
			if roundContains(m.b.Bounds, m.b.radii, px, py) && (m.b.roundInner.Empty() || !roundContains(m.b.roundInner, m.b.innerRadii, px, py)) {
				n++
			}
		}
	}
	return color.Alpha{uint8(math.Round(float64(n) * 255 / 4))}
}
