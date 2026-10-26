package document

import (
	"strconv"
	"strings"
)

func importance(v string) (string, bool) {
	if i := strings.LastIndexByte(v, '!'); i >= 0 && strings.EqualFold(strings.TrimSpace(v[i+1:]), "important") {
		return strings.TrimSpace(v[:i]), true
	}
	return v, false
}

type attrSelector struct {
	name       string
	op         string
	val        string
	ignoreCase bool
}

type pseudoSelector struct {
	name string
	arg  string
	sub  *selectorPart
	a, b int
}

type selectorPart struct {
	terms   []string
	attrs   []attrSelector
	pseudos []pseudoSelector
	comb    byte // 0, ' ', '>', '+', '~'
	child   bool
}

func (p selectorPart) termCount() int {
	return max(1, len(p.terms)+len(p.attrs)+len(p.pseudos))
}

type selector []selectorPart

func parseSelector(s string) (selector, int, bool) {
	sel, score, _, ok := parseSelectorWithPseudo(s)
	return sel, score, ok
}

// Bound both selector length and matching work. Unsupported syntax rejects the
// entire selector; it must never accidentally become a broader match.
func parseSelectorWithPseudo(s string) (selector, int, string, bool) {
	s = strings.TrimFunc(s, cssSpace)
	if len(s) == 0 || len(s) > 1024 {
		return nil, 0, "", false
	}
	var out selector
	score, terms := 0, 0
	comb := byte(0)
	pseudoElem := ""
	for len(s) > 0 {
		if pseudoElem != "" {
			return nil, 0, "", false
		}
		part := selectorPart{comb: comb, child: comb == '>'}
		partScore, partTerms, pe, rest, ok := parseCompoundPart(s, &part, true)
		if !ok {
			return nil, 0, "", false
		}
		score += partScore
		terms += partTerms
		if terms > 64 {
			return nil, 0, "", false
		}
		pseudoElem = pe
		out = append(out, part)
		s = strings.TrimLeftFunc(rest, cssSpace)
		if len(s) == 0 {
			break
		}
		comb = ' '
		if s[0] == '>' || s[0] == '+' || s[0] == '~' {
			comb = s[0]
			s = strings.TrimLeftFunc(s[1:], cssSpace)
			if len(s) == 0 {
				return nil, 0, "", false
			}
		}
	}
	return out, score, pseudoElem, len(out) > 0
}

