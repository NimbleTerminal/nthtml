package document

import (
	"image"
	"math"
	"strconv"
	"strings"
)

const maxGridTracks = 16

type gridTrack struct {
	minLen     cssLength
	maxLen     cssLength
	fr         float64
	autoRepeat bool
	autoFit    bool
}

type gridItem struct {
	node       *node
	cs         style
	margin     [4]int
	colStart   int
	colSpan    int
	opStart    int
	opEnd      int
	innerH     int
	finalW     int
	itemX      int
}

func parseGridFr(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.HasSuffix(s, "fr") {
		return 0, false
	}
	s = strings.TrimSpace(strings.TrimSuffix(s, "fr"))
	if len(s) > 64 || !cssNumber.MatchString(s) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1000 {
		return 0, false
	}
	return v, true
}

func parseSingleGridTrack(tok string) (gridTrack, bool) {
	tok = strings.ToLower(strings.TrimSpace(tok))
	switch tok {
	case "auto", "min-content", "max-content":
		return gridTrack{minLen: cssLength{unit: lengthAuto}, maxLen: cssLength{unit: lengthAuto}}, true
	}
	if fr, ok := parseGridFr(tok); ok {
		return gridTrack{fr: fr}, true
	}
	if strings.HasPrefix(tok, "fit-content(") && strings.HasSuffix(tok, ")") {
		inner := strings.TrimSpace(tok[len("fit-content(") : len(tok)-1])
		l, ok := parseLength(inner, false, false)
		if !ok {
			return gridTrack{}, false
		}
		return gridTrack{minLen: cssLength{unit: lengthAuto}, maxLen: l}, true
	}
	if strings.HasPrefix(tok, "minmax(") && strings.HasSuffix(tok, ")") {
		inner := strings.TrimSpace(tok[len("minmax(") : len(tok)-1])
		parts, ok := splitSelectorList(inner)
		if !ok || len(parts) != 2 {
			return gridTrack{}, false
		}
		minStr := strings.TrimSpace(parts[0])
		maxStr := strings.TrimSpace(parts[1])
		var tr gridTrack
		switch minStr {
		case "auto", "min-content", "max-content":
			tr.minLen = cssLength{unit: lengthAuto}
		default:
			l, ok := parseLength(minStr, false, false)
			if !ok {
				return gridTrack{}, false
			}
			tr.minLen = l
		}
		if fr, ok := parseGridFr(maxStr); ok {
			tr.fr = fr
			return tr, true
		}
		switch maxStr {
		case "auto", "min-content", "max-content":
			tr.maxLen = cssLength{unit: lengthAuto}
		default:
			l, ok := parseLength(maxStr, false, false)
			if !ok {
				return gridTrack{}, false
			}
			tr.maxLen = l
		}
		return tr, true
	}
	l, ok := parseLength(tok, false, false)
	if !ok {
		return gridTrack{}, false
	}
	return gridTrack{minLen: l, maxLen: l}, true
}

func parseGridTemplateColumns(v string) ([]gridTrack, bool) {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "none") {
		return nil, true
	}
	tokens, ok := cssTokens(v)
	if !ok || len(tokens) == 0 {
		return nil, false
	}
	var out []gridTrack
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]") {
			continue
		}
		lower := strings.ToLower(tok)
		if strings.HasPrefix(lower, "repeat(") && strings.HasSuffix(lower, ")") {
			inner := strings.TrimSpace(tok[len("repeat(") : len(tok)-1])
			parts, ok := splitSelectorList(inner)
			if !ok || len(parts) != 2 {
				return nil, false
			}
			countStr := strings.ToLower(strings.TrimSpace(parts[0]))
			subToks, ok := cssTokens(parts[1])
			if !ok || len(subToks) == 0 {
				return nil, false
			}
			var subTracks []gridTrack
			for _, st := range subToks {
				if strings.HasPrefix(st, "[") && strings.HasSuffix(st, "]") {
					continue
				}
				tr, ok := parseSingleGridTrack(st)
				if !ok {
					return nil, false
				}
				subTracks = append(subTracks, tr)
			}
			if len(subTracks) == 0 {
				return nil, false
			}
			if countStr == "auto-fill" || countStr == "auto-fit" {
				for i := range subTracks {
					subTracks[i].autoRepeat = true
					subTracks[i].autoFit = countStr == "auto-fit"
				}
				if len(out)+len(subTracks) > maxGridTracks {
					return nil, false
				}
				out = append(out, subTracks...)
				continue
			}
			count, err := strconv.Atoi(countStr)
			if err != nil || count < 1 || count > maxGridTracks || len(out)+count*len(subTracks) > maxGridTracks {
				return nil, false
			}
			for c := 0; c < count; c++ {
				out = append(out, subTracks...)
			}
			continue
		}
		tr, ok := parseSingleGridTrack(tok)
		if !ok || len(out)+1 > maxGridTracks {
			return nil, false
		}
		out = append(out, tr)
	}
	return out, len(out) > 0
}

