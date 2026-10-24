package document

import (
	"image"
	"strings"

	"golang.org/x/image/font"
)

type tableRow struct {
	style  style
	cells  []*node
	styles []style
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

// preferredCell is a bounded preferred-width estimate, not a browser's
// min/max-content algorithm. Words guide compression; long tokens can wrap.
func (l *layout) preferredCell(n *node, s style, basis int) (int, int) {
	preferred, word := 0, 0
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
		}
		if n.tag == "" {
			f := l.face(st)
			for _, part := range strings.Fields(n.text) {
				if !l.tableStep() {
					return
				}
				width := min(4096, font.MeasureString(f, part).Ceil())
				preferred = min(4096, preferred+width+font.MeasureString(f, " ").Ceil())
				word = max(word, width)
			}
		}
		for _, c := range n.children {
			visit(c, st)
		}
	}
	// The cell style is already resolved; only visit descendants.
	for _, c := range n.children {
		visit(c, s)
	}
	p := resolveEdges(s.padding, basis)
	extra := p[1] + p[3] + 2*s.bw
	preferred += extra
	word += extra
	if s.width.definite(basis) {
		requested := s.width.resolve(basis, 4096)
		if !s.borderBox {
			requested += extra
		}
		preferred = max(preferred, requested)
	}
	return max(1, min(word, 4096)), max(1, min(preferred, 4096))
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
		growth[i] = max(1, preferred[i]-base[i])
	}
	extra := apportion(growth, width-sum)
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
	var rows []tableRow
	columns := 0
	var collect func(*node, style)
	collect = func(parent *node, ps style) {
		for _, child := range parent.children {
			if !l.tableStep() {
				return
			}
			cs := l.d.styled(child, ps)
			if cs.display == "none" {
				continue
			}
			switch child.tag {
			case "thead", "tbody", "tfoot":
				collect(child, cs)
			case "tr":
				if cs.bg.A == 0 {
					cs.bg = ps.bg
				}
				row := tableRow{style: cs}
				for _, cell := range child.children {
					if cell.tag != "td" && cell.tag != "th" {
						continue
					}
					cellStyle := l.d.styled(cell, cs)
					if cellStyle.display == "none" {
						continue
					}
					row.cells = append(row.cells, cell)
					row.styles = append(row.styles, cellStyle)
				}
				columns = max(columns, len(row.cells))
				rows = append(rows, row)
			}
		}
	}
	collect(n, s)
	if columns > 256 || columns > w {
		l.err = limitError("table columns")
		return y
	}
	if columns == 0 {
		return y
	}
	minimum, preferred := make([]int, columns), make([]int, columns)
	for _, row := range rows {
		for i, cell := range row.cells {
			a, b := l.preferredCell(cell, row.styles[i], w)
			minimum[i] = max(minimum[i], a)
			preferred[i] = max(preferred[i], b)
		}
	}
	if l.err != nil {
		return y
	}
	widths := tableWidths(minimum, preferred, w)
	for _, row := range rows {
		if !l.tableStep() {
			return y
		}
		rowStart := len(l.ops)
		l.add(Box{Tag: "tr"}, nil)
		type laidCell struct {
			start, end, bottom int
			s                  style
		}
		var cells []laidCell
		right, bottom := x, y
		for i, cell := range row.cells {
			width := widths[i]
			if width == 0 {
				continue
			}
			cs := row.styles[i]
			// Column allocation owns the cell's outer width; width declarations are
			// sizing hints above. Cell margins are ignored, as in CSS tables.
			cs.width = cssLength{}
			cs.maxWidth = cssLength{}
			cs.margin = [4]cssLength{}
			// Keep a content pixel at narrow sizes; shrink excessive cell padding.
			padding := resolveEdges(cs.padding, width)
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
			endY := l.blockStyled(cell, right, y, width, parentHeight, cs)
			cells = append(cells, laidCell{first, len(l.ops), endY, cs})
			bottom = max(bottom, endY)
			right += width
		}
		if row.style.height.definite(parentHeight) {
			bottom = max(bottom, y+row.style.height.resolve(parentHeight, l.d.limits.Height))
		}
		if l.err != nil {
			return bottom
		}
		l.ops[rowStart].box = Box{Tag: "tr", Bounds: image.Rect(x, y, x+w, bottom), Color: row.style.bg}
		for _, cell := range cells {
			delta := bottom - cell.bottom
			if cell.start >= cell.end {
				continue
			}
			box := &l.ops[cell.start].box
			box.Bounds.Max.Y = bottom
			contentEnd := cell.end
			if cell.s.bw > 0 {
				contentEnd -= 4
				// blockStyled emits top, bottom, left, right borders after descendants.
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
		y = bottom
	}
	return y
}
