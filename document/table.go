package document

import (
	"image"
	"image/color"
)

type tableRow struct {
	spans     []cellSpan
	groupEnd  int
	anchorIDs []int
	anchors   []string
	style     style
	cells     []*node
	styles    []style
}

// Table work is cumulative across nested tables, so repeated intrinsic
// measurement cannot multiply the parser's node budget without a ceiling.
func (l *layout) tableStep() bool {
	if l.err != nil {
		return false
	}
	if err := l.ctx.Err(); err != nil {
		l.err = err
		return false
	}
	l.tableWork++
	if l.tableWork > max(100000, l.d.limits.Nodes*20) {
		l.err = limitError("table measurement")
		return false
	}
	return true
}

// apportion distributes an exact pixel budget, with deterministic remainder
// handling. Columns may have zero width when there are more columns than pixels.
func apportion(weights []int, total int) []int {
	out := make([]int, len(weights))
	sum := 0
	for _, v := range weights {
		sum += v
	}
	if sum == 0 {
		return out
	}
	used, acc := 0, 0
	for i, v := range weights {
		acc += v
		next := int(int64(total) * int64(acc) / int64(sum))
		out[i] = next - used
		used = next
	}
	return out
}
func tableWidths(minimum, preferred []int, width int) []int {
	return tableWidthsFixed(minimum, preferred, nil, width)
}

func tableWidthsFixed(minimum, preferred []int, fixed []bool, width int) []int {
	n := len(preferred)
	if n == 0 {
		return nil
	}
	base := make([]int, n)
	sum := 0
	for i := range base {
		base[i] = minimum[i]
		sum += base[i]
	}
	if sum >= width {
		if width < n {
			return apportion(base, width)
		}
		for i := range base {
			base[i] = max(0, base[i]-1)
		}
		out := apportion(base, width-n)
		for i := range out {
			out[i]++
		}
		return out
	}
	growth := make([]int, n)
	for i := range growth {
		growth[i] = max(0, preferred[i]-base[i])
	}
	room := width - sum
	demand := 0
	for _, v := range growth {
		demand += v
	}
	extra := apportion(growth, min(room, demand))
	if room > demand {
		weights := preferred
		if len(fixed) == n {
			flexSum := 0
			for i, p := range preferred {
				if !fixed[i] {
					flexSum += p
				}
			}
			if flexSum > 0 {
				weights = make([]int, n)
				for i, p := range preferred {
					if !fixed[i] {
						weights[i] = p
					}
				}
			}
		}
		surplus := apportion(weights, room-demand)
		for i := range extra {
			extra[i] += surplus[i]
		}
	}
	for i := range base {
		base[i] += extra[i]
	}
	return base
}

