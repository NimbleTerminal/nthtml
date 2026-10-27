package document

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

type gradientStop struct {
	col    color.RGBA
	pos    float64
	hasPos bool
}

func parseBackgroundURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 5 || !strings.EqualFold(s[:4], "url(") || s[len(s)-1] != ')' {
		return "", false
	}
	closeRel := findMatchingParen(s[3:])
	if closeRel != len(s)-4 {
		return "", false
	}
	inner := strings.TrimSpace(s[4 : len(s)-1])
	if len(inner) >= 2 && (inner[0] == '"' || inner[0] == '\'') {
		q := inner[0]
		if inner[len(inner)-1] != q {
			return "", false
		}
		var b strings.Builder
		for i := 1; i < len(inner)-1; i++ {
			if inner[i] == '\\' && i+1 < len(inner)-1 {
				i++
			}
			b.WriteByte(inner[i])
		}
		inner = strings.TrimSpace(b.String())
	}
	if inner == "" || len(inner) > 2048 || strings.ContainsAny(inner, "\x00\r\n\t ") {
		return "", false
	}
	return inner, true
}

func isGradientFunc(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	openIdx := strings.IndexByte(s, '(')
	if openIdx < 0 || s[len(s)-1] != ')' {
		return "", "", false
	}
	closeRel := findMatchingParen(s[openIdx:])
	if closeRel != len(s)-1-openIdx {
		return "", "", false
	}
	name := strings.ToLower(strings.TrimSpace(s[:openIdx]))
	switch name {
	case "linear-gradient", "repeating-linear-gradient", "radial-gradient", "repeating-radial-gradient":
		return name, strings.TrimSpace(s[openIdx+1 : len(s)-1]), true
	}
	return "", "", false
}

func parseSingleBackgroundImage(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "none") {
		return "none", true
	}
	if _, ok := parseBackgroundURL(s); ok {
		return s, true
	}
	if name, inner, ok := isGradientFunc(s); ok {
		if strings.Contains(name, "linear") {
			if _, _, ok := parseLinearGradientArgs(inner, color.RGBA{0, 0, 0, 255}, 100); ok {
				return s, true
			}
		} else {
			if _, _, _, _, ok := parseRadialGradientArgs(inner, color.RGBA{0, 0, 0, 255}, 100); ok {
				return s, true
			}
		}
	}
	return "", false
}

func parseBackgroundImage(v string) (string, bool) {
	layers, ok := splitSelectorList(v)
	if !ok || len(layers) == 0 || len(layers) > 8 {
		return "", false
	}
	chosen := ""
	for _, layer := range layers {
		parsed, ok := parseSingleBackgroundImage(layer)
		if !ok {
			return "", false
		}
		if chosen == "" || chosen == "none" {
			chosen = parsed
		}
	}
	return chosen, chosen != ""
}

func extractBackgroundURLs(v string) []string {
	layers, ok := splitSelectorList(v)
	if !ok {
		return nil
	}
	var out []string
	for _, layer := range layers {
		if u, ok := parseBackgroundURL(layer); ok {
			out = append(out, u)
		}
	}
	return out
}

func parseBackgroundSize(v string) (string, [2]cssLength, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if layers, ok := splitSelectorList(v); ok && len(layers) > 1 {
		v = strings.TrimSpace(layers[0])
	}
	switch v {
	case "auto":
		return "auto", [2]cssLength{{unit: lengthAuto}, {unit: lengthAuto}}, true
	case "cover", "contain":
		return v, [2]cssLength{}, true
	}
	toks, ok := cssTokens(v)
	if !ok || len(toks) < 1 || len(toks) > 2 {
		return "", [2]cssLength{}, false
	}
	w, ok1 := parseLength(toks[0], true, false)
	if !ok1 {
		return "", [2]cssLength{}, false
	}
	h := cssLength{unit: lengthAuto}
	if len(toks) == 2 {
		var ok2 bool
		h, ok2 = parseLength(toks[1], true, false)
		if !ok2 {
			return "", [2]cssLength{}, false
		}
	}
	return "explicit", [2]cssLength{w, h}, true
}

