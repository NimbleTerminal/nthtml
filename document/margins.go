package document

import "strings"

// Nonnegative adjoining margins form a strut whose size is their maximum.
// Negative margins, floats, positioning, and clearance remain outside our CSS
// subset. Each profile traversal shares a cumulative layout work budget.
type blockMargins struct {
	before, after        int
	top, bottom, through bool
	block                bool
}

func boxWidth(s style, w int) (margin, padding [4]int, target int) {
	margin, padding = resolveEdges(s.margin, w), resolveEdges(s.padding, w)
	extra := padding[1] + padding[3] + 2*s.bw
	available := max(1, w-margin[1]-margin[3])
	target = available
	outer := func(v cssLength) int {
		size := v.resolve(w, 16384)
		if !s.borderBox {
			size += extra
		}
		return size
	}
	if s.width.definite(w) {
		target = outer(s.width)
	}
	if s.maxWidth.definite(w) {
		target = min(target, outer(s.maxWidth))
	}
	if s.minWidth.definite(w) {
		target = max(target, outer(s.minWidth))
	}
	target = max(0, min(available, target))
	spare := max(0, available-target)
	if s.margin[3].unit == lengthAuto {
		margin[3] = spare
		if s.margin[1].unit == lengthAuto {
			margin[3] /= 2
		}
	}
	return
}

// These elements either establish a layout boundary or have specialized
// contents (table grids, replaced graphics, or list markers).
func marginBoundary(n *node) bool {
	switch n.tag {
	case "document", "html", "table", "td", "th", "tr", "thead", "tbody", "tfoot", "img", "ul", "ol", "li":
		return true
	}
	kind, _ := diagramSource(n)
	return kind != ""
}

func (l *layout) margins(n *node, s style, w, parentHeight int) blockMargins {
	if l.err != nil {
		return blockMargins{}
	}
	l.marginWork++
	if l.marginWork > 1000000 {
		l.err = limitError("margin collapse work")
		return blockMargins{}
	}
	if err := l.ctx.Err(); err != nil {
		l.err = err
		return blockMargins{}
	}
	margin, padding, width := boxWidth(s, w)
	info := blockMargins{before: margin[0], after: margin[2], block: true}
	if s.display == "none" {
		return blockMargins{through: true}
	}
	if marginBoundary(n) || s.display == "inline-block" || s.position == "absolute" || s.floatSide == "left" || s.floatSide == "right" || s.clear != "" && s.clear != "none" {
		return info
	}
	cw := max(1, width-padding[1]-padding[3]-2*s.bw)
	height := -1
	if s.height.definite(parentHeight) {
		height = s.height.resolve(parentHeight, l.d.limits.Height)
		if s.borderBox {
			height = max(0, height-padding[0]-padding[2]-2*s.bw)
		}
	}
	specifiedHeight := height
	if height >= 0 {
		content := func(value cssLength) int {
			h := value.resolve(parentHeight, l.d.limits.Height)
			if s.borderBox {
				h = max(0, h-padding[0]-padding[2]-2*s.bw)
			}
			return h
		}
		if s.maxHeight.definite(parentHeight) {
			height = min(height, content(s.maxHeight))
		}
		if s.minHeight.definite(parentHeight) {
			height = max(height, content(s.minHeight))
		}
	}
	minHeight := 0
	if s.minHeight.definite(parentHeight) {
		minHeight = s.minHeight.resolve(parentHeight, l.d.limits.Height)
	}
	canTop := s.bw == 0 && padding[0] == 0
	canBottom := s.bw == 0 && padding[2] == 0 && specifiedHeight < 0 && minHeight == 0
	allThrough := s.bw == 0 && padding[0] == 0 && padding[2] == 0 && specifiedHeight <= 0 && minHeight == 0
	// Only profiles for children at the two edges are needed, but collect once
	// to handle a chain of empty blocks adjoining either edge.
	var children []blockMargins
	var inline []*node
	flush := func() {
		if l.inlineLine(inline, s) {
			children = append(children, blockMargins{})
			allThrough = false
		}
		inline = nil
	}
	for _, child := range n.children {
		cs := l.d.styled(child, s)
		if cs.display == "none" || cs.position == "absolute" || cs.floatSide == "left" || cs.floatSide == "right" {
			continue
		}
		if child.tag == "" || cs.display == "inline" || cs.display == "inline-block" || child.tag == "br" {
			inline = append(inline, child)
			continue
		}
		flush()
		childInfo := l.margins(child, cs, cw, height)
		if cs.clear != "" && cs.clear != "none" {
			childInfo.block = false
		}
		children = append(children, childInfo)
		allThrough = allThrough && childInfo.through
		if l.err != nil {
			return info
		}
	}
	flush()
	// An inline line has no collapsible edges. Block profiles have explicit
	// eligibility recorded below so zero margins still form adjoining edges.
	if canTop {
		for _, c := range children {
			if !c.block {
				break
			}
			info.before = max(info.before, c.before)
			info.top = true
			if !c.through {
				break
			}
		}
	}
	if canBottom {
		for i := len(children) - 1; i >= 0; i-- {
			c := children[i]
			if !c.block {
				break
			}
			info.after = max(info.after, c.after)
			info.bottom = true
			if !c.through {
				break
			}
		}
	}
	info.through = allThrough
	if allThrough {
		for _, c := range children {
			info.before = max(info.before, c.before, c.after)
		}
		info.before = max(info.before, info.after)
		info.after = info.before
		info.top = true
		info.bottom = true
	}
	// Every block exposes its own outer margins, including a zero margin.
	return info
}

// inlineLine distinguishes collapsible HTML whitespace from a real line box.
// Inline element boundaries alone do not create a line.
func (l *layout) inlineLine(nodes []*node, parent style) bool {
	for _, n := range nodes {
		l.marginWork++
		if l.marginWork > 1000000 {
			l.err = limitError("margin collapse work")
			return true
		}
		if n.tag == "" {
			if (parent.pre && n.text != "") || (parent.whiteSpace == "pre-line" && strings.ContainsAny(n.text, "\n\r")) {
				return true
			}
			for _, r := range n.text {
				if !cssSpace(r) {
					return true
				}
			}
			continue
		}
		s := l.d.styled(n, parent)
		if s.display == "none" || s.position == "absolute" || s.floatSide == "left" || s.floatSide == "right" {
			continue
		}
		if n.tag == "br" || n.tag == "img" || s.display == "inline-block" {
			return true
		}
		if kind, _ := diagramSource(n); kind != "" {
			return true
		}
		if l.inlineLine(n.children, s) {
			return true
		}
	}
	return false
}
