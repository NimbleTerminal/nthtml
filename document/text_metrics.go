package document

import (
	"image"
	"strings"
	"unicode"

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
func zeroWidth(r rune) bool     { return r == '\u200b' || r == '\ufeff' }
func stripZeroWidth(s string) string {
	return strings.Map(func(r rune) rune {
		if zeroWidth(r) {
			return -1
		}
		return r
	}, s)
}
func glyphAdvanceWithLetter(f font.Face, r rune, spacing, letterSpacing int) int {
	if zeroWidth(r) {
		return 0
	}
	advance, _ := f.GlyphAdvance(r)
	if wordSeparator(r) {
		return max(0, advance.Ceil()+spacing+letterSpacing)
	}
	if letterSpacing != 0 {
		return max(0, advance.Ceil()+letterSpacing)
	}
	return max(1, advance.Ceil())
}
func glyphAdvance(f font.Face, r rune, spacing int) int {
	return glyphAdvanceWithLetter(f, r, spacing, 0)
}
func (l *layout) glyphAdvanceStyled(st style, f font.Face, r rune) int {
	if st.smallCaps && unicode.IsLower(r) {
		sc := st
		sc.size = smallCapSize(st.size)
		return glyphAdvanceWithLetter(l.face(sc), unicode.ToUpper(r), st.wordSpacing, st.letterSpacing)
	}
	return glyphAdvanceWithLetter(f, r, st.wordSpacing, st.letterSpacing)
}
func measureText(f font.Face, text string, spacing int) int {
	width := 0
	for _, r := range text {
		width += glyphAdvance(f, r, spacing)
	}
	return width
}
func (l *layout) measureTextStyled(st style, f font.Face, text string) int {
	width := 0
	for _, r := range text {
		width += l.glyphAdvanceStyled(st, f, r)
	}
	return width
}
func drawText(drawer *font.Drawer, text string, spacing int) {
	drawTextWithLetter(drawer, nil, text, spacing, 0)
}
func drawTextWithLetter(drawer *font.Drawer, smallFace font.Face, text string, spacing, letterSpacing int) {
	text = stripZeroWidth(text)
	if text == "" {
		return
	}
	if spacing == 0 && letterSpacing == 0 && smallFace == nil {
		drawer.DrawString(text)
		return
	}
	baseFace := drawer.Face
	for _, r := range text {
		useFace := baseFace
		drawRune := r
		if smallFace != nil && unicode.IsLower(r) {
			useFace = smallFace
			drawRune = unicode.ToUpper(r)
		}
		drawer.Face = useFace
		drawer.DrawString(string(drawRune))
		advance, _ := useFace.GlyphAdvance(drawRune)
		delta := glyphAdvanceWithLetter(useFace, drawRune, spacing, letterSpacing) - advance.Ceil()
		drawer.Dot.X += fixed.I(delta)
	}
	drawer.Face = baseFace
}
func textInk(face font.Face, b Box) image.Rectangle {
	text := stripZeroWidth(b.Text)
	if text == "" {
		return image.Rectangle{}
	}
	if b.SmallCaps {
		text = strings.ToUpper(text)
	}
	ink, _ := font.BoundString(face, text)
	rect := image.Rect(ink.Min.X.Floor(), ink.Min.Y.Floor(), ink.Max.X.Ceil(), ink.Max.Y.Ceil()).Add(image.Pt(b.Bounds.Min.X, b.Bounds.Min.Y+b.Baseline))
	// Enclose horizontal spacing changes; vertical ink bounds are exact and
	// keep short/zero line heights from clipping glyph overflow on export.
	extra := b.WordSpacing*(strings.Count(text, " ")+strings.Count(text, "\u00a0")) + b.LetterSpacing*len([]rune(text))
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
	if !b.Clip.Empty() {
		b.Clip = b.Clip.Add(delta)
	}
}
func (b Box) paintBottom() int {
	bottom := b.Bounds.Max.Y
	if !b.InkBounds.Empty() {
		bottom = max(bottom, b.InkBounds.Max.Y)
	}
	if !b.Clip.Empty() {
		bottom = min(bottom, b.Clip.Max.Y)
	}
	return bottom
}
