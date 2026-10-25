package document

import "image"

type floatBox struct {
	bounds image.Rectangle
	side   string
}
type floatArea struct {
	l       *layout
	boxes   []floatBox
	lastTop int
}

func (a *floatArea) step() bool {
	if a.l.err != nil {
		return false
	}
	if err := a.l.ctx.Err(); err != nil {
		a.l.err = err
		return false
	}
	a.l.floatWork++
	if a.l.floatWork > 100000 {
		a.l.err = limitError("float exclusion work")
		return false
	}
	return true
}
func (a *floatArea) room(x, y, w, h int) (left, right, next int) {
	left, right, next = x, x+w, -1
	for _, box := range a.boxes {
		if !a.step() {
			return
		}
		r := box.bounds
		if r.Empty() || r.Max.Y <= y || r.Min.Y >= y+max(1, h) || r.Max.X <= x || r.Min.X >= x+w {
			continue
		}
		if box.side == "left" {
			left = max(left, r.Max.X)
		} else {
			right = min(right, r.Min.X)
		}
		if next < 0 || r.Max.Y < next {
			next = r.Max.Y
		}
	}
	return
}
func (a *floatArea) line(x, y, w, h int) (int, int, int) {
	for {
		left, right, next := a.room(x, y, w, h)
		if a.l.err != nil || right > left || next <= y {
			return left, y, max(1, right-left)
		}
		y = next
	}
}
func (a *floatArea) clear(y int, side string) int {
	if side == "" || side == "none" {
		return y
	}
	for _, box := range a.boxes {
		if !a.step() {
			return y
		}
		if side == "both" || side == box.side {
			y = max(y, box.bounds.Max.Y)
		}
	}
	return y
}
func (a *floatArea) bottom() int {
	bottom := 0
	if len(a.boxes) > 0 {
		bottom = a.boxes[0].bounds.Max.Y
	}
	for _, box := range a.boxes {
		bottom = max(bottom, box.bounds.Max.Y)
	}
	return bottom
}
func (a *floatArea) place(n *node, s style, x, y, w, parentHeight int) {
	if len(a.boxes) > 0 {
		y = max(y, a.lastTop)
	}
	y = a.clear(y, s.clear)
	for i := range s.margin {
		if s.margin[i].unit == lengthPercent {
			s.margin[i] = px(s.margin[i].resolve(w, 256))
		}
		if s.margin[i].unit == lengthAuto {
			s.margin[i] = px(0)
		}
		if s.padding[i].unit == lengthPercent {
			s.padding[i] = px(s.padding[i].resolve(w, 256))
		}
	}
	for _, v := range []*cssLength{&s.width, &s.minWidth, &s.maxWidth} {
		if v.unit == lengthPercent {
			*v = px(v.resolve(w, 4096))
		}
	}
	if !s.width.definite(w) {
		minimum, preferred := a.l.preferredCell(n, s, w)
		var room int
		for {
			left, right, next := a.room(x, y, w, 1)
			room = max(0, right-left)
			if a.l.err != nil {
				return
			}
			if room >= minimum || next <= y {
				break
			}
			y = next
		}
		margin, padding := resolveEdges(s.margin, w), resolveEdges(s.padding, w)
		size := max(0, min(preferred, max(minimum, room))-margin[1]-margin[3])
		if !s.borderBox {
			size = max(0, size-padding[1]-padding[3]-2*s.bw)
		}
		s.width = px(size)
	}
	first := len(a.l.ops)
	end := a.l.blockStyled(n, 0, 0, w, parentHeight, s)
	if a.l.err != nil || first >= len(a.l.ops) {
		return
	}
	margin := resolveEdges(s.margin, w)
	width := a.l.ops[first].box.Bounds.Dx() + margin[1] + margin[3]
	left, right := x, x+w
	for {
		var next int
		left, right, next = a.room(x, y, w, end)
		if a.l.err != nil {
			return
		}
		if right-left >= width || next <= y {
			break
		}
		y = next
	}
	target := left
	if s.floatSide == "right" {
		target = right - width
	}
	shift := image.Pt(target, y)
	for i := first; i < len(a.l.ops); i++ {
		a.l.ops[i].box.translate(shift)
	}
	r := image.Rect(target, y, target+width, y+end)
	if r.Max.Y > a.l.d.limits.Height {
		a.l.err = limitError("float height")
		return
	}
	a.boxes = append(a.boxes, floatBox{r, s.floatSide})
	a.lastTop = y
}
func floatContext(n *node, s style) bool {
	if s.position == "absolute" || s.floatSide == "left" || s.floatSide == "right" {
		return true
	}
	switch n.tag {
	case "document", "html", "table", "td", "th":
		return true
	}
	return false
}
