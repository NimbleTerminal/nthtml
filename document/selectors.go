package document

import "strings"

func importance(v string) (string, bool) {
	if i := strings.LastIndexByte(v, '!'); i >= 0 && strings.EqualFold(strings.TrimSpace(v[i+1:]), "important") {
		return strings.TrimSpace(v[:i]), true
	}
	return v, false
}

type selectorPart struct {
	terms []string
	child bool
}
type selector []selectorPart

// Bound both selector length and matching work. Unsupported syntax rejects the
// entire selector; it must never accidentally become a broader match.
func parseSelector(s string) (selector, int, bool) {
	if len(s) > 1024 {
		return nil, 0, false
	}
	var out selector
	score, terms := 0, 0
	child := false
	for len(s) > 0 {
		part := selectorPart{child: child}
		for len(s) > 0 && s[0] != '>' && !cssSpace(rune(s[0])) {
			prefix := byte(0)
			if s[0] == '.' || s[0] == '#' {
				prefix = s[0]
				s = s[1:]
			}
			end := 0
			if len(s) > 0 && s[0] == '*' && prefix == 0 {
				end = 1
			} else {
				for end < len(s) && identByte(s[end]) {
					end++
				}
			}
			if end == 0 || (s[0] != '*' && !identStart(s[:end])) || (prefix == 0 && len(part.terms) > 0) {
				return nil, 0, false
			}
			term := s[:end]
			s = s[end:]
			if prefix != 0 {
				term = string(prefix) + term
			}
			part.terms = append(part.terms, term)
			terms++
			if terms > 64 {
				return nil, 0, false
			}
			switch prefix {
			case '#':
				score += 1 << 20
			case '.':
				score += 1 << 10
			default:
				if term != "*" {
					score++
				}
			}
		}
		if len(part.terms) == 0 {
			return nil, 0, false
		}
		out = append(out, part)
		s = strings.TrimLeftFunc(s, cssSpace)
		child = false
		if strings.HasPrefix(s, ">") {
			child = true
			s = strings.TrimLeftFunc(s[1:], cssSpace)
			if s == "" {
				return nil, 0, false
			}
		}
	}
	return out, score, len(out) > 0
}
func identByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}
func cssSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' }
func (p selectorPart) matches(n *node) bool {
	if n == nil || n.tag == "" || n.tag == "document" {
		return false
	}
	for _, term := range p.terms {
		switch term[0] {
		case '*':
		case '#':
			if n.attrs["id"] != term[1:] {
				return false
			}
		case '.':
			found := false
			for _, c := range strings.Fields(n.attrs["class"]) {
				if c == term[1:] {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		default:
			if !strings.EqualFold(n.tag, term) {
				return false
			}
		}
	}
	return true
}
func (s selector) matches(n *node) bool {
	if len(s) == 0 || !s[len(s)-1].matches(n) {
		return false
	}
	if len(s) == 1 {
		return true
	}
	// Dynamic programming across the ancestor chain avoids backtracking on
	// repeated descendant selectors (at most 64 ancestors × 64 compounds).
	var ancestors [65]*node
	count := 0
	for ; n != nil && count < len(ancestors); n = n.parent {
		ancestors[count] = n
		count++
	}
	var prev [65]bool
	for part := 0; part < len(s); part++ {
		var next [65]bool
		ancestorMatch := false
		for i := count - 1; i >= 0; i-- {
			if i+1 < count {
				ancestorMatch = ancestorMatch || prev[i+1]
			}
			eligible := part == 0 || ancestorMatch
			if part > 0 && s[part].child {
				eligible = i+1 < count && prev[i+1]
			}
			next[i] = eligible && s[part].matches(ancestors[i])
		}
		prev = next
	}
	return len(s) > 0 && prev[0]
}

func identStart(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] == '-' {
		s = s[1:]
		if len(s) > 0 && s[0] == '-' {
			return true
		}
	}
	return len(s) > 0 && (s[0] == '_' || s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z')
}
