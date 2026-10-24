package document

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"unicode"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Box describes a painted rectangle or text run in document pixel coordinates.
// Text and bounds can also be consumed by a host's text renderer (e.g. nttypeset).
type Box struct {
	Bounds                    image.Rectangle
	Tag, Text                 string
	FontSize                  int
	Bold                      bool
	FontFamily                string
	Italic, Underline, Strike bool
	Monospace                 bool
	LinkID                    int
	Color                     color.RGBA
}
type Result struct {
	document *Document
	Image    *image.RGBA
	Boxes    []Box
	Links    []Link
}
type operation struct {
	box    Box
	source image.Image
}
type layout struct {
	tableWork int
	ctx       context.Context
	d         *Document
	opts      Options
	ops       []operation
	faces     map[faceKey]font.Face
	fonts     map[string]*opentype.Font
	fontBytes int
	err       error
}

func (l *layout) add(b Box, src image.Image) {
	if l.err != nil {
		return
	}
	if e := l.ctx.Err(); e != nil {
		l.err = e
		return
	}
	if b.Bounds.Max.Y > l.d.limits.Height {
		l.err = limitError("height")
		return
	}
	if len(l.ops) >= 100000 {
		l.err = limitError("paint operations")
		return
	}
	l.ops = append(l.ops, operation{b, src})
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
	defer func() {
		for _, f := range l.faces {
			f.Close()
		}
	}()
	h := l.block(d.root, 0, 0, o.Width, -1, style{size: 16, line: 22, fg: color.RGBA{30, 36, 46, 255}})
	if l.err != nil {
		return nil, l.err
	}
	h = max(1, h)
	if h > d.limits.Height || o.Width > d.limits.Pixels/h {
		return nil, limitError("pixels")
	}
	img := image.NewRGBA(image.Rect(0, 0, o.Width, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	result := &Result{document: d, Image: img, Boxes: make([]Box, 0, len(l.ops))}
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
		if i, ok := linkIndex[b.LinkID]; ok && b.LinkID != 0 {
			region := b.Bounds.Intersect(img.Bounds())
			if !region.Empty() {
				appendLinkRegion(&links[i], region)
			}
		}
		if op.source != nil {
			xdraw.ApproxBiLinear.Scale(img, b.Bounds, op.source, op.source.Bounds(), draw.Over, nil)
		} else if b.Text != "" {
			f := l.face(style{size: b.FontSize, bold: b.Bold, mono: b.Monospace, family: b.FontFamily, italic: b.Italic})
			dr := font.Drawer{Dst: img, Src: image.NewUniform(b.Color), Face: f, Dot: fixed.P(b.Bounds.Min.X, b.Bounds.Min.Y+f.Metrics().Ascent.Ceil())}
			dr.DrawString(b.Text)
			if b.Strike {
				yy := b.Bounds.Min.Y + f.Metrics().Ascent.Ceil()*2/3
				draw.Draw(img, image.Rect(b.Bounds.Min.X, yy, b.Bounds.Max.X, yy+1), image.NewUniform(b.Color), image.Point{}, draw.Over)
			}
			if b.Underline {
				under := image.Rect(b.Bounds.Min.X, b.Bounds.Min.Y+f.Metrics().Ascent.Ceil()+1, b.Bounds.Max.X, b.Bounds.Min.Y+f.Metrics().Ascent.Ceil()+2)
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

	margin, padding := resolveEdges(s.margin, w), resolveEdges(s.padding, w)
	extra := padding[1] + padding[3] + 2*s.bw
	available := max(1, w-margin[1]-margin[3])
	target := available
	if s.width.definite(w) {
		target = s.width.resolve(w, 16384)
		if !s.borderBox {
			target += extra
		}
	}
	if s.maxWidth.definite(w) {
		cap := s.maxWidth.resolve(w, 16384)
		if !s.borderBox {
			cap += extra
		}
		target = min(target, cap)
	}
	target = max(1, min(available, target))
	spare := max(0, available-target)
	if s.margin[3].unit == lengthAuto {
		if s.margin[1].unit == lengthAuto {
			margin[3] = spare / 2
		} else {
			margin[3] = spare
		}
	}
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
	start := y
	opStart := len(l.ops)
	l.add(Box{Tag: n.tag}, nil)
	cx, cy, cw := x+padding[3]+s.bw, y+padding[0]+s.bw, max(1, w-extra)
	contentY := cy
	if n.tag == "table" {
		cy = l.table(n, cx, cy, cw, height, s)
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
			l.add(Box{Bounds: image.Rect(cx, markerY, cx+indent, markerY+s.line), Tag: "marker", Text: marker, FontSize: s.size, Bold: s.bold, FontFamily: s.family, Italic: s.italic, Color: s.fg}, nil)
			cy = max(cy, markerY+s.line)
		}
	} else if n.tag == "img" {
		src := l.opts.Images[n.attrs["src"]]
		if src != nil && !src.Bounds().Empty() {
			h := max(0, height)
			if h == 0 {
				h = int(float64(cw) * float64(src.Bounds().Dy()) / float64(src.Bounds().Dx()))
			}
			h = min(h, l.d.limits.Height)
			l.add(Box{Bounds: image.Rect(cx, cy, cx+cw, cy+h), Tag: "img", LinkID: s.linkID}, src)
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
		flush := func() {
			if len(inline) > 0 {
				cy = l.flow(inline, cx, cy, cw, s)
				inline = nil
			}
		}
		for _, c := range n.children {
			cs := l.d.styled(c, s)
			if c.tag == "" || cs.display == "inline" || c.tag == "br" {
				inline = append(inline, c)
			} else {
				flush()
				cy = l.block(c, cx, cy, cw, height, s)
			}
		}
		flush()
	}
	cy = max(cy, contentY+max(0, height)) + padding[2] + s.bw
	if l.err != nil {
		return cy
	}
	if opStart < len(l.ops) {
		l.ops[opStart].box = Box{Bounds: image.Rect(x, start, x+w, cy), Tag: n.tag, Color: s.bg}
	}
	if s.bw > 0 {
		r := image.Rect(x, start, x+w, cy)
		c := s.border
		if c.A == 0 {
			c = s.fg
		}
		b := s.bw
		l.rect(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+b), c, "border")
		l.rect(image.Rect(r.Min.X, r.Max.Y-b, r.Max.X, r.Max.Y), c, "border")
		l.rect(image.Rect(r.Min.X, r.Min.Y, r.Min.X+b, r.Max.Y), c, "border")
		l.rect(image.Rect(r.Max.X-b, r.Min.Y, r.Max.X, r.Max.Y), c, "border")
	}
	if cy+margin[2] > l.d.limits.Height {
		l.err = limitError("height")
	}
	return cy + margin[2]
}
func (l *layout) flow(nodes []*node, x, y, w int, s style) int {
	px := x
	lineHeight := s.line
	lineStart := len(l.ops)
	hasText := false
	pendingSpace := false
	preStart := true
	preColumn := 0
	newline := func() {
		used := px - x
		shift := 0
		if s.align == "center" {
			shift = max(0, (w-used)/2)
		} else if s.align == "right" {
			shift = max(0, w-used)
		}
		for i := lineStart; i < len(l.ops); i++ {
			l.ops[i].box.Bounds = l.ops[i].box.Bounds.Add(image.Pt(shift, 0))
		}
		y += lineHeight
		px = x
		lineHeight = s.line
		lineStart = len(l.ops)
		hasText = false
	}
	var visit func(*node, style)
	visit = func(n *node, st style) {
		if l.err != nil {
			return
		}
		if n.tag == "br" {
			newline()
			return
		}
		if n.tag != "" {
			st = l.d.styled(n, st)
			if st.display == "none" {
				return
			}
			for _, c := range n.children {
				visit(c, st)
			}
			return
		}
		f := l.face(st)
		if st.pre {
			text, nextColumn := preTextAt(n.text, preColumn)
			preColumn = nextColumn
			if preStart {
				text = strings.TrimPrefix(text, "\n")
				preStart = false
			}
			for _, r := range text {
				if e := l.ctx.Err(); e != nil {
					l.err = e
					return
				}
				if r == '\n' {
					newline()
					continue
				}
				a, _ := f.GlyphAdvance(r)
				adv := max(1, a.Ceil())
				if px > x && px+adv > x+w {
					newline()
				}
				l.add(Box{Bounds: image.Rect(px, y, px+adv, y+st.line), Tag: "text", Text: string(r), FontSize: st.size, Bold: st.bold, FontFamily: st.family, Italic: st.italic, Underline: st.underline, Strike: st.strike, Monospace: st.mono, Color: st.fg, LinkID: st.linkID}, nil)
				px += adv
				hasText = true
				lineHeight = max(lineHeight, st.line)
			}
			return
		}
		// Words wrap at whitespace; oversized words break at rune boundaries.
		text := n.text
		for len(text) > 0 {
			if e := l.ctx.Err(); e != nil {
				l.err = e
				return
			}
			i := strings.IndexFunc(text, unicode.IsSpace)
			word := text
			if i >= 0 {
				word = text[:i]
			}
			if word != "" {
				space := 0
				if pendingSpace && px > x {
					space = font.MeasureString(f, " ").Ceil()
				}
				ww := font.MeasureString(f, word).Ceil()
				if px > x && px+space+ww > x+w {
					newline()
					space = 0
				}
				px += space
				var run strings.Builder
				runX := px
				emit := func() {
					if run.Len() > 0 {
						l.add(Box{Bounds: image.Rect(runX, y, px, y+st.line), Tag: "text", Text: run.String(), FontSize: st.size, Bold: st.bold, FontFamily: st.family, Italic: st.italic, Underline: st.underline, Strike: st.strike, Color: st.fg, Monospace: st.mono, LinkID: st.linkID}, nil)
						run.Reset()
						hasText = true
					}
				}
				for _, r := range word {
					a, _ := f.GlyphAdvance(r)
					adv := max(1, a.Ceil())
					if px > x && px+adv > x+w {
						emit()
						newline()
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
			text = strings.TrimLeftFunc(text[i:], unicode.IsSpace)
			pendingSpace = true
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
