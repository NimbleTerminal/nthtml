package document

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type lengthUnit uint8

const (
	lengthUnset lengthUnit = iota
	lengthPX
	lengthPercent
	lengthAuto
	lengthNone
	lengthEM
	lengthREM
	lengthVW
	lengthVH
	lengthVMin
	lengthVMax
)

type cssMathKind uint8

const (
	mathNumber cssMathKind = iota
	mathDimension
	mathAdd
	mathSub
	mathMul
	mathDiv
	mathMin
	mathMax
	mathClamp
)

type cssMathNode struct {
	kind cssMathKind
	val  float64
	unit lengthUnit
	args []*cssMathNode
}

func (n *cssMathNode) hasPercent() bool {
	if n == nil {
		return false
	}
	if n.kind == mathDimension && n.unit == lengthPercent {
		return true
	}
	for _, a := range n.args {
		if a.hasPercent() {
			return true
		}
	}
	return false
}

func (n *cssMathNode) computed(fontSize, rootSize, vw, vh int) *cssMathNode {
	if n == nil {
		return nil
	}
	if n.kind == mathDimension {
		cp := *n
		switch cp.unit {
		case lengthEM:
			cp.val *= float64(fontSize)
			cp.unit = lengthPX
		case lengthREM:
			cp.val *= float64(rootSize)
			cp.unit = lengthPX
		case lengthVW:
			cp.val *= float64(vw) / 100
			cp.unit = lengthPX
		case lengthVH:
			cp.val *= float64(vh) / 100
			cp.unit = lengthPX
		case lengthVMin:
			cp.val *= float64(min(vw, vh)) / 100
			cp.unit = lengthPX
		case lengthVMax:
			cp.val *= float64(max(vw, vh)) / 100
			cp.unit = lengthPX
		}
		return &cp
	}
	if len(n.args) == 0 {
		return n
	}
	cp := *n
	cp.args = make([]*cssMathNode, len(n.args))
	for i, a := range n.args {
		cp.args[i] = a.computed(fontSize, rootSize, vw, vh)
	}
	return &cp
}

func (n *cssMathNode) eval(base float64) float64 {
	if n == nil {
		return 0
	}
	switch n.kind {
	case mathNumber:
		return n.val
	case mathDimension:
		if n.unit == lengthPercent {
			return n.val * base / 100
		}
		return n.val
	case mathAdd:
		return n.args[0].eval(base) + n.args[1].eval(base)
	case mathSub:
		return n.args[0].eval(base) - n.args[1].eval(base)
	case mathMul:
		return n.args[0].eval(base) * n.args[1].eval(base)
	case mathDiv:
		d := n.args[1].eval(base)
		if d == 0 {
			return 0
		}
		return n.args[0].eval(base) / d
	case mathMin:
		res := n.args[0].eval(base)
		for _, a := range n.args[1:] {
			res = min(res, a.eval(base))
		}
		return res
	case mathMax:
		res := n.args[0].eval(base)
		for _, a := range n.args[1:] {
			res = max(res, a.eval(base))
		}
		return res
	case mathClamp:
		lo, mid, hi := n.args[0].eval(base), n.args[1].eval(base), n.args[2].eval(base)
		return max(lo, min(mid, hi))
	}
	return 0
}

type cssLength struct {
	value float64
	unit  lengthUnit
	expr  *cssMathNode
}

func px(n int) cssLength         { return cssLength{value: float64(n), unit: lengthPX} }
func uniform(n int) [4]cssLength { return [4]cssLength{px(n), px(n), px(n), px(n)} }

func htmlLength(s string) (cssLength, bool) {
	l, ok := parseLength(s, false, false)
	if !ok || l.expr != nil || (l.unit != lengthPX && l.unit != lengthPercent) {
		return cssLength{}, false
	}
	return l, true
}

func isMathFunc(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, fn := range []string{"calc(", "min(", "max(", "clamp("} {
		if strings.HasPrefix(s, fn) {
			return true
		}
	}
	return false
}