func (l *layout) table(n *node, x, y, w, parentHeight int, autoWidth bool, minWidth int, s style) (int, int) {
	rows, columns, err := l.d.collectTable(n, s, l.tableStep)
	if err != nil {
		l.err = err
		return y, w
	}
	if columns > 256 || columns > w {
		l.err = limitError("table columns")
		return y, w
	}
	if columns == 0 {
		for _, child := range n.children {
			if child.tag == "caption" {
				y = l.block(child, x, y, w, parentHeight, s)
			}
		}
		return y, w
	}
	var grid *collapsedGrid
	if s.collapse {
		grid = l.collapseGrid(rows, columns, s)
		if l.err != nil {
			return y, w
		}
	}
	minimum, preferred := make([]int, columns), make([]int, columns)
	fixed := make([]bool, columns)
	sx, sy := s.spacing[0], s.spacing[1]
	if s.collapse {
		sx, sy = 0, 0
	}
	// Keep one content pixel per column; constrain spacing on narrow viewports.
	sx = min(sx, max(0, (w-columns)/(columns+1)))
	for ri, row := range rows {
		for i, cell := range row.cells {
			cs := row.styles[i]
			sp := row.spans[i]
			if sp.cols == 1 && cs.width.definite(w) {
				fixed[sp.col] = true
			}
			if grid != nil {
				cs.bw = grid.spanInsets(ri, sp)
			}
			a, b := l.preferredCell(cell, cs, w)
			spreadRequirement(minimum, sp.col, sp.cols, max(1, a-sx*(sp.cols-1)))
			spreadRequirement(preferred, sp.col, sp.cols, max(1, b-sx*(sp.cols-1)))
		}
	}
	if colSpecs := l.d.collectColSpecs(n, s); len(colSpecs) > 0 {
		for c := 0; c < columns && c < len(colSpecs); c++ {
			if colSpecs[c].width.definite(w) {
				fixed[c] = true
				cw := max(1, colSpecs[c].width.resolve(w, w))
				preferred[c] = max(preferred[c], cw)
				minimum[c] = max(minimum[c], min(cw, preferred[c]))
			}
		}
	}
	if l.err != nil {
		return y, w
	}
	if autoWidth {
		pref := sx * (columns + 1)
		for _, v := range preferred {
			pref += v
		}
		for _, child := range n.children {
			if child.tag == "caption" {
				cs := l.d.styled(child, s)
				if cs.display != "none" {
					_, b := l.preferredCell(child, cs, w)
					pref = max(pref, b)
				}
			}
		}
		w = max(columns+sx*(columns+1), min(w, max(minWidth, pref)))
	}
	isBottomCaption := func(c *node) bool {
		cs := l.d.styled(c, s)
		return cs.captionSide == "bottom" || (cs.captionSide == "" && s.captionSide == "bottom")
	}
	for _, child := range n.children {
		if child.tag == "caption" && !isBottomCaption(child) {
			y = l.block(child, x, y, w, parentHeight, s)
		}
	}
	widths := tableWidthsFixed(minimum, preferred, fixed, w-sx*(columns+1))
	xs := []int{x + sx}
	for _, width := range widths {
		xs = append(xs, xs[len(xs)-1]+width+sx)
	}
	type laidCell struct {
		start, end, height, row int
		span                    cellSpan
		s                       style
	}
	var cells []laidCell
	heights := make([]int, len(rows))
	// Allocate every row background before cell contents so a rowspan cannot
	// be overpainted by the background of a later row.
	rowOps := make([]int, len(rows))
	rowPieces := make([][]int, len(rows))
	for i, row := range rows {
		rowOps[i] = len(l.ops)
		l.add(Box{Tag: "tr"}, nil)
		if sx > 0 && row.style.bg.A > 0 {
			for range widths {
				rowPieces[i] = append(rowPieces[i], len(l.ops))
				l.add(Box{Tag: "row-background"}, nil)
			}
		}
		if row.style.height.definite(parentHeight) {
			heights[i] = row.style.height.resolve(parentHeight, l.d.limits.Height)
		}
	}
	for ri, row := range rows {
		for i, cell := range row.cells {
			if !l.tableStep() {
				return y, w
			}
			sp := row.spans[i]
			width := xs[sp.col+sp.cols] - xs[sp.col] - sx
			cs := row.styles[i]
			cs.width = cssLength{}
			cs.maxWidth = cssLength{}
			cs.minWidth = cssLength{}
			cs.margin = [4]cssLength{}
			padding := resolveEdges(cs.padding, width)
			cellBorder := cs
			if grid != nil {
				inset := grid.spanInsets(ri, sp)
				for j := range padding {
					padding[j] += inset[j]
				}
				cellBorder.bw = [4]int{}
			} else {
				if cellBorder.borderH() > width {
					sides := apportion([]int{cellBorder.bw[3], cellBorder.bw[1]}, max(0, width))
					cellBorder.bw[3], cellBorder.bw[1] = sides[0], sides[1]
				}
				room := max(0, width-cellBorder.borderH())
				if padding[1]+padding[3] > room {
					sides := apportion([]int{padding[3], padding[1]}, room)
					padding[3], padding[1] = sides[0], sides[1]
				}
				for j := range padding {
					padding[j] += cellBorder.bw[j]
				}
			}
			cs.bw = [4]int{}
			for j := range padding {
				cs.padding[j] = px(padding[j])
			}
			cellMinH := 0
			if cs.height.definite(parentHeight) {
				cellMinH = cs.height.resolve(parentHeight, l.d.limits.Height)
				if !cs.borderBox {
					cellMinH += padding[0] + padding[2]
				}
			}
			cs.height = cssLength{}
			first := len(l.ops)
			prevSource := l.activeSource
			if len(row.anchors) > 0 {
				l.activeSource = row.anchors[len(row.anchors)-1]
			}
			h := l.blockStyled(cell, xs[sp.col], 0, width, parentHeight, cs)
			l.activeSource = prevSource
			cells = append(cells, laidCell{first, len(l.ops), h, ri, sp, cellBorder})
			reqH := max(h, cellMinH)
			if sp.rows == 1 {
				heights[ri] = max(heights[ri], reqH)
			} else {
				spreadRequirement(heights, ri, sp.rows, max(0, reqH-sy*(sp.rows-1)))
			}
		}
	}
	for _, cell := range cells {
		if cell.span.rows > 1 {
			spreadRequirement(heights, cell.row, cell.span.rows, max(0, cell.height-sy*(cell.span.rows-1)))
		}
	}
	rowY := []int{y + sy}
	for _, h := range heights {
		rowY = append(rowY, rowY[len(rowY)-1]+h+sy)
	}
	if rowY[len(rowY)-1] > l.pageHeightLimit() {
		l.err = limitError("table height")
		return y, w
	}
	if l.err != nil {
		return y, w
	}
	for ri, row := range rows {
		top, bottom := rowY[ri], rowY[ri+1]-sy
		rowBg := row.style.bg
		if row.style.hidden {
			rowBg = color.RGBA{}
		}
		rowSource := l.activeSource
		if len(row.anchors) > 0 {
			rowSource = row.anchors[len(row.anchors)-1]
		}
		var rowSrc image.Image
		if !row.style.hidden && row.style.bgImage != "" && row.style.bgImage != "none" {
			rowSrc = l.renderBackgroundImage(w-2*sx, bottom-top, row.style)
		}
		l.ops[rowOps[ri]].box = Box{Tag: "tr", Bounds: image.Rect(x+sx, top, x+w-sx, bottom), Color: rowBg, SourceID: rowSource}
		l.ops[rowOps[ri]].source = rowSrc
		if len(rowPieces[ri]) > 0 {
			l.ops[rowOps[ri]].box.Color.A = 0
			l.ops[rowOps[ri]].source = nil
			for col, op := range rowPieces[ri] {
				l.ops[op].box = Box{Tag: "row-background", Bounds: image.Rect(xs[col], top, xs[col]+widths[col], bottom), Color: rowBg, SourceID: rowSource}
				if !row.style.hidden && row.style.bgImage != "" && row.style.bgImage != "none" {
					l.ops[op].source = l.renderBackgroundImage(widths[col], bottom-top, row.style)
				}
			}
		}
		for _, name := range row.anchors {
			l.add(Box{Tag: "anchor", Anchor: name, SourceID: name, Bounds: image.Rect(x, top, x+1, top+1)}, nil)
		}
	}
	for _, cell := range cells {
		if cell.start >= cell.end {
			continue
		}
		top := rowY[cell.row]
		bottom := rowY[cell.row+cell.span.rows] - sy
		delta := bottom - top - cell.height
		for i := cell.start; i < cell.end; i++ {
			l.ops[i].box.translate(image.Pt(0, top))
		}
		l.ops[cell.start].box.Bounds.Max.Y = bottom
		if !cell.s.hidden && cell.s.bgImage != "" && cell.s.bgImage != "none" && bottom-top != cell.height {
			l.ops[cell.start].source = l.renderBackgroundImage(l.ops[cell.start].box.Bounds.Dx(), bottom-top, cell.s)
		}
		shift := 0
		switch cell.s.vertical {
		case "middle":
			shift = delta / 2
		case "bottom":
			shift = delta
		}
		for i := cell.start + 1; i < cell.end; i++ {
			l.ops[i].box.translate(image.Pt(0, shift))
		}
		if !cell.s.hidden {
			l.paintBorders(l.ops[cell.start].box.Bounds, cell.s)
		}
	}
	if grid != nil {
		grid.paint(l, x, widths, rowY)
	}
	endY := rowY[len(rowY)-1]
	for _, child := range n.children {
		if child.tag == "caption" && isBottomCaption(child) {
			endY = l.block(child, x, endY, w, parentHeight, s)
		}
	}
	return endY, w
}
