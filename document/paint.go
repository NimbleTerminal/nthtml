package document

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Bounds is the page in CSS pixels: its width as laid out, and its height.
func (r *Result) Bounds() image.Rectangle {
	if r == nil {
		return image.Rectangle{}
	}
	return image.Rectangle{Max: r.size}
}

// Background is the canvas colour Paint (and Render's Image) fills before
// drawing any box: the propagated root/body background when the page sets one,
// else Options.Colors.Background when the page sets no author colours, else
// white.
func (r *Result) Background() color.RGBA {
	if r == nil || r.bg.A == 0 {
		return color.RGBA{255, 255, 255, 255}
	}
	return r.bg
}

// Paint draws area, a rectangle of the page in CSS pixels, scale device
// pixels to the CSS pixel: a band of a long page as it scrolls by, or a
// high-density picture, without painting the whole page. The image's
// origin is area.Min, at (0, 0). Area is clipped to the page; scale is
// 1/16 to 16, and the image within the document's pixel limit.
func (r *Result) Paint(ctx context.Context, area image.Rectangle, scale float64) (*image.RGBA, error) {
	if r == nil || r.document == nil {
		return nil, limitError("nil result")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	area = area.Intersect(r.Bounds())
	if area.Empty() {
		return nil, limitError("paint area outside the page")
	}
	if !(scale >= 1.0/16 && scale <= 16) {
		return nil, limitError("scale outside 1/16..16")
	}
	w, h := int(math.Ceil(float64(area.Dx())*scale)), int(math.Ceil(float64(area.Dy())*scale))
	pixLimit := r.limits.Pixels
	if pixLimit <= 0 {
		pixLimit = r.document.limits.Pixels
	}
	if w < 1 || h < 1 || w > pixLimit/h {
		return nil, limitError("pixels")
	}
	l := &layout{ctx: ctx, d: r.document, opts: r.opts, lim: r.limits, canvasBg: r.Background(), maxHeight: max(r.size.Y, r.document.limits.Height), faces: map[faceKey]font.Face{}, fonts: map[string]*opentype.Font{}, scale: scale}
	defer func() {
		for _, f := range l.faces {
			f.Close()
		}
	}()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := l.paint(img, r.ops, area, scale); err != nil {
		return nil, err
	}
	return img, nil
}

// paint draws the operations that reach area into dst, whose origin is
// area.Min, scale device pixels to the CSS pixel.
func (l *layout) paint(dst *image.RGBA, ops []operation, area image.Rectangle, scale float64) error {
	bg := l.canvasBg
	if bg.A == 0 {
		bg = color.RGBA{255, 255, 255, 255}
	}
	draw.Draw(dst, dst.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	x := func(v int) int { return int(math.Floor(float64(v-area.Min.X) * scale)) }
	y := func(v int) int { return int(math.Floor(float64(v-area.Min.Y) * scale)) }
	rect := func(r image.Rectangle) image.Rectangle {
		d := image.Rect(x(r.Min.X), y(r.Min.Y), x(r.Max.X), y(r.Max.Y))
		// A line or rule thinner than a device pixel still shows.
		if d.Dx() == 0 && r.Dx() > 0 {
			d.Max.X++
		}
		if d.Dy() == 0 && r.Dy() > 0 {
			d.Max.Y++
		}
		return d
	}
	for _, op := range ops {
		if e := l.ctx.Err(); e != nil {
			return e
		}
		b := op.box
		if !b.Bounds.Union(b.InkBounds).Overlaps(area) {
			continue
		}
		opDst := draw.Image(dst)
		if !b.Clip.Empty() {
			sub := rect(b.Clip).Intersect(dst.Bounds())
			if sub.Empty() {
				continue
			}
			opDst = dst.SubImage(sub).(*image.RGBA)
		}
		switch {
		case op.source != nil:
			xdraw.ApproxBiLinear.Scale(opDst, rect(b.Bounds), op.source, op.source.Bounds(), draw.Over, nil)
		case b.Text != "":
			if b.FontSize == 0 {
				continue
			}
			f := l.face(style{size: b.FontSize, bold: b.Bold, mono: b.Monospace, family: b.FontFamily, italic: b.Italic})
			var scFace font.Face
			if b.SmallCaps {
				scFace = l.face(style{size: smallCapSize(b.FontSize), bold: b.Bold, mono: b.Monospace, family: b.FontFamily, italic: b.Italic})
			}
			dot := fixed.Point26_6{
				X: fixed.Int26_6(math.Round(float64(b.Bounds.Min.X-area.Min.X) * scale * 64)),
				Y: fixed.Int26_6(math.Round(float64(b.Bounds.Min.Y+b.Baseline-area.Min.Y) * scale * 64)),
			}
			if strings.Contains(strings.ToLower(b.FontFamily), "ahem") {
				// Ahem's squares start on a whole pixel.
				dot.Y += f.Metrics().Ascent - fixed.I(f.Metrics().Ascent.Ceil())
			}
			dr := font.Drawer{Dst: opDst, Src: image.NewUniform(b.Color), Face: f, Dot: dot}
			drawTextWithLetter(&dr, scFace, b.Text, int(math.Round(float64(b.WordSpacing)*scale)), int(math.Round(float64(b.LetterSpacing)*scale)))
			if b.Strike {
				// A third of the ascent above the baseline, in CSS pixels.
				yy := b.Bounds.Min.Y + b.Baseline - int(math.Round(float64(f.Metrics().Ascent.Ceil()/3)/scale))
				draw.Draw(opDst, rect(image.Rect(b.Bounds.Min.X, yy, b.Bounds.Max.X, yy+1)), image.NewUniform(b.Color), image.Point{}, draw.Over)
			}
			if b.Underline {
				under := image.Rect(b.Bounds.Min.X, b.Bounds.Min.Y+b.Baseline+1, b.Bounds.Max.X, b.Bounds.Min.Y+b.Baseline+2)
				draw.Draw(opDst, rect(under), image.NewUniform(b.Color), image.Point{}, draw.Over)
			}
		default:
			draw.Draw(opDst, rect(b.Bounds), image.NewUniform(b.Color), image.Point{}, draw.Over)
		}
	}
	return l.err
}