func parseBackgroundPosition(v string) ([2]cssLength, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if layers, ok := splitSelectorList(v); ok && len(layers) > 1 {
		v = strings.TrimSpace(layers[0])
	}
	toks, ok := cssTokens(v)
	if !ok || len(toks) == 0 || len(toks) > 4 {
		return [2]cssLength{}, false
	}
	pct := func(p float64) cssLength {
		return cssLength{value: p, unit: lengthPercent}
	}
	if len(toks) == 1 {
		switch toks[0] {
		case "left":
			return [2]cssLength{pct(0), pct(50)}, true
		case "center":
			return [2]cssLength{pct(50), pct(50)}, true
		case "right":
			return [2]cssLength{pct(100), pct(50)}, true
		case "top":
			return [2]cssLength{pct(50), pct(0)}, true
		case "bottom":
			return [2]cssLength{pct(50), pct(100)}, true
		default:
			l, ok := parseInset(toks[0])
			if !ok || l.unit == lengthAuto {
				return [2]cssLength{}, false
			}
			return [2]cssLength{l, pct(50)}, true
		}
	}
	if len(toks) == 2 {
		t0, t1 := toks[0], toks[1]
		if (t0 == "top" || t0 == "bottom") && (t1 == "left" || t1 == "right" || t1 == "center") {
			t0, t1 = t1, t0
		}
		var x, y cssLength
		switch t0 {
		case "left":
			x = pct(0)
		case "center":
			x = pct(50)
		case "right":
			x = pct(100)
		default:
			l, ok := parseInset(t0)
			if !ok || l.unit == lengthAuto {
				return [2]cssLength{}, false
			}
			x = l
		}
		switch t1 {
		case "top":
			y = pct(0)
		case "center":
			y = pct(50)
		case "bottom":
			y = pct(100)
		default:
			l, ok := parseInset(t1)
			if !ok || l.unit == lengthAuto {
				return [2]cssLength{}, false
			}
			y = l
		}
		return [2]cssLength{x, y}, true
	}
	// 3 or 4 token syntax (e.g. right 10px bottom 10px)
	x, y := pct(0), pct(0)
	for i := 0; i < len(toks); i++ {
		switch toks[i] {
		case "left":
			x = pct(0)
			if i+1 < len(toks) {
				if l, ok := parseInset(toks[i+1]); ok && l.unit != lengthAuto {
					x = l
					i++
				}
			}
		case "right":
			x = pct(100)
			if i+1 < len(toks) {
				if l, ok := parseInset(toks[i+1]); ok && l.unit != lengthAuto {
					x = cssLength{unit: lengthPercent, expr: &cssMathNode{kind: mathSub, args: []*cssMathNode{{kind: mathDimension, val: 100, unit: lengthPercent}, {kind: mathDimension, val: l.value, unit: l.unit}}}}
					i++
				}
			}
		case "top":
			y = pct(0)
			if i+1 < len(toks) {
				if l, ok := parseInset(toks[i+1]); ok && l.unit != lengthAuto {
					y = l
					i++
				}
			}
		case "bottom":
			y = pct(100)
			if i+1 < len(toks) {
				if l, ok := parseInset(toks[i+1]); ok && l.unit != lengthAuto {
					y = cssLength{unit: lengthPercent, expr: &cssMathNode{kind: mathSub, args: []*cssMathNode{{kind: mathDimension, val: 100, unit: lengthPercent}, {kind: mathDimension, val: l.value, unit: l.unit}}}}
					i++
				}
			}
		case "center":
		default:
			return [2]cssLength{}, false
		}
	}
	return [2]cssLength{x, y}, true
}

