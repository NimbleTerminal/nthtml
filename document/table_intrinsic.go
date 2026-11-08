package document

import (
	"reflect"
	"strings"
	"unicode"
)

// Intrinsic widths track unbroken words across inline nodes and the longest
// explicit line, rather than summing unrelated paragraphs. Both are capped
// at the renderer's maximum width; traversal shares the table work budget.
type intrinsicKey struct {
	node  *node
	basis int
}
type intrinsicEntry struct {
	style              style
	minimum, preferred int
}

func (l *layout) preferredCell(n *node, s style, basis int) (minimum, preferred int) {
	if !l.tableStep() {
		return 0, 0
	}
	key := intrinsicKey{n, basis}
	for _, entry := range l.intrinsic[key] {
		if reflect.DeepEqual(entry.style, s) {
			return entry.minimum, entry.preferred
		}
	}
	minimum, preferred = l.measurePreferredCell(n, s, basis)
	if l.err == nil && l.intrinsicCount < 2048 && len(l.intrinsic[key]) < 4 {
		if l.intrinsic == nil {
			l.intrinsic = map[intrinsicKey][]intrinsicEntry{}
		}
		l.intrinsic[key] = append(l.intrinsic[key], intrinsicEntry{s, minimum, preferred})
		l.intrinsicCount++
	}
	return
}

func (l *layout) measurePreferredCell(n *node, s style, basis int) (int, int) {
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
			if n.svg != "" || n.tag == "img" || st.display == "inline-block" || st.display == "inline-flex" || st.display == "inline-grid" {
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
		if kind == "svg" {
			w, _ := svgIntrinsicDimensions(n.svg)
			minimum, preferred = w, w
		}
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
	} else if s.display == "flex" || s.display == "inline-flex" {
		column := s.flexDirection == "column" || s.flexDirection == "column-reverse"
		count := 0
		for _, child := range displayedChildren(n) {
			if !l.tableStep() {
				break
			}
			cs := s
			if child.tag == "" {
				if strings.TrimSpace(child.text) == "" {
					continue
				}
				child = &node{tag: "span", parent: n, children: []*node{child}}
				cs.display = "block"
			} else {
				cs = l.d.styled(child, s)
			}
			if cs.display == "none" || cs.position == "absolute" {
				continue
			}
			a, b := l.preferredCell(child, cs, basis)
			if column {
				minimum = max(minimum, a)
				preferred = max(preferred, b)
			} else {
				preferred = min(4096, preferred+b)
				if s.flexWrap == "wrap" {
					minimum = max(minimum, a)
				} else {
					minimum = min(4096, minimum+a)
				}
			}
			count++
		}
		if !column && count > 1 {
			gap := s.colGap.resolve(basis, 256) * (count - 1)
			preferred = min(4096, preferred+gap)
			if s.flexWrap != "wrap" {
				minimum = min(4096, minimum+gap)
			}
		}
	} else if s.display == "grid" || s.display == "inline-grid" {
		minimum, preferred = l.measureGrid(n, s, basis)
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
		if fixedIntrinsicWidth(n, s) {
			// A non-replaced, non-table box with an absolute width contributes
			// exactly that width; wider content overflows rather than growing
			// the box (CSS Sizing 3 §5.1).
			minimum = min(4096, width)
			preferred = minimum
		} else {
			preferred = max(preferred, min(4096, width))
		}
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

// fixedIntrinsicWidth reports whether an absolute width fixes the box's
// intrinsic contribution. Tables and cells keep HTML's minimum-width
// semantics; replaced elements size through their own branches above;
// percentages and percentage calc() stay content-driven.
func fixedIntrinsicWidth(n *node, s style) bool {
	switch n.tag {
	case "table", "td", "th", "img":
		return false
	}
	if n.svg != "" || s.width.unit != lengthPX {
		return false
	}
	return s.width.expr == nil || !s.width.expr.hasPercent()
}

// measureGrid sizes a grid container's content the way its layout places
// items: columns from the template, items auto-placed with their spans. A
// fixed track is its length; any other track is as wide as the widest item
// that starts in it (a spanning item charges its first track). A template
// without tracks, or with auto-repeat, wraps items onto new rows as needed,
// so only one item sets the minimum. Primer buttons are three-column grids
// (icon, label, icon) whose width is the sum, not the widest child.
func (l *layout) measureGrid(n *node, s style, basis int) (minimum, preferred int) {
	type gridMeasure struct {
		cs        style
		min, pref int
	}
	var items []gridMeasure
	for _, child := range displayedChildren(n) {
		if !l.tableStep() {
			break
		}
		cs := s
		if child.tag == "" {
			if strings.TrimSpace(child.text) == "" {
				continue
			}
			child = &node{tag: "span", parent: n, children: []*node{child}}
			cs.display = "block"
		} else {
			cs = l.d.styled(child, s)
		}
		if cs.display == "none" || cs.position == "absolute" {
			continue
		}
		a, b := l.preferredCell(child, cs, basis)
		items = append(items, gridMeasure{cs: cs, min: a, pref: b})
	}
	if len(items) == 0 {
		return 0, 0
	}
	gap := s.colGap.resolve(basis, 256)
	wraps := len(s.gridCols) == 0
	for _, tr := range s.gridCols {
		if tr.autoRepeat {
			wraps = true
		}
	}
	if wraps {
		for _, it := range items {
			minimum = max(minimum, it.min)
			preferred = min(4096, preferred+it.pref)
		}
		return minimum, min(4096, preferred+gap*(len(items)-1))
	}
	numCols := len(s.gridCols)
	colMin, colPref := make([]int, numCols), make([]int, numCols)
	fixed := make([]bool, numCols)
	floor := make([]bool, numCols) // a definite minimum (minmax(0, auto)): the track may shrink to it
	for i, tr := range s.gridCols {
		if tr.fr == 0 && tr.minLen.definite(basis) && tr.maxLen.definite(basis) {
			lo, hi := tr.minLen.resolve(basis, 4096), tr.maxLen.resolve(basis, 4096)
			if lo == hi {
				colMin[i], colPref[i], fixed[i] = lo, lo, true
				continue
			}
		}
		if tr.minLen.definite(basis) {
			colMin[i], floor[i] = tr.minLen.resolve(basis, 4096), true
		}
	}
	cursor := 0
	for _, it := range items {
		start, span := resolveGridItemSpan(it.cs, numCols)
		if start >= 0 {
			cursor = start
		} else if cursor+span > numCols {
			cursor = 0
		}
		if cursor >= numCols {
			cursor = 0
		}
		if !fixed[cursor] {
			if !floor[cursor] {
				colMin[cursor] = max(colMin[cursor], it.min)
			}
			colPref[cursor] = max(colPref[cursor], it.pref)
		}
		cursor += max(1, span)
		if cursor >= numCols {
			cursor = 0
		}
	}
	for i := range colMin {
		minimum = min(4096, minimum+colMin[i])
		preferred = min(4096, preferred+colPref[i])
	}
	gaps := gap * (numCols - 1)
	return min(4096, minimum+gaps), min(4096, preferred+gaps)
}
