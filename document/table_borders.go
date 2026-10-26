package document

import (
	"image"
	"image/color"
)

type tableBorder struct {
	width, priority int
	color           color.RGBA
	hidden          bool
}

func borderSideOf(s style, side, priority int) tableBorder {
	if s.borderStyle[side] == "hidden" {
		return tableBorder{width: 0, priority: 100, hidden: true}
	}
	c := s.borderColor(side)
	if s.hidden {
		c = color.RGBA{}
	}
	return tableBorder{width: s.bw[side], priority: priority, color: c}
}

func chooseBorder(a, b tableBorder) tableBorder {
	if a.hidden {
		return a
	}
	if b.hidden {
		return b
	}
	// Wider wins; cells beat rows, rows beat tables.
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
			*top = chooseBorder(*top, borderSideOf(row.style, 0, 1))
			*bottom = chooseBorder(*bottom, borderSideOf(row.style, 2, 1))
			if c == 0 {
				*left = chooseBorder(*left, borderSideOf(row.style, 3, 1))
			}
			if c == cols-1 {
				*right = chooseBorder(*right, borderSideOf(row.style, 1, 1))
			}

			if r == 0 {
				*top = chooseBorder(*top, borderSideOf(s, 0, 0))
			}
			if r == len(rows)-1 {
				*bottom = chooseBorder(*bottom, borderSideOf(s, 2, 0))
			}
			if c == 0 {
				*left = chooseBorder(*left, borderSideOf(s, 3, 0))
			}
			if c == cols-1 {
				*right = chooseBorder(*right, borderSideOf(s, 1, 0))
			}
		}
	}
	for r, row := range rows {
		for i, sp := range row.spans {
			cs := row.styles[i]
			for c := sp.col; c < sp.col+sp.cols; c++ {
				top, bottom := &g.h[r*cols+c], &g.h[(r+sp.rows)*cols+c]
				*top = chooseBorder(*top, borderSideOf(cs, 0, 2))
				*bottom = chooseBorder(*bottom, borderSideOf(cs, 2, 2))
			}
			for rr := r; rr < r+sp.rows; rr++ {
				left, right := &g.v[rr*(cols+1)+sp.col], &g.v[rr*(cols+1)+sp.col+sp.cols]
				*left = chooseBorder(*left, borderSideOf(cs, 3, 2))
				*right = chooseBorder(*right, borderSideOf(cs, 1, 2))
			}
		}
	}
	// No row or table border may draw across the interior of a spanning cell.
	for r, row := range rows {
		for _, sp := range row.spans {
			for rr := r + 1; rr < r+sp.rows; rr++ {
				for c := sp.col; c < sp.col+sp.cols; c++ {
					g.h[rr*cols+c] = tableBorder{}
				}
			}
			for rr := r; rr < r+sp.rows; rr++ {
				for c := sp.col + 1; c < sp.col+sp.cols; c++ {
					g.v[rr*(cols+1)+c] = tableBorder{}
				}
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
	vWidthAt := func(r, c int) int {
		w := 0
		if r > 0 {
			w = max(w, g.v[(r-1)*(g.cols+1)+c].width)
		}
		if r < g.rows {
			w = max(w, g.v[r*(g.cols+1)+c].width)
		}
		return w
	}
	for r := 0; r < g.rows; r++ {
		for c := 0; c <= g.cols; c++ {
			b := g.v[r*(g.cols+1)+c]
			vx := xs[c] - b.width/2
			if c == 0 {
				vx = xs[c]
			}
			if c == g.cols {
				vx = xs[c] - b.width
			}
			l.rect(image.Rect(vx, ys[r], vx+b.width, ys[r+1]).Intersect(bounds), b.color, "collapsed-border")
		}
	}
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
			hx0, hx1 := xs[c], xs[c+1]
			if c > 0 {
				vW := vWidthAt(r, c)
				leftH := g.h[r*g.cols+c-1]
				if chooseBorder(leftH, b) == b {
					hx0 = xs[c] - vW/2
				} else {
					hx0 = xs[c] - vW/2 + vW
				}
			}
			if c < g.cols-1 {
				vW := vWidthAt(r, c+1)
				rightH := g.h[r*g.cols+c+1]
				if chooseBorder(b, rightH) == rightH {
					hx1 = xs[c+1] - vW/2
				} else {
					hx1 = xs[c+1] - vW/2 + vW
				}
			}
			l.rect(image.Rect(hx0, y, hx1, y+b.width).Intersect(bounds), b.color, "collapsed-border")
		}
	}
}

func (g *collapsedGrid) spanInsets(r int, sp cellSpan) [4]int {
	var out [4]int
	for c := sp.col; c < sp.col+sp.cols; c++ {
		a := g.insets(r, c)
		b := g.insets(r+sp.rows-1, c)
		out[0] = max(out[0], a[0])
		out[2] = max(out[2], b[2])
	}
	for rr := r; rr < r+sp.rows; rr++ {
		a := g.insets(rr, sp.col)
		b := g.insets(rr, sp.col+sp.cols-1)
		out[3] = max(out[3], a[3])
		out[1] = max(out[1], b[1])
	}
	return out
}