func parseBackgroundRepeat(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if layers, ok := splitSelectorList(v); ok && len(layers) > 1 {
		v = strings.TrimSpace(layers[0])
	}
	switch v {
	case "repeat", "repeat-x", "repeat-y", "no-repeat", "space", "round":
		return v, true
	case "repeat no-repeat":
		return "repeat-x", true
	case "no-repeat repeat":
		return "repeat-y", true
	case "no-repeat no-repeat":
		return "no-repeat", true
	case "repeat repeat":
		return "repeat", true
	}
	return "", false
}

func tokenizeBackgroundLayer(s string) ([]string, bool) {
	var out []string
	parenDepth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(':
			parenDepth++
		case ')':
			if parenDepth == 0 {
				return nil, false
			}
			parenDepth--
		case '/':
			if parenDepth == 0 {
				if start < i {
					out = append(out, s[start:i])
				}
				out = append(out, "/")
				start = i + 1
			}
		default:
			if parenDepth == 0 && cssSpace(rune(c)) {
				if start < i {
					out = append(out, s[start:i])
				}
				start = i + 1
			}
		}
	}
	if parenDepth != 0 || quote != 0 {
		return nil, false
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out, len(out) > 0
}

func parseBackgroundShorthand(v string) (map[string]string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, false
	}
	res := map[string]string{
		"background-color":    "transparent",
		"background-image":    "none",
		"background-size":     "auto",
		"background-position": "0% 0%",
		"background-repeat":   "repeat",
	}
	if _, ok := cssColor(v, color.RGBA{}); ok {
		res["background-color"] = v
		return res, true
	}
	layers, ok := splitSelectorList(v)
	if !ok || len(layers) == 0 || len(layers) > 8 {
		return nil, false
	}
	hasImageLayer, hasPos, hasSize, hasRepeat := false, false, false, false
	for li, layer := range layers {
		toks, ok := tokenizeBackgroundLayer(layer)
		if !ok || len(toks) == 0 {
			return nil, false
		}
		var posToks []string
		var sizeToks []string
		afterSlash := false
		layerImg := ""
		layerRepeat := ""
		layerColor := ""
		for i := 0; i < len(toks); i++ {
			tok := toks[i]
			lt := strings.ToLower(tok)
			if tok == "/" {
				if afterSlash || len(posToks) == 0 {
					return nil, false
				}
				afterSlash = true
				continue
			}
			if afterSlash {
				if lt == "auto" || lt == "cover" || lt == "contain" {
					sizeToks = append(sizeToks, tok)
					if lt == "cover" || lt == "contain" || len(sizeToks) == 2 {
						afterSlash = false
					}
					continue
				}
				if _, ok := parseLength(tok, true, false); ok && len(sizeToks) < 2 {
					sizeToks = append(sizeToks, tok)
					if len(sizeToks) == 2 {
						afterSlash = false
					}
					continue
				}
				afterSlash = false
			}
			if img, ok := parseSingleBackgroundImage(tok); ok {
				layerImg = img
				continue
			}
			if rep, ok := parseBackgroundRepeat(lt); ok {
				layerRepeat = rep
				continue
			}
			if lt == "scroll" || lt == "fixed" || lt == "local" || lt == "border-box" || lt == "padding-box" || lt == "content-box" {
				continue
			}
			if lt == "left" || lt == "right" || lt == "top" || lt == "bottom" || lt == "center" {
				posToks = append(posToks, lt)
				continue
			}
			if _, ok := parseInset(tok); ok && !strings.EqualFold(tok, "auto") {
				posToks = append(posToks, tok)
				continue
			}
			if _, ok := cssColor(tok, color.RGBA{}); ok && li == len(layers)-1 && layerColor == "" {
				layerColor = tok
				continue
			}
			return nil, false
		}
		if afterSlash && len(sizeToks) == 0 {
			return nil, false
		}
		if len(posToks) > 0 {
			posStr := strings.Join(posToks, " ")
			if _, ok := parseBackgroundPosition(posStr); !ok {
				return nil, false
			}
			if !hasPos {
				res["background-position"] = posStr
				hasPos = true
			}
		}
		if len(sizeToks) > 0 {
			sizeStr := strings.Join(sizeToks, " ")
			if _, _, ok := parseBackgroundSize(sizeStr); !ok {
				return nil, false
			}
			if !hasSize {
				res["background-size"] = sizeStr
				hasSize = true
			}
		}
		if layerRepeat != "" && !hasRepeat {
			res["background-repeat"] = layerRepeat
			hasRepeat = true
		}
		if layerImg != "" && !hasImageLayer {
			res["background-image"] = layerImg
			if layerImg != "none" {
				hasImageLayer = true
			}
		}
		if layerColor != "" {
			res["background-color"] = layerColor
		}
	}
	return res, true
}

