package document

import (
	"image/color"
	"strconv"
	"strings"
)

// Supports reports the renderer's bounded CSS feature-query capabilities. It
// performs no layout, variable substitution, resource loading, or host action.
func Supports(condition string) bool {
	if len(condition) > maxMediaPreludeBytes || !supportsBalanced(condition) {
		return false
	}
	value, valid := supportsCondition(strings.TrimSpace(condition), 0)
	return valid && value
}

func supportsBalanced(s string) bool {
	var stack []byte
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			i++
			if i >= len(s) {
				return false
			}
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			stack = append(stack, c)
			if len(stack) > maxMediaParenDepth {
				return false
			}
		case ')', ']', '}':
			if len(stack) == 0 {
				return false
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if c == ')' && open != '(' || c == ']' && open != '[' || c == '}' && open != '{' {
				return false
			}
		}
	}
	return quote == 0 && len(stack) == 0
}

func supportsCondition(s string, depth int) (bool, bool) {
	if depth > maxMediaDepth || s == "" {
		return false, false
	}
	negate := false
	if len(s) > 3 && strings.EqualFold(s[:3], "not") && cssSpace(rune(s[3])) {
		negate = true
		s = strings.TrimSpace(s[3:])
	}
	result, rest, valid := supportsTerm(s, depth+1)
	if !valid {
		return false, false
	}
	if negate {
		return !result, rest == ""
	}
	operator := ""
	for rest != "" {
		i := strings.IndexFunc(rest, cssSpace)
		if i < 0 {
			return false, false
		}
		op := strings.ToLower(rest[:i])
		if op != "and" && op != "or" || operator != "" && operator != op {
			return false, false
		}
		operator = op
		value, next, ok := supportsTerm(strings.TrimSpace(rest[i:]), depth+1)
		if !ok {
			return false, false
		}
		if op == "and" {
			result = result && value
		} else {
			result = result || value
		}
		rest = next
	}
	return result, true
}

func supportsTerm(s string, depth int) (bool, string, bool) {
	if s == "" || depth > maxMediaDepth {
		return false, "", false
	}
	open := strings.IndexByte(s, '(')
	if open < 0 {
		return false, "", false
	}
	fn := s[:open]
	if fn != "" {
		for _, c := range fn {
			if !isMediaIdentChar(byte(c)) {
				return false, "", false
			}
		}
	}
	end := findMatchingParen(s[open:])
	if end < 0 {
		return false, "", false
	}
	end += open
	rest := s[end+1:]
	if rest != "" && !cssSpace(rune(rest[0])) {
		return false, "", false
	}
	rest = strings.TrimSpace(rest)
	inner := strings.TrimSpace(s[open+1 : end])
	if fn != "" {
		if strings.EqualFold(fn, "selector") {
			_, _, _, ok := parseSelectorWithPseudo(inner)
			return ok, rest, true
		}
		return false, rest, true // general-enclosed: unknown functions are unsupported
	}
	if key, value, ok := strings.Cut(inner, ":"); ok && !strings.ContainsAny(strings.TrimSpace(key), "() \t\r\n") {
		return supportsDeclaration(strings.TrimSpace(key), strings.TrimSpace(value)), rest, true
	}
	if strings.HasPrefix(inner, "(") || strings.HasPrefix(strings.ToLower(inner), "not ") || strings.Contains(inner, "(") {
		value, ok := supportsCondition(inner, depth+1)
		return value, rest, ok
	}
	// Unknown parenthesized syntax is general-enclosed, not a declaration.
	return false, rest, inner != ""
}

func supportsDeclaration(k, v string) bool {
	if !supportsBalanced(v) || strings.ContainsAny(v, ";!") {
		return false
	}
	if isCustomPropertyName(k) {
		return true
	}
	k = strings.ToLower(k)
	if v == "" {
		return false
	}
	decl := declarations(k + ":" + v)
	if len(decl) == 0 {
		return false
	}
	for key, value := range decl {
		if !supportsLonghand(key, value) {
			return false
		}
	}
	return true
}

