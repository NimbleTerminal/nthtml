package document

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func isControl(n *node) bool {
	switch n.tag {
	case "input", "button", "select", "textarea", "progress", "meter":
		return true
	}
	return false
}
func inputType(n *node) string { return strings.ToLower(strings.TrimSpace(n.attrs["type"])) }
func charCount(n *node, key string, fallback int) int {
	v, err := strconv.Atoi(n.attrs[key])
	if err != nil || v <= 0 {
		return fallback
	}
	return min(v, 4096)
}
func textContent(n *node) string {
	var b strings.Builder
	var walk func(*node)
	walk = func(n *node) {
		if n.pseudo != "" {
			return
		}
		b.WriteString(n.text)
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
func controlText(n *node) (string, bool) {
	switch n.tag {
	case "input":
		value := n.attrs["value"]
		switch inputType(n) {
		case "hidden", "checkbox", "radio":
			return "", false
		case "submit":
			if !hasAttr(n, "value") {
				value = "Submit"
			}
		case "reset":
			if !hasAttr(n, "value") {
				value = "Reset"
			}
		case "password":
			value = strings.Repeat("•", utf8.RuneCountInString(value))
		}
		if value == "" && inputType(n) != "button" {
			value = n.attrs["placeholder"]
		}
		return value, true
	case "select":
		var first, selected *node
		var walk func(*node)
		walk = func(p *node) {
			for _, c := range p.children {
				if c.tag == "option" {
					if first == nil {
						first = c
					}
					if selected == nil && hasAttr(c, "selected") {
						selected = c
					}
				} else {
					walk(c)
				}
			}
		}
		walk(n)
		if selected == nil {
			selected = first
		}
		if selected != nil {
			return strings.TrimSpace(textContent(selected)) + " ▾", true
		}
		return "▾", true
	}
	return "", false
}

func controlDefaults(n *node, s *style, bw *[4]int) {
	if !isControl(n) && n.tag != "fieldset" {
		return
	}
	s.display = "inline-block"
	s.padding = uniform(2)
	s.overflow = "hidden"
	*bw = [4]int{1, 1, 1, 1}
	s.borderStyle = [4]string{"solid", "solid", "solid", "solid"}
	if n.tag == "fieldset" {
		s.display = "block"
		s.padding = uniform(8)
		*bw = [4]int{2, 2, 2, 2}
		s.overflow = ""
	}
	if n.tag == "textarea" {
		s.family = "Go Mono"
		s.mono = true
		s.pre = true
		s.whiteSpace = "pre-wrap"
		s.overflowWrap = "anywhere"
	}
	if n.tag == "input" {
		s.whiteSpace = "nowrap"
		if inputType(n) == "hidden" {
			s.display = "none"
		}
		if n.attrs["value"] == "" && n.attrs["placeholder"] != "" {
			s.fg = color.RGBA{117, 117, 117, 255}
		}
	}
	if n.tag == "button" || n.tag == "select" || n.tag == "input" && (inputType(n) == "submit" || inputType(n) == "reset" || inputType(n) == "button") {
		s.bg = color.RGBA{239, 239, 239, 255}
		s.padding = [4]cssLength{px(2), px(6), px(2), px(6)}
	}
	if n.tag == "progress" || n.tag == "meter" {
		s.width = cssLength{value: 10, unit: lengthEM}
		s.height = cssLength{value: 1, unit: lengthEM}
		s.padding = uniform(0)
		s.bg = color.RGBA{230, 230, 230, 255}
	}
	if n.tag == "input" && (inputType(n) == "checkbox" || inputType(n) == "radio") {
		s.width = px(13)
		s.height = px(13)
		s.borderBox = true
		s.padding = uniform(0)
	}
}

// Character dimensions use the computed font, after author font declarations.
func (l *layout) controlSize(n *node, s style) style {
	cols, rows := 0, 0
	if n.tag == "textarea" {
		cols, rows = charCount(n, "cols", 20), charCount(n, "rows", 2)
	}
	if n.tag == "input" {
		switch inputType(n) {
		case "hidden", "checkbox", "radio", "submit", "reset", "button":
		default:
			cols, rows = charCount(n, "size", 20), 1
		}
	}
	if cols > 0 {
		if _, ok := n.decl["width"]; !ok {
			s.width = px(min(16384, cols*l.glyphAdvanceStyled(s, l.face(s), '0')))
		}
		if _, ok := n.decl["height"]; !ok {
			s.height = px(min(l.d.limits.Height, rows*s.line))
		}
	}
	return s
}

func controlFraction(n *node) float64 {
	number := func(key string, fallback float64) float64 {
		v, err := strconv.ParseFloat(n.attrs[key], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return fallback
		}
		return v
	}
	low := 0.0
	if n.tag == "meter" {
		low = number("min", 0)
	}
	high := number("max", 1)
	if high <= low {
		high = low + 1
	}
	return max(0, min(1, (number("value", 0)-low)/(high-low)))
}

func (l *layout) paintControl(n *node, r image.Rectangle, s style) {
	if s.hidden {
		return
	}
	if n.tag == "progress" || n.tag == "meter" {
		r.Max.X = r.Min.X + int(float64(r.Dx())*controlFraction(n))
		l.rect(r, s.fg, "control-fill")
	}
	if n.tag == "input" && hasAttr(n, "checked") && inputType(n) == "checkbox" {
		l.rect(r.Inset(3), s.fg, "control-check")
	}
}

// Circles are emitted as bounded scanline rectangles, using the same operation
// budget as all other painting. No separate raster allocation is required.
func (l *layout) paintRadio(r image.Rectangle, n *node, s style) {
	radius := float64(min(r.Dx(), r.Dy())) / 2
	cx, cy := float64(r.Min.X)+float64(r.Dx())/2, float64(r.Min.Y)+float64(r.Dy())/2
	border := s.fg
	if s.borderSet[0] {
		border = s.border[0]
	}
	for y := r.Min.Y; y < r.Max.Y && l.err == nil; y++ {
		dy := float64(y) + 0.5 - cy
		if math.Abs(dy) >= radius {
			continue
		}
		dx := math.Sqrt(radius*radius - dy*dy)
		left, right := int(math.Ceil(cx-dx)), int(math.Ceil(cx+dx))
		if s.bw[0] > 0 {
			inner := max(0, radius-float64(s.bw[0]))
			if math.Abs(dy) >= inner {
				l.rect(image.Rect(left, y, right, y+1), border, "border")
			} else {
				ix := math.Sqrt(inner*inner - dy*dy)
				l.rect(image.Rect(left, y, int(math.Ceil(cx-ix)), y+1), border, "border")
				l.rect(image.Rect(int(math.Ceil(cx+ix)), y, right, y+1), border, "border")
			}
		}
		dot := max(0, radius-3)
		if hasAttr(n, "checked") && math.Abs(dy) < dot {
			dx = math.Sqrt(dot*dot - dy*dy)
			l.rect(image.Rect(int(math.Ceil(cx-dx)), y, int(math.Ceil(cx+dx)), y+1), s.fg, "control-check")
		}
	}
}