func parseGradientAngle(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(s, "to ") {
		rest := strings.Fields(s[3:])
		if len(rest) == 1 {
			switch rest[0] {
			case "top":
				return 0, true
			case "right":
				return 90, true
			case "bottom":
				return 180, true
			case "left":
				return 270, true
			}
		} else if len(rest) == 2 {
			a, b := rest[0], rest[1]
			if (a == "top" && b == "right") || (a == "right" && b == "top") {
				return 45, true
			}
			if (a == "bottom" && b == "right") || (a == "right" && b == "bottom") {
				return 135, true
			}
			if (a == "bottom" && b == "left") || (a == "left" && b == "bottom") {
				return 225, true
			}
			if (a == "top" && b == "left") || (a == "left" && b == "top") {
				return 315, true
			}
		}
		return 0, false
	}
	factor := 0.0
	for _, u := range []struct {
		suffix string
		factor float64
	}{
		{"deg", 1},
		{"grad", 0.9},
		{"rad", 180 / math.Pi},
		{"turn", 360},
	} {
		if strings.HasSuffix(s, u.suffix) {
			s = strings.TrimSuffix(s, u.suffix)
			factor = u.factor
			break
		}
	}
	if factor == 0 {
		if s == "0" {
			return 0, true
		}
		return 0, false
	}
	if len(s) > 64 || !cssNumber.MatchString(s) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v * factor, true
}

func parseColorStops(args []string, currentFg color.RGBA, lineLen int) ([]gradientStop, bool) {
	if len(args) < 2 || len(args) > 16 {
		return nil, false
	}
	var stops []gradientStop
	for _, arg := range args {
		toks, ok := cssTokens(strings.TrimSpace(arg))
		if !ok || len(toks) == 0 || len(toks) > 3 {
			return nil, false
		}
		col, ok := cssColor(toks[0], currentFg)
		if !ok {
			return nil, false
		}
		if len(toks) == 1 {
			stops = append(stops, gradientStop{col: col})
			continue
		}
		for _, posTok := range toks[1:] {
			l, ok := parseInset(posTok)
			if !ok || l.unit == lengthAuto {
				return nil, false
			}
			var pos float64
			if l.unit == lengthPercent && l.expr == nil {
				pos = l.value / 100
			} else {
				pos = float64(l.offset(lineLen)) / float64(max(1, lineLen))
			}
			stops = append(stops, gradientStop{col: col, pos: pos, hasPos: true})
		}
	}
	if len(stops) < 2 {
		return nil, false
	}
	if !stops[0].hasPos {
		stops[0].pos = 0
		stops[0].hasPos = true
	}
	if !stops[len(stops)-1].hasPos {
		stops[len(stops)-1].pos = max(1, stops[0].pos)
		stops[len(stops)-1].hasPos = true
	}
	for i := 1; i < len(stops); i++ {
		if stops[i].hasPos && stops[i].pos < stops[i-1].pos {
			stops[i].pos = stops[i-1].pos
		}
	}
	for i := 0; i < len(stops); {
		if stops[i].hasPos {
			i++
			continue
		}
		start := i - 1
		end := i
		for end < len(stops) && !stops[end].hasPos {
			end++
		}
		span := float64(end - start)
		for j := start + 1; j < end; j++ {
			stops[j].pos = stops[start].pos + (stops[end].pos-stops[start].pos)*float64(j-start)/span
			stops[j].hasPos = true
		}
		i = end + 1
	}
	return stops, true
}