func parseCompoundPart(s string, part *selectorPart, allowPseudoElem bool) (int, int, string, string, bool) {
	score, terms := 0, 0
	pseudoElem := ""
	hasTag := false
	for len(s) > 0 && s[0] != '>' && s[0] != '+' && s[0] != '~' && !cssSpace(rune(s[0])) {
		if pseudoElem != "" {
			return 0, 0, "", "", false
		}
		if s[0] == '[' {
			attr, rest, ok := parseAttrSelector(s)
			if !ok {
				return 0, 0, "", "", false
			}
			part.attrs = append(part.attrs, attr)
			score += 1 << 10
			terms++
			s = rest
			continue
		}
		if s[0] == ':' {
			doubleColon := strings.HasPrefix(s, "::")
			if doubleColon {
				s = s[2:]
			} else {
				s = s[1:]
			}
			end := 0
			for end < len(s) && identByte(s[end]) {
				end++
			}
			if end == 0 || !identStart(s[:end]) {
				return 0, 0, "", "", false
			}
			name := strings.ToLower(s[:end])
			s = s[end:]
			if name == "before" || name == "after" || name == "first-line" || name == "first-letter" {
				if !allowPseudoElem || (len(s) > 0 && s[0] == '(') {
					return 0, 0, "", "", false
				}
				pseudoElem = name
				score++
				terms++
				continue
			}
			if doubleColon {
				return 0, 0, "", "", false
			}
			ps, psScore, rest, ok := parsePseudoClass(name, s)
			if !ok {
				return 0, 0, "", "", false
			}
			part.pseudos = append(part.pseudos, ps)
			score += psScore
			terms++
			s = rest
			continue
		}
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
		if end == 0 || (s[0] != '*' && !identStart(s[:end])) || (prefix == 0 && (hasTag || len(part.terms) > 0 || len(part.attrs) > 0 || len(part.pseudos) > 0)) {
			return 0, 0, "", "", false
		}
		term := s[:end]
		s = s[end:]
		if prefix != 0 {
			term = string(prefix) + term
		} else {
			hasTag = true
		}
		part.terms = append(part.terms, term)
		terms++
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
	if terms == 0 {
		return 0, 0, "", "", false
	}
	return score, terms, pseudoElem, s, true
}

func parseAttrSelector(s string) (attrSelector, string, bool) {
	// s starts with '['
	s = s[1:]
	s = strings.TrimLeftFunc(s, cssSpace)
	end := 0
	for end < len(s) && identByte(s[end]) {
		end++
	}
	if end == 0 || !identStart(s[:end]) {
		return attrSelector{}, "", false
	}
	name := strings.ToLower(s[:end])
	s = strings.TrimLeftFunc(s[end:], cssSpace)
	if len(s) == 0 {
		return attrSelector{}, "", false
	}
	if s[0] == ']' {
		return attrSelector{name: name}, s[1:], true
	}
	op := ""
	for _, candidate := range []string{"~=", "|=", "^=", "$=", "*=", "="} {
		if strings.HasPrefix(s, candidate) {
			op = candidate
			s = s[len(candidate):]
			break
		}
	}
	if op == "" {
		return attrSelector{}, "", false
	}
	s = strings.TrimLeftFunc(s, cssSpace)
	if len(s) == 0 {
		return attrSelector{}, "", false
	}
	var val string
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		s = s[1:]
		var b strings.Builder
		closed := false
		for i := 0; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
				continue
			}
			if s[i] == quote {
				val = b.String()
				s = s[i+1:]
				closed = true
				break
			}
			b.WriteByte(s[i])
		}
		if !closed {
			return attrSelector{}, "", false
		}
	} else {
		vEnd := 0
		for vEnd < len(s) && s[vEnd] != ']' && !cssSpace(rune(s[vEnd])) && s[vEnd] != '"' && s[vEnd] != '\'' {
			vEnd++
		}
		if vEnd == 0 {
			return attrSelector{}, "", false
		}
		val = s[:vEnd]
		s = s[vEnd:]
	}
	s = strings.TrimLeftFunc(s, cssSpace)
	ignoreCase := false
	if len(s) > 0 && (s[0] == 'i' || s[0] == 'I' || s[0] == 's' || s[0] == 'S') {
		ignoreCase = s[0] == 'i' || s[0] == 'I'
		s = strings.TrimLeftFunc(s[1:], cssSpace)
	}
	if len(s) == 0 || s[0] != ']' {
		return attrSelector{}, "", false
	}
	return attrSelector{name: name, op: op, val: val, ignoreCase: ignoreCase}, s[1:], true
}

func parsePseudoClass(name, s string) (pseudoSelector, int, string, bool) {
	switch name {
	case "root", "first-child", "last-child", "only-child", "empty", "first-of-type", "last-of-type", "only-of-type", "link", "visited", "hover", "active", "focus":
		if len(s) > 0 && s[0] == '(' {
			return pseudoSelector{}, 0, "", false
		}
		return pseudoSelector{name: name}, 1 << 10, s, true
	case "lang", "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type", "not":
		if len(s) == 0 || s[0] != '(' {
			return pseudoSelector{}, 0, "", false
		}
		depth := 0
		closeIdx := -1
		for i := 0; i < len(s); i++ {
			if s[i] == '(' {
				depth++
			} else if s[i] == ')' {
				depth--
				if depth == 0 {
					closeIdx = i
					break
				}
			}
		}
		if closeIdx < 0 {
			return pseudoSelector{}, 0, "", false
		}
		arg := strings.TrimFunc(s[1:closeIdx], cssSpace)
		rest := s[closeIdx+1:]
		if name == "lang" {
			if arg == "" || !identStart(arg) {
				return pseudoSelector{}, 0, "", false
			}
			return pseudoSelector{name: name, arg: strings.ToLower(arg)}, 1 << 10, rest, true
		}
		if name == "not" {
			var sub selectorPart
			subScore, _, pe, subRest, ok := parseCompoundPart(arg, &sub, false)
			if !ok || pe != "" || strings.TrimFunc(subRest, cssSpace) != "" || len(sub.pseudos) > 0 && sub.pseudos[0].name == "not" {
				return pseudoSelector{}, 0, "", false
			}
			return pseudoSelector{name: name, sub: &sub}, subScore, rest, true
		}
		a, b, ok := parseNth(arg)
		if !ok {
			return pseudoSelector{}, 0, "", false
		}
		return pseudoSelector{name: name, a: a, b: b}, 1 << 10, rest, true
	}
	return pseudoSelector{}, 0, "", false
}