func parseGridLineOrSpan(s string) (line int, span int, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "auto" {
		return 0, 0, true
	}
	fields := strings.Fields(s)
	if len(fields) == 2 && fields[0] == "span" {
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 1 || n > maxGridTracks {
			return 0, 0, false
		}
		return 0, n, true
	}
	if len(fields) == 2 && fields[1] == "span" {
		n, err := strconv.Atoi(fields[0])
		if err != nil || n < 1 || n > maxGridTracks {
			return 0, 0, false
		}
		return 0, n, true
	}
	if len(fields) == 1 {
		n, err := strconv.Atoi(fields[0])
		if err != nil || n == 0 || n < -maxGridTracks-1 || n > maxGridTracks+1 {
			return 0, 0, false
		}
		return n, 0, true
	}
	return 0, 0, false
}

func parseGridColumnShorthand(v string) (startVal, endVal string, ok bool) {
	if a, b, hasSlash := strings.Cut(v, "/"); hasSlash {
		a = strings.TrimSpace(a)
		b = strings.TrimSpace(b)
		if _, _, ok1 := parseGridLineOrSpan(a); !ok1 {
			return "", "", false
		}
		if _, _, ok2 := parseGridLineOrSpan(b); !ok2 {
			return "", "", false
		}
		return a, b, true
	}
	v = strings.TrimSpace(v)
	if _, _, ok := parseGridLineOrSpan(v); !ok {
		return "", "", false
	}
	return v, "auto", true
}

func resolveGridItemSpan(cs style, numCols int) (startCol int, span int) {
	span = 1
	startCol = -1
	sLine, sSpan := cs.gridColStart, cs.gridColSpan
	eLine, eSpan := cs.gridColEnd, cs.gridColEndSpan

	resolveLine := func(line int) int {
		if line > 0 {
			return min(numCols, max(0, line-1))
		}
		if line < 0 {
			return max(0, min(numCols, numCols+1+line))
		}
		return -1
	}

	if sSpan > 0 {
		span = min(numCols, max(1, sSpan))
	} else if sLine != 0 {
		startCol = min(numCols-1, max(0, resolveLine(sLine)))
	}
	if eSpan > 0 {
		span = min(numCols, max(1, eSpan))
	} else if eLine != 0 {
		endIdx := resolveLine(eLine)
		if startCol >= 0 && endIdx > startCol {
			span = min(numCols-startCol, endIdx-startCol)
		} else if startCol < 0 && endIdx > 0 {
			startCol = 0
			span = min(numCols, endIdx)
		}
	}
	if startCol >= 0 && startCol+span > numCols {
		span = max(1, numCols-startCol)
	}
	return startCol, max(1, min(numCols, span))
}

func resolveGridColumns(tracks []gridTrack, cw, cGap, itemCount int) []int {
	if len(tracks) == 0 {
		return []int{max(1, cw)}
	}
	expanded := make([]gridTrack, 0, maxGridTracks)
	for _, tr := range tracks {
		if tr.autoRepeat {
			minW := 100
			if tr.minLen.definite(cw) {
				minW = max(1, tr.minLen.resolve(cw, cw))
			} else if tr.maxLen.definite(cw) {
				minW = max(1, tr.maxLen.resolve(cw, cw))
			}
			avail := max(1, cw-len(expanded)*(minW+cGap))
			n := max(1, (avail+cGap)/max(1, minW+cGap))
			if tr.autoFit && itemCount > 0 && itemCount < n {
				n = itemCount
			}
			for i := 0; i < n && len(expanded) < maxGridTracks; i++ {
				cp := tr
				cp.autoRepeat = false
				if cp.fr == 0 && !cp.maxLen.definite(cw) {
					cp.fr = 1
				}
				expanded = append(expanded, cp)
			}
			continue
		}
		if len(expanded) < maxGridTracks {
			expanded = append(expanded, tr)
		}
	}
	numCols := len(expanded)
	if numCols == 0 {
		return []int{max(1, cw)}
	}
	widths := make([]int, numCols)
	minWidths := make([]int, numCols)
	totalFixed := cGap * (numCols - 1)
	var totalFr float64
	for i := range expanded {
		tr := &expanded[i]
		if tr.minLen.definite(cw) {
			minWidths[i] = tr.minLen.resolve(cw, cw)
		}
		if tr.fr == 0 && tr.minLen.unit == lengthAuto && tr.maxLen.unit == lengthAuto {
			tr.fr = 1
		}
		if tr.fr > 0 {
			widths[i] = minWidths[i]
			totalFr += tr.fr
		} else if tr.maxLen.definite(cw) {
			widths[i] = max(minWidths[i], tr.maxLen.resolve(cw, cw))
			totalFixed += widths[i]
		} else {
			widths[i] = max(1, minWidths[i])
			totalFixed += widths[i]
		}
	}
	free := cw - totalFixed
	if free > 0 && totalFr > 0 {
		// Respect minWidths on fr tracks.
		rem := free
		acc := 0.0
		prev := 0
		for i, tr := range expanded {
			if tr.fr <= 0 {
				continue
			}
			acc += tr.fr
			next := int(float64(free)*acc/totalFr + 0.5)
			share := max(minWidths[i], next-prev)
			widths[i] = max(1, min(rem, share))
			prev = next
			rem = max(0, rem-widths[i])
		}
	} else {
		for i := range widths {
			widths[i] = max(1, widths[i])
		}
	}
	return widths
}

