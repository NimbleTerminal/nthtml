package document

import (
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	maxMediaDepth        = 8
	maxMediaPreludeBytes = 1024
	// TTYWidthPxPerColumn is the CSS pixel width assumed per terminal column when
	// evaluating @media width queries in TTY and TTYLayout modes (80 cols = 800 px).
	ttyWidthPxPerColumn = 10
	maxCustomProps      = 256
	maxVarSubstDepth    = 16
	maxVarValueBytes    = 1024
)

type rule struct {
	selector    selector
	decl        map[string]string
	media       []string
	specificity int
	pseudo      string
}
type style struct {
	lineMode                                   uint8 // 0: document default; 1: absolute; 2: font-size multiplier
	wordSpacing, letterSpacing                 int
	zIndex                                     int
	floatSide, clear                           string
	position                                   string
	inset                                      [4]cssLength
	borderSet                                  [4]bool
	borderStyle                                [4]string
	rootSize                                   int
	minWidth, minHeight, maxHeight             cssLength
	textIndent                                 cssLength
	whiteSpace, overflowWrap, textTransform    string
	listStyleType, captionSide                 string
	overflow, textOverflow                     string
	flexDirection, flexWrap                    string
	justifyContent, alignItems                 string
	flexGrow, flexShrink                       float64
	flexBasis, rowGap, colGap                  cssLength
	spacing                                    [2]int
	cellPadding                                int
	hasCellPadding, tableCellBorder            bool
	centerBlock                                bool
	collapse                                   bool
	size, line                                 int
	fg, bg                                     color.RGBA
	defaultFg, defaultLink                     color.RGBA
	border                                     [4]color.RGBA
	margin, padding                            [4]cssLength // top, right, bottom, left
	bw                                         [4]int       // top, right, bottom, left
	width, height, maxWidth                    cssLength
	borderBox                                  bool
	vertical                                   string
	display, align, family                     string
	bold, italic, underline, strike, smallCaps bool
	mono, pre, hidden, explicitFg              bool
	linkID                                     int
	lineMultiplier                             float64
	vw, vh                                     int
	opacity, ownOpacity                        float64
	opacitySet                                 bool
	aspectRatio                                float64
	objectFit                                  string
	gridCols                                   []gridTrack
	gridColStart, gridColSpan                  int
	gridColEnd, gridColEndSpan                 int
}

func (s style) effectiveOpacity() float64 {
	if !s.opacitySet {
		return 1
	}
	return s.opacity
}

func (s style) computeLen(l cssLength) cssLength {
	return l.computedViewport(s.size, s.rootSize, s.vw, s.vh)
}

func scaleColorAlpha(c color.RGBA, alpha float64) color.RGBA {
	if alpha >= 1 || c.A == 0 {
		return c
	}
	if alpha <= 0 {
		return color.RGBA{}
	}
	scale := func(v uint8) uint8 {
		return uint8(max(0, min(255, int(math.Round(float64(v)*alpha)))))
	}
	return color.RGBA{R: scale(c.R), G: scale(c.G), B: scale(c.B), A: scale(c.A)}
}

func parseOpacity(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	isPct := strings.HasSuffix(v, "%")
	if isPct {
		v = strings.TrimSpace(strings.TrimSuffix(v, "%"))
	}
	if len(v) > 64 || !cssNumber.MatchString(v) {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if isPct {
		f /= 100
	}
	return max(0, min(1, f)), true
}

func parseAspectRatio(v string) (float64, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "auto" {
		return 0, true
	}
	fields := strings.Fields(v)
	if len(fields) >= 2 {
		if fields[0] == "auto" {
			v = strings.Join(fields[1:], " ")
		} else if fields[len(fields)-1] == "auto" {
			v = strings.Join(fields[:len(fields)-1], " ")
		}
	}
	if numStr, denStr, ok := strings.Cut(v, "/"); ok {
		numStr = strings.TrimSpace(numStr)
		denStr = strings.TrimSpace(denStr)
		if !cssNumber.MatchString(numStr) || !cssNumber.MatchString(denStr) {
			return 0, false
		}
		num, err1 := strconv.ParseFloat(numStr, 64)
		den, err2 := strconv.ParseFloat(denStr, 64)
		if err1 != nil || err2 != nil || !(num > 0 && num <= 16384) || !(den > 0 && den <= 16384) {
			return 0, false
		}
		return num / den, true
	}
	if !cssNumber.MatchString(v) {
		return 0, false
	}
	r, err := strconv.ParseFloat(v, 64)
	if err != nil || !(r > 0 && r <= 16384) {
		return 0, false
	}
	return r, true
}

func isObjectFit(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "fill", "contain", "cover", "none", "scale-down":
		return true
	}
	return false
}

func (s style) borderH() int { return s.bw[1] + s.bw[3] }
func (s style) borderV() int { return s.bw[0] + s.bw[2] }
func (s style) hasBorder() bool {
	return s.bw[0] > 0 || s.bw[1] > 0 || s.bw[2] > 0 || s.bw[3] > 0
}
func (s style) borderColor(side int) color.RGBA {
	if s.borderSet[side] {
		return s.border[side]
	}
	return s.fg
}

var borderSides = [4]string{"top", "right", "bottom", "left"}

func isCSSKeyword(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "inherit", "initial", "unset":
		return true
	}
	return false
}

func isInheritedProperty(k string) bool {
	switch k {
	case "color", "font-family", "font-size", "font-style", "font-variant", "font-weight", "line-height", "word-spacing", "letter-spacing", "text-indent", "text-transform", "list-style-type", "caption-side", "white-space", "overflow-wrap", "word-wrap", "border-collapse", "border-spacing", "text-align", "visibility":
		return true
	}
	return false
}

func resolveCSSKeyword(k, v string) string {
	kw := strings.ToLower(strings.TrimSpace(v))
	if kw == "unset" {
		if isInheritedProperty(k) {
			return "inherit"
		}
		return "initial"
	}
	return kw
}

