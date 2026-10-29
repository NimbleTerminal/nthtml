package document

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// TTYStyle describes terminal text attributes. A zero-alpha color means the
// terminal default. Nonzero-alpha colors are emitted as opaque RGB.
type TTYStyle struct {
	Foreground, Background                 color.RGBA
	Bold, Italic, Underline, Strikethrough bool
}

// TTYStyleSpan uses UTF-8 byte offsets in Text and terminal-cell bounds.
// Generated spans are sorted, non-overlapping, and never cross a newline.
type TTYStyleSpan struct {
	Start, End int
	Bounds     image.Rectangle
	Style      TTYStyle
}

const ttyStylePrefix = "\x1b]1337;nthtml-style="
const ttyStyleEnd = ttyStylePrefix + "0\x07"

type ttyStyles struct {
	ids    map[TTYStyle]int
	values []TTYStyle
	err    error
}

func ttyTextStyle(s style) TTYStyle {
	return TTYStyle{Foreground: s.fg, Background: s.bg, Bold: s.bold, Italic: s.italic, Underline: s.underline, Strikethrough: s.strike}
}

func (s *ttyStyles) id(st style) int {
	value := ttyTextStyle(st)
	if value == (TTYStyle{}) {
		return 0
	}
	if id := s.ids[value]; id != 0 {
		return id
	}
	if len(s.values) >= 100000 {
		s.err = limitError("TTY styles")
		return 0
	}
	if s.ids == nil {
		s.ids = map[TTYStyle]int{}
		s.values = []TTYStyle{{}}
	}
	id := len(s.values)
	s.ids[value] = id
	s.values = append(s.values, value)
	return id
}
func ttyStyleMarker(id int) string { return ttyStylePrefix + strconv.Itoa(id) + "\x07" }
func (s *ttyStyles) mark(text string, st style) string {
	if ansi.StringWidth(text) == 0 {
		return text
	}
	id := s.id(st)
	if id == 0 {
		return text
	}
	return ttyStyleMarker(id) + text + ttyStyleEnd
}

// Backgrounds cover descendant text cells in TTY; transparent descendants
// retain the nearest colored ancestor, without painting a pixel-sized box.
func (d *Document) ttyStyled(n *node, parent style) style {
	s := d.styled(n, parent)
	if s.bg.A == 0 {
		s.bg = parent.bg
	}
	return s
}

// Table layout collects rows with raster styles. Reconstruct the effective
// terminal background through table/row-group ancestors for each cell.
func (d *Document) ttyParentStyle(n *node) style {
	var parents []*node
	for p := n.parent; p != nil; p = p.parent {
		parents = append(parents, p)
	}
	s := style{size: 16, line: 22}
	for i := len(parents) - 1; i >= 0; i-- {
		s = d.ttyStyled(parents[i], s)
	}
	return s
}

func (f *ttyFlow) styledRuns() []ttyRun {
	runs := make([]ttyRun, len(f.pending.runs))
	copy(runs, f.pending.runs)
	for i := range runs {
		if !runs[i].structural {
			runs[i].text = f.styles.mark(runs[i].text, runs[i].policy)
		}
	}
	return runs
}

func (s TTYStyle) sgr(profile colorprofile.Profile, highlight bool) string {
	attributes := ansi.Style{}
	if s.Bold {
		attributes = attributes.Bold()
	}
	if s.Italic {
		attributes = attributes.Italic(true)
	}
	if s.Underline {
		attributes = attributes.Underline(true)
	}
	if s.Strikethrough {
		attributes = attributes.Strikethrough(true)
	}
	var fg, bg color.Color
	if s.Foreground.A > 0 {
		fg = profile.Convert(color.RGBA{s.Foreground.R, s.Foreground.G, s.Foreground.B, 255})
	}
	if s.Background.A > 0 {
		bg = profile.Convert(color.RGBA{s.Background.R, s.Background.G, s.Background.B, 255})
	}
	if highlight && fg != nil && bg != nil {
		fr, fgg, fb, _ := fg.RGBA()
		br, bgg, bb, _ := bg.RGBA()
		if fr == br && fgg == bgg && fb == bb {
			fg = profile.Convert(color.Black)
			bg = profile.Convert(color.White)
		}
	}
	if fg != nil {
		attributes = attributes.ForegroundColor(fg)
	}
	if bg != nil {
		attributes = attributes.BackgroundColor(bg)
	}
	var out strings.Builder
	if len(attributes) > 0 {
		out.WriteString(attributes.String())
	}
	if highlight {
		out.WriteString("\x1b[7m")
	}
	return out.String()
}

// ANSIWithProfile emits CSS styling and OSC 8 links using an explicit profile.
// ASCII keeps emphasis, inverse highlights and links but omits colors.
// NoTTY and Unknown return escape-free Text. ANSI() defaults to TrueColor.
func (r *TTYResult) ANSIWithProfile(profile colorprofile.Profile) string {
	return r.ansiStyled(nil, profile)
}

// ANSIHighlightsWithProfile composes highlights, CSS styling and OSC 8 links.
func (r *TTYResult) ANSIHighlightsWithProfile(ranges []Match, profile colorprofile.Profile) string {
	return r.ansiStyled(ranges, profile)
}
