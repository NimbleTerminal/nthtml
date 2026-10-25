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
			if st.display == "none" || st.position == "absolute" {
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
			if st.size == 0 {
				return
			}
			f := l.face(st)
			text := n.text
			if st.pre {
				text, _ = preTextAt(text, 0)
			}
			for _, r := range text {
				if !l.tableStep() {
					return
				}
				if (st.pre || st.whiteSpace == "pre-line") && r == '\n' {
					flushLine()
					continue
				}
				if !st.pre && cssSpace(r) {
					flushWord()
					pending = line > 0
					continue
				}
				if pending {
					line = min(4096, line+glyphAdvance(f, ' ', st.wordSpacing))
					pending = false
				}
				w := glyphAdvance(f, r, st.wordSpacing)
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
		rows, cols, err := l.d.collectTable(n, s, l.tableStep)
		if err != nil {
			l.err = err
			return 1, 1
		}
		mins, prefs := make([]int, cols), make([]int, cols)
		for _, row := range rows {
			for i, cell := range row.cells {
				a, b := l.preferredCell(cell, row.styles[i], basis)
				sp := row.spans[i]
				spreadRequirement(mins, sp.col, sp.cols, a)
				spreadRequirement(prefs, sp.col, sp.cols, b)
			}
		}
		for i := range mins {
			minimum = min(4096, minimum+mins[i])
			preferred = min(4096, preferred+prefs[i])
		}
		if !s.collapse {
			minimum = min(4096, minimum+s.spacing[0]*(cols+1))
			preferred = min(4096, preferred+s.spacing[0]*(cols+1))
		}
		for _, child := range n.children {
			if child.tag == "caption" {
				cs := l.d.styled(child, s)
				if cs.display != "none" {
					a, b := l.preferredCell(child, cs, basis)
					minimum = max(minimum, a)
					preferred = max(preferred, b)
				}
			}
		}

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
	if s.whiteSpace == "nowrap" || s.whiteSpace == "pre" {
		minimum = preferred
	} else if s.overflowWrap == "anywhere" {
		minimum = 1
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
	if s.maxWidth.definite(basis) {
		v := s.maxWidth.resolve(basis, 4096)
		if !s.borderBox {
			v += extra
		}
		minimum = min(minimum, v)
		preferred = min(preferred, v)
	}
	if s.minWidth.definite(basis) {
		v := s.minWidth.resolve(basis, 4096)
		if !s.borderBox {
			v += extra
		}
		minimum = max(minimum, v)
		preferred = max(preferred, v)
	}
	return max(1, min(4096, minimum)), max(1, min(4096, preferred))
}
