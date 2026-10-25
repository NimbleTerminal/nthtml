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

func isBorderStyle(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none", "hidden", "solid", "dashed", "dotted":
		return true
	}
	return false
}

func cssTokens(v string) ([]string, bool) {
	var parts []string
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
	return parts, len(parts) > 0
}

func expandFour(parts []string) ([4]string, bool) {
	if len(parts) < 1 || len(parts) > 4 {
		return [4]string{}, false
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
	return sides, true
}

// Border shorthands accept CSS's unordered width/style/color components.
func borderValues(v string) (map[string]string, bool) {
	parts, ok := cssTokens(v)
	if !ok || len(parts) > 3 {
		return nil, false
	}
	result := map[string]string{"width": "medium", "style": "none", "color": "currentColor"}
	width, style, hasColor := false, false, false
	for _, part := range parts {
		if _, ok := borderWidth(part); ok {
			if width {
				return nil, false
			}
			width = true
			result["width"] = part
		} else if isBorderStyle(part) {
			if style {
				return nil, false
			}
			style = true
			result["style"] = strings.ToLower(part)
		} else if _, ok := cssColor(part, color.RGBA{}); ok {
			if hasColor {
				return nil, false
			}
			hasColor = true
			result["color"] = part
		} else {
			return nil, false
		}
	}
	return result, true
}
