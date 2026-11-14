package document

import "strings"

// CSSRuleCount returns the number of rules charged by the stylesheet parser,
// including expanded nested selectors, layers and supported @supports branches.
// It applies MaxLimits.Bytes and MaxLimits.Rules and the parser's normal work
// bounds. Hosts should reject a sheet on error rather than use a partial count.
// This only parses CSS; it never fetches referenced resources.
func CSSRuleCount(css string) (int, error) {
	if len(css) > MaxLimits.Bytes {
		return 0, limitError("stylesheet bytes")
	}
	rules, err := parseCSS(strings.TrimPrefix(css, "\uFEFF"), MaxLimits.Rules)
	if err != nil {
		return 0, err
	}
	return len(rules), nil
}