func splitDeclarations(s string) []string {
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
			if parenDepth > 0 {
				parenDepth--
			}
		case ';':
			if parenDepth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func isCustomPropertyName(s string) bool {
	if len(s) < 3 || !strings.HasPrefix(s, "--") || len(s) > 128 {
		return false
	}
	for i := 2; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func containsVar(s string) bool {
	lower := strings.ToLower(s)
	idx := 0
	for {
		i := strings.Index(lower[idx:], "var(")
		if i < 0 {
			return false
		}
		pos := idx + i
		if pos == 0 || !isMediaIdentChar(lower[pos-1]) {
			return true
		}
		idx = pos + 4
	}
}

func expandVarDeclaration(k, v string, put func(string, string)) {
	marker := "\x00" + k + ":" + v
	switch k {
	case "font":
		for _, sub := range []string{"font-family", "font-size", "font-style", "font-variant", "font-weight", "line-height"} {
			put(sub, marker)
		}
	case "list-style":
		put("list-style-type", marker)
	case "gap", "grid-gap":
		put("row-gap", "\x00gap:"+v)
		put("column-gap", "\x00gap:"+v)
	case "grid-row-gap":
		put("row-gap", "\x00row-gap:"+v)
	case "grid-column-gap":
		put("column-gap", "\x00column-gap:"+v)
	case "grid-column":
		put("grid-column-start", marker)
		put("grid-column-end", marker)
	case "flex":
		put("flex-grow", marker)
		put("flex-shrink", marker)
		put("flex-basis", marker)
	case "margin", "padding":
		for _, side := range borderSides {
			put(k+"-"+side, marker)
		}
	case "inset":
		for _, side := range borderSides {
			put(side, marker)
		}
	case "margin-block", "padding-block":
		base := strings.TrimSuffix(k, "-block")
		put(base+"-top", marker)
		put(base+"-bottom", marker)
	case "margin-inline", "padding-inline":
		base := strings.TrimSuffix(k, "-inline")
		put(base+"-left", marker)
		put(base+"-right", marker)
	case "margin-block-start", "padding-block-start":
		target := strings.TrimSuffix(k, "-block-start") + "-top"
		put(target, "\x00"+target+":"+v)
	case "margin-block-end", "padding-block-end":
		target := strings.TrimSuffix(k, "-block-end") + "-bottom"
		put(target, "\x00"+target+":"+v)
	case "margin-inline-start", "padding-inline-start":
		target := strings.TrimSuffix(k, "-inline-start") + "-left"
		put(target, "\x00"+target+":"+v)
	case "margin-inline-end", "padding-inline-end":
		target := strings.TrimSuffix(k, "-inline-end") + "-right"
		put(target, "\x00"+target+":"+v)
	case "border":
		for _, side := range borderSides {
			for _, sub := range []string{"width", "style", "color"} {
				put("border-"+side+"-"+sub, marker)
			}
		}
	case "border-top", "border-right", "border-bottom", "border-left":
		for _, sub := range []string{"width", "style", "color"} {
			put(k+"-"+sub, marker)
		}
	case "border-width", "border-style", "border-color":
		sub := strings.TrimPrefix(k, "border-")
		for _, side := range borderSides {
			put("border-"+side+"-"+sub, marker)
		}
	case "word-wrap":
		put("overflow-wrap", "\x00overflow-wrap:"+v)
	case "background":
		put("background-color", marker)
	case "text-decoration":
		put("text-decoration-line", "\x00text-decoration-line:"+v)
	default:
		put(k, marker)
	}
}

func resolveElementVars(decl map[string]string, parentVars map[string]string) map[string]string {
	var customKeys []string
	for k := range decl {
		if strings.HasPrefix(k, "--") {
			customKeys = append(customKeys, k)
		}
	}
	if len(customKeys) == 0 {
		return parentVars
	}
	sort.Strings(customKeys)
	vars := make(map[string]string, min(maxCustomProps, len(parentVars)+len(customKeys)))
	for k, v := range parentVars {
		if len(vars) >= maxCustomProps {
			break
		}
		vars[k] = v
	}
	for _, k := range customKeys {
		v := decl[k]
		if isCSSKeyword(v) {
			kw := strings.ToLower(strings.TrimSpace(v))
			if kw == "initial" {
				delete(vars, k)
			} else if pv, ok := parentVars[k]; ok {
				if _, exists := vars[k]; exists || len(vars) < maxCustomProps {
					vars[k] = pv
				}
			} else {
				delete(vars, k)
			}
			continue
		}
		if _, exists := vars[k]; exists || len(vars) < maxCustomProps {
			vars[k] = v
		}
	}
	var resolved map[string]string
	for _, k := range customKeys {
		v, ok := vars[k]
		if !ok || !containsVar(v) {
			continue
		}
		if resolved == nil {
			resolved = make(map[string]string, len(customKeys))
		}
		sub, valid := substituteVars(v, vars, 0)
		if valid && !containsVar(sub) {
			resolved[k] = sub
		} else {
			resolved[k] = "\x00invalid"
		}
	}
	for k, v := range resolved {
		if v == "\x00invalid" {
			delete(vars, k)
		} else {
			vars[k] = v
		}
	}
	return vars
}

func resolveDeclaredValues(decl map[string]string, vars map[string]string) map[string]string {
	needsResolve := false
	for k, v := range decl {
		if strings.HasPrefix(k, "--") || strings.HasPrefix(v, "\x00") || containsVar(v) {
			needsResolve = true
			break
		}
	}
	if !needsResolve {
		return decl
	}
	out := make(map[string]string, len(decl))
	keys := make([]string, 0, len(decl))
	for k := range decl {
		if !strings.HasPrefix(k, "--") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := decl[k]
		if strings.HasPrefix(v, "\x00") {
			origProp, rawVal, ok := strings.Cut(v[1:], ":")
			if !ok {
				continue
			}
			resolved, ok := substituteVars(rawVal, vars, 0)
			if !ok || containsVar(resolved) {
				continue
			}
			parsed := declarations(origProp + ":" + resolved)
			if pv, ok := parsed[k]; ok && !strings.HasPrefix(pv, "\x00") {
				out[k] = pv
			}
			continue
		}
		if containsVar(v) {
			resolved, ok := substituteVars(v, vars, 0)
			if !ok || containsVar(resolved) {
				continue
			}
			parsed := declarations(k + ":" + resolved)
			if pv, ok := parsed[k]; ok && !strings.HasPrefix(pv, "\x00") {
				out[k] = pv
			}
			continue
		}
		out[k] = v
	}
	return out
}

func substituteVars(s string, vars map[string]string, depth int) (string, bool) {
	if depth > maxVarSubstDepth {
		return "", false
	}
	var out strings.Builder
	for len(s) > 0 {
		lower := strings.ToLower(s)
		pos := -1
		idx := 0
		for {
			i := strings.Index(lower[idx:], "var(")
			if i < 0 {
				break
			}
			cand := idx + i
			if cand == 0 || !isMediaIdentChar(lower[cand-1]) {
				pos = cand
				break
			}
			idx = cand + 4
		}
		if pos < 0 {
			out.WriteString(s)
			if out.Len() > maxVarValueBytes {
				return "", false
			}
			return out.String(), true
		}
		out.WriteString(s[:pos])
		closeRel := findMatchingParen(s[pos+3:])
		if closeRel < 0 {
			return "", false
		}
		inner := s[pos+4 : pos+3+closeRel]
		s = s[pos+3+closeRel+1:]
		name, fallback, hasFallback := splitVarArgs(inner)
		name = strings.TrimSpace(name)
		if !isCustomPropertyName(name) {
			return "", false
		}
		val, exists := vars[name]
		if exists && containsVar(val) {
			resolvedVal, ok := substituteVars(val, vars, depth+1)
			if !ok {
				exists = false
			} else {
				val = resolvedVal
			}
		}
		if !exists {
			if !hasFallback {
				return "", false
			}
			fallback = strings.TrimSpace(fallback)
			if containsVar(fallback) {
				resolvedFallback, ok := substituteVars(fallback, vars, depth+1)
				if !ok {
					return "", false
				}
				val = resolvedFallback
			} else {
				val = fallback
			}
		}
		out.WriteString(val)
		if out.Len() > maxVarValueBytes {
			return "", false
		}
	}
	return out.String(), true
}

func findMatchingParen(s string) int {
	depth := 0
	var quote byte
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
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitVarArgs(s string) (string, string, bool) {
	parenDepth := 0
	var quote byte
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
			if parenDepth > 0 {
				parenDepth--
			}
		case ',':
			if parenDepth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}

func declarations(s string) map[string]string {
	m := map[string]string{}
	important := false
	put := func(k, v string) {
		_, old := importance(m[k])
		if old && !important {
			return
		}
		if important {
			v += " !important"
		}
		m[k] = v
	}
	for i, p := range splitDeclarations(s) {
		if i >= 128 {
			break
		}
		rawKey, v, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		rawKey = strings.TrimSpace(rawKey)
		v, important = importance(strings.TrimSpace(v))
		if isCustomPropertyName(rawKey) {
			if len(v) <= maxVarValueBytes {
				put(rawKey, v)
			}
			continue
		}
		k := strings.ToLower(rawKey)
		if containsVar(v) {
			if len(v) <= maxVarValueBytes {
				expandVarDeclaration(k, v, put)
			}
			continue
		}
		if isCSSKeyword(v) {
			kw := strings.ToLower(v)
			switch k {
			case "font":
				for _, sub := range []string{"font-family", "font-size", "font-style", "font-variant", "font-weight", "line-height"} {
					put(sub, kw)
				}
			case "list-style":
				put("list-style-type", kw)
			case "gap", "grid-gap":
				put("row-gap", kw)
				put("column-gap", kw)
			case "grid-row-gap":
				put("row-gap", kw)
			case "grid-column-gap":
				put("column-gap", kw)
			case "grid-column":
				put("grid-column-start", kw)
				put("grid-column-end", kw)
			case "flex":
				put("flex-grow", kw)
				put("flex-shrink", kw)
				put("flex-basis", kw)
			case "margin", "padding":
				for _, side := range borderSides {
					put(k+"-"+side, kw)
				}
			case "inset":
				for _, side := range borderSides {
					put(side, kw)
				}
			case "margin-block", "padding-block":
				base := strings.TrimSuffix(k, "-block")
				put(base+"-top", kw)
				put(base+"-bottom", kw)
			case "margin-inline", "padding-inline":
				base := strings.TrimSuffix(k, "-inline")
				put(base+"-left", kw)
				put(base+"-right", kw)
			case "margin-block-start", "padding-block-start":
				put(strings.TrimSuffix(k, "-block-start")+"-top", kw)
			case "margin-block-end", "padding-block-end":
				put(strings.TrimSuffix(k, "-block-end")+"-bottom", kw)
			case "margin-inline-start", "padding-inline-start":
				put(strings.TrimSuffix(k, "-inline-start")+"-left", kw)
			case "margin-inline-end", "padding-inline-end":
				put(strings.TrimSuffix(k, "-inline-end")+"-right", kw)
			case "border":
				for _, side := range borderSides {
					for _, sub := range []string{"width", "style", "color"} {
						put("border-"+side+"-"+sub, kw)
					}
				}
			case "border-top", "border-right", "border-bottom", "border-left":
				for _, sub := range []string{"width", "style", "color"} {
					put(k+"-"+sub, kw)
				}
			case "border-width", "border-style", "border-color":
				sub := strings.TrimPrefix(k, "border-")
				for _, side := range borderSides {
					put("border-"+side+"-"+sub, kw)
				}
			case "word-wrap":
				put("overflow-wrap", kw)
			case "background":
				put("background-color", kw)
			case "text-decoration":
				put("text-decoration-line", kw)
			default:
				put(k, kw)
			}
			continue
		}
		if k == "color" || k == "background" || k == "background-color" {
			if _, ok := cssColor(v, color.RGBA{}); !ok {
				continue
			}
		}
		if k == "float" {
			v = strings.ToLower(v)
			if v != "none" && v != "left" && v != "right" {
				continue
			}
		}
		if k == "clear" {
			v = strings.ToLower(v)
			if v != "none" && v != "left" && v != "right" && v != "both" {
				continue
			}
		}
		if k == "position" {
			v = strings.ToLower(v)
			if v != "static" && v != "relative" && v != "absolute" {
				continue
			}
		}
		if k == "top" || k == "right" || k == "bottom" || k == "left" {
			if _, ok := parseInset(v); !ok {
				continue
			}
		}
		if k == "word-spacing" || k == "letter-spacing" {
			if _, ok := wordSpacingValue(v); !ok {
				continue
			}
		}
		if k == "text-indent" {
			if _, ok := parseInset(v); !ok {
				continue
			}
		}
		if k == "font-size" {
			if _, ok := fontSizeKeyword(v, 16); !ok {
				if _, ok := parseLength(v, false, false); !ok {
					continue
				}
			}
		}
		if k == "line-height" {
			v = strings.ToLower(v)
			if v != "normal" {
				if _, ok := parseLength(v, false, false); !ok {
					continue
				}
			}
		}
		if k == "white-space" {
			v = strings.ToLower(v)
			if v != "normal" && v != "nowrap" && v != "pre" && v != "pre-wrap" && v != "pre-line" && v != "break-spaces" {
				continue
			}
		}
		if k == "overflow-wrap" || k == "word-wrap" {
			v = strings.ToLower(v)
			if v != "normal" && v != "break-word" && v != "anywhere" {
				continue
			}
		}
		if k == "border-spacing" {
			parts, ok := cssTokens(v)
			valid := ok && (len(parts) == 1 || len(parts) == 2)
			for _, p := range parts {
				l, ok := parseLength(p, false, false)
				valid = valid && ok && l.absolute()
			}
			if !valid {
				continue
			}
		}
		switch k {
		case "font":
			if values, ok := fontValues(v); ok {
				for key, value := range values {
					put(key, value)
				}
			}
		case "margin", "padding", "inset":
			parts, ok := cssTokens(v)
			if !ok {
				continue
			}
			sides, ok := expandFour(parts)
			if !ok {
				continue
			}
			valid := true
			for _, part := range parts {
				if k == "margin" || k == "inset" {
					_, ok = parseInset(part)
				} else {
					_, ok = parseLength(part, false, false)
				}
				if !ok {
					valid = false
				}
			}
			if !valid {
				continue
			}
			for j, side := range borderSides {
				if k == "inset" {
					put(side, sides[j])
				} else {
					put(k+"-"+side, sides[j])
				}
			}
		case "margin-block", "padding-block", "margin-inline", "padding-inline":
			parts, ok := cssTokens(v)
			if !ok || len(parts) < 1 || len(parts) > 2 {
				continue
			}
			isMargin := strings.HasPrefix(k, "margin")
			valid := true
			for _, part := range parts {
				if isMargin {
					_, ok = parseInset(part)
				} else {
					_, ok = parseLength(part, false, false)
				}
				if !ok {
					valid = false
				}
			}
			if !valid {
				continue
			}
			first, second := parts[0], parts[0]
			if len(parts) == 2 {
				second = parts[1]
			}
			base := "padding"
			if isMargin {
				base = "margin"
			}
			if strings.HasSuffix(k, "-block") {
				put(base+"-top", first)
				put(base+"-bottom", second)
			} else {
				put(base+"-left", first)
				put(base+"-right", second)
			}
		case "margin-block-start", "margin-block-end", "margin-inline-start", "margin-inline-end":
			if _, ok := parseInset(v); !ok {
				continue
			}
			switch k {
			case "margin-block-start":
				put("margin-top", v)
			case "margin-block-end":
				put("margin-bottom", v)
			case "margin-inline-start":
				put("margin-left", v)
			case "margin-inline-end":
				put("margin-right", v)
			}
		case "padding-block-start", "padding-block-end", "padding-inline-start", "padding-inline-end":
			if _, ok := parseLength(v, false, false); !ok {
				continue
			}
			switch k {
			case "padding-block-start":
				put("padding-top", v)
			case "padding-block-end":
				put("padding-bottom", v)
			case "padding-inline-start":
				put("padding-left", v)
			case "padding-inline-end":
				put("padding-right", v)
			}
		case "border":
			if values, ok := borderValues(v); ok {
				for _, side := range borderSides {
					for sub, value := range values {
						put("border-"+side+"-"+sub, value)
					}
				}
			}
		case "border-top", "border-right", "border-bottom", "border-left":
			if values, ok := borderValues(v); ok {
				for sub, value := range values {
					put(k+"-"+sub, value)
				}
			}
		case "border-width":
			parts, ok := cssTokens(v)
			if !ok {
				continue
			}
			sides, ok := expandFour(parts)
			if !ok {
				continue
			}
			valid := true
			for _, part := range parts {
				if _, ok := borderWidth(part); !ok {
					valid = false
				}
			}
			if valid {
				for j, side := range borderSides {
					put("border-"+side+"-width", sides[j])
				}
			}
		case "border-style":
			parts := strings.Fields(v)
			sides, ok := expandFour(parts)
			if !ok {
				continue
			}
			valid := true
			for _, part := range parts {
				if !isBorderStyle(part) {
					valid = false
				}
			}
			if valid {
				for j, side := range borderSides {
					put("border-"+side+"-style", strings.ToLower(sides[j]))
				}
			}
		case "border-color":
			parts, ok := cssTokens(v)
			if !ok {
				continue
			}
			sides, ok := expandFour(parts)
			if !ok {
				continue
			}
			valid := true
			for _, part := range parts {
				if _, ok := cssColor(part, color.RGBA{}); !ok {
					valid = false
				}
			}
			if valid {
				for j, side := range borderSides {
					put("border-"+side+"-color", sides[j])
				}
			}
		case "word-wrap":
			put("overflow-wrap", v)
		case "background":
			put("background-color", v)
		case "text-decoration":
			put("text-decoration-line", v)
		case "list-style":
			for _, tok := range strings.Fields(strings.ToLower(v)) {
				switch tok {
				case "none", "disc", "circle", "square", "decimal", "decimal-leading-zero", "lower-alpha", "lower-latin", "upper-alpha", "upper-latin", "lower-roman", "upper-roman":
					put("list-style-type", tok)
				}
			}
		case "gap", "grid-gap":
			parts, ok := cssTokens(v)
			if !ok {
				continue
			}
			if len(parts) == 1 {
				if _, ok := parseLength(parts[0], false, false); ok {
					put("row-gap", parts[0])
					put("column-gap", parts[0])
				}
			} else if len(parts) == 2 {
				_, ok1 := parseLength(parts[0], false, false)
				_, ok2 := parseLength(parts[1], false, false)
				if ok1 && ok2 {
					put("row-gap", parts[0])
					put("column-gap", parts[1])
				}
			}
		case "grid-row-gap":
			if _, ok := parseLength(v, false, false); ok {
				put("row-gap", v)
			}
		case "grid-column-gap":
			if _, ok := parseLength(v, false, false); ok {
				put("column-gap", v)
			}
		case "flex":
			lv := strings.ToLower(strings.TrimSpace(v))
			if lv == "none" {
				put("flex-grow", "0")
				put("flex-shrink", "0")
				put("flex-basis", "auto")
			} else if lv == "auto" {
				put("flex-grow", "1")
				put("flex-shrink", "1")
				put("flex-basis", "auto")
			} else {
				parts, ok := cssTokens(lv)
				if !ok {
					continue
				}
				if len(parts) == 1 {
					if _, err := strconv.ParseFloat(parts[0], 64); err == nil {
						put("flex-grow", parts[0])
						put("flex-shrink", "1")
						put("flex-basis", "0px")
					} else if _, ok := parseLength(parts[0], true, false); ok {
						put("flex-grow", "1")
						put("flex-shrink", "1")
						put("flex-basis", parts[0])
					}
				} else if len(parts) == 2 {
					if _, err := strconv.ParseFloat(parts[0], 64); err == nil {
						put("flex-grow", parts[0])
						if _, err2 := strconv.ParseFloat(parts[1], 64); err2 == nil {
							put("flex-shrink", parts[1])
							put("flex-basis", "0px")
						} else if _, ok := parseLength(parts[1], true, false); ok {
							put("flex-shrink", "1")
							put("flex-basis", parts[1])
						}
					}
				} else if len(parts) == 3 {
					_, err1 := strconv.ParseFloat(parts[0], 64)
					_, err2 := strconv.ParseFloat(parts[1], 64)
					_, ok3 := parseLength(parts[2], true, false)
					if err1 == nil && err2 == nil && ok3 {
						put("flex-grow", parts[0])
						put("flex-shrink", parts[1])
						put("flex-basis", parts[2])
					}
				}
			}
		case "opacity":
			if _, ok := parseOpacity(v); ok {
				put(k, v)
			}
		case "aspect-ratio":
			if _, ok := parseAspectRatio(v); ok {
				put(k, v)
			}
		case "object-fit":
			if isObjectFit(v) {
				put(k, strings.ToLower(strings.TrimSpace(v)))
			}
		case "grid-template-columns":
			if _, ok := parseGridTemplateColumns(v); ok {
				put(k, v)
			}
		case "grid-column":
			if sVal, eVal, ok := parseGridColumnShorthand(v); ok {
				put("grid-column-start", sVal)
				put("grid-column-end", eVal)
			}
		case "grid-column-start", "grid-column-end":
			if _, _, ok := parseGridLineOrSpan(v); ok {
				put(k, v)
			}
		case "z-index":
			if strings.EqualFold(v, "auto") {
				put(k, "auto")
			} else if _, err := strconv.Atoi(v); err == nil {
				put(k, v)
			}
		case "content":
			if parsed, ok := parseContentValue(v); ok {
				put(k, parsed)
			}
		default:
			if strings.HasPrefix(k, "margin-") {
				if _, ok := parseInset(v); !ok {
					continue
				}
			} else if strings.HasPrefix(k, "padding-") || k == "width" || k == "height" || k == "max-width" || k == "min-width" || k == "min-height" || k == "max-height" || k == "row-gap" || k == "column-gap" || k == "flex-basis" {
				if _, ok := parseLength(v, k == "width" || k == "height" || k == "flex-basis", k == "max-width" || k == "max-height"); !ok {
					continue
				}
			} else if strings.HasPrefix(k, "border-") {
				if strings.HasSuffix(k, "-width") {
					if _, ok := borderWidth(v); !ok {
						continue
					}
				} else if strings.HasSuffix(k, "-style") {
					if !isBorderStyle(v) {
						continue
					}
					v = strings.ToLower(v)
				} else if strings.HasSuffix(k, "-color") {
					if _, ok := cssColor(v, color.RGBA{}); !ok {
						continue
					}
				}
			}
			put(k, v)
		}
	}
	return m
}
func parseContentValue(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "none") || strings.EqualFold(v, "normal") {
		return "none", true
	}
	var out strings.Builder
	out.WriteByte('\x01')
	for len(v) > 0 {
		v = strings.TrimLeftFunc(v, cssSpace)
		if len(v) == 0 {
			break
		}
		if v[0] != '"' && v[0] != '\'' {
			return "", false
		}
		quote := v[0]
		v = v[1:]
		closed := false
		for i := 0; i < len(v); i++ {
			if v[i] == '\\' && i+1 < len(v) {
				i++
				out.WriteByte(v[i])
				continue
			}
			if v[i] == quote {
				v = v[i+1:]
				closed = true
				break
			}
			out.WriteByte(v[i])
		}
		if !closed || out.Len()-1 > 256 {
			return "", false
		}
	}
	return out.String(), true
}

func resolveContent(v string) (string, bool) {
	if strings.HasPrefix(v, "\x01") {
		return v[1:], true
	}
	return "", false
}

// parseCSS parses style rules and bounded @media blocks from s, counting all
// parsed rules against budget. Statement at-rules (@import, @charset,
// @namespace) are skipped up to ';'. Block at-rules other than @media
// (@font-face, @keyframes, @supports, @layer, @page, and unknown block
// at-rules) are skipped with brace matching; @supports bodies are skipped,
// not evaluated.
func parseCSS(s string, budget int) ([]rule, error) {
	for {
		a := strings.Index(s, "/*")
		if a < 0 {
			break
		}
		b := strings.Index(s[a+2:], "*/")
		if b < 0 {
			s = s[:a]
			break
		}
		s = s[:a] + s[a+2+b+2:]
	}
	var out []rule
	if err := parseCSSBlock(s, nil, 0, &out, budget); err != nil {
		return nil, err
	}
	return out, nil
}

func parseCSSBlock(s string, media []string, depth int, out *[]rule, budget int) error {
	for {
		s = strings.TrimLeftFunc(s, cssSpace)
		if len(s) == 0 {
			break
		}
		if s[0] == '@' {
			idx := scanPreludeEnd(s)
			if idx < 0 {
				break
			}
			if s[idx] == ';' {
				s = s[idx+1:]
				continue
			}
			closeRel := findMatchingBrace(s[idx:])
			if closeRel < 0 {
				break
			}
			prelude := strings.TrimSpace(s[1:idx])
			inner := s[idx+1 : idx+closeRel]
			s = s[idx+closeRel+1:]
			if isMediaAtRule(prelude) {
				query := strings.TrimSpace(prelude[5:])
				if depth < maxMediaDepth && len(prelude) <= maxMediaPreludeBytes {
					nextMedia := make([]string, len(media)+1)
					copy(nextMedia, media)
					nextMedia[len(media)] = query
					if err := parseCSSBlock(inner, nextMedia, depth+1, out, budget); err != nil {
						return err
					}
				}
			}
			continue
		}
		openIdx := scanRuleOpenBrace(s)
		if openIdx < 0 {
			break
		}
		closeRel := findMatchingBrace(s[openIdx:])
		if closeRel < 0 {
			break
		}
		sel := s[:openIdx]
		body := s[openIdx+1 : openIdx+closeRel]
		s = s[openIdx+closeRel+1:]
		var group []rule
		selList, valid := splitSelectorList(sel)
		for _, v := range selList {
			compiled, sp, pe, ok := parseSelectorWithPseudo(strings.TrimSpace(v))
			if !ok {
				valid = false
				break
			}
			group = append(group, rule{selector: compiled, media: media, specificity: sp, pseudo: pe})
			if len(*out)+len(group) > budget {
				return limitError("more CSS rules than the budget")
			}
		}
		if valid {
			decl := declarations(body)
			for _, r := range group {
				r.decl = decl
				*out = append(*out, r)
			}
			if len(*out) > budget {
				return limitError("more CSS rules than the budget")
			}
		}
	}
	return nil
}

func isMediaAtRule(prelude string) bool {
	if len(prelude) < 5 || !strings.EqualFold(prelude[:5], "media") {
		return false
	}
	if len(prelude) == 5 {
		return true
	}
	c := prelude[5]
	return cssSpace(rune(c)) || c == '('
}

func splitSelectorList(s string) ([]string, bool) {
	var out []string
	parenDepth, bracketDepth := 0, 0
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
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth == 0 {
				return nil, false
			}
			bracketDepth--
		case ',':
			if parenDepth == 0 && bracketDepth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if parenDepth != 0 || bracketDepth != 0 || quote != 0 {
		return nil, false
	}
	out = append(out, s[start:])
	return out, true
}

func scanPreludeEnd(s string) int {
	parenDepth := 0
	var quote byte
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
			if parenDepth > 0 {
				parenDepth--
			}
		case ';', '{':
			if parenDepth == 0 {
				return i
			}
		}
	}
	return -1
}

