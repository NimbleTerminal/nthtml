package document

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Box describes a painted rectangle or text run in document pixel coordinates.
// Text and bounds can also be consumed by a host's text renderer (e.g. nttypeset).
type Box struct {
	Bounds image.Rectangle
	// Baseline is the font baseline offset from Bounds.Min.Y. InkBounds encloses visible glyph overflow.
	Baseline, WordSpacing     int
	InkBounds                 image.Rectangle
	Tag, Text                 string
	Anchor, TextPrefix        string
	FontSize                  int
	Bold                      bool
	FontFamily                string
	Italic, Underline, Strike bool
	Monospace                 bool
	LinkID                    int
	Color                     color.RGBA
}
type Result struct {
	// Warnings describe diagrams replaced by readable fallbacks.
	Warnings []string
	Anchors  map[string]image.Point
	document *Document
	Image    *image.RGBA
	Boxes    []Box
	Links    []Link
}
type operation struct {
	owner  *positionFrame
	box    Box
	source image.Image
}
type layout struct {
	floats                      *floatArea
	floatWork                   int
	frame, rootFrame            *positionFrame
	absolute                    []absoluteBox
	diagramCount, diagramPixels int
	warnings                    []string
	tableWork                   int
	marginWork                  int
	ctx                         context.Context
	d                           *Document
	opts                        Options
	ops                         []operation
	faces                       map[faceKey]font.Face
	fonts                       map[string]*opentype.Font
	fontBytes                   int
	err                         error
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
	if b.paintBottom() > l.d.limits.Height {
		l.err = limitError("height")
		return
	}
	if len(l.ops) >= 100000 {
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
	l := &layout{ctx: ctx, d: d, opts: o, faces: map[faceKey]font.Face{}}
	l.fonts = map[string]*opentype.Font{}
	l.rootFrame = &positionFrame{containing: true, op: -1, initial: image.Rect(0, 0, o.Width, 0)}
	l.frame = l.rootFrame
	defer func() {
		for _, f := range l.faces {
			f.Close()
		}
	}()
	h := l.block(d.root, 0, 0, o.Width, -1, style{rootSize: 16, size: 16, line: 22, fg: color.RGBA{30, 36, 46, 255}})
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
	if h > d.limits.Height || o.Width > d.limits.Pixels/h {
		return nil, limitError("pixels")
	}
	img := image.NewRGBA(image.Rect(0, 0, o.Width, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	result := &Result{Anchors: map[string]image.Point{}, Warnings: l.warnings, document: d, Image: img, Boxes: make([]Box, 0, len(l.ops))}
	links, err := d.Links(ctx)
	if err != nil {
		return nil, err
	}
	linkIndex := map[int]int{}
	for i, link := range links {
		linkIndex[link.ID] = i
	}
	for _, op := range l.ops {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
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
			region = region.Intersect(img.Bounds())
			if !region.Empty() {
				appendLinkRegion(&links[i], region)
			}
		}
		if op.source != nil {
			xdraw.ApproxBiLinear.Scale(img, b.Bounds, op.source, op.source.Bounds(), draw.Over, nil)
		} else if b.Text != "" {
			if b.FontSize == 0 {
				continue
			}
			f := l.face(style{size: b.FontSize, bold: b.Bold, mono: b.Monospace, family: b.FontFamily, italic: b.Italic})
			dr := font.Drawer{Dst: img, Src: image.NewUniform(b.Color), Face: f, Dot: fixed.P(b.Bounds.Min.X, b.Bounds.Min.Y+b.Baseline)}
			drawText(&dr, b.Text, b.WordSpacing)
			if b.Strike {
				yy := b.Bounds.Min.Y + b.Baseline - f.Metrics().Ascent.Ceil()/3
				draw.Draw(img, image.Rect(b.Bounds.Min.X, yy, b.Bounds.Max.X, yy+1), image.NewUniform(b.Color), image.Point{}, draw.Over)
			}
			if b.Underline {
				under := image.Rect(b.Bounds.Min.X, b.Bounds.Min.Y+b.Baseline+1, b.Bounds.Max.X, b.Bounds.Min.Y+b.Baseline+2)
				draw.Draw(img, under, image.NewUniform(b.Color), image.Point{}, draw.Over)
			}
		} else {
			draw.Draw(img, b.Bounds, image.NewUniform(b.Color), image.Point{}, draw.Over)
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
		s.bw = 0
	}
	profile := l.margins(n, s, w, parentHeight)
	margin, padding, target := boxWidth(s, w)
	margin[0], margin[2] = profile.before, profile.after
	extra := padding[1] + padding[3] + 2*s.bw
	x += margin[3]
	y += margin[0]
	w = target
	height := -1
	if s.height.definite(parentHeight) {
		height = s.height.resolve(parentHeight, l.d.limits.Height)
		if s.borderBox {
			height = max(0, height-padding[0]-padding[2]-2*s.bw)
		}
	}
	kind, _ := diagramSource(n)
	constrainHeight := kind == "" && n.tag != "img" && n.tag != "table" && n.tag != "tr" && n.tag != "td" && n.tag != "th" && n.tag != "thead" && n.tag != "tbody" && n.tag != "tfoot"
	clampHeight := func(h int) int {
		content := func(v cssLength) int {
			size := v.resolve(parentHeight, l.d.limits.Height)
			if s.borderBox {
				size = max(0, size-padding[0]-padding[2]-2*s.bw)
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
		frame := &positionFrame{parent: previous, node: n, op: opStart, border: s.bw, containing: s.position == "relative" || s.position == "absolute", floating: s.position != "relative" && s.position != "absolute"}
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
	cx, cy, cw := x+padding[3]+s.bw, y+padding[0]+s.bw, max(1, w-extra)
	contentY := cy
	if kind, source := diagramSource(n); kind != "" {
		cy = l.diagram(n, kind, source, cx, cy, cw, height, s)
	} else if n.tag == "table" {
		cy = l.table(n, cx, cy, cw, height, tableStyle)
	} else if n.tag == "ul" || n.tag == "ol" {
		index := listStart(n)
		for _, child := range n.children {
			if child.tag != "li" {
				continue
			}
			if l.d.styled(child, s).display == "none" {
				continue
			}
			if n.tag == "ol" {
				index = listNumber(child.attrs["value"], index)
			}
			marker := listMarker(n, index)
			index++
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
			l.add(Box{Bounds: image.Rect(cx, markerY, cx+indent, markerY+s.line), Tag: "marker", Text: marker, Baseline: l.baseline(s), WordSpacing: s.wordSpacing, FontSize: s.size, Bold: s.bold, FontFamily: s.family, Italic: s.italic, Color: s.fg}, nil)
			cy = max(cy, markerY+s.line)
		}
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
		flush := func() {
			if len(inline) > 0 {
				before := cy
				cy = l.flow(inline, cx, cy, cw, s)
				if cy != before {
					pending = 0
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
			if kind == "" && (c.tag == "" || cs.display == "inline" || c.tag == "br") {
				inline = append(inline, c)
				continue
			}
			flush()
			childMargins := l.margins(c, cs, cw, height)
			input := cy - min(pending, childMargins.before)
			if atTop && profile.top {
				input = cy - childMargins.before
			}
			if l.floats != nil {
				input = max(input, l.floats.clear(input+childMargins.before, cs.clear)-childMargins.before)
			}
			cy = l.blockStyled(c, cx, input, cw, height, cs)
			if atTop && profile.top && childMargins.through {
				pending = 0
				continue
			}
			if childMargins.through {
				pending = max(pending, childMargins.after)
			} else {
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
	cy += padding[2] + s.bw
	if l.err != nil {
		return cy
	}
	if opStart < len(l.ops) {
		l.ops[opStart].box = Box{Bounds: image.Rect(x, start, x+w, cy), Tag: n.tag, Anchor: anchorName(n), Color: s.bg}
	}
	if s.bw > 0 {
		r := image.Rect(x, start, x+w, cy)
		c := s.border
		if !s.borderSet {
			c = s.fg
		}
		borderRect := func(r image.Rectangle, c color.RGBA, tag string) { l.add(Box{Bounds: r, Color: c, Tag: tag}, nil) }
		b := min(s.bw, r.Dx()/2, r.Dy()/2)
		borderRect(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+b), c, "border")
		borderRect(image.Rect(r.Min.X, r.Max.Y-b, r.Max.X, r.Max.Y), c, "border")
		borderRect(image.Rect(r.Min.X, r.Min.Y+b, r.Min.X+b, r.Max.Y-b), c, "border")
		borderRect(image.Rect(r.Max.X-b, r.Min.Y+b, r.Max.X, r.Max.Y-b), c, "border")
	}
	if cy+margin[2] > l.d.limits.Height {
		l.err = limitError("height")
	}
	if profile.through {
		return cy
	}
	return cy + margin[2]
}
func (l *layout) flow(nodes []*node, x, y, w int, s style) int {
	baseX, baseW := x, w
	lineHeight := s.line
	px := x
	lineStart := len(l.ops)
	hasText, lineReady := false, false
	prepareLine := func() {
		if lineReady {
			return
		}
		old := image.Pt(x, y)
		if l.floats != nil && len(l.floats.boxes) > 0 {
			x, y, w = l.floats.line(baseX, y, baseW, lineHeight)
		}
		delta := image.Pt(x, y).Sub(old)
		for i := lineStart; i < len(l.ops); i++ {
			op := &l.ops[i]
			if op.owner == l.frame && (op.box.Tag == "anchor" || op.box.Tag == "position-anchor") {
				op.box.translate(delta)
			}
		}
		px = x
		lineReady = true
	}
	pendingSpace := false
	preStart := true
	preColumn := 0
	prefix := "\n"
	newline := func() {
		prepareLine()
		used := px - x
		shift := 0
		if s.align == "center" {
			shift = max(0, (w-used)/2)
		} else if s.align == "right" {
			shift = max(0, w-used)
		}
		for i := lineStart; i < len(l.ops); i++ {
			if l.ops[i].owner == l.frame {
				l.ops[i].box.translate(image.Pt(shift, 0))
			}
		}
		y += lineHeight
		if y > l.d.limits.Height {
			l.err = limitError("height")
		}
		lineHeight = s.line
		x, w = baseX, baseW
		px = x
		lineReady = false
		lineStart = len(l.ops)
		hasText = false
	}
	var visit func(*node, style)
	visit = func(n *node, st style) {
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
				l.floats.place(n, computed, baseX, y, baseW, -1)
				lineReady = false
				return
			}
			if computed.position == "absolute" {
				l.queueAbsolute(n, computed, px, y)
				return
			}
		}
		if n.tag == "br" {
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
			y = l.block(n, x, y, w, -1, st)
			px = x
			lineStart = len(l.ops)
			lineHeight = s.line
			hasText = false
			pendingSpace = false
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
			if name := anchorName(n); name != "" {
				l.add(Box{Tag: "anchor", Anchor: name, Bounds: image.Rect(px, y, px+1, y+st.line)}, nil)
			}
			for _, c := range n.children {
				visit(c, st)
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
		if st.pre {
			text, nextColumn := preTextAt(n.text, preColumn)
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
					prefix += "\n"
					newline()
					continue
				}
				lineHeight = max(lineHeight, st.line)
				prepareLine()
				if st.whiteSpace != "pre" && !cssSpace(r) && (index == 0 || cssSpace(runes[index-1])) {
					end := index + 1
					for end < len(runes) && !cssSpace(runes[end]) {
						end++
					}
					ww := measureText(f, string(runes[index:end]), st.wordSpacing)
					if px > x && px+ww > x+w {
						newline()
						prepareLine()
					}
				}
				adv := glyphAdvance(f, r, st.wordSpacing)
				if st.whiteSpace != "pre" && (cssSpace(r) || st.overflowWrap != "normal") && px > x && px+adv > x+w {
					newline()
					prepareLine()
				}
				l.add(Box{Bounds: image.Rect(px, y, px+adv, y+st.line), Tag: "text", Text: string(r), Baseline: l.baseline(st), WordSpacing: st.wordSpacing, TextPrefix: prefix, FontSize: st.size, Bold: st.bold, FontFamily: st.family, Italic: st.italic, Underline: st.underline, Strike: st.strike, Monospace: st.mono, Color: st.fg, LinkID: st.linkID}, nil)
				prefix = ""
				px += adv
				hasText = true
				lineHeight = max(lineHeight, st.line)
			}
			return
		}
		// Words wrap at whitespace; oversized words break at rune boundaries.
		text := strings.ReplaceAll(strings.ReplaceAll(n.text, "\r\n", "\n"), "\r", "\n")
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
				if pendingSpace && px > x {
					space = glyphAdvance(f, ' ', st.wordSpacing)
				}
				ww := measureText(f, word, st.wordSpacing)
				if st.whiteSpace != "nowrap" && px > x && px+space+ww > x+w {
					newline()
					prepareLine()
					space = 0
				}
				px += space
				var run strings.Builder
				runX := px
				emit := func() {
					if run.Len() > 0 {
						l.add(Box{Bounds: image.Rect(runX, y, px, y+st.line), Tag: "text", Text: run.String(), Baseline: l.baseline(st), WordSpacing: st.wordSpacing, TextPrefix: prefix, FontSize: st.size, Bold: st.bold, FontFamily: st.family, Italic: st.italic, Underline: st.underline, Strike: st.strike, Color: st.fg, Monospace: st.mono, LinkID: st.linkID}, nil)
						run.Reset()
						prefix = ""
						hasText = true
					}
				}
				for _, r := range word {
					adv := glyphAdvance(f, r, st.wordSpacing)
					if st.whiteSpace != "nowrap" && st.overflowWrap != "normal" && px > x && px+adv > x+w {
						emit()
						newline()
						prepareLine()
						runX = px
					}
					run.WriteRune(r)
					px += adv
					lineHeight = max(lineHeight, st.line)
				}
				emit()
				pendingSpace = false
			}
			if i < 0 {
				break
			}
			text = text[i:]
			for len(text) > 0 && cssSpace(rune(text[0])) {
				if text[0] == '\n' && st.whiteSpace == "pre-line" {
					prefix += "\n"
					newline()
					pendingSpace = false
				} else {
					pendingSpace = true
				}
				text = text[1:]
			}
		}
	}
	for _, n := range nodes {
		visit(n, s)
	}
	if hasText {
		newline()
	}
	return y
}
