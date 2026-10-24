package document

import (
	"unicode"
)

// Intrinsic widths track unbroken words across inline nodes and the longest
// explicit line, rather than summing unrelated paragraphs. Both are capped
// at the renderer's maximum width; traversal shares the table work budget.
func (l *layout) preferredCell(n *node, s style, basis int) (int, int) {
	line, word, minimum, preferred := 0, 0, 0, 0
	pending := false
	flushWord := func() { minimum = max(minimum, word); word = 0 }
	flushLine := func() { flushWord(); preferred = max(preferred, line); line = 0; pending = false }
	var visit func(*node, style)
	visit = func(n *node, st style) {
		if !l.tableStep() {
			return
		}
		if n.tag != "" {
			st = l.d.styled(n, st)
			if st.display == "none" {
				return
			}
			if n.tag == "br" {
				flushLine()
				return
			}
			if st.display != "inline" {
				flushLine()
				a, b := l.preferredCell(n, st, basis)
				minimum = max(minimum, a)
				preferred = max(preferred, b)
				return
			}
		}
		if n.tag == "" {
			f := l.face(st)
			text := n.text
			if st.pre {
				text, _ = preTextAt(text, 0)
			}
			for _, r := range text {
				if !l.tableStep() {
					return
				}
				if st.pre && r == '\n' {
					flushLine()
					continue
				}
				if !st.pre && unicode.IsSpace(r) {
					flushWord()
					pending = line > 0
					continue
				}
				if pending {
					advance, _ := f.GlyphAdvance(' ')
					line = min(4096, line+max(1, advance.Ceil()))
					pending = false
				}
				advance, _ := f.GlyphAdvance(r)
				w := max(1, advance.Ceil())
				line = min(4096, line+w)
				if unicode.IsSpace(r) {
					flushWord()
				} else {
					word = min(4096, word+w)
				}
			}
		}
		for _, c := range n.children {
			visit(c, st)
		}
	}
	if kind, _ := diagramSource(n); kind != "" {
		minimum, preferred = 32, 300
	} else if n.tag == "img" {
		if src := l.opts.Images[n.attrs["src"]]; src != nil {
			preferred = min(4096, src.Bounds().Dx())
			minimum = preferred
		}
	} else if n.tag == "table" {
		// Nested tables contribute the sum of their shared column requirements.
		var mins, prefs []int
		var rows func(*node, style)
		rows = func(n *node, st style) {
			for _, c := range n.children {
				if !l.tableStep() {
					return
				}
				cs := l.d.styled(c, st)
				if cs.display == "none" {
					continue
				}
				switch c.tag {
				case "thead", "tbody", "tfoot":
					rows(c, cs)
				case "tr":
					i := 0
					for _, cell := range c.children {
						if cell.tag != "td" && cell.tag != "th" {
							continue
						}
						ss := l.d.styled(cell, cs)
						if ss.display == "none" {
							continue
						}
						a, b := l.preferredCell(cell, ss, basis)
						if i == len(mins) {
							mins = append(mins, 0)
							prefs = append(prefs, 0)
						}
						mins[i] = max(mins[i], a)
						prefs[i] = max(prefs[i], b)
						i++
					}
				case "caption":
					a, b := l.preferredCell(c, cs, basis)
					minimum = max(minimum, a)
					preferred = max(preferred, b)
				}
			}
		}
		rows(n, s)
		a, b := 0, 0
		for i := range mins {
			a = min(4096, a+mins[i])
			b = min(4096, b+prefs[i])
		}
		minimum = max(minimum, a)
		preferred = max(preferred, b)
	} else {
		for _, c := range n.children {
			visit(c, s)
		}
		flushLine()
	}
	p := resolveEdges(s.padding, basis)
	extra := p[1] + p[3] + 2*s.bw
	if n.tag != "td" && n.tag != "th" {
		m := resolveEdges(s.margin, basis)
		extra += m[1] + m[3]
	}
	minimum = min(4096, minimum+extra)
	preferred = min(4096, preferred+extra)
	if s.width.definite(basis) {
		width := s.width.resolve(basis, 4096)
		if !s.borderBox {
			width += extra
		}
		preferred = max(preferred, min(4096, width))
	}
	return max(1, minimum), max(1, preferred)
}