func scanRuleOpenBrace(s string) int {
	parenDepth, bracketDepth := 0, 0
	var quote byte
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
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			if parenDepth == 0 && bracketDepth == 0 {
				return i
			}
		}
	}
	return -1
}

func findMatchingBrace(s string) int {
	depth := 0
	var quote byte
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
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func (r rule) matchesMedia(width int, scheme string) bool {
	for _, m := range r.media {
		if !evalMediaQueryList(m, width, scheme) {
			return false
		}
	}
	return true
}

func evalMediaQueryList(prelude string, width int, scheme string) bool {
	prelude = strings.TrimSpace(prelude)
	if prelude == "" {
		return true
	}
	parts, ok := splitMediaQueries(prelude)
	if !ok || len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if evalSingleMediaQuery(part, width, scheme) {
			return true
		}
	}
	return false
}

func splitMediaQueries(s string) ([]string, bool) {
	var out []string
	parenDepth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			parenDepth++
		case ')':
			if parenDepth == 0 {
				return nil, false
			}
			parenDepth--
		case ',':
			if parenDepth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if parenDepth != 0 {
		return nil, false
	}
	out = append(out, s[start:])
	return out, true
}

func evalSingleMediaQuery(q string, width int, scheme string) bool {
	tokens, ok := tokenizeMediaQuery(q)
	if !ok || len(tokens) == 0 {
		return false
	}
	negate := false
	if strings.EqualFold(tokens[0], "not") {
		negate = true
		tokens = tokens[1:]
	} else if strings.EqualFold(tokens[0], "only") {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return false
	}
	var result bool
	if strings.HasPrefix(tokens[0], "(") {
		val, valid := evalMediaFeature(tokens[0], width, scheme)
		if !valid {
			return false
		}
		result = val
	} else {
		switch strings.ToLower(tokens[0]) {
		case "and", "not", "only", "or":
			return false
		case "screen", "all":
			result = true
		default:
			result = false
		}
	}
	tokens = tokens[1:]
	for len(tokens) > 0 {
		if len(tokens) < 2 || !strings.EqualFold(tokens[0], "and") || !strings.HasPrefix(tokens[1], "(") {
			return false
		}
		val, valid := evalMediaFeature(tokens[1], width, scheme)
		if !valid {
			return false
		}
		result = result && val
		tokens = tokens[2:]
	}
	if negate {
		return !result
	}
	return result
}

