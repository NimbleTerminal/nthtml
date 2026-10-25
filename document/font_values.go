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
	result := map[string]string{"font-style": "normal", "font-weight": "normal", "line-height": "normal"}
	styleSet, weightSet := false, false
	i := 0
	for ; i < len(tokens); i++ {
		t := strings.ToLower(tokens[i].text)
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
		case "bold":
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