func parseLinearGradientArgs(inner string, currentFg color.RGBA, lineLen int) (float64, []gradientStop, bool) {
	parts, ok := splitSelectorList(inner)
	if !ok || len(parts) == 0 {
		return 0, nil, false
	}
	deg := 180.0
	stopParts := parts
	if d, ok := parseGradientAngle(parts[0]); ok {
		deg = d
		stopParts = parts[1:]
	}
	stops, ok := parseColorStops(stopParts, currentFg, lineLen)
	if !ok {
		return 0, nil, false
	}
	return deg, stops, true
}

func parseRadialGradientArgs(inner string, currentFg color.RGBA, lineLen int) (isCircle bool, cxPct, cyPct float64, stops []gradientStop, ok bool) {
	parts, ok := splitSelectorList(inner)
	if !ok || len(parts) == 0 {
		return false, 0, 0, nil, false
	}
	cxPct, cyPct = 0.5, 0.5
	stopParts := parts
	first := strings.ToLower(strings.TrimSpace(parts[0]))
	firstToks, _ := cssTokens(first)
	if len(firstToks) == 0 {
		return false, 0, 0, nil, false
	}
	if _, isCol := cssColor(firstToks[0], currentFg); !isCol {
		stopParts = parts[1:]
		for i := 0; i < len(firstToks); i++ {
			switch firstToks[i] {
			case "circle":
				isCircle = true
			case "ellipse", "closest-side", "farthest-side", "closest-corner", "farthest-corner":
			case "at":
				posStr := strings.Join(firstToks[i+1:], " ")
				pos, ok := parseBackgroundPosition(posStr)
				if !ok {
					return false, 0, 0, nil, false
				}
				if pos[0].unit == lengthPercent && pos[0].expr == nil {
					cxPct = pos[0].value / 100
				}
				if pos[1].unit == lengthPercent && pos[1].expr == nil {
					cyPct = pos[1].value / 100
				}
				i = len(firstToks)
			default:
				if _, ok := parseLength(firstToks[i], false, false); !ok {
					return false, 0, 0, nil, false
				}
			}
		}
	}
	stops, ok = parseColorStops(stopParts, currentFg, lineLen)
	if !ok {
		return false, 0, 0, nil, false
	}
	return isCircle, cxPct, cyPct, stops, true
}

func sampleGradientStops(stops []gradientStop, t float64, repeating bool) color.RGBA {
	if len(stops) == 0 {
		return color.RGBA{}
	}
	first := stops[0].pos
	last := stops[len(stops)-1].pos
	if repeating && last > first {
		period := last - first
		t = first + math.Mod(t-first, period)
		if t < first {
			t += period
		}
	}
	if t <= first {
		return stops[0].col
	}
	if t >= last {
		return stops[len(stops)-1].col
	}
	for i := 0; i < len(stops)-1; i++ {
		a, b := stops[i], stops[i+1]
		if t >= a.pos && t <= b.pos {
			span := b.pos - a.pos
			if span <= 1e-9 {
				return b.col
			}
			f := (t - a.pos) / span
			lerp := func(x, y uint8) uint8 {
				return uint8(max(0, min(255, int(math.Round(float64(x)*(1-f)+float64(y)*f)))))
			}
			return color.RGBA{
				R: lerp(a.col.R, b.col.R),
				G: lerp(a.col.G, b.col.G),
				B: lerp(a.col.B, b.col.B),
				A: lerp(a.col.A, b.col.A),
			}
		}
	}
	return stops[len(stops)-1].col
}