func parseNth(s string) (int, int, bool) {
	s = strings.ToLower(strings.Join(strings.Fields(s), ""))
	switch s {
	case "odd":
		return 2, 1, true
	case "even":
		return 2, 0, true
	case "":
		return 0, 0, false
	}
	nIdx := strings.IndexByte(s, 'n')
	if nIdx < 0 {
		b, err := strconv.Atoi(s)
		return 0, b, err == nil
	}
	aStr := s[:nIdx]
	bStr := s[nIdx+1:]
	var a int
	switch aStr {
	case "", "+":
		a = 1
	case "-":
		a = -1
	default:
		var err error
		a, err = strconv.Atoi(aStr)
		if err != nil {
			return 0, 0, false
		}
	}
	b := 0
	if bStr != "" {
		if bStr[0] != '+' && bStr[0] != '-' {
			return 0, 0, false
		}
		var err error
		b, err = strconv.Atoi(bStr)
		if err != nil {
			return 0, 0, false
		}
	}
	return a, b, true
}

func matchNth(a, b, index int) bool {
	if a == 0 {
		return index == b
	}
	diff := index - b
	if (diff > 0 && a < 0) || (diff < 0 && a > 0) {
		return false
	}
	return diff%a == 0
}

func identByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}
func cssSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' }

func (a attrSelector) matches(n *node) bool {
	actual, exists := n.attrs[a.name]
	if !exists {
		return false
	}
	if a.op == "" {
		return true
	}
	av, tv := actual, a.val
	if a.ignoreCase {
		av = strings.ToLower(av)
		tv = strings.ToLower(tv)
	}
	switch a.op {
	case "=":
		return av == tv
	case "~=":
		if tv == "" || strings.IndexFunc(tv, cssSpace) >= 0 {
			return false
		}
		for _, word := range strings.Fields(av) {
			if word == tv {
				return true
			}
		}
		return false
	case "|=":
		return av == tv || strings.HasPrefix(av, tv+"-")
	case "^=":
		return tv != "" && strings.HasPrefix(av, tv)
	case "$=":
		return tv != "" && strings.HasSuffix(av, tv)
	case "*=":
		return tv != "" && strings.Contains(av, tv)
	}
	return false
}

