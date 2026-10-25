package document

import (
	"image/color"
	"sort"
	"strconv"
	"strings"
)

type rule struct {
	selector    selector
	decl        map[string]string
	specificity int
}
type style struct {
	floatSide, clear                string
	position                        string
	inset                           [4]cssLength
	borderSet                       bool
	rootSize                        int
	minWidth, minHeight, maxHeight  cssLength
	whiteSpace, overflowWrap        string
	spacing                         [2]int
	collapse                        bool
	size, line                      int
	fg, bg, border                  color.RGBA
	margin, padding                 [4]cssLength // top, right, bottom, left
	bw                              int
	width, height, maxWidth         cssLength
	borderBox                       bool
	vertical                        string
	display, align, family          string
	bold, italic, underline, strike bool
	mono, pre                       bool
	linkID                          int
	lineMultiplier                  float64
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
	for i, p := range strings.Split(s, ";") {
		if i >= 128 {
			break
		}
		k, v, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v, important = importance(strings.TrimSpace(v))
		if k == "color" || k == "background" || k == "background-color" || k == "border-color" {
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
		if k == "font-size" {
			if _, ok := parseLength(v, false, false); !ok {
				continue
			}
		}
		if k == "line-height" && v != "normal" {
			if _, ok := parseLength(v, false, false); !ok {
				continue
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
			parts := strings.Fields(v)
			valid := len(parts) == 1 || len(parts) == 2
			for _, p := range parts {
				l, ok := parseLength(p, false, false)
				valid = valid && ok && l.absolute()
			}
			if !valid {
				continue
			}
		}
		switch k {
		case "margin", "padding":
			parts := strings.Fields(v)
			if len(parts) < 1 || len(parts) > 4 {
				continue
			}
			valid := true
			for _, part := range parts {
				if _, ok := parseLength(part, k == "margin", false); !ok {
					valid = false
				}
			}
			if !valid {
				continue
			}
			sides := [4]string{parts[0], parts[0], parts[0], parts[0]}
			if len(parts) > 1 {
				sides[1] = parts[1]
				sides[3] = parts[1]
			}
			if len(parts) > 2 {
				sides[2] = parts[2]
			}
			if len(parts) > 3 {
				sides[3] = parts[3]
			}
			for j, side := range []string{"top", "right", "bottom", "left"} {
				put(k+"-"+side, sides[j])
			}
		case "border":
			if values, ok := borderValues(v); ok {
				for key, value := range values {
					put(key, value)
				}
			}
		case "border-width":
			if _, ok := borderWidth(v); ok {
				put(k, v)
			}
		case "border-style":
			if strings.EqualFold(v, "solid") || strings.EqualFold(v, "none") {
				put(k, strings.ToLower(v))
			}
		case "word-wrap":
			put("overflow-wrap", v)
		case "background":
			put("background-color", v)
		case "text-decoration":
			put("text-decoration-line", v)
		default:
			if strings.HasPrefix(k, "margin-") || strings.HasPrefix(k, "padding-") || k == "width" || k == "height" || k == "max-width" || k == "min-width" || k == "min-height" || k == "max-height" {
				if _, ok := parseLength(v, strings.HasPrefix(k, "margin-") || k == "width" || k == "height", k == "max-width" || k == "max-height"); !ok {
					continue
				}
			}
			put(k, v)
		}
	}
	return m
}
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
	for {
		sel, rest, ok := strings.Cut(s, "{")
		if !ok {
			break
		}
		body, tail, ok := strings.Cut(rest, "}")
		if !ok {
			break
		}
		s = tail
		var group []rule
		valid := true
		for _, v := range strings.Split(sel, ",") {
			compiled, sp, ok := parseSelector(strings.TrimSpace(v))
			if !ok {
				valid = false
				break
			}
			group = append(group, rule{compiled, nil, sp})
			if len(group) > budget {
				return nil, limitError("more CSS rules than the budget")
			}
		}
		if valid {
			decl := declarations(body)
			for _, r := range group {
				r.decl = decl
				out = append(out, r)
			}
			if len(out) > budget {
				return nil, limitError("more CSS rules than the budget")
			}
		}
	}
	return out, nil
}
func (d *Document) styled(n *node, parent style) style {
	s := style{rootSize: parent.rootSize, whiteSpace: parent.whiteSpace, overflowWrap: parent.overflowWrap, spacing: parent.spacing, collapse: parent.collapse, family: parent.family, italic: parent.italic, underline: parent.underline, strike: parent.strike, lineMultiplier: parent.lineMultiplier, mono: parent.mono, pre: parent.pre, linkID: parent.linkID, size: parent.size, line: parent.line, fg: parent.fg, bold: parent.bold, align: parent.align, display: "block"}
	if s.rootSize == 0 {
		s.rootSize = 16
	}
	if n.tag == "svg" {
		s.width, _ = parseLength(n.attrs["width"], true, false)
		s.height, _ = parseLength(n.attrs["height"], true, false)
	}
	switch n.tag {
	case "span", "a", "em", "strong", "b", "i", "code", "small", "u", "s", "del":
		s.display = "inline"
	case "script", "style", "head", "meta", "link", "title", "template":
		s.display = "none"
	case "h1":
		s.size = 32
		s.line = 40
		s.lineMultiplier = 0
		s.bold = true
		s.margin = uniform(12)
	case "h2":
		s.size = 24
		s.line = 32
		s.lineMultiplier = 0
		s.bold = true
		s.margin = uniform(10)
	case "body":
		s.margin = uniform(8)
	case "p":
		s.margin = [4]cssLength{{value: 1, unit: lengthEM}, {}, {value: 1, unit: lengthEM}, {}}
	case "ul", "ol":
		s.margin = uniform(8)
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
		s.padding = uniform(12)
		s.margin = uniform(8)
		s.bg = color.RGBA{238, 241, 246, 255}
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
			s.fg = color.RGBA{40, 92, 186, 255}
		}
	}
	if n.tag == "strong" || n.tag == "b" || n.tag == "th" {
		s.bold = true
	}
	values := n.decl
	if c, ok := cssColor(values["color"], parent.fg); ok {
		s.fg = c
	}
	if value, ok := parseLength(values["font-size"], false, false); ok {
		value = value.computed(max(1, parent.size), s.rootSize)
		if size := value.resolve(parent.size, 128); size > 0 {
			s.size = size
		}
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
		switch k {
		case "font-size": // Resolved before all font-relative properties.

		case "font-family":
			s.family = v
			s.mono = false
		case "font-style":
			switch strings.ToLower(v) {
			case "italic", "oblique":
				s.italic = true
			case "normal":
				s.italic = false
			}
		case "font-weight":
			if v == "bold" {
				s.bold = true
			} else if v == "normal" {
				s.bold = false
			} else if weight, e := strconv.Atoi(v); e == nil && weight >= 100 && weight <= 900 {
				s.bold = weight >= 600
			}
		case "line-height":
			if v == "normal" {
				s.lineMultiplier = 1.375
			} else if cssNumber.MatchString(v) {
				if l, ok := parseLength(v, false, false); ok && l.value > 0 {
					s.lineMultiplier = min(l.value, 16)
				}
			} else if l, ok := parseLength(v, false, false); ok {
				if line := l.computed(s.size, s.rootSize).resolve(s.size, 256); line > 0 {
					s.line = line
					s.lineMultiplier = 0
				}
			}
		case "text-decoration-line":
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
			s.bg, _ = cssColor(v, s.fg)
		case "border-color":
			s.border, _ = cssColor(v, s.fg)
			s.borderSet = true
		case "border-collapse":
			if v == "collapse" {
				s.collapse = true
			} else if v == "separate" {
				s.collapse = false
			}
		case "border-width":
			if l, ok := borderWidth(v); ok {
				s.bw = l.computed(s.size, s.rootSize).resolve(0, 32)
			}
		case "width":
			s.width, _ = parseLength(v, true, false)
		case "height":
			s.height, _ = parseLength(v, true, false)
		case "max-width":
			s.maxWidth, _ = parseLength(v, false, true)
		case "min-width":
			s.minWidth, _ = parseLength(v, false, false)
		case "min-height":
			s.minHeight, _ = parseLength(v, false, false)
		case "max-height":
			s.maxHeight, _ = parseLength(v, false, true)
		case "float":
			s.floatSide = v
		case "clear":
			s.clear = v
		case "position":
			s.position = v
		case "top", "right", "bottom", "left":
			for i, side := range []string{"top", "right", "bottom", "left"} {
				if k == side {
					s.inset[i], _ = parseInset(v)
				}
			}
		case "box-sizing":
			if v == "border-box" {
				s.borderBox = true
			} else if v == "content-box" {
				s.borderBox = false
			}
		case "display":
			if v == "none" || v == "block" || v == "inline" {
				s.display = v
			}
		case "white-space":
			switch v {
			case "normal", "nowrap", "pre", "pre-wrap", "pre-line", "break-spaces":
				s.whiteSpace = v
				s.pre = v == "pre" || v == "pre-wrap" || v == "break-spaces"
			}
		case "overflow-wrap", "word-wrap":
			switch v {
			case "normal", "break-word", "anywhere":
				s.overflowWrap = v
			}
		case "border-spacing":
			parts := strings.Fields(v)
			if len(parts) == 1 || len(parts) == 2 {
				a, ok := parseLength(parts[0], false, false)
				b := a
				valid := ok && a.absolute()
				if len(parts) == 2 {
					b, ok = parseLength(parts[1], false, false)
					valid = valid && ok && b.absolute()
				}
				if valid {
					s.spacing = [2]int{a.computed(s.size, s.rootSize).resolve(0, 256), b.computed(s.size, s.rootSize).resolve(0, 256)}
				}
			}
		case "vertical-align":
			if v == "top" || v == "middle" || v == "bottom" {
				s.vertical = v
			}
		case "text-align":
			s.align = v
		default:
			for i, side := range []string{"top", "right", "bottom", "left"} {
				if k == "margin-"+side {
					s.margin[i], _ = parseLength(v, true, false)
				}
				if k == "padding-"+side {
					s.padding[i], _ = parseLength(v, false, false)
				}
			}
		}
	}
	for i := range s.inset {
		s.inset[i] = s.inset[i].computed(s.size, s.rootSize)
	}
	if n.tag == "document" || n.tag == "html" || n.tag == "li" || n.tag == "caption" || n.tag == "tr" || n.tag == "td" || n.tag == "th" || n.tag == "thead" || n.tag == "tbody" || n.tag == "tfoot" {
		s.position = "static"
		s.floatSide = "none"
	}
	if s.position == "relative" && s.display == "inline" {
		s.position = "static"
	}
	if s.position == "absolute" {
		s.floatSide = "none"
	}
	if (s.position == "absolute" || s.floatSide == "left" || s.floatSide == "right") && s.display != "none" {
		s.display = "block"
	}
	if values["border-style"] == "none" {
		s.bw = 0
	} else if values["border-style"] == "solid" && values["border-width"] == "" {
		s.bw = 3
	}
	for _, p := range []*cssLength{&s.width, &s.height, &s.maxWidth, &s.minWidth, &s.minHeight, &s.maxHeight} {
		*p = p.computed(s.size, s.rootSize)
	}
	for i := range s.margin {
		s.margin[i] = s.margin[i].computed(s.size, s.rootSize)
		s.padding[i] = s.padding[i].computed(s.size, s.rootSize)
	}
	if s.lineMultiplier > 0 {
		s.line = min(256, int(float64(s.size)*s.lineMultiplier))
	}

	s.line = max(s.line, s.size+2)
	return s
}

func (d *Document) cascade(n *node) map[string]string {
	values := map[string]string{}
	scores := map[string]int{}
	for _, r := range d.rules {
		if r.selector.matches(n) {
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
	return values
}