func parseLength(s string, auto, none bool) (cssLength, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if auto && s == "auto" {
		return cssLength{unit: lengthAuto}, true
	}
	if none && s == "none" {
		return cssLength{unit: lengthNone}, true
	}
	if isMathFunc(s) {
		return parseMathLength(s)
	}
	unit := lengthPX
	factor := 1.0
	for _, u := range []struct {
		suffix string
		unit   lengthUnit
		factor float64
	}{
		{"vmin", lengthVMin, 1}, {"vmax", lengthVMax, 1}, {"vw", lengthVW, 1}, {"vh", lengthVH, 1},
		{"rem", lengthREM, 1}, {"em", lengthEM, 1}, {"px", lengthPX, 1}, {"pt", lengthPX, 96.0 / 72}, {"pc", lengthPX, 16}, {"in", lengthPX, 96}, {"cm", lengthPX, 96.0 / 2.54}, {"mm", lengthPX, 96.0 / 25.4}, {"q", lengthPX, 96.0 / 101.6}, {"%", lengthPercent, 1},
	} {
		if strings.HasSuffix(s, u.suffix) {
			s = strings.TrimSuffix(s, u.suffix)
			unit, factor = u.unit, u.factor
			break
		}
	}
	if len(s) > 64 || !cssNumber.MatchString(s) {
		return cssLength{}, false
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 16384 {
		return cssLength{}, false
	}
	return cssLength{value: v * factor, unit: unit}, true
}

const (
	calcTypeInvalid uint8 = iota
	calcTypeNumber
	calcTypeZero
	calcTypeDim
)

func isCalcDim(t uint8) bool { return t == calcTypeDim || t == calcTypeZero }
func isCalcNum(t uint8) bool { return t == calcTypeNumber || t == calcTypeZero }

func parseMathLength(s string) (cssLength, bool) {
	if len(s) > 256 {
		return cssLength{}, false
	}
	openIdx := strings.IndexByte(s, '(')
	if openIdx < 0 {
		return cssLength{}, false
	}
	closeRel := findMatchingParen(s[openIdx:])
	if closeRel < 0 || openIdx+closeRel != len(s)-1 {
		return cssLength{}, false
	}
	nodes := 0
	node, kind, ok := parseMathFuncCall(s[:openIdx], s[openIdx+1:len(s)-1], 0, &nodes)
	if !ok || !isCalcDim(kind) {
		return cssLength{}, false
	}
	u := lengthPX
	if node.hasPercent() {
		u = lengthPercent
	}
	return cssLength{unit: u, expr: node}, true
}

func parseMathFuncCall(fn, inner string, depth int, nodes *int) (*cssMathNode, uint8, bool) {
	if depth > 8 {
		return nil, calcTypeInvalid, false
	}
	*nodes++
	if *nodes > 32 {
		return nil, calcTypeInvalid, false
	}
	fn = strings.ToLower(strings.TrimSpace(fn))
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return nil, calcTypeInvalid, false
	}
	switch fn {
	case "calc":
		return parseCalcSum(inner, depth+1, nodes)
	case "min", "max":
		parts, ok := splitSelectorList(inner)
		if !ok || len(parts) == 0 || len(parts) > 16 {
			return nil, calcTypeInvalid, false
		}
		var args []*cssMathNode
		for _, p := range parts {
			argNode, kind, ok := parseCalcSum(p, depth+1, nodes)
			if !ok || !isCalcDim(kind) {
				return nil, calcTypeInvalid, false
			}
			args = append(args, argNode)
		}
		k := mathMin
		if fn == "max" {
			k = mathMax
		}
		return &cssMathNode{kind: k, args: args}, calcTypeDim, true
	case "clamp":
		parts, ok := splitSelectorList(inner)
		if !ok || len(parts) != 3 {
			return nil, calcTypeInvalid, false
		}
		var args []*cssMathNode
		for _, p := range parts {
			argNode, kind, ok := parseCalcSum(p, depth+1, nodes)
			if !ok || !isCalcDim(kind) {
				return nil, calcTypeInvalid, false
			}
			args = append(args, argNode)
		}
		return &cssMathNode{kind: mathClamp, args: args}, calcTypeDim, true
	}
	return nil, calcTypeInvalid, false
}

type calcParser struct {
	s     string
	pos   int
	depth int
	nodes *int
}

func parseCalcSum(s string, depth int, nodes *int) (*cssMathNode, uint8, bool) {
	if depth > 8 {
		return nil, calcTypeInvalid, false
	}
	p := &calcParser{s: strings.TrimSpace(s), depth: depth, nodes: nodes}
	left, lKind, ok := p.parseProduct()
	if !ok {
		return nil, calcTypeInvalid, false
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			break
		}
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			break
		}
		// In CSS calc(), binary + and - must be preceded by whitespace (or ')') and followed by whitespace (or '(').
		if p.pos > 0 && !cssSpace(rune(p.s[p.pos-1])) && p.s[p.pos-1] != ')' {
			return nil, calcTypeInvalid, false
		}
		p.pos++
		if p.pos >= len(p.s) || (!cssSpace(rune(p.s[p.pos])) && p.s[p.pos] != '(') {
			return nil, calcTypeInvalid, false
		}
		right, rKind, ok := p.parseProduct()
		if !ok {
			return nil, calcTypeInvalid, false
		}
		var resKind uint8
		if isCalcDim(lKind) && isCalcDim(rKind) {
			resKind = calcTypeDim
		} else if isCalcNum(lKind) && isCalcNum(rKind) {
			resKind = calcTypeNumber
		} else {
			return nil, calcTypeInvalid, false
		}
		*p.nodes++
		if *p.nodes > 32 {
			return nil, calcTypeInvalid, false
		}
		k := mathAdd
		if op == '-' {
			k = mathSub
		}
		left = &cssMathNode{kind: k, args: []*cssMathNode{left, right}}
		lKind = resKind
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return nil, calcTypeInvalid, false
	}
	return left, lKind, true
}