func (ps pseudoSelector) matches(n *node) bool {
	switch ps.name {
	case "root":
		if n.tag == "html" {
			return true
		}
		return n.parent != nil && n.parent.tag == "document"
	case "first-child":
		return n.parent != nil && n.parent.tag != "document" && n.prevSib == nil
	case "last-child":
		return n.parent != nil && n.parent.tag != "document" && n.nextSib == nil
	case "only-child":
		return n.parent != nil && n.parent.tag != "document" && n.prevSib == nil && n.nextSib == nil
	case "empty":
		for _, c := range n.children {
			if c.pseudo != "" {
				continue
			}
			if c.tag != "" || c.text != "" {
				return false
			}
		}
		return true
	case "first-of-type":
		if n.parent == nil || n.parent.tag == "document" {
			return false
		}
		for p := n.prevSib; p != nil; p = p.prevSib {
			if strings.EqualFold(p.tag, n.tag) {
				return false
			}
		}
		return true
	case "last-of-type":
		if n.parent == nil || n.parent.tag == "document" {
			return false
		}
		for nx := n.nextSib; nx != nil; nx = nx.nextSib {
			if strings.EqualFold(nx.tag, n.tag) {
				return false
			}
		}
		return true
	case "only-of-type":
		return (pseudoSelector{name: "first-of-type"}).matches(n) && (pseudoSelector{name: "last-of-type"}).matches(n)
	case "nth-child":
		if n.parent == nil || n.parent.tag == "document" {
			return false
		}
		idx := 1
		for p := n.prevSib; p != nil; p = p.prevSib {
			idx++
		}
		return matchNth(ps.a, ps.b, idx)
	case "nth-last-child":
		if n.parent == nil || n.parent.tag == "document" {
			return false
		}
		idx := 1
		for nx := n.nextSib; nx != nil; nx = nx.nextSib {
			idx++
		}
		return matchNth(ps.a, ps.b, idx)
	case "nth-of-type":
		if n.parent == nil || n.parent.tag == "document" {
			return false
		}
		idx := 1
		for p := n.prevSib; p != nil; p = p.prevSib {
			if strings.EqualFold(p.tag, n.tag) {
				idx++
			}
		}
		return matchNth(ps.a, ps.b, idx)
	case "nth-last-of-type":
		if n.parent == nil || n.parent.tag == "document" {
			return false
		}
		idx := 1
		for nx := n.nextSib; nx != nil; nx = nx.nextSib {
			if strings.EqualFold(nx.tag, n.tag) {
				idx++
			}
		}
		return matchNth(ps.a, ps.b, idx)
	case "not":
		return ps.sub != nil && !ps.sub.matches(n)
	case "link":
		return n.tag == "a" && n.attrs["href"] != ""
	case "lang":
		for cur := n; cur != nil && cur.tag != "document"; cur = cur.parent {
			if l, ok := cur.attrs["lang"]; ok {
				l = strings.ToLower(strings.TrimSpace(l))
				return l == ps.arg || strings.HasPrefix(l, ps.arg+"-")
			}
		}
		return false
	case "visited", "hover", "active", "focus":
		return false
	}
	return false
}

func (p selectorPart) matches(n *node) bool {
	if n == nil || n.tag == "" || n.tag == "document" || n.pseudo != "" {
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
	for _, attr := range p.attrs {
		if !attr.matches(n) {
			return false
		}
	}
	for _, ps := range p.pseudos {
		if !ps.matches(n) {
			return false
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
	hasSibling := false
	for i := 1; i < len(s); i++ {
		if s[i].comb == '+' || s[i].comb == '~' {
			hasSibling = true
			break
		}
	}
	if !hasSibling {
		// Dynamic programming across the ancestor chain avoids backtracking on
		// repeated descendant selectors (at most 64 ancestors × 64 compounds).
		var ancestors [65]*node
		count := 0
		for cur := n; cur != nil && count < len(ancestors); cur = cur.parent {
			ancestors[count] = cur
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
				if part > 0 && (s[part].comb == '>' || s[part].child) {
					eligible = i+1 < count && prev[i+1]
				}
				next[i] = eligible && s[part].matches(ancestors[i])
			}
			prev = next
		}
		return prev[0]
	}
	type state struct{ part, id int }
	failed := map[state]bool{}
	steps := 0
	var matchAt func(int, *node) bool
	matchAt = func(idx int, cur *node) bool {
		if idx == 0 {
			return true
		}
		st := state{idx, cur.id}
		if failed[st] {
			return false
		}
		steps++
		if steps > 1024 {
			return false
		}
		comb := s[idx].comb
		prevPart := s[idx-1]
		switch comb {
		case '>':
			p := cur.parent
			if p != nil && p.tag != "document" && prevPart.matches(p) && matchAt(idx-1, p) {
				return true
			}
		case ' ', 0:
			for p := cur.parent; p != nil && p.tag != "document"; p = p.parent {
				if prevPart.matches(p) && matchAt(idx-1, p) {
					return true
				}
			}
		case '+':
			p := cur.prevSib
			if p != nil && prevPart.matches(p) && matchAt(idx-1, p) {
				return true
			}
		case '~':
			for p := cur.prevSib; p != nil; p = p.prevSib {
				if prevPart.matches(p) && matchAt(idx-1, p) {
					return true
				}
			}
		}
		failed[st] = true
		return false
	}
	return matchAt(len(s)-1, n)
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
