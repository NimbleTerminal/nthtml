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
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Box describes a painted rectangle or text run in document pixel coordinates.
// Text and bounds can also be consumed by a host's text renderer (e.g. nttypeset).
type Box struct {
	Bounds    image.Rectangle
	Tag, Text string
	FontSize  int
	Bold      bool
	Color     color.RGBA
}
type Result struct {
	Image *image.RGBA
	Boxes []Box
}
type operation struct {
	box    Box
	source image.Image
}
type faceKey struct {
	size int
	bold bool
}
type layout struct {
	ctx           context.Context
	d             *Document
	opts          Options
	ops           []operation
	faces         map[faceKey]font.Face
	regular, bold *opentype.Font
	err           error
}

func (l *layout) face(s style) font.Face {
	k := faceKey{s.size, s.bold}
	if f := l.faces[k]; f != nil {
		return f
	}
	t := l.regular
	if s.bold {
		t = l.bold
	}
	f, e := opentype.NewFace(t, &opentype.FaceOptions{Size: float64(s.size), DPI: 72, Hinting: font.HintingFull})
	if e != nil {
		l.err = e
		return nil
	}
	l.faces[k] = f
	return f
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
	l.regular, _ = opentype.Parse(goregular.TTF)
	l.bold, _ = opentype.Parse(gobold.TTF)
	defer func() {
		for _, f := range l.faces {
			f.Close()
		}
	}()
	h := l.block(d.root, 0, 0, o.Width, style{size: 16, line: 22, fg: color.RGBA{30, 36, 46, 255}})
	if l.err != nil {
		return nil, l.err
	}
	h = max(1, h)
	if h > d.limits.Height || o.Width > d.limits.Pixels/h {
		return nil, limitError("pixels")
	}
	img := image.NewRGBA(image.Rect(0, 0, o.Width, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	result := &Result{Image: img, Boxes: make([]Box, 0, len(l.ops))}
	for _, op := range l.ops {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		b := op.box
		result.Boxes = append(result.Boxes, b)
		if op.source != nil {
			xdraw.ApproxBiLinear.Scale(img, b.Bounds, op.source, op.source.Bounds(), draw.Over, nil)
		} else if b.Text != "" {
			f := l.face(style{size: b.FontSize, bold: b.Bold})
			dr := font.Drawer{Dst: img, Src: image.NewUniform(b.Color), Face: f, Dot: fixed.P(b.Bounds.Min.X, b.Bounds.Min.Y+f.Metrics().Ascent.Ceil())}
			dr.DrawString(b.Text)
		} else {
			draw.Draw(img, b.Bounds, image.NewUniform(b.Color), image.Point{}, draw.Over)
		}
	}
	return result, nil
}

func (l *layout) block(n *node, x, y, w int, parent style) int {
	if l.err != nil {
		return y
	}
	if e := l.ctx.Err(); e != nil {
		l.err = e
		return y
	}
	s := l.d.styled(n, parent)
	if s.display == "none" {
		return y
	}
	x += s.margin
	y += s.margin
	w = max(1, w-2*s.margin)
	if s.width > 0 {
		w = min(w, s.width+2*(s.padding+s.bw))
	}
	start := y
	opStart := len(l.ops)
	l.add(Box{Tag: n.tag}, nil) // reserve background below descendants
	inset := s.padding + s.bw
	cx, cy, cw := x+inset, y+inset, max(1, w-2*inset)
	if n.tag == "img" {
		src := l.opts.Images[n.attrs["src"]]
		if src != nil && !src.Bounds().Empty() {
			h := s.height
			if h == 0 {
				h = int(float64(cw) * float64(src.Bounds().Dy()) / float64(src.Bounds().Dx()))
			}
			h = min(h, l.d.limits.Height)
			l.add(Box{Bounds: image.Rect(cx, cy, cx+cw, cy+h), Tag: "img"}, src)
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
			cy = max(cy, l.block(c, cx+left, y+inset, max(1, right-left), s))
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
				cy = l.block(c, cx, cy, cw, s)
			}
		}
		flush()
	}
	cy = max(cy, y+inset+s.height) + inset
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
	if cy+s.margin > l.d.limits.Height {
		l.err = limitError("height")
	}
	return cy + s.margin
}
func (l *layout) flow(nodes []*node, x, y, w int, s style) int {
	px := x
	lineHeight := s.line
	lineStart := len(l.ops)
	hasText := false
	pendingSpace := false
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
						l.add(Box{Bounds: image.Rect(runX, y, px, y+st.line), Tag: "text", Text: run.String(), FontSize: st.size, Bold: st.bold, Color: st.fg}, nil)
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