func (p *calcParser) skipSpace() {
	for p.pos < len(p.s) && cssSpace(rune(p.s[p.pos])) {
		p.pos++
	}
}

func (p *calcParser) parseProduct() (*cssMathNode, uint8, bool) {
	left, lKind, ok := p.parsePrimary()
	if !ok {
		return nil, calcTypeInvalid, false
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			break
		}
		op := p.s[p.pos]
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		right, rKind, ok := p.parsePrimary()
		if !ok {
			return nil, calcTypeInvalid, false
		}
		var resKind uint8
		if op == '*' {
			if isCalcNum(lKind) && isCalcNum(rKind) {
				resKind = calcTypeNumber
			} else if (isCalcDim(lKind) && isCalcNum(rKind)) || (isCalcNum(lKind) && isCalcDim(rKind)) {
				resKind = calcTypeDim
			} else {
				return nil, calcTypeInvalid, false
			}
		} else {
			if !isCalcNum(rKind) || rKind == calcTypeZero || (right.kind == mathNumber && right.val == 0) {
				return nil, calcTypeInvalid, false
			}
			if isCalcDim(lKind) {
				resKind = calcTypeDim
			} else {
				resKind = calcTypeNumber
			}
		}
		*p.nodes++
		if *p.nodes > 32 {
			return nil, calcTypeInvalid, false
		}
		k := mathMul
		if op == '/' {
			k = mathDiv
		}
		left = &cssMathNode{kind: k, args: []*cssMathNode{left, right}}
		lKind = resKind
	}
	return left, lKind, true
}

func (p *calcParser) parsePrimary() (*cssMathNode, uint8, bool) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return nil, calcTypeInvalid, false
	}
	if p.s[p.pos] == '(' {
		closeRel := findMatchingParen(p.s[p.pos:])
		if closeRel < 0 {
			return nil, calcTypeInvalid, false
		}
		inner := p.s[p.pos+1 : p.pos+closeRel]
		p.pos += closeRel + 1
		return parseCalcSum(inner, p.depth+1, p.nodes)
	}
	start := p.pos
	for p.pos < len(p.s) && !cssSpace(rune(p.s[p.pos])) && p.s[p.pos] != '(' && p.s[p.pos] != ')' && p.s[p.pos] != '*' && p.s[p.pos] != '/' {
		if (p.s[p.pos] == '+' || p.s[p.pos] == '-') && p.pos > start && p.s[p.pos-1] != 'e' && p.s[p.pos-1] != 'E' {
			break
		}
		p.pos++
	}
	tok := p.s[start:p.pos]
	if p.pos < len(p.s) && p.s[p.pos] == '(' {
		closeRel := findMatchingParen(p.s[p.pos:])
		if closeRel < 0 {
			return nil, calcTypeInvalid, false
		}
		inner := p.s[p.pos+1 : p.pos+closeRel]
		p.pos += closeRel + 1
		sign := 1.0
		if strings.HasPrefix(tok, "-") {
			sign = -1.0
			tok = tok[1:]
		} else if strings.HasPrefix(tok, "+") {
			tok = tok[1:]
		}
		node, kind, ok := parseMathFuncCall(tok, inner, p.depth+1, p.nodes)
		if !ok {
			return nil, calcTypeInvalid, false
		}
		if sign < 0 {
			node = &cssMathNode{kind: mathMul, args: []*cssMathNode{{kind: mathNumber, val: -1}, node}}
		}
		return node, kind, true
	}
	if tok == "" {
		return nil, calcTypeInvalid, false
	}
	*p.nodes++
	if *p.nodes > 32 {
		return nil, calcTypeInvalid, false
	}
	return parseCalcAtom(tok)
}

