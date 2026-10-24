package document

import (
	"image/color"
	"sort"
	"strconv"
	"strings"
)

type rule struct {
	selector    string
	decl        map[string]string
	specificity int
}
type style struct {
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
	for i, p := range strings.Split(s, ";") {
		if i >= 128 {
			break
		}
		k, v, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
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
				m[k+"-"+side] = sides[j]
			}
		case "border":
			parts := strings.Fields(v)
			if len(parts) > 0 {
				if _, ok := parseLength(parts[0], false, false); ok {
					m["border-width"] = parts[0]
				}
			}
			if len(parts) > 2 {
				m["border-color"] = parts[2]
			}
		case "background":
			m["background-color"] = v
		case "text-decoration":
			m["text-decoration-line"] = v
		default:
			if strings.HasPrefix(k, "margin-") || strings.HasPrefix(k, "padding-") || k == "width" || k == "height" || k == "max-width" {
				if _, ok := parseLength(v, strings.HasPrefix(k, "margin-") || k == "width" || k == "height", k == "max-width"); !ok {
					continue
				}
			}
			m[k] = v
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
		for _, v := range strings.Split(sel, ",") {
			v = strings.TrimSpace(v)
			if !simpleSelector(v) {
				continue
			}
			sp := 1
			if v == "*" {
				sp = 0
			} else if v[0] == '.' {
				sp = 10
			} else if v[0] == '#' {
				sp = 100
			}
			out = append(out, rule{v, declarations(body), sp})
			if len(out) > budget {
				return nil, ErrLimit
			}
		}
	}
	return out, nil
}
func simpleSelector(s string) bool {
	if s == "*" {
		return true
	}
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && (r == '.' || r == '#') {
			continue
		}
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func matches(n *node, s string) bool {
	if s == "*" {
		return true
	}
	if strings.HasPrefix(s, "#") {
		return n.attrs["id"] == s[1:]
	}
	if strings.HasPrefix(s, ".") {
		for _, c := range strings.Fields(n.attrs["class"]) {
			if c == s[1:] {
				return true
			}
		}
		return false
	}
	return n.tag == s
}
func number(s string) int {
	l, ok := parseLength(s, false, false)
	if !ok || l.unit != lengthPX {
		return 0
	}
	return l.resolve(0, 16384)
}
func paint(s string) color.RGBA {
	names := map[string]string{"black": "#000000", "white": "#ffffff", "red": "#ff0000", "green": "#008000", "blue": "#0000ff", "gray": "#808080", "transparent": ""}
	s = strings.ToLower(s)
	if v, ok := names[s]; ok {
		s = v
	}
	if len(s) == 4 && s[0] == '#' {
		s = "#" + strings.Repeat(s[1:2], 2) + strings.Repeat(s[2:3], 2) + strings.Repeat(s[3:4], 2)
	}
	if len(s) == 7 && s[0] == '#' {
		v, e := strconv.ParseUint(s[1:], 16, 32)
		if e == nil {
			return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
		}
	}
	return color.RGBA{}
}
func (d *Document) styled(n *node, parent style) style {
	s := style{collapse: parent.collapse, family: parent.family, italic: parent.italic, underline: parent.underline, strike: parent.strike, lineMultiplier: parent.lineMultiplier, mono: parent.mono, pre: parent.pre, linkID: parent.linkID, size: parent.size, line: parent.line, fg: parent.fg, bold: parent.bold, align: parent.align, display: "block"}
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
	case "p", "ul", "ol":
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
	values := map[string]string{}
	scores := map[string]int{}
	for _, r := range d.rules {
		if matches(n, r.selector) {
			for k, v := range r.decl {
				if sc, ok := scores[k]; !ok || r.specificity >= sc {
					values[k] = v
					scores[k] = r.specificity
				}
			}
		}
	}
	for k, v := range declarations(n.attrs["style"]) {
		values[k] = v
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := values[k]
		switch k {
		case "font-size":
			if value, ok := parseLength(v, false, false); ok {
				if size := value.resolve(parent.size, 128); size > 0 {
					s.size = size
				}
			}
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
			} else if !strings.HasSuffix(v, "px") && !strings.HasSuffix(v, "%") {
				if l, ok := parseLength(v, false, false); ok && l.value > 0 {
					s.lineMultiplier = min(l.value, 16)
				}
			} else if l, ok := parseLength(v, false, false); ok {
				if line := l.resolve(s.size, 256); line > 0 {
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
			s.fg = paint(v)
		case "background-color":
			s.bg = paint(v)
		case "border-color":
			s.border = paint(v)
		case "border-collapse":
			if v == "collapse" {
				s.collapse = true
			} else if v == "separate" {
				s.collapse = false
			}
		case "border-width":
			s.bw = min(number(v), 32)
		case "width":
			s.width, _ = parseLength(v, true, false)
		case "height":
			s.height, _ = parseLength(v, true, false)
		case "max-width":
			s.maxWidth, _ = parseLength(v, false, true)
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
	if s.lineMultiplier > 0 {
		s.line = min(256, int(float64(s.size)*s.lineMultiplier))
	}

	s.line = max(s.line, s.size+2)
	return s
}
