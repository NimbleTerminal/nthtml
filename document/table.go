package document

import (
	"image"
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
	if l.tableWork > 100000 {
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
		surplus := apportion(preferred, room-demand)
		for i := range extra {
			extra[i] += surplus[i]
		}
	}
	for i := range base {
		base[i] += extra[i]
	}
	return base
}

func (l *layout) table(n *node, x, y, w, parentHeight int, s style) int {
	for _, child := range n.children {
		if child.tag == "caption" {
			y = l.block(child, x, y, w, parentHeight, s)
		}
	}
	rows, columns, err := l.d.collectTable(n, s, l.tableStep)
	if err != nil {
		l.err = err
		return y
	}
	if columns > 256 || columns > w {
		l.err = limitError("table columns")
		return y
	}
	if columns == 0 {
		return y
	}
	minimum, preferred := make([]int, columns), make([]int, columns)
	sx, sy := s.spacing[0], s.spacing[1]
	if s.collapse {
		sx, sy = 0, 0
	}
	// Keep one content pixel per column; constrain spacing on narrow viewports.
	sx = min(sx, max(0, (w-columns)/(columns+1)))
	for _, row := range rows {
		for i, cell := range row.cells {
			a, b := l.preferredCell(cell, row.styles[i], w)
			sp := row.spans[i]
			spreadRequirement(minimum, sp.col, sp.cols, max(1, a-sx*(sp.cols-1)))
			spreadRequirement(preferred, sp.col, sp.cols, max(1, b-sx*(sp.cols-1)))
		}
	}
	if l.err != nil {
		return y
	}
	widths := tableWidths(minimum, preferred, w-sx*(columns+1))
	xs := []int{x + sx}
	for _, width := range widths {
		xs = append(xs, xs[len(xs)-1]+width+sx)
	}
	var grid *collapsedGrid
	if s.collapse {
		grid = l.collapseGrid(rows, columns, s)
		if l.err != nil {
			return y
		}
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
				return y
			}
			sp := row.spans[i]
			width := xs[sp.col+sp.cols] - xs[sp.col] - sx
			cs := row.styles[i]
			cs.width = cssLength{}
			cs.maxWidth = cssLength{}
			cs.margin = [4]cssLength{}
			padding := resolveEdges(cs.padding, width)
			if grid != nil {
				inset := grid.spanInsets(ri, sp)
				for j := range padding {
					padding[j] += inset[j]
				}
				cs.bw = 0
			}
			cs.bw = min(cs.bw, max(0, (width-1)/2))
			room := max(0, width-2*cs.bw-1)
			if padding[1]+padding[3] > room {
				sides := apportion([]int{padding[3], padding[1]}, room)
				padding[3], padding[1] = sides[0], sides[1]
			}
			for j := range padding {
				cs.padding[j] = px(padding[j])
			}
			first := len(l.ops)
			h := l.blockStyled(cell, xs[sp.col], 0, width, parentHeight, cs)
			cells = append(cells, laidCell{first, len(l.ops), h, ri, sp, cs})
			if sp.rows == 1 {
				heights[ri] = max(heights[ri], h)
			}
		}
	}
	for _, cell := range cells {
		spreadRequirement(heights, cell.row, cell.span.rows, max(0, cell.height-sy*(cell.span.rows-1)))
	}
	rowY := []int{y + sy}
	for _, h := range heights {
		rowY = append(rowY, rowY[len(rowY)-1]+h+sy)
	}
	if rowY[len(rowY)-1] > l.d.limits.Height {
		l.err = limitError("table height")
		return y
	}
	if l.err != nil {
		return y
	}
	for ri, row := range rows {
		top, bottom := rowY[ri], rowY[ri+1]-sy
		l.ops[rowOps[ri]].box = Box{Tag: "tr", Bounds: image.Rect(x+sx, top, x+w-sx, bottom), Color: row.style.bg}
		if len(rowPieces[ri]) > 0 {
			l.ops[rowOps[ri]].box.Color.A = 0
			for col, op := range rowPieces[ri] {
				l.ops[op].box = Box{Tag: "row-background", Bounds: image.Rect(xs[col], top, xs[col]+widths[col], bottom), Color: row.style.bg}
			}
		}
		for _, name := range row.anchors {
			l.add(Box{Tag: "anchor", Anchor: name, Bounds: image.Rect(x, top, x+1, top+1)}, nil)
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
			l.ops[i].box.Bounds = l.ops[i].box.Bounds.Add(image.Pt(0, top))
		}
		l.ops[cell.start].box.Bounds.Max.Y = bottom
		contentEnd := cell.end
		if cell.s.bw > 0 {
			contentEnd -= 4
			l.ops[contentEnd+1].box.Bounds = l.ops[contentEnd+1].box.Bounds.Add(image.Pt(0, delta))
			l.ops[contentEnd+2].box.Bounds.Max.Y = bottom
			l.ops[contentEnd+3].box.Bounds.Max.Y = bottom
		}
		shift := 0
		switch cell.s.vertical {
		case "middle":
			shift = delta / 2
		case "bottom":
			shift = delta
		}
		for i := cell.start + 1; i < contentEnd; i++ {
			l.ops[i].box.Bounds = l.ops[i].box.Bounds.Add(image.Pt(0, shift))
		}
	}
	if grid != nil {
		grid.paint(l, x, widths, rowY)
	}
	return rowY[len(rowY)-1]
}
