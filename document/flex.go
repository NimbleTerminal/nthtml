package document

import (
	"image"
	"strings"
)

type flexItem struct {
	node          *node
	cs            style
	margin        [4]int
	padding       [4]int
	minW, prefW   int
	baseW, finalW int
	opStart       int
	opEnd         int
	innerH        int
}

func (l *layout) flexStep() bool {
	if l.err != nil {
		return false
	}
	if err := l.ctx.Err(); err != nil {
		l.err = err
		return false
	}
	l.flexWork++
	if l.flexWork > 100000 {
		l.err = limitError("flexbox layout")
		return false
	}
	return true
}

func (l *layout) flex(n *node, cx, cy, cw, height int, s style) int {
	if !l.flexStep() {
		return cy
	}
	var items []*flexItem
	for _, c := range n.children {
		if !l.flexStep() {
			return cy
		}
		if c.tag == "" {
			if strings.TrimSpace(c.text) == "" {
				continue
			}
			anon := &node{parent: n, tag: "span", children: []*node{c}}
			cs := l.d.styled(anon, s)
			cs.display = "block"
			items = append(items, &flexItem{node: anon, cs: cs})
			continue
		}
		cs := l.d.styled(c, s)
		if cs.display == "none" {
			continue
		}
		if cs.position == "absolute" {
			l.queueAbsolute(c, cs, cx, cy)
			continue
		}
		cs.display = "block"
		cs.floatSide = "none"
		items = append(items, &flexItem{node: c, cs: cs})
	}
	if len(items) == 0 {
		return cy
	}

	isColumn := s.flexDirection == "column" || s.flexDirection == "column-reverse"
	isReverse := s.flexDirection == "row-reverse" || s.flexDirection == "column-reverse"
	rGap := s.rowGap.resolve(max(0, height), 256)
	cGap := s.colGap.resolve(cw, 256)

	if isColumn {
		return l.flexColumn(items, cx, cy, cw, height, rGap, isReverse, s)
	}
	return l.flexRow(items, cx, cy, cw, height, rGap, cGap, isReverse, s)
}

func (l *layout) flexRow(items []*flexItem, cx, cy, cw, height, rGap, cGap int, isReverse bool, s style) int {
	for _, it := range items {
		if !l.flexStep() {
			return cy
		}
		it.margin = resolveEdges(it.cs.margin, cw)
		it.padding = resolveEdges(it.cs.padding, cw)
		bH := it.cs.borderH()
		it.minW, it.prefW = l.preferredCell(it.node, it.cs, cw)
		if l.err != nil {
			return cy
		}
		baseW := min(cw, it.prefW)
		if it.cs.flexBasis.definite(cw) {
			bw := it.cs.flexBasis.resolve(cw, 16384)
			if !it.cs.borderBox {
				bw += it.padding[1] + it.padding[3] + bH
			}
			baseW = bw
		} else if it.cs.width.definite(cw) {
			bw := it.cs.width.resolve(cw, 16384)
			if !it.cs.borderBox {
				bw += it.padding[1] + it.padding[3] + bH
			}
			baseW = bw
		}
		if it.cs.maxWidth.definite(cw) {
			mw := it.cs.maxWidth.resolve(cw, 16384)
			if !it.cs.borderBox {
				mw += it.padding[1] + it.padding[3] + bH
			}
			baseW = min(baseW, mw)
		}
		if it.cs.minWidth.definite(cw) {
			mw := it.cs.minWidth.resolve(cw, 16384)
			if !it.cs.borderBox {
				mw += it.padding[1] + it.padding[3] + bH
			}
			baseW = max(baseW, mw)
		}
		it.baseW = max(1, baseW)
		it.finalW = it.baseW
	}

	var lines [][]*flexItem
	var curLine []*flexItem
	curUsed := 0
	for _, it := range items {
		outer := it.baseW + it.margin[1] + it.margin[3]
		gap := 0
		if len(curLine) > 0 {
			gap = cGap
		}
		if s.flexWrap == "wrap" && len(curLine) > 0 && curUsed+gap+outer > cw {
			lines = append(lines, curLine)
			curLine = []*flexItem{it}
			curUsed = outer
		} else {
			curLine = append(curLine, it)
			curUsed += gap + outer
		}
	}
	if len(curLine) > 0 {
		lines = append(lines, curLine)
	}

	lineY := cy
	for li, line := range lines {
		if li > 0 {
			lineY += rGap
		}
		if isReverse {
			for i, j := 0, len(line)-1; i < j; i, j = i+1, j-1 {
				line[i], line[j] = line[j], line[i]
			}
		}
		used := cGap * (len(line) - 1)
		var totalGrow, totalShrink float64
		for _, it := range line {
			used += it.baseW + it.margin[1] + it.margin[3]
			totalGrow += it.cs.flexGrow
			totalShrink += it.cs.flexShrink * float64(it.baseW)
		}
		free := cw - used
		if free > 0 && totalGrow > 0 {
			rem := free
			acc := 0.0
			prev := 0
			for _, it := range line {
				acc += it.cs.flexGrow
				next := int(float64(free)*acc/totalGrow + 0.5)
				add := min(rem, max(0, next-prev))
				it.finalW = it.baseW + add
				prev = next
				rem -= add
			}
		} else if free < 0 && totalShrink > 0 {
			deficit := -free
			acc := 0.0
			prev := 0
			for _, it := range line {
				acc += it.cs.flexShrink * float64(it.baseW)
				next := int(float64(deficit)*acc/totalShrink + 0.5)
				sub := next - prev
				prev = next
				minAllowed := max(1, min(it.baseW, it.minW))
				it.finalW = max(minAllowed, it.baseW-sub)
			}
		}

		lineH := 0
		for _, it := range line {
			if !l.flexStep() {
				return lineY
			}
			itemStyle := it.cs
			itemStyle.margin = [4]cssLength{}
			itemStyle.width = px(it.finalW)
			itemStyle.borderBox = true
			it.opStart = len(l.ops)
			it.innerH = l.blockStyled(it.node, 0, 0, max(1, it.finalW), height, itemStyle)
			it.opEnd = len(l.ops)
			lineH = max(lineH, it.innerH+it.margin[0]+it.margin[2])
		}
		if len(lines) == 1 && height >= 0 {
			lineH = max(lineH, height)
		}

		actualUsed := cGap * (len(line) - 1)
		for _, it := range line {
			actualUsed += it.finalW + it.margin[1] + it.margin[3]
		}
		spare := max(0, cw-actualUsed)
		startOffset, extraGap := justifyOffsets(s.justifyContent, spare, len(line))

		curX := cx + startOffset
		for idx, it := range line {
			if idx > 0 {
				curX += cGap + extraGap
			}
			outerH := it.innerH + it.margin[0] + it.margin[2]
			dy := it.margin[0]
			switch s.alignItems {
			case "center":
				dy = it.margin[0] + max(0, lineH-outerH)/2
			case "flex-end", "end":
				dy = it.margin[0] + max(0, lineH-outerH)
			case "", "stretch":
				if !it.cs.height.definite(height) && it.opStart < it.opEnd {
					targetH := max(it.innerH, lineH-it.margin[0]-it.margin[2])
					if targetH > it.innerH {
						l.ops[it.opStart].box.Bounds.Max.Y = targetH
					}
				}
			}
			shift := image.Pt(curX+it.margin[3], lineY+dy)
			for i := it.opStart; i < it.opEnd; i++ {
				l.ops[i].box.translate(shift)
			}
			curX += it.margin[3] + it.finalW + it.margin[1]
		}
		lineY += lineH
	}
	return lineY
}

