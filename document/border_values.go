package document

import (
	"image/color"
	"strings"
)

func borderWidth(v string) (cssLength, bool) {
	switch strings.ToLower(v) {
	case "thin":
		return px(1), true
	case "medium":
		return px(3), true
	case "thick":
		return px(5), true
	}
	l, ok := parseLength(v, false, false)
	return l, ok && l.absolute()
}

// Solid/none borders accept CSS's unordered width/style/color components.
func borderValues(v string) (map[string]string, bool) {
	parts := []string{}
	start, depth := 0, 0
	for i, r := range v {
		if r == '(' {
			depth++
		}
		if r == ')' {
			depth--
			if depth < 0 {
				return nil, false
			}
		}
		if cssSpace(r) && depth == 0 {
			if start < i {
				parts = append(parts, v[start:i])
			}
			start = i + 1
		}
	}
	if depth != 0 {
		return nil, false
	}
	if start < len(v) {
		parts = append(parts, v[start:])
	}
	if len(parts) == 0 || len(parts) > 3 {
		return nil, false
	}
	result := map[string]string{"border-width": "medium", "border-style": "none", "border-color": "currentColor"}
	width, style, hasColor := false, false, false
	for _, part := range parts {
		if _, ok := borderWidth(part); ok {
			if width {
				return nil, false
			}
			width = true
			result["border-width"] = part
		} else if strings.EqualFold(part, "solid") || strings.EqualFold(part, "none") {
			if style {
				return nil, false
			}
			style = true
			result["border-style"] = strings.ToLower(part)
		} else if _, ok := cssColor(part, color.RGBA{}); ok {
			if hasColor {
				return nil, false
			}
			hasColor = true
			result["border-color"] = part
		} else {
			return nil, false
		}
	}
	return result, true
}