func renderLinearGradient(boxW, boxH int, deg float64, stops []gradientStop, repeating bool, opacity float64) *image.RGBA {
	if boxW <= 0 || boxH <= 0 {
		return nil
	}
	w, h := min(boxW, 512), min(boxH, 512)
	rad := deg * math.Pi / 180
	dx := math.Sin(rad)
	dy := -math.Cos(rad)
	if math.Abs(dx) < 1e-6 {
		w = 1
	} else if math.Abs(dy) < 1e-6 {
		h = 1
	}
	halfW := float64(w) / 2
	halfH := float64(h) / 2
	halfLen := math.Abs(float64(w)*dx)/2 + math.Abs(float64(h)*dy)/2
	if halfLen < 1e-6 {
		halfLen = 0.5
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		py := float64(y) + 0.5 - halfH
		for x := 0; x < w; x++ {
			px := float64(x) + 0.5 - halfW
			proj := px*dx + py*dy
			t := (proj + halfLen) / (2 * halfLen)
			c := sampleGradientStops(stops, t, repeating)
			if opacity < 1 {
				c = scaleColorAlpha(c, opacity)
			}
			dst.SetRGBA(x, y, c)
		}
	}
	return dst
}

func renderRadialGradient(boxW, boxH int, isCircle bool, cxPct, cyPct float64, stops []gradientStop, repeating bool, opacity float64) *image.RGBA {
	if boxW <= 0 || boxH <= 0 {
		return nil
	}
	w, h := min(boxW, 256), min(boxH, 256)
	cx := cxPct * float64(w)
	cy := cyPct * float64(h)
	rx := max(1.0, math.Max(cx, float64(w)-cx))
	ry := max(1.0, math.Max(cy, float64(h)-cy))
	if isCircle {
		r := math.Hypot(rx, ry)
		rx, ry = r, r
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		ny := (float64(y) + 0.5 - cy) / ry
		for x := 0; x < w; x++ {
			nx := (float64(x) + 0.5 - cx) / rx
			t := math.Hypot(nx, ny)
			c := sampleGradientStops(stops, t, repeating)
			if opacity < 1 {
				c = scaleColorAlpha(c, opacity)
			}
			dst.SetRGBA(x, y, c)
		}
	}
	return dst
}

func (l *layout) resolveBackgroundURLImage(u string) image.Image {
	if l.opts.BackgroundImages != nil {
		if img := l.opts.BackgroundImages[u]; img != nil && !img.Bounds().Empty() {
			return img
		}
	}
	if l.opts.Images != nil {
		if img := l.opts.Images[u]; img != nil && !img.Bounds().Empty() {
			return img
		}
	}
	if strings.HasPrefix(strings.ToLower(u), "data:image/") {
		if img, err := DecodeDataURL(u); err == nil && img != nil && !img.Bounds().Empty() {
			return img
		}
	}
	return nil
}

