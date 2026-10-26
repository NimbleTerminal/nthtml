package document

import (
	"image/color"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/colornames"
)

func htmlColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return color.RGBA{}, false
	}
	if c, ok := cssColor(s, color.RGBA{}); ok {
		return c, true
	}
	if len(s) == 6 || len(s) == 3 {
		return cssColor("#"+s, color.RGBA{})
	}
	return color.RGBA{}, false
}

func cssColor(s string, current color.RGBA) (color.RGBA, bool) {
	if len(s) > 1024 {
		return color.RGBA{}, false
	}
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "currentcolor":
		return current, true
	case "transparent":
		return color.RGBA{}, true
	case "rebeccapurple":
		return color.RGBA{102, 51, 153, 255}, true
	}
	if c, ok := colornames.Map[s]; ok {
		return c, true
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 || len(hex) == 4 {
			var b strings.Builder
			for _, c := range hex {
				b.WriteRune(c)
				b.WriteRune(c)
			}
			hex = b.String()
		}
		if len(hex) != 6 && len(hex) != 8 {
			return color.RGBA{}, false
		}
		n, e := strconv.ParseUint(hex, 16, 32)
		if e != nil {
			return color.RGBA{}, false
		}
		if len(hex) == 6 {
			return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255}, true
		}
		return premultiplied(float64(n>>24), float64(n>>16&255), float64(n>>8&255), float64(n&255)/255), true
	}
	name, body, ok := strings.Cut(s, "(")
	if !ok || !strings.HasSuffix(body, ")") {
		return color.RGBA{}, false
	}
	body = strings.TrimSuffix(body, ")")
	if name != "rgb" && name != "rgba" && name != "hsl" && name != "hsla" {
		return color.RGBA{}, false
	}
	var parts []string
	legacy := strings.Contains(body, ",")
	if legacy {
		if strings.Contains(body, "/") {
			return color.RGBA{}, false
		}
		parts = strings.Split(body, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
	} else {
		channels, alpha, slash := strings.Cut(body, "/")
		parts = strings.Fields(channels)
		if len(parts) != 3 {
			return color.RGBA{}, false
		}
		if slash {
			parts = append(parts, strings.TrimSpace(alpha))
		}
	}
	if len(parts) != 3 && len(parts) != 4 {
		return color.RGBA{}, false
	}
	alpha := 1.0
	if len(parts) == 4 {
		var valid bool
		alpha, valid = colorNumber(parts[3], 1)
		if !valid {
			return color.RGBA{}, false
		}
		alpha = max(0, min(1, alpha))
	}
	if name == "rgb" || name == "rgba" {
		var c [3]float64
		for i := range c {
			v, valid := colorNumber(parts[i], 255)
			if !valid {
				return color.RGBA{}, false
			}
			c[i] = v
		}
		if legacy {
			pct := strings.HasSuffix(parts[0], "%")
			for _, v := range parts[1:3] {
				if strings.HasSuffix(v, "%") != pct {
					return color.RGBA{}, false
				}
			}
		}
		return premultiplied(c[0], c[1], c[2], alpha), true
	}
	hue := parts[0]
	factor := 1.0
	for _, u := range []struct {
		s string
		f float64
	}{{"grad", .9}, {"turn", 360}, {"deg", 1}, {"rad", 180 / math.Pi}} {
		if strings.HasSuffix(hue, u.s) {
			hue = strings.TrimSuffix(hue, u.s)
			factor = u.f
			break
		}
	}
	h, valid := plainColorNumber(hue)
	if !valid {
		return color.RGBA{}, false
	}
	if !strings.HasSuffix(parts[1], "%") || !strings.HasSuffix(parts[2], "%") {
		return color.RGBA{}, false
	}
	saturation, ok1 := colorNumber(parts[1], 1)
	lightness, ok2 := colorNumber(parts[2], 1)
	if !ok1 || !ok2 {
		return color.RGBA{}, false
	}
	saturation = max(0, min(1, saturation))
	lightness = max(0, min(1, lightness))
	h = math.Mod(h, 360/factor) * factor
	if h < 0 {
		h += 360
	}
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	x := chroma * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := lightness - chroma/2
	r, g, b := 0.0, 0.0, 0.0
	switch {
	case h < 60:
		r, g = chroma, x
	case h < 120:
		r, g = x, chroma
	case h < 180:
		g, b = chroma, x
	case h < 240:
		g, b = x, chroma
	case h < 300:
		r, b = x, chroma
	default:
		r, b = chroma, x
	}
	return premultiplied((r+m)*255, (g+m)*255, (b+m)*255, alpha), true
}
func plainColorNumber(s string) (float64, bool) {
	if len(s) > 64 || !cssNumber.MatchString(s) {
		return 0, false
	}
	n, e := strconv.ParseFloat(s, 64)
	return n, e == nil && !math.IsInf(n, 0) && !math.IsNaN(n)
}
func colorNumber(s string, scale float64) (float64, bool) {
	pct := strings.HasSuffix(s, "%")
	if pct {
		s = strings.TrimSuffix(s, "%")
	}
	n, ok := plainColorNumber(s)
	if pct {
		n = n / 100 * scale
	}
	return n, ok
}
func premultiplied(r, g, b, a float64) color.RGBA {
	byteValue := func(v float64) uint8 { return uint8(math.Round(max(0, min(255, v)))) }
	c := color.NRGBA{byteValue(r), byteValue(g), byteValue(b), byteValue(a * 255)}
	return color.RGBAModel.Convert(c).(color.RGBA)
}
