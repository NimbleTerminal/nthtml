package document

import (
	"image"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func (l *layout) baseline(s style) int {
	if s.size == 0 {
		return 0
	}
	metrics := l.face(s).Metrics()
	baseline := metrics.Ascent.Ceil()
	if s.lineMode != 0 {
		baseline += (s.line - metrics.Height.Ceil()) / 2
	}
	return baseline
}
func wordSeparator(r rune) bool { return r == ' ' || r == '\u00a0' }
func glyphAdvance(f font.Face, r rune, spacing int) int {
	advance, _ := f.GlyphAdvance(r)
	if wordSeparator(r) {
		return max(0, advance.Ceil()+spacing)
	}
	return max(1, advance.Ceil())
}
func measureText(f font.Face, text string, spacing int) int {
	width := 0
	for _, r := range text {
		width += glyphAdvance(f, r, spacing)
	}
	return width
}
func drawText(drawer *font.Drawer, text string, spacing int) {
	if spacing == 0 {
		drawer.DrawString(text)
		return
	}
	start := 0
	for i, r := range text {
		if wordSeparator(r) {
			drawer.DrawString(text[start:i])
			drawer.DrawString(string(r))
			advance, _ := drawer.Face.GlyphAdvance(r)
			delta := glyphAdvance(drawer.Face, r, spacing) - advance.Ceil()
			drawer.Dot.X += fixed.I(delta)
			start = i + len(string(r))
		}
	}
	drawer.DrawString(text[start:])
}
func textInk(face font.Face, b Box) image.Rectangle {
	ink, _ := font.BoundString(face, b.Text)
	rect := image.Rect(ink.Min.X.Floor(), ink.Min.Y.Floor(), ink.Max.X.Ceil(), ink.Max.Y.Ceil()).Add(image.Pt(b.Bounds.Min.X, b.Bounds.Min.Y+b.Baseline))
	// Enclose horizontal spacing changes; vertical ink bounds are exact and
	// keep short/zero line heights from clipping glyph overflow on export.
	extra := b.WordSpacing * (strings.Count(b.Text, " ") + strings.Count(b.Text, "\u00a0"))
	if extra >= 0 {
		rect.Max.X += extra
	} else {
		rect.Min.X += extra
	}
	return rect
}

func (b *Box) translate(delta image.Point) {
	b.Bounds = b.Bounds.Add(delta)
	if !b.InkBounds.Empty() {
		b.InkBounds = b.InkBounds.Add(delta)
	}
}
func (b Box) paintBottom() int {
	bottom := b.Bounds.Max.Y
	if !b.InkBounds.Empty() {
		bottom = max(bottom, b.InkBounds.Max.Y)
	}
	return bottom
}