func (l *layout) renderBackgroundImage(boxW, boxH int, s style) image.Image {
	if s.hidden || s.bgImage == "" || s.bgImage == "none" || boxW <= 0 || boxH <= 0 || boxW > 4096 || boxH > 4096 {
		return nil
	}
	eff := s.effectiveOpacity()
	if eff <= 0 {
		return nil
	}
	if name, inner, ok := isGradientFunc(s.bgImage); ok {
		repeating := strings.HasPrefix(name, "repeating-")
		if strings.Contains(name, "linear") {
			deg, stops, ok := parseLinearGradientArgs(inner, s.fg, max(boxW, boxH))
			if !ok {
				return nil
			}
			return renderLinearGradient(boxW, boxH, deg, stops, repeating, eff)
		}
		isCircle, cxPct, cyPct, stops, ok := parseRadialGradientArgs(inner, s.fg, max(boxW, boxH))
		if !ok {
			return nil
		}
		return renderRadialGradient(boxW, boxH, isCircle, cxPct, cyPct, stops, repeating, eff)
	}
	u, ok := parseBackgroundURL(s.bgImage)
	if !ok {
		return nil
	}
	src := l.resolveBackgroundURLImage(u)
	if src == nil || src.Bounds().Empty() {
		return nil
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}
	tw, th := sw, sh
	switch s.bgSizeMode {
	case "cover":
		scale := max(float64(boxW)/float64(sw), float64(boxH)/float64(sh))
		tw = max(1, int(math.Round(float64(sw)*scale)))
		th = max(1, int(math.Round(float64(sh)*scale)))
	case "contain":
		scale := min(float64(boxW)/float64(sw), float64(boxH)/float64(sh))
		tw = max(1, int(math.Round(float64(sw)*scale)))
		th = max(1, int(math.Round(float64(sh)*scale)))
	case "explicit":
		wDef := s.bgSize[0].definite(boxW)
		hDef := s.bgSize[1].definite(boxH)
		if wDef && hDef {
			tw = max(1, s.bgSize[0].resolve(boxW, 4096))
			th = max(1, s.bgSize[1].resolve(boxH, 4096))
		} else if wDef {
			tw = max(1, s.bgSize[0].resolve(boxW, 4096))
			th = max(1, int(math.Round(float64(tw)*float64(sh)/float64(sw))))
		} else if hDef {
			th = max(1, s.bgSize[1].resolve(boxH, 4096))
			tw = max(1, int(math.Round(float64(th)*float64(sw)/float64(sh))))
		}
	}
	tw = min(tw, 4096)
	th = min(th, 4096)

	var ox, oy int
	if s.bgPosSet {
		if s.bgPos[0].unit == lengthPercent && s.bgPos[0].expr == nil {
			ox = int(math.Round(float64(boxW-tw) * s.bgPos[0].value / 100))
		} else {
			ox = s.bgPos[0].offset(boxW)
		}
		if s.bgPos[1].unit == lengthPercent && s.bgPos[1].expr == nil {
			oy = int(math.Round(float64(boxH-th) * s.bgPos[1].value / 100))
		} else {
			oy = s.bgPos[1].offset(boxH)
		}
	}

	repeat := s.bgRepeat
	if repeat == "" {
		repeat = "repeat"
	}
	// Fast path: single unpositioned full-box tile with opacity == 1.
	if ox == 0 && oy == 0 && tw == boxW && th == boxH && eff >= 1 {
		return src
	}
	if boxW*boxH > 1<<20 {
		return nil
	}
	var tile image.Image = src
	if tw != sw || th != sh {
		scaled := image.NewRGBA(image.Rect(0, 0, tw, th))
		xdraw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), src, sb, draw.Src, nil)
		tile = scaled
	}
	tb := tile.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, boxW, boxH))
	repX := repeat == "repeat" || repeat == "repeat-x" || repeat == "space" || repeat == "round"
	repY := repeat == "repeat" || repeat == "repeat-y" || repeat == "space" || repeat == "round"
	startX, endX := ox, ox+tw
	if repX && tw > 0 {
		startX = ox % tw
		if startX > 0 {
			startX -= tw
		}
		endX = boxW
	}
	startY, endY := oy, oy+th
	if repY && th > 0 {
		startY = oy % th
		if startY > 0 {
			startY -= th
		}
		endY = boxH
	}
	tiles := 0
	for ty := startY; ty < endY; ty += max(1, th) {
		for tx := startX; tx < endX; tx += max(1, tw) {
			tiles++
			if tiles > 1024 {
				break
			}
			r := image.Rect(tx, ty, tx+tw, ty+th)
			if r.Overlaps(dst.Bounds()) {
				draw.Draw(dst, r, tile, tb.Min, draw.Over)
			}
			if !repX {
				break
			}
		}
		if !repY || tiles > 1024 {
			break
		}
	}
	if eff < 1 {
		for y := 0; y < boxH; y++ {
			for x := 0; x < boxW; x++ {
				dst.SetRGBA(x, y, scaleColorAlpha(dst.RGBAAt(x, y), eff))
			}
		}
	}
	return dst
}
