package document

import (
	"strconv"
	"strings"
)

type fontToken struct {
	text  string
	start int
}

func fontValues(value string) (map[string]string, bool) {
	if len(value) > 4096 {
		return nil, false
	}
	var tokens []fontToken
	for i := 0; i < len(value); {
		if cssSpace(rune(value[i])) {
			i++
			continue
		}
		start := i
		if value[i] == '/' {
			i++
		} else {
			quote := byte(0)
			for i < len(value) {
				c := value[i]
				if c == '\\' {
					return nil, false
				}
				if quote != 0 {
					if c == quote {
						quote = 0
					}
					i++
					continue
				}
				if c == '\'' || c == '"' {
					quote = c
					i++
					continue
				}
				if c == '/' || cssSpace(rune(c)) {
					break
				}
				i++
			}
			if quote != 0 {
				return nil, false
			}
		}
		tokens = append(tokens, fontToken{value[start:i], start})
		if len(tokens) > 64 {
			return nil, false
		}
	}
	result := map[string]string{"font-style": "normal", "font-variant": "normal", "font-weight": "normal", "line-height": "normal"}
	styleSet, variantSet, weightSet := false, false, false
	i := 0
	for ; i < len(tokens); i++ {
		t := strings.ToLower(tokens[i].text)
		if _, ok := fontSizeKeyword(t, 16); ok {
			result["font-size"] = t
			i++
			break
		}
		if l, ok := parseLength(t, false, false); ok && (!cssNumber.MatchString(t) || l.value == 0) {
			result["font-size"] = t
			i++
			break
		}
		switch t {
		case "normal":
		case "italic", "oblique":
			if styleSet {
				return nil, false
			}
			styleSet = true
			result["font-style"] = t
		case "small-caps":
			if variantSet {
				return nil, false
			}
			variantSet = true
			result["font-variant"] = t
		case "bold", "bolder", "lighter":
			if weightSet {
				return nil, false
			}
			weightSet = true
			result["font-weight"] = t
		default:
			n, err := strconv.Atoi(t)
			if err != nil || n < 100 || n > 900 || n%100 != 0 || weightSet {
				return nil, false
			}
			weightSet = true
			result["font-weight"] = t
		}
	}
	if result["font-size"] == "" || i >= len(tokens) {
		return nil, false
	}
	if tokens[i].text == "/" {
		i++
		if i >= len(tokens) {
			return nil, false
		}
		line := strings.ToLower(tokens[i].text)
		if line != "normal" {
			if _, ok := parseLength(line, false, false); !ok {
				return nil, false
			}
		}
		result["line-height"] = line
		i++
	}
	if i >= len(tokens) {
		return nil, false
	}
	family := strings.TrimSpace(value[tokens[i].start:])
	for _, part := range strings.Split(family, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		if part[0] == '\'' || part[0] == '"' {
			if len(part) < 2 || part[len(part)-1] != part[0] {
				return nil, false
			}
			part = part[1 : len(part)-1]
		}
		if strings.TrimSpace(part) == "" {
			return nil, false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == ' ') {
				return nil, false
			}
		}
	}
	result["font-family"] = family
	return result, true
}
func wordSpacingValue(v string) (cssLength, bool) {
	if strings.EqualFold(v, "normal") {
		return px(0), true
	}
	l, ok := parseInset(v)
	return l, ok && l.absolute()
}

func fontSizeKeyword(s string, parentSize int) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "xx-small":
		return 9, true
	case "x-small":
		return 10, true
	case "small":
		return 13, true
	case "medium":
		return 16, true
	case "large":
		return 18, true
	case "x-large":
		return 24, true
	case "xx-large":
		return 32, true
	case "xxx-large":
		return 48, true
	case "smaller":
		return max(1, (parentSize*85+50)/100), true
	case "larger":
		return max(1, min(128, (parentSize*120+50)/100)), true
	}
	return 0, false
}

func smallCapSize(size int) int {
	if size <= 1 {
		return size
	}
	return max(1, (size*80+50)/100)
}

func applyTextTransform(s, mode string) string {
	switch mode {
	case "uppercase":
		return strings.ToUpper(s)
	case "lowercase":
		return strings.ToLower(s)
	case "capitalize":
		var out strings.Builder
		capNext := true
		for _, r := range s {
			if cssSpace(r) {
				capNext = true
				out.WriteRune(r)
			} else if capNext {
				out.WriteString(strings.ToUpper(string(r)))
				capNext = false
			} else {
				out.WriteRune(r)
			}
		}
		return out.String()
	}
	return s
}