func tokenizeMediaQuery(q string) ([]string, bool) {
	var tokens []string
	for i := 0; i < len(q); {
		if cssSpace(rune(q[i])) {
			i++
			continue
		}
		if q[i] == '(' {
			j := i + 1
			for j < len(q) && q[j] != ')' {
				if q[j] == '(' {
					return nil, false
				}
				j++
			}
			if j >= len(q) {
				return nil, false
			}
			tokens = append(tokens, q[i:j+1])
			i = j + 1
			continue
		}
		if isMediaIdentChar(q[i]) {
			j := i + 1
			for j < len(q) && isMediaIdentChar(q[j]) {
				j++
			}
			tokens = append(tokens, q[i:j])
			i = j
			continue
		}
		return nil, false
	}
	return tokens, true
}

func isMediaIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
}

func evalMediaFeature(expr string, width int, scheme string) (bool, bool) {
	if len(expr) < 2 || expr[0] != '(' || expr[len(expr)-1] != ')' {
		return false, false
	}
	inner := strings.TrimSpace(expr[1 : len(expr)-1])
	name, valStr, ok := strings.Cut(inner, ":")
	if !ok {
		return false, false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	valStr = strings.TrimSpace(valStr)
	if name == "prefers-color-scheme" {
		v := strings.ToLower(valStr)
		if v != "light" && v != "dark" {
			return false, false
		}
		return v == normalizeColorScheme(scheme), true
	}
	if name != "width" && name != "min-width" && name != "max-width" {
		return false, false
	}
	l, ok := parseLength(valStr, false, false)
	if !ok || !l.absolute() {
		return false, false
	}
	pxVal := l.computed(16, 16).value
	switch name {
	case "width":
		return math.Abs(float64(width)-pxVal) < 1e-6, true
	case "min-width":
		return float64(width)+1e-9 >= pxVal, true
	case "max-width":
		return float64(width)-1e-9 <= pxVal, true
	default:
		return false, false
	}
}
func htmlFontSize(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	relative := strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	if relative {
		n = 3 + n
	}
	n = max(1, min(7, n))
	sizes := [8]int{0, 10, 13, 16, 18, 24, 32, 48}
	return sizes[n], true
}

func (d *Document) styled(n *node, parent style) style {
	vw, vh := parent.vw, parent.vh
	if vw <= 0 {
		if d != nil && d.cascadedWidth > 0 {
			vw = d.cascadedWidth
		} else {
			vw = 800
		}
	}
	if vh <= 0 {
		vh = 600
	}
	s := style{lineMode: parent.lineMode, wordSpacing: parent.wordSpacing, letterSpacing: parent.letterSpacing, textIndent: parent.textIndent, textTransform: parent.textTransform, smallCaps: parent.smallCaps, listStyleType: parent.listStyleType, captionSide: parent.captionSide, rootSize: parent.rootSize, whiteSpace: parent.whiteSpace, overflowWrap: parent.overflowWrap, spacing: parent.spacing, cellPadding: parent.cellPadding, hasCellPadding: parent.hasCellPadding, tableCellBorder: parent.tableCellBorder, centerBlock: parent.centerBlock, collapse: parent.collapse, family: parent.family, italic: parent.italic, underline: parent.underline, strike: parent.strike, lineMultiplier: parent.lineMultiplier, mono: parent.mono, pre: parent.pre, hidden: parent.hidden, explicitFg: parent.explicitFg, linkID: parent.linkID, size: parent.size, line: parent.line, fg: parent.fg, defaultFg: parent.defaultFg, defaultLink: parent.defaultLink, bold: parent.bold, align: parent.align, flexShrink: 1, display: "block", vw: vw, vh: vh, opacity: parent.effectiveOpacity(), ownOpacity: 1, opacitySet: parent.opacitySet}
	fgInherited := true
	rawBW := [4]int{3, 3, 3, 3}
	if n.tag == "svg" {
		s.width, _ = parseLength(n.attrs["width"], true, false)
		s.height, _ = parseLength(n.attrs["height"], true, false)
	}
	switch n.tag {
	case "span", "a", "em", "strong", "b", "i", "code", "small", "sub", "sup", "mark", "u", "s", "del", "img", "font":
		s.display = "inline"
	case "script", "style", "head", "meta", "link", "title", "template", "colgroup", "col":
		s.display = "none"
	case "h1":
		s.size = 32
		s.line = 40
		s.lineMultiplier = 0
		s.lineMode = 0
		s.bold = true
		s.margin = [4]cssLength{px(12), {}, px(12), {}}
	case "h2":
		s.size = 24
		s.line = 32
		s.lineMultiplier = 0
		s.lineMode = 0
		s.bold = true
		s.margin = [4]cssLength{px(10), {}, px(10), {}}
	case "h3":
		s.size = 20
		s.line = 28
		s.lineMultiplier = 0
		s.lineMode = 0
		s.bold = true
		s.margin = [4]cssLength{px(8), {}, px(8), {}}
	case "h4":
		s.size = 16
		s.line = 22
		s.lineMultiplier = 0
		s.lineMode = 0
		s.bold = true
		s.margin = [4]cssLength{px(8), {}, px(8), {}}
	case "h5":
		s.size = 14
		s.line = 20
		s.lineMultiplier = 0
		s.lineMode = 0
		s.bold = true
		s.margin = [4]cssLength{px(8), {}, px(8), {}}
	case "h6":
		s.size = 12
		s.line = 18
		s.lineMultiplier = 0
		s.lineMode = 0
		s.bold = true
		s.margin = [4]cssLength{px(8), {}, px(8), {}}
	case "body":
		s.margin = uniform(8)
	case "p", "dl":
		s.margin = [4]cssLength{{value: 1, unit: lengthEM}, {}, {value: 1, unit: lengthEM}, {}}
	case "blockquote":
		s.margin = [4]cssLength{{value: 1, unit: lengthEM}, px(40), {value: 1, unit: lengthEM}, px(40)}
	case "dt":
		s.bold = true
	case "dd":
		s.margin = [4]cssLength{{}, {}, {}, px(40)}
	case "hr":
		s.margin = [4]cssLength{px(8), {}, px(8), {}}
		s.height = px(1)
		s.bg = color.RGBA{180, 188, 200, 255}
	case "ul", "ol":
		s.margin = [4]cssLength{px(8), {}, px(8), {}}
	case "center":
		s.align = "center"
		s.centerBlock = true
	case "table":
		s.align = ""
		s.centerBlock = false
		s.vertical = "middle"
		s.hasCellPadding = false
		s.cellPadding = 0
		s.tableCellBorder = false
		if cs := strings.TrimSpace(n.attrs["cellspacing"]); cs != "" {
			if v, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(cs), "px")); err == nil && v >= 0 && v <= 256 {
				s.spacing = [2]int{v, v}
			}
		}
		if cp := strings.TrimSpace(n.attrs["cellpadding"]); cp != "" {
			if v, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(cp), "px")); err == nil && v >= 0 && v <= 256 {
				s.hasCellPadding = true
				s.cellPadding = v
			}
		}
		if bAttr, hasB := n.attrs["border"]; hasB {
			bTrim := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(bAttr)), "px"))
			v, err := strconv.Atoi(bTrim)
			if bTrim == "" || (err == nil && v > 0) {
				s.tableCellBorder = true
				rawBW = [4]int{1, 1, 1, 1}
				s.borderStyle = [4]string{"solid", "solid", "solid", "solid"}
			}
		}
	case "thead", "tbody", "tfoot", "tr":
		s.vertical = parent.vertical
	case "td", "th":
		s.vertical = parent.vertical
		if parent.hasCellPadding {
			s.padding = uniform(parent.cellPadding)
		} else {
			s.padding = [4]cssLength{{}, px(4), {}, px(4)}
		}
		if parent.tableCellBorder {
			rawBW = [4]int{1, 1, 1, 1}
			s.borderStyle = [4]string{"solid", "solid", "solid", "solid"}
		}
	}
	if parent.centerBlock && s.display == "block" {
		s.margin[1] = cssLength{unit: lengthAuto}
		s.margin[3] = cssLength{unit: lengthAuto}
	}
	// Presentational HTML attributes (§15.3) apply below any author CSS rule.
	switch n.tag {
	case "body", "table", "thead", "tbody", "tfoot", "tr", "td", "th":
		if c, ok := htmlColor(n.attrs["bgcolor"]); ok {
			s.bg = c
		}
	}
	if n.tag == "body" {
		if c, ok := htmlColor(n.attrs["text"]); ok {
			s.fg = c
			s.defaultFg = c
			s.explicitFg = true
			fgInherited = false
		}
		if c, ok := htmlColor(n.attrs["link"]); ok {
			s.defaultLink = c
		}
	}
	switch n.tag {
	case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "td", "th", "tr", "thead", "tbody", "tfoot", "caption":
		switch al := strings.ToLower(strings.TrimSpace(n.attrs["align"])); al {
		case "left", "right", "justify":
			s.align = al
			s.centerBlock = false
		case "center":
			s.align = "center"
			s.centerBlock = true
		}
	case "table":
		switch strings.ToLower(strings.TrimSpace(n.attrs["align"])) {
		case "center":
			s.margin[1] = cssLength{unit: lengthAuto}
			s.margin[3] = cssLength{unit: lengthAuto}
		case "left":
			s.margin[1] = cssLength{unit: lengthAuto}
			s.margin[3] = cssLength{}
		case "right":
			s.margin[1] = cssLength{}
			s.margin[3] = cssLength{unit: lengthAuto}
		}
	}
	switch n.tag {
	case "td", "th", "tr", "thead", "tbody", "tfoot":
		switch val := strings.ToLower(strings.TrimSpace(n.attrs["valign"])); val {
		case "top", "middle", "bottom", "baseline":
			s.vertical = val
		}
	}
	switch n.tag {
	case "table", "td", "th":
		if w, ok := htmlLength(n.attrs["width"]); ok {
			s.width = w
		}
		if h, ok := htmlLength(n.attrs["height"]); ok {
			s.height = h
		}
	case "tr":
		if h, ok := htmlLength(n.attrs["height"]); ok {
			s.height = h
		}
	}
	if n.tag == "font" {
		if c, ok := htmlColor(n.attrs["color"]); ok {
			s.fg = c
			s.explicitFg = true
			fgInherited = false
		}
		if face := strings.TrimSpace(n.attrs["face"]); face != "" {
			s.family = face
			s.mono = strings.EqualFold(face, "monospace") || strings.EqualFold(face, "Go Mono") || strings.EqualFold(face, "IBM Plex Mono")
		}
		if sz, ok := htmlFontSize(n.attrs["size"]); ok {
			s.size = sz
		}
	}
	if n.tag == "small" && parent.size > 0 {
		s.size = max(1, (parent.size*85+50)/100)
	}
	if (n.tag == "sub" || n.tag == "sup") && parent.size > 0 {
		s.size = max(1, (parent.size*80+50)/100)
		s.vertical = n.tag
	}
	if n.tag == "mark" {
		s.bg = color.RGBA{255, 243, 163, 255}
		s.fg = color.RGBA{0, 0, 0, 255}
		s.explicitFg = true
		fgInherited = false
	}
	if n.tag == "em" || n.tag == "i" {
		s.italic = true
	}
	if n.tag == "u" {
		s.underline = true
	}
	if n.tag == "s" || n.tag == "del" {
		s.strike = true
	}
	if n.tag == "pre" {
		s.family = "Go Mono"
		s.mono = true
		s.pre = true
		s.whiteSpace = "pre-wrap"
		s.margin = [4]cssLength{px(8), {}, px(8), {}}
	}
	if n.tag == "code" {
		s.family = "Go Mono"
		s.mono = true
	}
	if n.tag == "a" {
		s.linkID = 0
		if SafeURL(n.attrs["href"]) != "" {
			s.linkID = n.id
			s.underline = true
			fgInherited = false
			if s.defaultLink.A > 0 {
				s.fg = s.defaultLink
				s.explicitFg = true
			} else {
				s.fg = color.RGBA{40, 92, 186, 255}
				s.explicitFg = false
			}
		}
	}
	if n.tag == "strong" || n.tag == "b" || n.tag == "th" {
		s.bold = true
	}
	values := n.decl
	if cv := values["color"]; isCSSKeyword(cv) {
		if resolveCSSKeyword("color", cv) == "inherit" {
			s.fg = parent.fg
			s.explicitFg = parent.explicitFg
			fgInherited = true
		} else if s.defaultFg.A > 0 {
			s.fg = s.defaultFg
			s.explicitFg = true
			fgInherited = false
		} else {
			s.fg = color.RGBA{0, 0, 0, 255}
			s.explicitFg = false
			fgInherited = false
		}
	} else if c, ok := cssColor(cv, parent.fg); ok {
		s.fg = c
		s.explicitFg = true
		fgInherited = !strings.EqualFold(strings.TrimSpace(cv), "currentcolor")
	}
	if fsv := values["font-size"]; isCSSKeyword(fsv) {
		if resolveCSSKeyword("font-size", fsv) == "inherit" {
			s.size = parent.size
		} else {
			s.size = 16
		}
	} else if sz, ok := fontSizeKeyword(fsv, parent.size); ok {
		s.size = sz
	} else if value, ok := parseLength(fsv, false, false); ok {
		value = value.computedViewport(parent.size, s.rootSize, s.vw, s.vh)
		s.size = value.resolve(parent.size, 128)
	}
	if n.tag == "html" {
		s.rootSize = s.size
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := values[k]
		kw := ""
		if isCSSKeyword(v) {
			kw = resolveCSSKeyword(k, v)
		}
		switch k {
		case "font-size": // Resolved before all font-relative properties.

		case "font-family":
			if kw == "inherit" {
				s.family, s.mono = parent.family, parent.mono
			} else if kw == "initial" {
				s.family, s.mono = "", false
			} else {
				s.family = v
				s.mono = false
			}
		case "font-style":
			if kw == "inherit" {
				s.italic = parent.italic
			} else if kw == "initial" {
				s.italic = false
			} else {
				switch strings.ToLower(v) {
				case "italic", "oblique":
					s.italic = true
				case "normal":
					s.italic = false
				}
			}
		case "font-variant":
			if kw == "inherit" {
				s.smallCaps = parent.smallCaps
			} else if kw == "initial" {
				s.smallCaps = false
			} else {
				switch strings.ToLower(v) {
				case "small-caps":
					s.smallCaps = true
				case "normal":
					s.smallCaps = false
				}
			}
		case "font-weight":
			if kw == "inherit" {
				s.bold = parent.bold
			} else if kw == "initial" {
				s.bold = false
			} else if strings.EqualFold(v, "bold") || strings.EqualFold(v, "bolder") {
				s.bold = true
			} else if strings.EqualFold(v, "normal") || strings.EqualFold(v, "lighter") {
				s.bold = false
			} else if weight, e := strconv.Atoi(v); e == nil && weight >= 100 && weight <= 900 {
				s.bold = weight >= 600
			}
		case "line-height":
			if kw == "inherit" {
				s.line, s.lineMode, s.lineMultiplier = parent.line, parent.lineMode, parent.lineMultiplier
			} else if kw == "initial" || v == "normal" {
				s.lineMultiplier = 1.375
				s.lineMode = 2
			} else if cssNumber.MatchString(v) {
				if l, ok := parseLength(v, false, false); ok {
					s.lineMultiplier = min(l.value, 16)
					s.lineMode = 2
				}
			} else if l, ok := parseLength(v, false, false); ok {
				s.line = s.computeLen(l).resolve(s.size, 256)
				s.lineMultiplier = 0
				s.lineMode = 1
			}
		case "word-spacing":
			if kw == "inherit" {
				s.wordSpacing = parent.wordSpacing
			} else if kw == "initial" {
				s.wordSpacing = 0
			} else if l, ok := wordSpacingValue(v); ok {
				s.wordSpacing = max(-256, min(256, s.computeLen(l).offset(0)))
			}
		case "letter-spacing":
			if kw == "inherit" {
				s.letterSpacing = parent.letterSpacing
			} else if kw == "initial" {
				s.letterSpacing = 0
			} else if l, ok := wordSpacingValue(v); ok {
				s.letterSpacing = max(-256, min(256, s.computeLen(l).offset(0)))
			}
		case "text-indent":
			if kw == "inherit" {
				s.textIndent = parent.textIndent
			} else if kw == "initial" {
				s.textIndent = cssLength{}
			} else if l, ok := parseInset(v); ok && l.unit != lengthAuto {
				s.textIndent = s.computeLen(l)
			}
		case "text-transform":
			if kw == "inherit" {
				s.textTransform = parent.textTransform
			} else if kw == "initial" {
				s.textTransform = ""
			} else {
				switch strings.ToLower(v) {
				case "uppercase", "lowercase", "capitalize":
					s.textTransform = strings.ToLower(v)
				case "none":
					s.textTransform = ""
				}
			}
		case "list-style-type":
			if kw == "inherit" {
				s.listStyleType = parent.listStyleType
			} else if kw == "initial" {
				s.listStyleType = ""
			} else {
				switch strings.ToLower(v) {
				case "none", "disc", "circle", "square", "decimal", "decimal-leading-zero", "lower-alpha", "lower-latin", "upper-alpha", "upper-latin", "lower-roman", "upper-roman":
					s.listStyleType = strings.ToLower(v)
				}
			}
		case "caption-side":
			if kw == "inherit" {
				s.captionSide = parent.captionSide
			} else if kw == "initial" {
				s.captionSide = ""
			} else {
				switch strings.ToLower(v) {
				case "top", "bottom":
					s.captionSide = strings.ToLower(v)
				}
			}
		case "overflow", "overflow-x", "overflow-y":
			if kw == "inherit" {
				s.overflow = parent.overflow
			} else if kw == "initial" {
				s.overflow = ""
			} else {
				switch strings.ToLower(v) {
				case "visible", "hidden", "clip", "auto", "scroll":
					s.overflow = strings.ToLower(v)
				}
			}
		case "text-overflow":
			if kw == "inherit" {
				s.textOverflow = parent.textOverflow
			} else if kw == "initial" {
				s.textOverflow = ""
			} else {
				switch strings.ToLower(v) {
				case "clip", "ellipsis":
					s.textOverflow = strings.ToLower(v)
				}
			}
		case "flex-direction":
			if kw == "inherit" {
				s.flexDirection = parent.flexDirection
			} else if kw == "initial" {
				s.flexDirection = "row"
			} else {
				switch strings.ToLower(v) {
				case "row", "row-reverse", "column", "column-reverse":
					s.flexDirection = strings.ToLower(v)
				}
			}
		case "flex-wrap":
			if kw == "inherit" {
				s.flexWrap = parent.flexWrap
			} else if kw == "initial" {
				s.flexWrap = "nowrap"
			} else {
				switch strings.ToLower(v) {
				case "nowrap", "wrap":
					s.flexWrap = strings.ToLower(v)
				}
			}
		case "justify-content":
			if kw == "inherit" {
				s.justifyContent = parent.justifyContent
			} else if kw == "initial" {
				s.justifyContent = "flex-start"
			} else {
				switch strings.ToLower(v) {
				case "flex-start", "flex-end", "center", "space-between", "space-around", "space-evenly", "start", "end":
					s.justifyContent = strings.ToLower(v)
				}
			}
		case "align-items":
			if kw == "inherit" {
				s.alignItems = parent.alignItems
			} else if kw == "initial" {
				s.alignItems = "stretch"
			} else {
				switch strings.ToLower(v) {
				case "stretch", "flex-start", "flex-end", "center", "baseline", "start", "end":
					s.alignItems = strings.ToLower(v)
				}
			}
		case "flex-grow":
			if kw == "inherit" {
				s.flexGrow = parent.flexGrow
			} else if kw == "initial" {
				s.flexGrow = 0
			} else if g, err := strconv.ParseFloat(v, 64); err == nil && g >= 0 && g <= 1000 {
				s.flexGrow = g
			}
		case "flex-shrink":
			if kw == "inherit" {
				s.flexShrink = parent.flexShrink
			} else if kw == "initial" {
				s.flexShrink = 1
			} else if sh, err := strconv.ParseFloat(v, 64); err == nil && sh >= 0 && sh <= 1000 {
				s.flexShrink = sh
			}
		case "flex-basis":
			if kw == "inherit" {
				s.flexBasis = parent.flexBasis
			} else if kw == "initial" {
				s.flexBasis = cssLength{}
			} else if l, ok := parseLength(v, true, false); ok {
				s.flexBasis = s.computeLen(l)
			}
		case "row-gap":
			if kw == "inherit" {
				s.rowGap = parent.rowGap
			} else if kw == "initial" {
				s.rowGap = cssLength{}
			} else if l, ok := parseLength(v, false, false); ok {
				s.rowGap = s.computeLen(l)
			}
		case "column-gap":
			if kw == "inherit" {
				s.colGap = parent.colGap
			} else if kw == "initial" {
				s.colGap = cssLength{}
			} else if l, ok := parseLength(v, false, false); ok {
				s.colGap = s.computeLen(l)
			}
		case "text-decoration-line":
			if kw == "inherit" {
				s.underline, s.strike = parent.underline, parent.strike
				continue
			} else if kw == "initial" {
				s.underline, s.strike = false, false
				continue
			}
			underline, strike, valid := false, false, true
			tokens := strings.Fields(v)
			if len(tokens) == 0 {
				valid = false
			}
			for _, token := range tokens {
				switch token {
				case "underline":
					underline = true
				case "line-through":
					strike = true
				case "none":
					if len(tokens) != 1 {
						valid = false
					}
				default:
					valid = false
				}
			}
			if valid {
				s.underline, s.strike = underline, strike
			}
		case "color":
			// Color resolves first so currentColor is independent of key order.
		case "background-color":
			if kw == "inherit" {
				s.bg = parent.bg
			} else if kw == "initial" {
				s.bg = color.RGBA{}
			} else {
				s.bg, _ = cssColor(v, s.fg)
			}
		case "border-collapse":
			if kw == "inherit" {
				s.collapse = parent.collapse
			} else if kw == "initial" || v == "separate" {
				s.collapse = false
			} else if v == "collapse" {
				s.collapse = true
			}
		case "width":
			if kw == "inherit" {
				s.width = parent.width
			} else if kw == "initial" {
				s.width = cssLength{}
			} else {
				s.width, _ = parseLength(v, true, false)
			}
		case "height":
			if kw == "inherit" {
				s.height = parent.height
			} else if kw == "initial" {
				s.height = cssLength{}
			} else {
				s.height, _ = parseLength(v, true, false)
			}
		case "max-width":
			if kw == "inherit" {
				s.maxWidth = parent.maxWidth
			} else if kw == "initial" {
				s.maxWidth = cssLength{}
			} else {
				s.maxWidth, _ = parseLength(v, false, true)
			}
		case "min-width":
			if kw == "inherit" {
				s.minWidth = parent.minWidth
			} else if kw == "initial" {
				s.minWidth = cssLength{}
			} else {
				s.minWidth, _ = parseLength(v, false, false)
			}
		case "min-height":
			if kw == "inherit" {
				s.minHeight = parent.minHeight
			} else if kw == "initial" {
				s.minHeight = cssLength{}
			} else {
				s.minHeight, _ = parseLength(v, false, false)
			}
		case "max-height":
			if kw == "inherit" {
				s.maxHeight = parent.maxHeight
			} else if kw == "initial" {
				s.maxHeight = cssLength{}
			} else {
				s.maxHeight, _ = parseLength(v, false, true)
			}
		case "float":
			if kw == "inherit" {
				s.floatSide = parent.floatSide
			} else if kw == "initial" {
				s.floatSide = "none"
			} else {
				s.floatSide = v
			}
		case "clear":
			if kw == "inherit" {
				s.clear = parent.clear
			} else if kw == "initial" {
				s.clear = "none"
			} else {
				s.clear = v
			}
		case "position":
			if kw == "inherit" {
				s.position = parent.position
			} else if kw == "initial" {
				s.position = "static"
			} else {
				s.position = v
			}
		case "z-index":
			if kw == "inherit" {
				s.zIndex = parent.zIndex
			} else if kw == "initial" || strings.EqualFold(v, "auto") {
				s.zIndex = 0
			} else if z, err := strconv.Atoi(v); err == nil {
				s.zIndex = max(-1000000, min(1000000, z))
			}
		case "top", "right", "bottom", "left":
			for i, side := range borderSides {
				if k == side {
					if kw == "inherit" {
						s.inset[i] = parent.inset[i]
					} else if kw == "initial" {
						s.inset[i] = cssLength{}
					} else {
						s.inset[i], _ = parseInset(v)
					}
				}
			}
		case "box-sizing":
			if kw == "inherit" {
				s.borderBox = parent.borderBox
			} else if kw == "initial" || v == "content-box" {
				s.borderBox = false
			} else if v == "border-box" {
				s.borderBox = true
			}
		case "display":
			if kw == "inherit" {
				s.display = parent.display
			} else if kw == "initial" {
				s.display = "inline"
			} else if v == "none" || v == "block" || v == "inline" || v == "inline-block" || v == "flex" || v == "inline-flex" || v == "grid" || v == "inline-grid" {
				s.display = v
			}
		case "grid-template-columns":
			if kw == "inherit" {
				if len(parent.gridCols) > 0 {
					s.gridCols = append([]gridTrack(nil), parent.gridCols...)
				} else {
					s.gridCols = nil
				}
			} else if kw == "initial" {
				s.gridCols = nil
			} else if cols, ok := parseGridTemplateColumns(v); ok {
				for i := range cols {
					cols[i].minLen = s.computeLen(cols[i].minLen)
					cols[i].maxLen = s.computeLen(cols[i].maxLen)
				}
				s.gridCols = cols
			}
		case "grid-column-start":
			if kw == "inherit" {
				s.gridColStart, s.gridColSpan = parent.gridColStart, parent.gridColSpan
			} else if kw == "initial" {
				s.gridColStart, s.gridColSpan = 0, 0
			} else if line, span, ok := parseGridLineOrSpan(v); ok {
				s.gridColStart, s.gridColSpan = line, span
			}
		case "grid-column-end":
			if kw == "inherit" {
				s.gridColEnd, s.gridColEndSpan = parent.gridColEnd, parent.gridColEndSpan
			} else if kw == "initial" {
				s.gridColEnd, s.gridColEndSpan = 0, 0
			} else if line, span, ok := parseGridLineOrSpan(v); ok {
				s.gridColEnd, s.gridColEndSpan = line, span
			}
		case "white-space":
			if kw == "inherit" {
				s.whiteSpace, s.pre = parent.whiteSpace, parent.pre
			} else if kw == "initial" {
				s.whiteSpace, s.pre = "normal", false
			} else {
				switch v {
				case "normal", "nowrap", "pre", "pre-wrap", "pre-line", "break-spaces":
					s.whiteSpace = v
					s.pre = v == "pre" || v == "pre-wrap" || v == "break-spaces"
				}
			}
		case "overflow-wrap", "word-wrap":
			if kw == "inherit" {
				s.overflowWrap = parent.overflowWrap
			} else if kw == "initial" {
				s.overflowWrap = "normal"
			} else {
				switch v {
				case "normal", "break-word", "anywhere":
					s.overflowWrap = v
				}
			}
		case "border-spacing":
			if kw == "inherit" {
				s.spacing = parent.spacing
			} else if kw == "initial" {
				s.spacing = [2]int{2, 2}
			} else {
				parts, ok := cssTokens(v)
				if ok && (len(parts) == 1 || len(parts) == 2) {
					a, ok := parseLength(parts[0], false, false)
					b := a
					valid := ok && a.absolute()
					if len(parts) == 2 {
						b, ok = parseLength(parts[1], false, false)
						valid = valid && ok && b.absolute()
					}
					if valid {
						s.spacing = [2]int{s.computeLen(a).resolve(0, 256), s.computeLen(b).resolve(0, 256)}
					}
				}
			}
		case "opacity":
			if kw == "inherit" {
				own := 1.0
				if parent.opacitySet {
					own = parent.ownOpacity
				}
				s.ownOpacity = own
				s.opacity = parent.effectiveOpacity() * own
				s.opacitySet = parent.opacitySet
			} else if kw == "initial" {
				s.ownOpacity = 1
				s.opacity = parent.effectiveOpacity()
				s.opacitySet = parent.opacitySet
			} else if op, ok := parseOpacity(v); ok {
				s.ownOpacity = op
				s.opacity = parent.effectiveOpacity() * op
				s.opacitySet = true
			}
		case "aspect-ratio":
			if kw == "inherit" {
				s.aspectRatio = parent.aspectRatio
			} else if kw == "initial" {
				s.aspectRatio = 0
			} else if ar, ok := parseAspectRatio(v); ok {
				s.aspectRatio = ar
			}
		case "object-fit":
			if kw == "inherit" {
				s.objectFit = parent.objectFit
			} else if kw == "initial" {
				s.objectFit = ""
			} else if isObjectFit(v) {
				s.objectFit = strings.ToLower(strings.TrimSpace(v))
			}
		case "vertical-align":
			if kw == "inherit" {
				s.vertical = parent.vertical
			} else if kw == "initial" {
				s.vertical = ""
			} else if v == "top" || v == "middle" || v == "bottom" || v == "sub" || v == "super" || v == "baseline" {
				s.vertical = v
			}
		case "text-align":
			if kw == "inherit" {
				s.align = parent.align
				s.centerBlock = parent.centerBlock
			} else if kw == "initial" {
				s.align = ""
				s.centerBlock = false
			} else {
				switch strings.ToLower(v) {
				case "left", "right", "center", "justify":
					s.align = strings.ToLower(v)
				case "start":
					s.align = "left"
				case "end":
					s.align = "right"
				}
				if s.align != "center" {
					s.centerBlock = false
				}
			}
		case "visibility":
			if kw == "inherit" {
				s.hidden = parent.hidden
			} else if kw == "initial" {
				s.hidden = false
			} else {
				switch strings.ToLower(v) {
				case "hidden", "collapse":
					s.hidden = true
				case "visible":
					s.hidden = false
				}
			}
		default:
			for i, side := range borderSides {
				if k == "margin-"+side {
					if kw == "inherit" {
						s.margin[i] = parent.margin[i]
					} else if kw == "initial" {
						s.margin[i] = cssLength{}
					} else {
						s.margin[i], _ = parseInset(v)
					}
				}
				if k == "padding-"+side {
					if kw == "inherit" {
						s.padding[i] = parent.padding[i]
					} else if kw == "initial" {
						s.padding[i] = cssLength{}
					} else {
						s.padding[i], _ = parseLength(v, false, false)
					}
				}
				if k == "border-"+side+"-width" {
					if kw == "inherit" {
						rawBW[i] = parent.bw[i]
					} else if kw == "initial" {
						rawBW[i] = 3
					} else if l, ok := borderWidth(v); ok {
						rawBW[i] = s.computeLen(l).resolve(0, 256)
					}
				}
				if k == "border-"+side+"-style" {
					if kw == "inherit" {
						s.borderStyle[i] = parent.borderStyle[i]
					} else if kw == "initial" {
						s.borderStyle[i] = "none"
					} else if isBorderStyle(v) {
						s.borderStyle[i] = strings.ToLower(v)
					}
				}
				if k == "border-"+side+"-color" {
					if kw == "inherit" {
						s.border[i], s.borderSet[i] = parent.border[i], parent.borderSet[i]
					} else if kw == "initial" {
						s.border[i], s.borderSet[i] = color.RGBA{}, false
					} else if c, ok := cssColor(v, s.fg); ok {
						s.border[i], s.borderSet[i] = c, true
					}
				}
			}
		}
	}
	if s.opacitySet {
		eff := s.effectiveOpacity()
		if eff <= 0 {
			s.hidden = true
		} else if eff < 1 {
			if fgInherited {
				s.fg = scaleColorAlpha(s.fg, s.ownOpacity)
			} else {
				s.fg = scaleColorAlpha(s.fg, eff)
			}
			s.bg = scaleColorAlpha(s.bg, eff)
			for i := range s.border {
				if s.borderSet[i] {
					s.border[i] = scaleColorAlpha(s.border[i], eff)
				}
			}
		}
	}
	for i := range s.inset {
		s.inset[i] = s.computeLen(s.inset[i])
	}
	if n.tag == "document" || n.tag == "html" || n.tag == "li" || n.tag == "caption" || n.tag == "tr" || n.tag == "td" || n.tag == "th" || n.tag == "thead" || n.tag == "tbody" || n.tag == "tfoot" {
		s.position = "static"
		s.floatSide = "none"
	}
	if s.position == "relative" && s.display == "inline" && n.tag != "img" {
		s.position = "static"
	}
	if s.position == "absolute" {
		s.floatSide = "none"
	}
	if (s.position == "absolute" || s.floatSide == "left" || s.floatSide == "right") && s.display != "none" {
		s.display = "block"
	}
	for i := range s.bw {
		switch s.borderStyle[i] {
		case "solid", "dashed", "dotted":
			s.bw[i] = rawBW[i]
		default:
			s.bw[i] = 0
		}
	}
	for _, p := range []*cssLength{&s.width, &s.height, &s.maxWidth, &s.minWidth, &s.minHeight, &s.maxHeight} {
		*p = s.computeLen(*p)
	}
	for i := range s.margin {
		s.margin[i] = s.computeLen(s.margin[i])
		s.padding[i] = s.computeLen(s.padding[i])
	}
	if s.lineMode == 2 {
		s.line = min(256, int(float64(s.size)*s.lineMultiplier))
	}

	if s.lineMode == 0 {
		if s.size == 0 {
			s.line = 0
		} else {
			s.line = max(s.line, s.size+2)
		}
	}
	return s
}

func (d *Document) cascade(n *node) map[string]string {
	return d.cascadePseudo(n, "")
}

func (d *Document) cascadePseudo(n *node, pseudo string) map[string]string {
	values := map[string]string{}
	scores := map[string]int{}
	w := d.cascadedWidth
	if w <= 0 {
		w = 800
	}
	scheme := normalizeColorScheme(d.cascadedScheme)
	for _, r := range d.rules {
		if r.pseudo == pseudo && r.matchesMedia(w, scheme) && r.selector.matches(n) {
			for k, v := range r.decl {
				v, important := importance(v)
				score := r.specificity
				if important {
					score += 1 << 30
				}
				if sc, ok := scores[k]; !ok || score >= sc {
					values[k] = v
					scores[k] = score
				}
			}
		}
	}
	if pseudo == "" {
		for k, v := range declarations(n.attrs["style"]) {
			v, important := importance(v)
			score := 1 << 29
			if important {
				score += 1 << 30
			}
			if score >= scores[k] {
				values[k] = v
			}
		}
	}
	return values
}