// Reuse the resolver's value parsers. Properties implemented only as hints
// (masks, transforms, animation, container metadata) deliberately stay absent.
func supportsLonghand(k, v string) bool {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "\x00") {
		_, v, _ = strings.Cut(v, ":")
	}
	if containsVar(v) {
		// Validate var() names/fallback structure without resolving against a node.
		for start := indexVar(v); start >= 0; start = indexVar(v) {
			end := findMatchingParen(v[start+3:])
			if end < 0 {
				return false
			}
			end += start + 3
			name, _, _ := splitVarArgs(v[start+4 : end])
			if !isCustomPropertyName(strings.TrimSpace(name)) {
				return false
			}
			v = v[:start] + "0" + v[end+1:]
		}
		return supportedProperty(k)
	}
	if !supportedProperty(k) {
		return false
	}
	if isCSSKeyword(v) {
		return true
	}
	v = strings.ToLower(v)
	switch k {
	case "display":
		return supportedDisplay(v)
	case "position":
		return supportedPosition(v)
	case "color", "background-color", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color":
		if strings.HasPrefix(v, "light-dark(") && strings.HasSuffix(v, ")") {
			a, b, ok := splitVarArgs(v[len("light-dark(") : len(v)-1])
			return ok && supportsLonghand(k, a) && supportsLonghand(k, b)
		}
		_, ok := cssColor(v, color.RGBA{})
		return ok
	case "width", "height", "min-width", "min-height", "max-width", "max-height", "flex-basis":
		_, ok := parseLength(v, true, strings.HasPrefix(k, "max-"))
		return ok
	case "top", "right", "bottom", "left":
		_, ok := parseInset(v)
		return ok
	case "font-size":
		if oneOf(v, "xx-small x-small small medium large x-large xx-large smaller larger") {
			return true
		}
		_, ok := parseLength(v, false, false)
		return ok
	case "margin-top", "margin-right", "margin-bottom", "margin-left":
		_, ok := parseInset(v)
		return ok
	case "padding-top", "padding-right", "padding-bottom", "padding-left", "row-gap", "column-gap":
		_, ok := parseLength(v, false, false)
		return ok
	case "border-top-width", "border-right-width", "border-bottom-width", "border-left-width":
		if oneOf(v, "thin medium thick") {
			return true
		}
		_, ok := parseLength(v, false, false)
		return ok
	case "border-top-style", "border-right-style", "border-bottom-style", "border-left-style":
		return oneOf(v, "none hidden solid dashed dotted double groove ridge inset outset")
	case "z-index":
		if v == "auto" && k == "z-index" {
			return true
		}
		_, err := strconv.Atoi(v)
		return err == nil
	case "opacity":
		_, ok := parseOpacity(v)
		return ok
	case "aspect-ratio":
		_, ok := parseAspectRatio(v)
		return ok || v == "auto"
	case "grid-template-columns":
		_, ok := parseGridTemplateColumns(v)
		return ok || v == "none"
	case "font-weight":
		if oneOf(v, "normal bold bolder lighter") {
			return true
		}
		n, e := strconv.Atoi(v)
		return e == nil && n >= 1 && n <= 1000
	case "font-style":
		return oneOf(v, "normal italic oblique")
	case "font-variant":
		return oneOf(v, "normal small-caps")
	case "line-height":
		if v == "normal" {
			return true
		}
		_, ok := parseLength(v, false, false)
		n, e := strconv.ParseFloat(v, 64)
		return ok || e == nil && n >= 0 && n <= 1000
	case "text-transform":
		return oneOf(v, "none uppercase lowercase capitalize")
	case "text-align":
		return oneOf(v, "left right center justify start end")
	case "white-space":
		return oneOf(v, "normal nowrap pre pre-wrap pre-line break-spaces")
	case "visibility":
		return oneOf(v, "visible hidden collapse")
	case "box-sizing":
		return oneOf(v, "content-box border-box")
	case "overflow", "overflow-x", "overflow-y":
		return oneOf(v, "visible hidden clip auto scroll")
	case "flex-direction":
		return oneOf(v, "row row-reverse column column-reverse")
	case "flex-wrap":
		return oneOf(v, "nowrap wrap")
	case "justify-content":
		return oneOf(v, "start end flex-start flex-end center space-between space-around space-evenly")
	case "align-items":
		return oneOf(v, "stretch start end flex-start flex-end center baseline")
	case "flex-grow", "flex-shrink":
		n, e := strconv.ParseFloat(v, 64)
		return e == nil && n >= 0 && n <= 1e6
	case "color-scheme":
		return oneOf(v, "normal light dark") || v == "light dark" || v == "dark light" || v == "only light" || v == "only dark"
	case "background-image":
		_, ok := parseBackgroundImage(v)
		return ok
	case "background-size":
		_, _, ok := parseBackgroundSize(v)
		return ok
	case "background-position", "background-position-x", "background-position-y":
		_, ok := parseBackgroundPosition(v)
		return ok
	case "background-repeat":
		_, ok := parseBackgroundRepeat(v)
		return ok
	case "border-top-left-radius", "border-top-right-radius", "border-bottom-left-radius", "border-bottom-right-radius":
		_, ok := radiusValues(v, false)
		return ok
	case "border-collapse":
		return oneOf(v, "collapse separate")
	case "border-spacing":
		parts, ok := cssTokens(v)
		if !ok || len(parts) < 1 || len(parts) > 2 {
			return false
		}
		for _, part := range parts {
			if _, ok := parseLength(part, false, false); !ok {
				return false
			}
		}
		return true
	case "font-family":
		parts, ok := splitSelectorList(v)
		if !ok || len(parts) == 0 {
			return false
		}
		for _, part := range parts {
			if strings.TrimSpace(part) == "" || strings.ContainsAny(part, "(){};!") {
				return false
			}
		}
		return true
	case "word-spacing", "letter-spacing":
		_, ok := wordSpacingValue(v)
		return ok
	case "text-indent":
		_, ok := parseInset(v)
		return ok && v != "auto"
	case "list-style-type":
		return oneOf(v, "none disc circle square decimal decimal-leading-zero lower-alpha lower-latin upper-alpha upper-latin lower-roman upper-roman")
	case "caption-side":
		return oneOf(v, "top bottom")
	case "text-overflow":
		return oneOf(v, "clip ellipsis")
	case "text-decoration-line":
		for _, word := range strings.Fields(v) {
			if !oneOf(word, "none underline line-through") {
				return false
			}
		}
		return v != ""
	case "justify-items", "justify-self":
		return oneOf(v, "start end center stretch left right flex-start flex-end self-start self-end normal auto")
	case "float":
		return oneOf(v, "none left right")
	case "clear":
		return oneOf(v, "none left right both")
	case "grid-column-start", "grid-column-end":
		_, _, ok := parseGridLineOrSpan(v)
		return ok
	case "overflow-wrap", "word-wrap":
		return oneOf(v, "normal break-word anywhere")
	case "fill", "stroke":
		return svgPaintValue(v)
	case "object-fit":
		return isObjectFit(v)
	case "vertical-align":
		return oneOf(v, "top middle bottom sub super baseline")
	case "content":
		_, ok := parseContentValue(v)
		return ok
	}
	return false
}
func oneOf(v, values string) bool {
	for _, word := range strings.Fields(values) {
		if word == v {
			return true
		}
	}
	return false
}
func supportedDisplay(v string) bool {
	return oneOf(v, "none block inline inline-block flex inline-flex grid inline-grid -webkit-box -webkit-inline-box")
}
func supportedPosition(v string) bool { return oneOf(v, "static relative absolute fixed") }
func supportedProperty(k string) bool {
	switch k {
	case "background-image", "background-size", "background-position", "background-position-x", "background-position-y", "background-repeat", "border-top-left-radius", "border-top-right-radius", "border-bottom-left-radius", "border-bottom-right-radius", "border-collapse", "border-spacing", "font-family", "word-spacing", "letter-spacing", "text-indent", "list-style-type", "caption-side", "text-overflow", "text-decoration-line", "justify-items", "justify-self", "float", "clear", "grid-column-start", "grid-column-end", "overflow-wrap", "word-wrap", "fill", "stroke", "object-fit", "vertical-align":
		return true
	case "display", "position", "color", "background-color", "width", "height", "min-width", "min-height", "max-width", "max-height", "top", "right", "bottom", "left", "font-size", "font-weight", "font-style", "font-variant", "line-height", "margin-top", "margin-right", "margin-bottom", "margin-left", "padding-top", "padding-right", "padding-bottom", "padding-left", "row-gap", "column-gap", "border-top-width", "border-right-width", "border-bottom-width", "border-left-width", "border-top-style", "border-right-style", "border-bottom-style", "border-left-style", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color", "z-index", "opacity", "aspect-ratio", "grid-template-columns", "text-transform", "text-align", "white-space", "visibility", "box-sizing", "overflow", "overflow-x", "overflow-y", "flex-direction", "flex-wrap", "justify-content", "align-items", "flex-grow", "flex-shrink", "flex-basis", "color-scheme", "content":
		return true
	}
	return false
}
