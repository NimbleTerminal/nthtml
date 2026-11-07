package document

import "strings"

// Evaluate the width subset of media range syntax. Keep token count bounded
// and reject malformed/unsupported expressions, including under `not`.
func evalMediaWidthRange(expr string, width int) (bool, bool) {
	var values, ops []string
	start := 0
	for i := 0; i < len(expr); i++ {
		if expr[i] != '<' && expr[i] != '>' && expr[i] != '=' {
			continue
		}
		values = append(values, strings.TrimSpace(expr[start:i]))
		op := expr[i : i+1]
		if expr[i] != '=' && i+1 < len(expr) && expr[i+1] == '=' {
			i++
			op += "="
		}
		ops = append(ops, op)
		if len(ops) > 2 {
			return false, false
		}
		start = i + 1
	}
	values = append(values, strings.TrimSpace(expr[start:]))
	if len(ops) == 0 {
		return false, false
	}
	nums := make([]float64, len(values))
	widthAt := -1
	for i, value := range values {
		if strings.EqualFold(value, "width") {
			if widthAt >= 0 {
				return false, false
			}
			widthAt = i
			nums[i] = float64(width)
			continue
		}
		l, ok := parseInset(value)
		if !ok || !l.absolute() {
			return false, false
		}
		nums[i] = l.computed(16, 16).value
	}
	if widthAt < 0 {
		return false, false
	}
	if len(ops) == 2 && (widthAt != 1 || ops[0][0] == '=' || ops[1][0] != ops[0][0]) {
		return false, false
	}
	result := true
	for i, op := range ops {
		a, b := nums[i], nums[i+1]
		switch op {
		case "<":
			result = result && a < b
		case "<=":
			result = result && a <= b
		case ">":
			result = result && a > b
		case ">=":
			result = result && a >= b
		case "=":
			result = result && a == b
		}
	}
	return result, true
}
