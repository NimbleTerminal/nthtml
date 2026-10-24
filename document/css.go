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
	size, line                         int
	fg, bg, border                     color.RGBA
	margin, padding, bw, width, height int
	display, align                     string
	bold                               bool
}

func declarations(s string) map[string]string {
	m := map[string]string{}
	for i, p := range strings.Split(s, ";") {
		if i >= 128 {
			break
		}
		k, v, ok := strings.Cut(p, ":")
		if ok {
			m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
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
	v, e := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 64)
	if e != nil || v < 0 || v > 16384 {
		return 0
	}
	return int(v)
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
	s := style{size: parent.size, line: parent.line, fg: parent.fg, bold: parent.bold, align: parent.align, display: "block"}
	switch n.tag {
	case "span", "a", "em", "strong", "b", "i", "code", "small":
		s.display = "inline"
	case "script", "style", "head", "meta", "link", "title", "template":
		s.display = "none"
	case "h1":
		s.size = 32
		s.line = 40
		s.bold = true
		s.margin = 12
	case "h2":
		s.size = 24
		s.line = 32
		s.bold = true
		s.margin = 10
	case "p", "ul", "ol":
		s.margin = 8
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
			if z := number(v); z > 0 {
				s.size = min(z, 128)
			}
		case "line-height":
			if z := number(v); z > 0 {
				s.line = min(z, 256)
			}
		case "color":
			s.fg = paint(v)
		case "background", "background-color":
			s.bg = paint(v)
		case "border-color":
			s.border = paint(v)
		case "border-width":
			s.bw = min(number(v), 32)
		case "border":
			p := strings.Fields(v)
			if len(p) > 0 {
				s.bw = min(number(p[0]), 32)
			}
			if len(p) > 2 {
				s.border = paint(p[2])
			}
		case "margin":
			s.margin = min(number(v), 256)
		case "padding":
			s.padding = min(number(v), 256)
		case "width":
			s.width = number(v)
		case "height":
			s.height = number(v)
		case "display":
			if v == "none" || v == "block" || v == "inline" {
				s.display = v
			}
		case "text-align":
			s.align = v
		case "font-weight":
			s.bold = v == "bold" || v == "700"
		}
	}
	s.line = max(s.line, s.size+2)
	return s
}
