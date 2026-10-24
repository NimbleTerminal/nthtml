package document

import (
	"image"
	"math"
	"sort"
	"strings"
)

type positionFrame struct {
	parent     *positionFrame
	children   []*positionFrame
	node       *node
	op, border int
	shift      image.Point
	initial    image.Rectangle
}
type absoluteBox struct {
	node   *node
	style  style
	scope  *positionFrame
	anchor int
}

func parseInset(v string) (cssLength, bool) {
	v = strings.TrimSpace(v)
	negative := strings.HasPrefix(v, "-")
	if negative {
		v = v[1:]
		if len(v) == 0 || v[0] == '+' || cssSpace(rune(v[0])) {
			return cssLength{}, false
		}
	}
	l, ok := parseLength(v, true, false)
	if negative {
		if l.unit == lengthAuto {
			return cssLength{}, false
		}
		l.value = -l.value
	}
	return l, ok
}
func (l cssLength) offset(base int) int {
	value := l.value
	if l.unit == lengthPercent {
		if base < 0 {
			return 0
		}
		value = value * float64(base) / 100
	}
	return int(max(-16384, min(16384, value+math.Copysign(1e-9, value))))
}
func relativeOffset(s style, width, height int) image.Point {
	var p image.Point
	if s.inset[3].definite(width) {
		p.X = s.inset[3].offset(width)
	} else if s.inset[1].definite(width) {
		p.X = -s.inset[1].offset(width)
	}
	if s.inset[0].definite(height) {
		p.Y = s.inset[0].offset(height)
	} else if s.inset[2].definite(height) {
		p.Y = -s.inset[2].offset(height)
	}
	return p
}
func (l *layout) queueAbsolute(n *node, s style, x, y int) {
	if l.err != nil {
		return
	}
	if len(l.absolute) >= l.d.limits.Nodes {
		l.err = limitError("positioned boxes")
		return
	}
	index := len(l.ops)
	l.add(Box{Tag: "position-anchor", Bounds: image.Rect(x, y, x, y)}, nil)
	l.absolute = append(l.absolute, absoluteBox{n, s, l.frame, index})
}
func (f *positionFrame) bounds(l *layout) image.Rectangle {
	if f.op < 0 {
		return f.initial
	}
	return l.ops[f.op].box.Bounds.Inset(f.border)
}
func (l *layout) placeAbsolute(job absoluteBox) {
	if l.err != nil {
		return
	}
	cb := job.scope.bounds(l)
	s := job.style
	width, height := cb.Dx(), cb.Dy()
	// Percentages in absolute box sizing/spacing use the containing padding box,
	// independently of the remaining width after offsets are applied.
	for i := range s.margin {
		if s.margin[i].unit == lengthPercent {
			s.margin[i] = px(s.margin[i].resolve(width, 256))
		}
		if s.padding[i].unit == lengthPercent {
			s.padding[i] = px(s.padding[i].resolve(width, 256))
		}
	}
	for _, p := range []*cssLength{&s.width, &s.minWidth, &s.maxWidth} {
		if p.unit == lengthPercent {
			*p = px(p.resolve(width, 4096))
		}
	}
	for _, p := range []*cssLength{&s.height, &s.minHeight, &s.maxHeight} {
		if p.unit == lengthPercent {
			*p = px(p.resolve(height, l.d.limits.Height))
		}
	}
	margin, padding := resolveEdges(s.margin, width), resolveEdges(s.padding, width)
	left, right := s.inset[3].definite(width), s.inset[1].definite(width)
	top, bottom := s.inset[0].definite(height), s.inset[2].definite(height)
	available := width
	if left {
		available -= s.inset[3].offset(width)
	}
	if right {
		available -= s.inset[1].offset(width)
	}
	extra := padding[1] + padding[3] + 2*s.bw
	if !s.width.definite(width) {
		outer := max(0, available-margin[1]-margin[3])
		if !(left && right) {
			minimum, preferred := l.preferredCell(job.node, s, width)
			outer = max(0, min(max(minimum, available), preferred)-margin[1]-margin[3])
		}
		if s.borderBox {
			s.width = px(outer)
		} else {
			s.width = px(max(0, outer-extra))
		}
	}
	if !s.height.definite(height) && top && bottom {
		outer := max(0, height-s.inset[0].offset(height)-s.inset[2].offset(height)-margin[0]-margin[2])
		if s.borderBox {
			s.height = px(outer)
		} else {
			s.height = px(max(0, outer-padding[0]-padding[2]-2*s.bw))
		}
	}
	outerWidth := s.width.resolve(width, 4096)
	if !s.borderBox {
		outerWidth += extra
	}
	if s.minWidth.definite(width) {
		floor := s.minWidth.resolve(width, 4096)
		if !s.borderBox {
			floor += extra
		}
		outerWidth = max(outerWidth, floor)
	}
	layoutWidth := min(4096, max(1, outerWidth+margin[1]+margin[3]))
	if left && right {
		layoutWidth = min(4096, max(layoutWidth, available))
	}
	anchor := l.ops[job.anchor].box.Bounds.Min
	x, y := anchor.X, anchor.Y
	if left {
		x = cb.Min.X + s.inset[3].offset(width)
	}
	if top {
		y = cb.Min.Y + s.inset[0].offset(height)
	}
	first := len(l.ops)
	previous := l.frame
	l.frame = job.scope
	l.blockStyled(job.node, x, y, layoutWidth, height, s)
	l.frame = previous
	if l.err != nil || first >= len(l.ops) {
		return
	}
	bounds := l.ops[first].box.Bounds
	shift := image.Point{}
	if !left && right {
		shift.X = cb.Max.X - s.inset[1].offset(width) - margin[1] - bounds.Max.X
	}
	if !top && bottom {
		shift.Y = cb.Max.Y - s.inset[2].offset(height) - margin[2] - bounds.Max.Y
	}
	for i := first; i < len(l.ops); i++ {
		l.ops[i].box.Bounds = l.ops[i].box.Bounds.Add(shift)
	}
}

func (l *layout) position(height int) {
	l.rootFrame.initial.Max.Y = max(1, height)
	for i := 0; i < len(l.absolute) && l.err == nil; i++ {
		l.placeAbsolute(l.absolute[i])
	}
	if l.err != nil || len(l.rootFrame.children) == 0 {
		return
	}
	buckets := make(map[*positionFrame][]operation)
	for _, op := range l.ops {
		buckets[op.owner] = append(buckets[op.owner], op)
	}
	output := make([]operation, 0, len(l.ops))
	var emit func(*positionFrame, image.Point)
	emit = func(frame *positionFrame, inherited image.Point) {
		shift := inherited.Add(frame.shift)
		for _, op := range buckets[frame] {
			if op.box.Tag == "position-anchor" {
				continue
			}
			if err := l.ctx.Err(); err != nil {
				l.err = err
				return
			}
			op.box.Bounds = op.box.Bounds.Add(shift)
			if op.box.Bounds.Max.Y > l.d.limits.Height || op.box.Bounds.Min.Y < -l.d.limits.Height {
				l.err = limitError("positioned height")
				return
			}
			output = append(output, op)
		}
		sort.SliceStable(frame.children, func(i, j int) bool { return frame.children[i].node.id < frame.children[j].node.id })
		for _, child := range frame.children {
			emit(child, shift)
			if l.err != nil {
				return
			}
		}
	}
	emit(l.rootFrame, image.Point{})
	if l.err == nil {
		l.ops = output
	}
}