func (l *layout) grid(n *node, cx, cy, cw, height int, s style) int {
	if !l.flexStep() {
		return cy
	}
	var rawItems []*gridItem
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
			rawItems = append(rawItems, &gridItem{node: anon, cs: cs})
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
		rawItems = append(rawItems, &gridItem{node: c, cs: cs})
	}
	if len(rawItems) == 0 {
		return cy
	}

	rGap := s.rowGap.resolve(max(0, height), 256)
	cGap := s.colGap.resolve(cw, 256)
	colWidths := resolveGridColumns(s.gridCols, cw, cGap, len(rawItems))
	numCols := len(colWidths)
	colX := make([]int, numCols)
	accX := cx
	for i, w := range colWidths {
		colX[i] = accX
		accX += w + cGap
	}

	var rows [][]*gridItem
	var curRow []*gridItem
	colCursor := 0
	for _, it := range rawItems {
		startCol, span := resolveGridItemSpan(it.cs, numCols)
		if startCol >= 0 {
			if startCol < colCursor && len(curRow) > 0 {
				rows = append(rows, curRow)
				curRow = nil
			}
			colCursor = startCol
		} else if colCursor+span > numCols && len(curRow) > 0 {
			rows = append(rows, curRow)
			curRow = nil
			colCursor = 0
		}
		if colCursor+span > numCols {
			span = max(1, numCols-colCursor)
		}
		it.colStart = colCursor
		it.colSpan = span
		curRow = append(curRow, it)
		colCursor += span
		if colCursor >= numCols {
			rows = append(rows, curRow)
			curRow = nil
			colCursor = 0
		}
	}
	if len(curRow) > 0 {
		rows = append(rows, curRow)
	}

	rowY := cy
	for ri, row := range rows {
		if ri > 0 {
			rowY += rGap
		}
		rowH := 0
		for _, it := range row {
			if !l.flexStep() {
				return rowY
			}
			slotW := 0
			for c := it.colStart; c < it.colStart+it.colSpan && c < numCols; c++ {
				slotW += colWidths[c]
			}
			slotW += cGap * (it.colSpan - 1)
			it.margin = resolveEdges(it.cs.margin, slotW)
			padding := resolveEdges(it.cs.padding, slotW)
			bH := it.cs.borderH()
			itemW := max(1, slotW-it.margin[1]-it.margin[3])
			if it.cs.width.definite(slotW) {
				bw := it.cs.width.resolve(slotW, 16384)
				if !it.cs.borderBox {
					bw += padding[1] + padding[3] + bH
				}
				itemW = min(itemW, max(1, bw))
			}
			if it.cs.maxWidth.definite(slotW) {
				mw := it.cs.maxWidth.resolve(slotW, 16384)
				if !it.cs.borderBox {
					mw += padding[1] + padding[3] + bH
				}
				itemW = min(itemW, max(1, mw))
			}
			if it.cs.minWidth.definite(slotW) {
				mw := it.cs.minWidth.resolve(slotW, 16384)
				if !it.cs.borderBox {
					mw += padding[1] + padding[3] + bH
				}
				itemW = max(itemW, mw)
			}
			it.finalW = itemW
			it.itemX = colX[it.colStart] + it.margin[3]
			itemStyle := it.cs
			itemStyle.margin = [4]cssLength{}
			itemStyle.width = px(itemW)
			itemStyle.borderBox = true
			it.opStart = len(l.ops)
			it.innerH = l.blockStyled(it.node, 0, 0, itemW, height, itemStyle)
			it.opEnd = len(l.ops)
			rowH = max(rowH, it.innerH+it.margin[0]+it.margin[2])
		}
		if len(rows) == 1 && height >= 0 {
			rowH = max(rowH, height)
		}
		for _, it := range row {
			outerH := it.innerH + it.margin[0] + it.margin[2]
			dy := it.margin[0]
			switch s.alignItems {
			case "center":
				dy = it.margin[0] + max(0, rowH-outerH)/2
			case "flex-end", "end":
				dy = it.margin[0] + max(0, rowH-outerH)
			case "", "stretch":
				if !it.cs.height.definite(height) && it.opStart < it.opEnd {
					targetH := max(it.innerH, rowH-it.margin[0]-it.margin[2])
					if targetH > it.innerH {
						l.ops[it.opStart].box.Bounds.Max.Y = targetH
					}
				}
			}
			shift := image.Pt(it.itemX, rowY+dy)
			for i := it.opStart; i < it.opEnd; i++ {
				l.ops[i].box.translate(shift)
			}
		}
		rowY += rowH
	}
	return rowY
}