func (l *layout) flexColumn(items []*flexItem, cx, cy, cw, height, rGap int, isReverse bool, s style) int {
	if isReverse {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
	totalH := rGap * (len(items) - 1)
	var totalGrow float64
	for _, it := range items {
		if !l.flexStep() {
			return cy
		}
		it.margin = resolveEdges(it.cs.margin, cw)
		it.padding = resolveEdges(it.cs.padding, cw)
		bH := it.cs.borderH()
		_, it.prefW = l.preferredCell(it.node, it.cs, cw)
		itemW := max(1, cw-it.margin[1]-it.margin[3])
		if it.cs.width.definite(cw) {
			bw := it.cs.width.resolve(cw, 16384)
			if !it.cs.borderBox {
				bw += it.padding[1] + it.padding[3] + bH
			}
			itemW = min(itemW, max(1, bw))
		} else if s.alignItems != "" && s.alignItems != "stretch" {
			itemW = min(itemW, max(1, it.prefW))
		}
		it.finalW = itemW
		itemStyle := it.cs
		itemStyle.margin = [4]cssLength{}
		itemStyle.width = px(itemW)
		itemStyle.borderBox = true
		if it.cs.flexBasis.definite(height) {
			itemStyle.height = it.cs.flexBasis
		}
		it.opStart = len(l.ops)
		it.innerH = l.blockStyled(it.node, 0, 0, itemW, height, itemStyle)
		it.opEnd = len(l.ops)
		totalH += it.innerH + it.margin[0] + it.margin[2]
		totalGrow += it.cs.flexGrow
	}

	spareH := 0
	if height > totalH {
		spareH = height - totalH
	}
	if spareH > 0 && totalGrow > 0 {
		acc := 0.0
		prev := 0
		for _, it := range items {
			acc += it.cs.flexGrow
			next := int(float64(spareH)*acc/totalGrow + 0.5)
			add := max(0, next-prev)
			prev = next
			if add > 0 {
				it.innerH += add
				if it.opStart < it.opEnd {
					l.ops[it.opStart].box.Bounds.Max.Y = it.innerH
				}
			}
		}
		spareH = 0
	}

	startOffset, extraGap := justifyOffsets(s.justifyContent, spareH, len(items))
	curY := cy + startOffset
	for idx, it := range items {
		if idx > 0 {
			curY += rGap + extraGap
		}
		outerW := it.finalW + it.margin[1] + it.margin[3]
		dx := it.margin[3]
		switch s.alignItems {
		case "center":
			dx = it.margin[3] + max(0, cw-outerW)/2
		case "flex-end", "end":
			dx = it.margin[3] + max(0, cw-outerW)
		}
		shift := image.Pt(cx+dx, curY+it.margin[0])
		for i := it.opStart; i < it.opEnd; i++ {
			l.ops[i].box.translate(shift)
		}
		curY += it.margin[0] + it.innerH + it.margin[2]
	}
	return curY
}

func justifyOffsets(mode string, spare, count int) (int, int) {
	if spare <= 0 || count <= 0 {
		return 0, 0
	}
	switch mode {
	case "flex-end", "end":
		return spare, 0
	case "center":
		return spare / 2, 0
	case "space-between":
		if count > 1 {
			return 0, spare / (count - 1)
		}
	case "space-around":
		step := spare / count
		return step / 2, step
	case "space-evenly":
		step := spare / (count + 1)
		return step, step
	}
	return 0, 0
}
