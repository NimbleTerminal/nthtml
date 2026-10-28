package document

import (
	"unicode"
)

// Intrinsic widths track unbroken words across inline nodes and the longest
// explicit line, rather than summing unrelated paragraphs. Both are capped
// at the renderer's maximum width; traversal shares the table work budget.
func (l *layout) preferredCell(n *node, s style, basis int) (int, int) {
	s = l.controlSize(n, s)
	line, word, minimum, preferred := 0, 0, 0, 0
	pending, hasContent := false, false
	flushWord := func() { minimum = max(minimum, word); word = 0 }
	flushLine := func() { flushWord(); preferred = max(preferred, line); line = 0; pending = false; hasContent = false }
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
			if n.tag == "img" || st.display == "inline-block" || st.display == "inline-flex" || st.display == "inline-grid" {
				if n.tag == "img" && !l.imgHasBox(n, st) {
					return
				}
				if st.display != "inline" && st.display != "inline-block" && st.display != "inline-flex" && st.display != "inline-grid" {
					flushLine()
					a, b := l.preferredCell(n, st, basis)
					minimum = max(minimum, a)
					preferred = max(preferred, b)
					return
				}
				a, b := l.preferredCell(n, st, basis)
				if pending {
					line = min(4096, line+l.glyphAdvanceStyled(st, l.face(st), ' '))
					pending = false
					flushWord()
				}
				line = min(4096, line+b)
				word = min(4096, word+a)
				hasContent = true
				return
			}
			if st.display != "inline" {
				flushLine()
				a, b := l.preferredCell(n, st, basis)
				minimum = max(minimum, a)
				preferred = max(preferred, b)
				return
			}
			m, p := resolveEdges(st.margin, basis), resolveEdges(st.padding, basis)
			inlineExtra := m[1] + m[3] + p[1] + p[3] + st.borderH()
			line = min(4096, max(0, line+inlineExtra))
			word = min(4096, max(0, word+inlineExtra))
		}
		if n.tag == "" {
			if st.size == 0 {
				return
			}
			f := l.face(st)
			text := applyTextTransform(n.text, st.textTransform)
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
					pending = hasContent
					continue
				}
				if pending {
					line = min(4096, line+l.glyphAdvanceStyled(st, f, ' '))
					pending = false
				}
				w := l.glyphAdvanceStyled(st, f, r)
				line = min(4096, line+w)
				hasContent = true
				if unicode.IsSpace(r) {
					flushWord()
				} else {
					word = min(4096, word+w)
				}
			}
		}
		for _, c := range displayedChildren(n) {
			visit(c, st)
		}
	}
	if kind, _ := diagramSource(n); kind != "" {
		minimum, preferred = 32, 300
	} else if n.tag == "img" {
		if src := l.opts.Images[n.attrs["src"]]; src != nil && !src.Bounds().Empty() {
			iw, _ := imageSizeStyled(n, s, src.Bounds().Size(), s.width.definite(basis), max(1, basis), -1)
			preferred = min(4096, iw)
			minimum = preferred
		} else if alt := cleanText(n.attrs["alt"]); alt != "" {
			return l.preferredCell(&node{children: []*node{{text: "[image: " + alt + "]"}}}, s, basis)
		} else if iw, _, ok := missingImageSpacer(n, s, max(1, basis), -1, l.d.limits.Height); ok {
			preferred = min(4096, iw)
			minimum = preferred
		} else {
			return 0, 0
		}
	} else if n.tag == "table" {
		rows, cols, err := l.d.collectTable(n, s, l.tableStep)
		if err != nil {
			l.err = err
			return 1, 1
		}
		sx := s.spacing[0]
		var grid *collapsedGrid
		if s.collapse {
			sx = 0
			grid = l.collapseGrid(rows, cols, s)
			if l.err != nil {
				return 1, 1
			}
		}
		mins, prefs := make([]int, cols), make([]int, cols)
		for ri, row := range rows {
			for i, cell := range row.cells {
				cs := row.styles[i]
				sp := row.spans[i]
				if grid != nil {
					cs.bw = grid.spanInsets(ri, sp)
				}
				a, b := l.preferredCell(cell, cs, basis)
				spreadRequirement(mins, sp.col, sp.cols, max(1, a-sx*(sp.cols-1)))
				spreadRequirement(prefs, sp.col, sp.cols, max(1, b-sx*(sp.cols-1)))
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
		for _, child := range displayedChildren(n) {
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
		for _, c := range displayedChildren(n) {
			visit(c, s)
		}
		flushLine()
	}
	p := resolveEdges(s.padding, basis)
	bh := s.borderH()
	if n.tag == "table" && s.collapse {
		bh = 0
	}
	extra := p[1] + p[3] + bh
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
