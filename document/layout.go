package document

import (
	"context"
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Box describes a painted rectangle or text run in document pixel coordinates.
// Text and bounds can also be consumed by a host's text renderer (e.g. nttypeset).
type Box struct {
	Bounds, Clip image.Rectangle
	// Baseline is the font baseline offset from Bounds.Min.Y. InkBounds encloses visible glyph overflow.
	Baseline, WordSpacing, LetterSpacing int
	InkBounds                            image.Rectangle
	Tag, Text                            string
	Anchor, TextPrefix                   string
	FontSize                             int
	Bold, SmallCaps                      bool
	FontFamily                           string
	Italic, Underline, Strike            bool
	Monospace                            bool
	LinkID                               int
	Color                                color.RGBA
}
type Result struct {
	// Warnings describe diagrams replaced by readable fallbacks.
	Warnings []string
	Anchors  map[string]image.Point
	document *Document
	// Image is the whole page, unless Options.NoImage left it to Paint.
	Image  *image.RGBA
	Boxes  []Box
	Links  []Link
	ops    []operation
	opts   Options
	limits Limits
	size   image.Point
}
type operation struct {
	owner  *positionFrame
	box    Box
	source image.Image
}
type layout struct {
	floats                      *floatArea
	floatWork, flexWork         int
	frame, rootFrame            *positionFrame
	absolute                    []absoluteBox
	diagramCount, diagramPixels int
	warnings                    []string
	tableWork                   int
	marginWork                  int
	ctx                         context.Context
	d                           *Document
	opts                        Options
	lim                         Limits
	maxHeight                   int
	ops                         []operation
	faces                       map[faceKey]font.Face
	fonts                       map[string]*opentype.Font
	fontBytes                   int
	err                         error
	// scale is device pixels per CSS pixel for the faces drawn; zero is one.
	scale float64
}

func (l *layout) pageHeightLimit() int {
	if l.maxHeight > 0 {
		return l.maxHeight
	}
	return l.d.limits.Height
}

func (l *layout) add(b Box, src image.Image) {
	if l.err != nil {
		return
	}
	if e := l.ctx.Err(); e != nil {
		l.err = e
		return
	}
	if b.Text != "" && b.FontSize > 0 {
		f := l.face(style{size: b.FontSize, bold: b.Bold, mono: b.Monospace, family: b.FontFamily, italic: b.Italic})
		b.InkBounds = textInk(f, b)
	}
	if b.paintBottom() > l.pageHeightLimit() {
		l.err = limitError("height")
		return
	}
	if len(l.ops) >= max(250000, l.d.limits.Nodes*25) {
		l.err = limitError("paint operations")
		return
	}
	l.ops = append(l.ops, operation{box: b, source: src, owner: l.frame})
}
func (l *layout) rect(r image.Rectangle, c color.RGBA, tag string) {
	if c.A > 0 && !r.Empty() {
		l.add(Box{Bounds: r, Color: c, Tag: tag}, nil)
	}
}

func (d *Document) Render(ctx context.Context, o Options) (*Result, error) {
	if d == nil || d.root == nil {
		return nil, limitError("nil document")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if o.Width == 0 {
		o.Width = 800
	}
	if o.Width < 32 || o.Width > 4096 {
		return nil, limitError("width")
	}
	lim := d.limits
	if o.Limits != (Limits{}) {
		ol, err := limits(o.Limits)
		if err != nil {
			return nil, err
		}
		if o.Limits.Pixels != 0 {
			lim.Pixels = ol.Pixels
		}
		if o.Limits.Height != 0 {
			lim.Height = ol.Height
		}
		if o.Limits.LayoutHeight != 0 {
			lim.LayoutHeight = ol.LayoutHeight
		}
	}
	maxH := lim.Height
	if o.NoImage {
		maxH = max(lim.LayoutHeight, lim.Height)
	}
	l := &layout{ctx: ctx, d: d, opts: o, lim: lim, maxHeight: maxH, faces: map[faceKey]font.Face{}}
	l.fonts = map[string]*opentype.Font{}
	l.rootFrame = &positionFrame{containing: true, op: -1, initial: image.Rect(0, 0, o.Width, 0)}
	l.frame = l.rootFrame
	defer func() {
		for _, f := range l.faces {
			f.Close()
		}
	}()
	fg := color.RGBA{0, 0, 0, 255}
	var linkColor color.RGBA
	if !d.hasAuthorColors {
		if o.Colors.Text.A > 0 {
			fg = o.Colors.Text
		}
		linkColor = o.Colors.Link
	}
	h := l.block(d.root, 0, 0, o.Width, -1, style{rootSize: 16, size: 16, line: 22, spacing: [2]int{2, 2}, fg: fg, defaultFg: fg, defaultLink: linkColor})
	if l.err != nil {
		return nil, l.err
	}
	l.position(h)
	if l.err != nil {
		return nil, l.err
	}
	// Visible overflow contributes to the exported canvas, not normal flow.
	for _, op := range l.ops {
		h = max(h, op.box.paintBottom())
	}
	h = max(1, h)
	if h > maxH {
		return nil, limitError("height")
	}
	if !o.NoImage && o.Width > lim.Pixels/h {
		return nil, limitError("pixels")
	}
	page := image.Rect(0, 0, o.Width, h)
	result := &Result{Anchors: map[string]image.Point{}, Warnings: l.warnings, document: d, Boxes: make([]Box, 0, len(l.ops)), ops: l.ops, opts: o, limits: lim, size: page.Max}
	links, err := d.Links(ctx)
	if err != nil {
		return nil, err
	}
	linkIndex := map[int]int{}
	for i, link := range links {
		linkIndex[link.ID] = i
	}
	for _, op := range l.ops {
		b := op.box
		result.Boxes = append(result.Boxes, b)
		if b.Anchor != "" {
			if _, exists := result.Anchors[b.Anchor]; !exists {
				result.Anchors[b.Anchor] = b.Bounds.Min
			}
		}
		if i, ok := linkIndex[b.LinkID]; ok && b.LinkID != 0 {
			region := b.Bounds
			if region.Empty() {
				region = b.InkBounds
			}
			region = region.Intersect(page)
			if !region.Empty() {
				appendLinkRegion(&links[i], region)
			}
		}
	}
	if !o.NoImage {
		result.Image = image.NewRGBA(page)
		if err := l.paint(result.Image, l.ops, page, 1); err != nil {
			return nil, err
		}
	}
	for _, link := range links {
		if len(link.Regions) > 0 {
			result.Links = append(result.Links, link)
		}
	}
	if l.err != nil {
		return nil, l.err
	}
	return result, nil
}

func (l *layout) block(n *node, x, y, w, parentHeight int, parent style) int {
	if l.err != nil {
		return y
	}
	if e := l.ctx.Err(); e != nil {
		l.err = e
		return y
	}
	return l.blockStyled(n, x, y, w, parentHeight, l.d.styled(n, parent))
}

func (l *layout) blockStyled(n *node, x, y, w, parentHeight int, s style) int {
	if l.err != nil {
		return y
	}
	if s.display == "none" {
		return y
	}

	previousFloats := l.floats
	containsFloats := floatContext(n, s)
	if containsFloats {
		l.floats = &floatArea{l: l}
		defer func() { l.floats = previousFloats }()
	}
	basisWidth := w
	tableStyle := s
	if n.tag == "table" && s.collapse {
		s.bw = [4]int{}
	}
	profile := l.margins(n, s, w, parentHeight)
	margin, padding, target := boxWidth(s, w)
	margin[0], margin[2] = profile.before, profile.after
	extra := padding[1] + padding[3] + s.borderH()
	x += margin[3]
	y += margin[0]
	w = target
	height := -1
	if s.height.definite(parentHeight) {
		height = s.height.resolve(parentHeight, l.d.limits.Height)
		if s.borderBox {
			height = max(0, height-padding[0]-padding[2]-s.borderV())
		}
	}
	kind, _ := diagramSource(n)
	constrainHeight := kind == "" && n.tag != "img" && n.tag != "table" && n.tag != "tr" && n.tag != "td" && n.tag != "th" && n.tag != "thead" && n.tag != "tbody" && n.tag != "tfoot"
	clampHeight := func(h int) int {
		content := func(v cssLength) int {
			size := v.resolve(parentHeight, l.d.limits.Height)
			if s.borderBox {
				size = max(0, size-padding[0]-padding[2]-s.borderV())
			}
			return size
		}
		if s.maxHeight.definite(parentHeight) {
			h = min(h, content(s.maxHeight))
		}
		if s.minHeight.definite(parentHeight) {
			h = max(h, content(s.minHeight))
		}
		return h
	}
	if height >= 0 && constrainHeight {
		height = clampHeight(height)
	}
	start := y
	opStart := len(l.ops)
	l.add(Box{Tag: n.tag}, nil)
	if s.position == "relative" || s.position == "absolute" || s.floatSide == "left" || s.floatSide == "right" {
		previous := l.frame
		zIdx := 0
		if s.position == "relative" || s.position == "absolute" {
			zIdx = s.zIndex
		}
		frame := &positionFrame{parent: previous, node: n, op: opStart, zIndex: zIdx, border: s.bw, containing: s.position == "relative" || s.position == "absolute", floating: s.position != "relative" && s.position != "absolute"}
		if s.position == "relative" {
			frame.shift = relativeOffset(s, basisWidth, parentHeight)
		}
		previous.children = append(previous.children, frame)
		l.frame = frame
		if opStart < len(l.ops) {
			l.ops[opStart].owner = frame
		}
		defer func() { l.frame = previous }()
	}
	cx, cy, cw := x+padding[3]+s.bw[3], y+padding[0]+s.bw[0], max(1, w-extra)
	contentY := cy
	if kind, source := diagramSource(n); kind != "" {
		cy = l.diagram(n, kind, source, cx, cy, cw, height, s)
	} else if n.tag == "table" {
		minContent := 0
		if s.minWidth.definite(basisWidth) {
			minContent = s.minWidth.resolve(basisWidth, 16384)
			if s.borderBox {
				minContent = max(0, minContent-extra)
			}
		}
		cy, cw = l.table(n, cx, cy, cw, height, !tableStyle.width.definite(basisWidth), minContent, tableStyle)
		used := min(w, cw+extra)
		if !tableStyle.width.definite(basisWidth) && s.margin[3].unit == lengthAuto {
			spare := max(0, w-used)
			if s.margin[1].unit == lengthAuto {
				spare /= 2
			}
			x += spare
			for i := opStart + 1; i < len(l.ops); i++ {
				l.ops[i].box.translate(image.Pt(spare, 0))
			}
		}
		w = used
	} else if n.tag == "ul" || n.tag == "ol" {
		index := listStart(n)
		step := listStep(n)
		for _, child := range n.children {
			if child.tag != "li" {
				continue
			}
			cs := l.d.styled(child, s)
			if cs.display == "none" {
				continue
			}
			if n.tag == "ol" {
				index = listNumber(child.attrs["value"], index)
			}
			marker := formatListMarker(resolveListStyleType(n, child, cs), index)
			index += step
			indent := min(max(0, cw-1), max(24, font.MeasureString(l.face(s), marker+" ").Ceil()))
			markerY := cy
			firstOp := len(l.ops)
			cy = l.block(child, cx+indent, cy, max(1, cw-indent), height, s)
			for _, op := range l.ops[firstOp:] {
				if op.box.Tag == "text" {
					markerY = op.box.Bounds.Min.Y
					break
				}
			}
			if marker != "" {
				l.add(Box{Bounds: image.Rect(cx, markerY, cx+indent, markerY+s.line), Tag: "marker", Text: marker, Baseline: l.baseline(s), WordSpacing: s.wordSpacing, LetterSpacing: s.letterSpacing, FontSize: s.size, Bold: s.bold, SmallCaps: s.smallCaps, FontFamily: s.family, Italic: s.italic, Color: s.fg}, nil)
			}
			cy = max(cy, markerY+s.line)
		}
	} else if s.display == "flex" || s.display == "inline-flex" {
		cy = l.flex(n, cx, cy, cw, height, s)
	} else if n.tag == "img" {
		src := l.opts.Images[n.attrs["src"]]
		if src != nil && !src.Bounds().Empty() {
			iw, h := imageSize(n, src.Bounds().Size(), s.width.definite(cw), cw, height)
			h = min(h, l.d.limits.Height)
			l.add(Box{Bounds: image.Rect(cx, cy, cx+iw, cy+h), Tag: "img", LinkID: s.linkID}, src)
			cy += h
		} else {
			cy = l.flow([]*node{{text: "[image: " + n.attrs["alt"] + "]"}}, cx, cy, cw, s)
		}
	} else if n.tag == "tr" {
		cells := []*node{}
		for _, c := range n.children {
			if c.tag == "td" || c.tag == "th" {
				cells = append(cells, c)
			}
		}
		for i, c := range cells {
			left := cw * i / len(cells)
			right := cw * (i + 1) / len(cells)
			cy = max(cy, l.block(c, cx+left, contentY, max(1, right-left), height, s))
		}
	} else {
		var inline []*node
		pending, atTop := 0, true
		var pendingStrut marginStrut
		flush := func() {
			if len(inline) > 0 {
				before := cy
				flowStyle := s
				if atTop && len(n.firstLineDecl) > 0 {
					flowStyle = l.d.styled(&node{tag: "span", decl: n.firstLineDecl}, s)
				}
				cy = l.flow(inline, cx, cy, cw, flowStyle)
				if cy != before {
					pending = 0
					pendingStrut = marginStrut{}
					atTop = false
				}
				inline = nil
			}
		}
		for _, c := range n.children {
			cs := l.d.styled(c, s)
			if cs.display == "none" {
				continue
			}
			if cs.position == "absolute" {
				flush()
				l.queueAbsolute(c, cs, cx, cy)
				continue
			}
			if cs.floatSide == "left" || cs.floatSide == "right" {
				flush()
				l.floats.place(c, cs, cx, cy, cw, height)
				continue
			}
			kind, _ := diagramSource(c)
			if kind == "" && (c.tag == "" || cs.display == "inline" || cs.display == "inline-block" || cs.display == "inline-flex" || c.tag == "br") {
				inline = append(inline, c)
				continue
			}
			flush()
			childMargins := l.margins(c, cs, cw, height)
			collapsed := pendingStrut.join(childMargins.beforeStrut).value()
			input := cy - pending + collapsed - childMargins.before
			if atTop && profile.top {
				input = cy - childMargins.before
			}
			if l.floats != nil {
				input = max(input, l.floats.clear(input+childMargins.before, cs.clear)-childMargins.before)
			}
			cy = l.blockStyled(c, cx, input, cw, height, cs)
			if atTop && profile.top && childMargins.through {
				pending = 0
				pendingStrut = marginStrut{}
				continue
			}
			if childMargins.through {
				pendingStrut = pendingStrut.join(childMargins.afterStrut)
				pending = pendingStrut.value()
			} else {
				pendingStrut = childMargins.afterStrut
				pending = childMargins.after
				atTop = false
			}
		}
		flush()
		if profile.bottom {
			cy -= pending
		}
	}
	if containsFloats && len(l.floats.boxes) > 0 {
		cy = max(cy, l.floats.bottom())
	}
	// Definite block heights size the box even when content overflows.
	// Tables/cells retain their minimum-height rules; replaced content keeps
	// the size chosen by its image/diagram layout.
	if height >= 0 && constrainHeight {
		cy = contentY + height
	} else {
		cy = max(cy, contentY+max(0, height))
	}
	if constrainHeight {
		cy = contentY + clampHeight(cy-contentY)
	}
	cy += padding[2] + s.bw[2]
	if l.err != nil {
		return cy
	}
	if opStart < len(l.ops) {
		l.ops[opStart].box = Box{Bounds: image.Rect(x, start, x+w, cy), Tag: n.tag, Anchor: anchorName(n), Color: s.bg}
	}
	if s.overflow == "hidden" || s.overflow == "clip" {
		clip := image.Rect(x+s.bw[3], start+s.bw[0], x+w-s.bw[1], cy-s.bw[2])
		if clip.Empty() {
			clip = image.Rect(-1, -1, 0, 0)
		}
		for i := opStart + 1; i < len(l.ops); i++ {
			if l.ops[i].box.Clip.Empty() {
				l.ops[i].box.Clip = clip
			} else {
				inter := l.ops[i].box.Clip.Intersect(clip)
				if inter.Empty() {
					inter = image.Rect(-1, -1, 0, 0)
				}
				l.ops[i].box.Clip = inter
			}
		}
	}
	l.paintBorders(image.Rect(x, start, x+w, cy), s)
	if cy+margin[2] > l.pageHeightLimit() {
		l.err = limitError("height")
	}
	if profile.through {
		return cy
	}
	return cy + margin[2]
}

func (l *layout) paintBorders(r image.Rectangle, s style) {
	if !s.hasBorder() || r.Empty() {
		return
	}
	bTop, bBottom := s.bw[0], s.bw[2]
	if bTop+bBottom > r.Dy() {
		sides := apportion([]int{bTop, bBottom}, r.Dy())
		bTop, bBottom = sides[0], sides[1]
	}
	bLeft, bRight := s.bw[3], s.bw[1]
	if bLeft+bRight > r.Dx() {
		sides := apportion([]int{bLeft, bRight}, r.Dx())
		bLeft, bRight = sides[0], sides[1]
	}
	paintEdge := func(side int, seg image.Rectangle, horizontal bool) {
		if s.bw[side] <= 0 {
			return
		}
		c := s.borderColor(side)
		st := s.borderStyle[side]
		if st == "dashed" || st == "dotted" {
			if seg.Empty() {
				return
			}
			if horizontal {
				t := max(1, seg.Dy())
				dash, step := t, 2*t
				if st == "dashed" {
					dash, step = 2*t, 3*t
				}
				for x := seg.Min.X; x < seg.Max.X && l.err == nil; x += step {
					l.add(Box{Bounds: image.Rect(x, seg.Min.Y, min(seg.Max.X, x+dash), seg.Max.Y), Color: c, Tag: "border"}, nil)
				}
			} else {
				t := max(1, seg.Dx())
				dash, step := t, 2*t
				if st == "dashed" {
					dash, step = 2*t, 3*t
				}
				for y := seg.Min.Y; y < seg.Max.Y && l.err == nil; y += step {
					l.add(Box{Bounds: image.Rect(seg.Min.X, y, seg.Max.X, min(seg.Max.Y, y+dash)), Color: c, Tag: "border"}, nil)
				}
			}
			return
		}
		l.add(Box{Bounds: seg, Color: c, Tag: "border"}, nil)
	}
	paintEdge(0, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+bTop), true)
	paintEdge(2, image.Rect(r.Min.X, r.Max.Y-bBottom, r.Max.X, r.Max.Y), true)
	paintEdge(3, image.Rect(r.Min.X, r.Min.Y+bTop, r.Min.X+bLeft, r.Max.Y-bBottom), false)
	paintEdge(1, image.Rect(r.Max.X-bRight, r.Min.Y+bTop, r.Max.X, r.Max.Y-bBottom), false)
}

func (l *layout) resolveInlineBlock(n *node, ib style, baseW int) (style, int) {
	for i := range ib.margin {
		if ib.margin[i].unit == lengthPercent {
			ib.margin[i] = px(ib.margin[i].resolveSigned(baseW, 256))
		}
		if ib.margin[i].unit == lengthAuto {
			ib.margin[i] = px(0)
		}
		if ib.padding[i].unit == lengthPercent {
			ib.padding[i] = px(ib.padding[i].resolve(baseW, 256))
		}
	}
	for _, v := range []*cssLength{&ib.width, &ib.minWidth, &ib.maxWidth} {
		if v.unit == lengthPercent {
			*v = px(v.resolve(baseW, 4096))
		}
	}
	if !ib.width.definite(baseW) {
		minimum, preferred := l.preferredCell(n, ib, baseW)
		m, p := resolveEdges(ib.margin, baseW), resolveEdges(ib.padding, baseW)
		avail := max(0, baseW-m[1]-m[3])
		size := max(0, min(preferred, max(minimum, avail))-m[1]-m[3])
		if !ib.borderBox {
			size = max(0, size-p[1]-p[3]-ib.borderH())
		}
		ib.width = px(size)
	}
	m, _, target := boxWidth(ib, baseW)
	return ib, target + m[1] + m[3]
}

func (l *layout) flow(nodes []*node, x, y, w int, s style) int {
	baseX, baseW := x, w
	lineHeight := 0
	px := x
	lineStart := len(l.ops)
	hasText, lineReady, firstLine := false, false, true
	prepareLine := func() {
		if lineReady {
			return
		}
		old := image.Pt(x, y)
		if l.floats != nil && len(l.floats.boxes) > 0 {
			x, y, w = l.floats.line(baseX, y, baseW, max(lineHeight, s.line))
		}
		delta := image.Pt(x, y).Sub(old)
		for i := lineStart; i < len(l.ops); i++ {
			op := &l.ops[i]
			if op.owner == l.frame && (op.box.Tag == "anchor" || op.box.Tag == "position-anchor") {
				op.box.translate(delta)
			}
		}
		px = x
		if firstLine {
			indent := max(-w, min(w, s.textIndent.offset(baseW)))
			px = max(x, min(x+w, x+indent))
		}
		lineReady = true
	}
	pendingSpace := false
	var pendingSpaceStyle style
	var pendingSpaceBg color.RGBA
	pendingLeft := 0
	spaceWraps := false
	preStart := true
	preColumn := 0
	prefix := "\n"
	paintSpace := func(px, space int) {
		if space <= 0 {
			return
		}
		if pendingSpaceBg.A > 0 {
			l.rect(image.Rect(px, y, px+space, y+pendingSpaceStyle.line), pendingSpaceBg, "span")
		}
		if (pendingSpaceStyle.underline || pendingSpaceStyle.strike) && pendingSpaceStyle.size > 0 && pendingSpaceStyle.fg.A > 0 {
			sf := l.face(pendingSpaceStyle)
			base := l.baseline(pendingSpaceStyle)
			if pendingSpaceStyle.vertical == "sub" {
				base += max(2, pendingSpaceStyle.size/4)
			} else if pendingSpaceStyle.vertical == "sup" || pendingSpaceStyle.vertical == "super" {
				base = max(pendingSpaceStyle.size, base-max(2, pendingSpaceStyle.size/3))
			}
			if pendingSpaceStyle.strike {
				yy := y + base - sf.Metrics().Ascent.Ceil()/3
				l.add(Box{Bounds: image.Rect(px, yy, px+space, yy+1), Color: pendingSpaceStyle.fg, Tag: "strike", LinkID: pendingSpaceStyle.linkID}, nil)
			}
			if pendingSpaceStyle.underline {
				l.add(Box{Bounds: image.Rect(px, y+base+1, px+space, y+base+2), Color: pendingSpaceStyle.fg, Tag: "underline", LinkID: pendingSpaceStyle.linkID}, nil)
			}
		}
	}
	finishLine := func(wrapped bool) {
		prepareLine()
		used := px - x
		shift := 0
		if s.align == "center" {
			shift = max(0, (w-used)/2)
		} else if s.align == "right" {
			shift = max(0, w-used)
		} else if wrapped && s.align == "justify" && used < w {
			var textOps []int
			for i := lineStart; i < len(l.ops); i++ {
				if l.ops[i].owner == l.frame && l.ops[i].box.Tag == "text" {
					textOps = append(textOps, i)
				}
			}
			if len(textOps) > 1 {
				spare := w - used
				gaps := len(textOps) - 1
				for idx := 1; idx < len(textOps); idx++ {
					dx := spare * idx / gaps
					l.ops[textOps[idx]].box.translate(image.Pt(dx, 0))
				}
			}
		}
		for i := lineStart; i < len(l.ops); i++ {
			if l.ops[i].owner == l.frame {
				l.ops[i].box.translate(image.Pt(shift, 0))
			}
		}
		y += lineHeight
		if y > l.pageHeightLimit() {
			l.err = limitError("height")
		}
		lineHeight = 0
		x, w = baseX, baseW
		px = x
		firstLine = false
		lineReady = false
		lineStart = len(l.ops)
		hasText = false
		pendingSpace = false
		pendingSpaceBg = color.RGBA{}
		spaceWraps = false
	}
	newline := func() { finishLine(false) }
	wrapLine := func() { finishLine(true) }
	parentH := -1
	if s.height.definite(-1) {
		parentH = s.height.resolve(-1, l.d.limits.Height)
	}
	var visit func(*node, style, color.RGBA)
	visit = func(n *node, st style, inlineBg color.RGBA) {
		if l.err != nil {
			return
		}
		if n.tag != "" {
			computed := l.d.styled(n, st)
			if computed.display == "none" {
				return
			}
			if computed.floatSide == "left" || computed.floatSide == "right" {
				if hasText {
					newline()
				}
				l.floats.place(n, computed, baseX, y, baseW, parentH)
				lineReady = false
				return
			}
			if computed.position == "absolute" {
				l.queueAbsolute(n, computed, px, y)
				return
			}
		}
		if n.tag == "br" {
			lineHeight = max(lineHeight, st.line)
			prefix += "\n"
			newline()
			return
		}
		if kind, _ := diagramSource(n); kind != "" {
			if l.d.styled(n, st).display == "none" {
				return
			}
			if hasText {
				newline()
			}
			y = l.block(n, x, y, w, parentH, st)
			px = x
			lineStart = len(l.ops)
			lineHeight = 0
			hasText = false
			pendingSpace = false
			pendingSpaceBg = color.RGBA{}
			return
		}
		if n.tag != "" {
			st = l.d.styled(n, st)
			if st.position == "absolute" {
				l.queueAbsolute(n, st, px, y)
				return
			}
			if st.display == "none" {
				return
			}
			if n.tag == "img" {
				src := l.opts.Images[n.attrs["src"]]
				if src == nil || src.Bounds().Empty() {
					if st.bg.A > 0 {
						inlineBg = st.bg
					}
					visit(&node{text: "[image: " + n.attrs["alt"] + "]"}, st, inlineBg)
					return
				}
				margin, padding := resolveEdges(st.margin, baseW), resolveEdges(st.padding, baseW)
				imgH := -1
				if st.height.definite(parentH) {
					imgH = st.height.resolve(parentH, l.d.limits.Height)
					if st.borderBox {
						imgH = max(0, imgH-padding[0]-padding[2]-st.borderV())
					}
				}
				if imgH >= 0 {
					if st.maxHeight.definite(parentH) {
						mh := st.maxHeight.resolve(parentH, l.d.limits.Height)
						if st.borderBox {
							mh = max(0, mh-padding[0]-padding[2]-st.borderV())
						}
						imgH = min(imgH, mh)
					}
					if st.minHeight.definite(parentH) {
						mh := st.minHeight.resolve(parentH, l.d.limits.Height)
						if st.borderBox {
							mh = max(0, mh-padding[0]-padding[2]-st.borderV())
						}
						imgH = max(imgH, mh)
					}
				}
				availW := max(1, baseW-margin[1]-margin[3]-padding[1]-padding[3]-st.borderH())
				hasCSSW := st.width.definite(baseW)
				targetW := availW
				if hasCSSW {
					targetW = st.width.resolve(baseW, 16384)
					if st.borderBox {
						targetW = max(0, targetW-padding[1]-padding[3]-st.borderH())
					}
				}
				if st.maxWidth.definite(baseW) {
					mw := st.maxWidth.resolve(baseW, 16384)
					if st.borderBox {
						mw = max(0, mw-padding[1]-padding[3]-st.borderH())
					}
					targetW = min(targetW, mw)
				}
				if st.minWidth.definite(baseW) {
					mw := st.minWidth.resolve(baseW, 16384)
					if st.borderBox {
						mw = max(0, mw-padding[1]-padding[3]-st.borderH())
					}
					targetW = max(targetW, mw)
				}
				targetW = max(1, min(availW, targetW))
				iw, ih := imageSize(n, src.Bounds().Size(), hasCSSW || st.maxWidth.definite(baseW) || st.minWidth.definite(baseW), targetW, imgH)
				ih = min(ih, l.d.limits.Height)
				outerW := iw + padding[1] + padding[3] + st.borderH() + margin[1] + margin[3]
				outerH := ih + padding[0] + padding[2] + st.borderV() + margin[0] + margin[2]
				prepareLine()
				if pendingSpace && prefix == "" {
					prefix = " "
				}
				space := 0
				if pendingSpace && hasText {
					space = l.glyphAdvanceStyled(pendingSpaceStyle, l.face(pendingSpaceStyle), ' ')
				}
				if (st.whiteSpace != "nowrap" && !st.pre || spaceWraps) && px > x && px+space+pendingLeft+outerW > x+w {
					wrapLine()
					prepareLine()
					space = 0
				}
				paintSpace(px, space)
				px += space + pendingLeft
				pendingLeft = 0
				pendingSpace = false
				pendingSpaceBg = color.RGBA{}
				spaceWraps = false
				ix := px + margin[3] + st.bw[3] + padding[3]
				iy := y + margin[0] + st.bw[0] + padding[0]
				var shift image.Point
				if st.position == "relative" {
					shift = relativeOffset(st, baseW, parentH)
				}
				l.add(Box{Bounds: image.Rect(ix, iy, ix+iw, iy+ih).Add(shift), Tag: "img", Anchor: anchorName(n), LinkID: st.linkID}, src)
				if st.hasBorder() {
					bRect := image.Rect(px+margin[3], y+margin[0], px+margin[3]+iw+padding[1]+padding[3]+st.borderH(), y+margin[0]+ih+padding[0]+padding[2]+st.borderV()).Add(shift)
					l.paintBorders(bRect, st)
				}
				px += outerW
				lineHeight = max(lineHeight, outerH)
				hasText = true
				prefix = ""
				return
			}
			if st.display == "inline-block" || st.display == "inline-flex" {
				ib, outerW := l.resolveInlineBlock(n, st, baseW)
				if st.display == "inline-flex" {
					ib.display = "flex"
				}
				prepareLine()
				if pendingSpace && prefix == "" {
					prefix = " "
				}
				space := 0
				if pendingSpace && hasText {
					space = l.glyphAdvanceStyled(pendingSpaceStyle, l.face(pendingSpaceStyle), ' ')
				}
				if (st.whiteSpace != "nowrap" && !st.pre || spaceWraps) && px > x && px+space+pendingLeft+outerW > x+w {
					wrapLine()
					prepareLine()
					space = 0
				}
				paintSpace(px, space)
				px += space + pendingLeft
				pendingLeft = 0
				pendingSpace = false
				pendingSpaceBg = color.RGBA{}
				spaceWraps = false
				endY := l.blockStyled(n, px, y, max(1, outerW), parentH, ib)
				px += outerW
				lineHeight = max(lineHeight, endY-y)
				hasText = true
				prefix = ""
				return
			}
			if st.bg.A > 0 {
				inlineBg = st.bg
			}
			margin, padding := resolveEdges(st.margin, baseW), resolveEdges(st.padding, baseW)
			pendingLeft += margin[3] + st.bw[3] + padding[3]
			if name := anchorName(n); name != "" {
				l.add(Box{Tag: "anchor", Anchor: name, Bounds: image.Rect(px, y, px+1, y+st.line)}, nil)
			}
			spanStartOp := len(l.ops)
			for _, c := range n.children {
				visit(c, st, inlineBg)
			}
			if st.hasBorder() {
				minX, maxX := px, px
				hasSpanBox := false
				for i := max(spanStartOp, lineStart); i < len(l.ops); i++ {
					b := l.ops[i].box
					if b.Tag == "text" || b.Tag == "img" {
						if !hasSpanBox {
							minX, maxX = b.Bounds.Min.X, b.Bounds.Max.X
							hasSpanBox = true
						} else {
							minX = min(minX, b.Bounds.Min.X)
							maxX = max(maxX, b.Bounds.Max.X)
						}
					}
				}
				if hasSpanBox {
					l.paintBorders(image.Rect(minX-padding[3]-st.bw[3], y, maxX+padding[1]+st.bw[1], y+st.line), st)
				}
			}
			if hasText && !pendingSpace {
				px += padding[1] + st.bw[1] + margin[1]
			} else {
				pendingLeft += padding[1] + st.bw[1] + margin[1]
			}
			return
		}
		if st.size == 0 {
			if strings.TrimSpace(n.text) != "" || st.pre {
				l.add(Box{Bounds: image.Rect(px, y, px, y+st.line), Tag: "text", Text: n.text, TextPrefix: prefix, FontSize: 0, FontFamily: st.family, Color: st.fg, LinkID: st.linkID}, nil)
				prefix = ""
				hasText = true
				lineHeight = max(lineHeight, st.line)
			}
			return
		}
		f := l.face(st)
		baseOffset := l.baseline(st)
		if st.vertical == "sub" {
			baseOffset += max(2, st.size/4)
		} else if st.vertical == "sup" || st.vertical == "super" {
			baseOffset = max(st.size, baseOffset-max(2, st.size/3))
		}
		rawText := applyTextTransform(n.text, st.textTransform)
		if st.pre {
			text, nextColumn := preTextAt(rawText, preColumn)
			preColumn = nextColumn
			if preStart {
				if n.parent != nil && n.parent.tag == "pre" {
					text = strings.TrimPrefix(text, "\n")
				}
				preStart = false
			}
			runes := []rune(text)
			for index, r := range runes {
				if l.err != nil {
					return
				}
				if e := l.ctx.Err(); e != nil {
					l.err = e
					return
				}
				if r == '\n' {
					lineHeight = max(lineHeight, st.line)
					prefix += "\n"
					newline()
					continue
				}
				lineHeight = max(lineHeight, st.line)
				prepareLine()
				if pendingSpace && hasText {
					space := l.glyphAdvanceStyled(pendingSpaceStyle, l.face(pendingSpaceStyle), ' ')
					paintSpace(px, space)
					px += space
					pendingSpace = false
					pendingSpaceBg = color.RGBA{}
					spaceWraps = false
				}
				px += pendingLeft
				pendingLeft = 0
				if st.whiteSpace != "pre" && !cssSpace(r) && (index == 0 || cssSpace(runes[index-1])) {
					end := index + 1
					for end < len(runes) && !cssSpace(runes[end]) {
						end++
					}
					ww := l.measureTextStyled(st, f, string(runes[index:end]))
					if px > x && px+ww > x+w {
						wrapLine()
						prepareLine()
					}
				}
				adv := l.glyphAdvanceStyled(st, f, r)
				if st.whiteSpace != "pre" && (cssSpace(r) || st.overflowWrap != "normal") && px > x && px+adv > x+w {
					wrapLine()
					prepareLine()
				}
				if inlineBg.A > 0 && adv > 0 {
					l.rect(image.Rect(px, y, px+adv, y+st.line), inlineBg, "span")
				}
				l.add(Box{Bounds: image.Rect(px, y, px+adv, y+st.line), Tag: "text", Text: string(r), Baseline: baseOffset, WordSpacing: st.wordSpacing, LetterSpacing: st.letterSpacing, TextPrefix: prefix, FontSize: st.size, Bold: st.bold, SmallCaps: st.smallCaps, FontFamily: st.family, Italic: st.italic, Underline: st.underline, Strike: st.strike, Monospace: st.mono, Color: st.fg, LinkID: st.linkID}, nil)
				prefix = ""
				px += adv
				hasText = true
				lineHeight = max(lineHeight, st.line)
			}
			return
		}
		// Words wrap at whitespace; oversized words break at rune boundaries.
		text := strings.ReplaceAll(strings.ReplaceAll(rawText, "\r\n", "\n"), "\r", "\n")
		for len(text) > 0 {
			if e := l.ctx.Err(); e != nil {
				l.err = e
				return
			}
			i := strings.IndexFunc(text, cssSpace)
			word := text
			if i >= 0 {
				word = text[:i]
			}
			if word != "" {
				lineHeight = max(lineHeight, st.line)
				prepareLine()
				if pendingSpace && prefix == "" {
					prefix = " "
				}
				space := 0
				if pendingSpace && hasText {
					space = l.glyphAdvanceStyled(pendingSpaceStyle, l.face(pendingSpaceStyle), ' ')
				}
				ww := l.measureTextStyled(st, f, word)
				checkW := ww
				if st.whiteSpace == "nowrap" && spaceWraps {
					checkW = l.measureTextStyled(st, f, strings.Join(strings.Fields(text), " "))
				}
				if (st.whiteSpace != "nowrap" || spaceWraps) && px > x && px+space+pendingLeft+checkW > x+w {
					wrapLine()
					prepareLine()
					space = 0
				}
				paintSpace(px, space)
				px += space + pendingLeft
				pendingLeft = 0
				var run strings.Builder
				runX := px
				emit := func() {
					if run.Len() > 0 {
						if inlineBg.A > 0 && px > runX {
							l.rect(image.Rect(runX, y, px, y+st.line), inlineBg, "span")
						}
						l.add(Box{Bounds: image.Rect(runX, y, px, y+st.line), Tag: "text", Text: run.String(), Baseline: baseOffset, WordSpacing: st.wordSpacing, LetterSpacing: st.letterSpacing, TextPrefix: prefix, FontSize: st.size, Bold: st.bold, SmallCaps: st.smallCaps, FontFamily: st.family, Italic: st.italic, Underline: st.underline, Strike: st.strike, Color: st.fg, Monospace: st.mono, LinkID: st.linkID}, nil)
						run.Reset()
						prefix = ""
						hasText = true
					}
				}
				useEllipsis := s.textOverflow == "ellipsis" && (s.overflow == "hidden" || s.overflow == "clip") && st.whiteSpace == "nowrap"
				ellW := 0
				willTruncate := false
				if useEllipsis {
					ellW = l.glyphAdvanceStyled(st, f, '…')
					restW := ww
					if i >= 0 {
						restW = l.measureTextStyled(st, f, strings.Join(strings.Fields(text), " "))
					}
					willTruncate = px+restW > x+w
				}
				truncated := false
				for _, r := range word {
					adv := l.glyphAdvanceStyled(st, f, r)
					if willTruncate && px+adv+ellW > x+w {
						run.WriteRune('…')
						px = min(x+w, px+ellW)
						truncated = true
						break
					}
					if st.whiteSpace != "nowrap" && st.overflowWrap != "normal" && px > x && px+adv > x+w {
						emit()
						wrapLine()
						prepareLine()
						runX = px
					}
					run.WriteRune(r)
					px += adv
					lineHeight = max(lineHeight, st.line)
				}
				emit()
				if truncated {
					return
				}
				pendingSpace = false
				pendingSpaceBg = color.RGBA{}
				spaceWraps = false
			}
			if i < 0 {
				break
			}
			text = text[i:]
			for len(text) > 0 && cssSpace(rune(text[0])) {
				if text[0] == '\n' && st.whiteSpace == "pre-line" {
					lineHeight = max(lineHeight, st.line)
					prefix += "\n"
					newline()
				} else if hasText {
					if !pendingSpace {
						pendingSpace = true
						pendingSpaceStyle = st
						pendingSpaceBg = inlineBg
					}
					if st.whiteSpace != "nowrap" {
						spaceWraps = true
					}
				}
				text = text[1:]
			}
		}
	}
	for _, n := range nodes {
		visit(n, s, color.RGBA{})
	}
	if hasText {
		newline()
	}
	return y
}