func parseCalcAtom(tok string) (*cssMathNode, uint8, bool) {
	s := strings.ToLower(strings.TrimSpace(tok))
	if cssNumber.MatchString(s) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 16384 {
			return nil, calcTypeInvalid, false
		}
		if v == 0 {
			return &cssMathNode{kind: mathDimension, val: 0, unit: lengthPX}, calcTypeZero, true
		}
		return &cssMathNode{kind: mathNumber, val: v}, calcTypeNumber, true
	}
	unit := lengthUnset
	factor := 1.0
	for _, u := range []struct {
		suffix string
		unit   lengthUnit
		factor float64
	}{
		{"vmin", lengthVMin, 1}, {"vmax", lengthVMax, 1}, {"vw", lengthVW, 1}, {"vh", lengthVH, 1},
		{"rem", lengthREM, 1}, {"em", lengthEM, 1}, {"px", lengthPX, 1}, {"pt", lengthPX, 96.0 / 72}, {"pc", lengthPX, 16}, {"in", lengthPX, 96}, {"cm", lengthPX, 96.0 / 2.54}, {"mm", lengthPX, 96.0 / 25.4}, {"q", lengthPX, 96.0 / 101.6}, {"%", lengthPercent, 1},
	} {
		if strings.HasSuffix(s, u.suffix) {
			s = strings.TrimSuffix(s, u.suffix)
			unit, factor = u.unit, u.factor
			break
		}
	}
	if unit == lengthUnset || len(s) > 64 || !cssNumber.MatchString(s) {
		return nil, calcTypeInvalid, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 16384 {
		return nil, calcTypeInvalid, false
	}
	return &cssMathNode{kind: mathDimension, val: v * factor, unit: unit}, calcTypeDim, true
}

func (l cssLength) resolve(base, ceiling int) int {
	v := l.value
	if l.expr != nil {
		if l.unit == lengthPercent && base < 0 {
			return 0
		}
		v = l.expr.eval(float64(base))
	} else if l.unit == lengthPercent {
		if base < 0 {
			return 0
		}
		v = v * float64(base) / 100
	} else if l.unit != lengthPX {
		return 0
	}
	return min(ceiling, max(0, int(v+1e-9)))
}
func (l cssLength) definite(base int) bool {
	return l.unit == lengthPX || l.unit == lengthPercent && base >= 0
}

func (l cssLength) resolveSigned(base, ceiling int) int {
	v := l.value
	if l.expr != nil {
		if l.unit == lengthPercent && base < 0 {
			return 0
		}
		v = l.expr.eval(float64(base))
	} else if l.unit == lengthPercent {
		if base < 0 {
			return 0
		}
		v = v * float64(base) / 100
	} else if l.unit != lengthPX {
		return 0
	}
	if v < 0 {
		return max(-ceiling, int(v-1e-9))
	}
	return min(ceiling, int(v+1e-9))
}

// Percent margins and padding (including vertical sides) resolve against the
// containing block's content width. Each side retains the existing ±256px cap.
func resolveEdges(edges [4]cssLength, width int) [4]int {
	var out [4]int
	for i, l := range edges {
		out[i] = l.resolveSigned(width, 256)
	}
	return out
}

var cssNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// Convert font-relative and viewport-relative units at computed-style time,
// leaving percentages for layout. Inherited absolute line heights must not
// scale with a child's font.
func (l cssLength) computed(fontSize, rootSize int) cssLength {
	return l.computedViewport(fontSize, rootSize, 800, 600)
}

func (l cssLength) computedViewport(fontSize, rootSize, vw, vh int) cssLength {
	if vw <= 0 {
		vw = 800
	}
	if vh <= 0 {
		vh = 600
	}
	if l.expr != nil {
		ce := l.expr.computed(fontSize, rootSize, vw, vh)
		if !ce.hasPercent() {
			return cssLength{value: ce.eval(0), unit: lengthPX}
		}
		return cssLength{unit: lengthPercent, expr: ce}
	}
	switch l.unit {
	case lengthEM:
		l.value *= float64(fontSize)
		l.unit = lengthPX
	case lengthREM:
		l.value *= float64(rootSize)
		l.unit = lengthPX
	case lengthVW:
		l.value *= float64(vw) / 100
		l.unit = lengthPX
	case lengthVH:
		l.value *= float64(vh) / 100
		l.unit = lengthPX
	case lengthVMin:
		l.value *= float64(min(vw, vh)) / 100
		l.unit = lengthPX
	case lengthVMax:
		l.value *= float64(max(vw, vh)) / 100
		l.unit = lengthPX
	}
	return l
}

func (l cssLength) absolute() bool {
	if l.expr != nil {
		return !l.expr.hasPercent()
	}
	return l.unit == lengthPX || l.unit == lengthEM || l.unit == lengthREM || l.unit == lengthVW || l.unit == lengthVH || l.unit == lengthVMin || l.unit == lengthVMax
}
