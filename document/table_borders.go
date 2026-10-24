package document

import (
	"image"
	"image/color"
)

type tableBorder struct {
	width, priority int
	color           color.RGBA
}

func borderOf(s style, priority int) tableBorder {
	c := s.border
	if c.A == 0 {
		c = s.fg
	}
	return tableBorder{s.bw, priority, c}
}
func chooseBorder(a, b tableBorder) tableBorder {
	// Solid borders only. Wider wins; cells beat rows, rows beat tables.
	// Equal candidates keep the earlier (top/left) cell.
	if b.width > a.width || b.width == a.width && b.priority > a.priority {
		return b
	}
	return a
}

type collapsedGrid struct {
	rows, cols int
	h, v       []tableBorder
}

func (l *layout) collapseGrid(rows []tableRow, cols int, s style) *collapsedGrid {
	count := (len(rows)+1)*cols + len(rows)*(cols+1)
	if count > 100000 {
		l.err = limitError("table border edges")
		return nil
	}
	g := &collapsedGrid{len(rows), cols, make([]tableBorder, (len(rows)+1)*cols), make([]tableBorder, len(rows)*(cols+1))}
	for r, row := range rows {
		for c := 0; c < cols; c++ {
			if !l.tableStep() {
				return nil
			}
			top, bottom, left, right := &g.h[r*cols+c], &g.h[(r+1)*cols+c], &g.v[r*(cols+1)+c], &g.v[r*(cols+1)+c+1]
			rb := borderOf(row.style, 1)
			*top = chooseBorder(*top, rb)
			*bottom = chooseBorder(*bottom, rb)
			if c == 0 {
				*left = chooseBorder(*left, rb)
			}
			if c == cols-1 {
				*right = chooseBorder(*right, rb)
			}
			if c < len(row.styles) {
				cb := borderOf(row.styles[c], 2)
				*top = chooseBorder(*top, cb)
				*bottom = chooseBorder(*bottom, cb)
				*left = chooseBorder(*left, cb)
				*right = chooseBorder(*right, cb)
			}
			tb := borderOf(s, 0)
			if r == 0 {
				*top = chooseBorder(*top, tb)
			}
			if r == len(rows)-1 {
				*bottom = chooseBorder(*bottom, tb)
			}
			if c == 0 {
				*left = chooseBorder(*left, tb)
			}
			if c == cols-1 {
				*right = chooseBorder(*right, tb)
			}
		}
	}
	return g
}
func (g *collapsedGrid) insets(r, c int) [4]int {
	top := g.h[r*g.cols+c].width
	bottom := g.h[(r+1)*g.cols+c].width
	left := g.v[r*(g.cols+1)+c].width
	right := g.v[r*(g.cols+1)+c+1].width
	if r > 0 {
		top = (top + 1) / 2
	}
	if r < g.rows-1 {
		bottom = bottom / 2
	}
	if c > 0 {
		left = (left + 1) / 2
	}
	if c < g.cols-1 {
		right = right / 2
	}
	return [4]int{top, right, bottom, left}
}
func (g *collapsedGrid) paint(l *layout, x int, widths, ys []int) {
	xs := []int{x}
	for _, w := range widths {
		xs = append(xs, xs[len(xs)-1]+w)
	}
	bounds := image.Rect(x, ys[0], xs[len(xs)-1], ys[len(ys)-1])
	for r := 0; r <= g.rows; r++ {
		for c := 0; c < g.cols; c++ {
			b := g.h[r*g.cols+c]
			y := ys[r] - b.width/2
			if r == 0 {
				y = ys[r]
			}
			if r == g.rows {
				y = ys[r] - b.width
			}
			l.rect(image.Rect(xs[c], y, xs[c+1], y+b.width).Intersect(bounds), b.color, "collapsed-border")
		}
	}
	for r := 0; r < g.rows; r++ {
		for c := 0; c <= g.cols; c++ {
			b := g.v[r*(g.cols+1)+c]
			x := xs[c] - b.width/2
			if c == 0 {
				x = xs[c]
			}
			if c == g.cols {
				x = xs[c] - b.width
			}
			l.rect(image.Rect(x, ys[r], x+b.width, ys[r+1]).Intersect(bounds), b.color, "collapsed-border")
		}
	}
}
