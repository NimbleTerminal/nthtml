package document

import (
	"context"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"image"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TextRun maps UTF-8 byte offsets to a visible text rectangle.
// For Typeset runs, CharX holds the right-edge X coordinate of each rune in
// Text[Start:End].
type TextRun struct {
	Start, End int
	Bounds     image.Rectangle
	CharX      []int
}

// TextIndex is a layout-specific text snapshot. Typeset runs are words or
// styled fragments; TTY runs are lines. Offsets must not cross layout snapshots.
type TextIndex struct {
	Text  string
	Runs  []TextRun
	Cells bool
}
type Match struct{ Start, End int }

func anchorName(n *node) string {
	if id := n.attrs["id"]; id != "" {
		return id
	}
	if n.tag == "a" {
		return n.attrs["name"]
	}
	return ""
}

// Fragment decodes local-only fragment destinations, including "#" for top.
func Fragment(href string) (string, bool) {
	if !strings.HasPrefix(href, "#") {
		return "", false
	}
	name, err := url.PathUnescape(href[1:])
	return name, err == nil
}
func (r *Result) Fragment(href string) (image.Point, bool) {
	if r == nil {
		return image.Point{}, false
	}
	name, ok := Fragment(href)
	if !ok {
		return image.Point{}, false
	}
	if name == "" {
		return image.Point{}, true
	}
	p, ok := r.Anchors[name]
	return p, ok
}
func (r *TTYResult) Fragment(href string) (image.Point, bool) {
	if r == nil {
		return image.Point{}, false
	}
	name, ok := Fragment(href)
	if !ok {
		return image.Point{}, false
	}
	if name == "" {
		return image.Point{}, true
	}
	p, ok := r.Anchors[name]
	return p, ok
}
func (r *Result) TextIndex() *TextIndex {
	out := &TextIndex{}
	if r == nil {
		return out
	}
	var l *layout
	if r.document != nil {
		l = &layout{
			ctx:   context.Background(),
			d:     r.document,
			opts:  r.opts,
			lim:   r.limits,
			faces: map[faceKey]font.Face{},
			fonts: map[string]*opentype.Font{},
		}
		defer func() {
			for _, f := range l.faces {
				f.Close()
			}
		}()
	}
	var text strings.Builder
	for _, b := range r.Boxes {
		if b.Tag != "text" || b.Text == "" {
			continue
		}
		if text.Len() > 0 {
			text.WriteString(b.TextPrefix)
		}
		start := text.Len()
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, b.Text)
		text.WriteString(clean)
		bounds := b.Bounds
		if bounds.Empty() {
			bounds = b.InkBounds
		}
		out.Runs = append(out.Runs, TextRun{
			Start:  start,
			End:    text.Len(),
			Bounds: bounds,
			CharX:  measureRunCharX(l, b, clean, bounds),
		})
	}
	out.Text = text.String()
	return out
}

func measureRunCharX(l *layout, b Box, clean string, bounds image.Rectangle) []int {
	n := utf8.RuneCountInString(clean)
	if n == 0 {
		return nil
	}
	charX := make([]int, 0, n)
	if l != nil && b.FontSize > 0 {
		st := style{
			size:          b.FontSize,
			bold:          b.Bold,
			mono:          b.Monospace,
			family:        b.FontFamily,
			italic:        b.Italic,
			smallCaps:     b.SmallCaps,
			wordSpacing:   b.WordSpacing,
			letterSpacing: b.LetterSpacing,
		}
		f := l.face(st)
		x := bounds.Min.X
		for _, ch := range clean {
			x += l.glyphAdvanceStyled(st, f, ch)
			charX = append(charX, min(bounds.Max.X, max(bounds.Min.X, x)))
		}
		if len(charX) > 0 {
			charX[len(charX)-1] = bounds.Max.X
		}
		return charX
	}
	for i := 0; i < n; i++ {
		charX = append(charX, bounds.Min.X+(i+1)*bounds.Dx()/n)
	}
	return charX
}

func (r *TTYResult) TextIndex() *TextIndex {
	out := &TextIndex{Cells: true}
	if r == nil {
		return out
	}
	out.Text = r.Text
	offset := 0
	for y, line := range strings.Split(r.Text, "\n") {
		if line != "" {
			out.Runs = append(out.Runs, TextRun{Start: offset, End: offset + len(line), Bounds: image.Rect(0, y, ansi.StringWidth(line), y+1)})
		}
		offset += len(line) + 1
	}
	return out
}

// Search is literal, Unicode simple-case-insensitive, and collapses whitespace.
// It scans in linear time, caps queries at 256 runes, and returns at most 1000 matches.
func (t *TextIndex) Search(query string) []Match {
	if t == nil || len(query) > 1024 || utf8.RuneCountInString(query) > 256 {
		return nil
	}
	q := []rune(strings.ToLower(strings.Join(strings.Fields(query), " ")))
	if len(q) == 0 || len(q) > 256 {
		return nil
	}
	failure := make([]int, len(q))
	for i, j := 1, 0; i < len(q); i++ {
		for j > 0 && q[i] != q[j] {
			j = failure[j-1]
		}
		if q[i] == q[j] {
			j++
		}
		failure[i] = j
	}
	starts := make([]int, len(q))
	matched, count := 0, 0
	space := false
	var out []Match
	for offset, r := range t.Text {
		end := offset + utf8.RuneLen(r)
		if unicode.IsSpace(r) {
			if space {
				continue
			}
			r = ' '
			space = true
		} else {
			space = false
			r = unicode.ToLower(r)
		}
		starts[count%len(q)] = offset
		count++
		for matched > 0 && r != q[matched] {
			matched = failure[matched-1]
		}
		if r == q[matched] {
			matched++
		}
		if matched == len(q) {
			out = append(out, Match{starts[(count-len(q))%len(q)], end})
			if len(out) == 1000 {
				return out
			}
			matched = failure[matched-1]
		}
	}
	return out
}
func (t *TextIndex) Range(start, end int) (int, int) {
	if start > end {
		start, end = end, start
	}
	start = max(0, min(len(t.Text), start))
	end = max(0, min(len(t.Text), end))
	for start > 0 && start < len(t.Text) && !utf8.RuneStart(t.Text[start]) {
		start--
	}
	for end < len(t.Text) && !utf8.RuneStart(t.Text[end]) {
		end++
	}
	return start, end
}
func (t *TextIndex) Selected(start, end int) string {
	if t == nil {
		return ""
	}
	start, end = t.Range(start, end)
	return t.Text[start:end]
}
func (t *TextIndex) Regions(start, end int) []image.Rectangle {
	if t == nil {
		return nil
	}
	start, end = t.Range(start, end)
	var regions []image.Rectangle
	for _, run := range t.Runs {
		if run.End <= start || run.Start >= end {
			continue
		}
		rect := run.Bounds
		a, b := max(start, run.Start), min(end, run.End)
		if t.Cells {
			rect.Min.X += ansi.StringWidth(t.Text[run.Start:a])
			rect.Max.X = run.Bounds.Min.X + ansi.StringWidth(t.Text[run.Start:b])
		} else if a > run.Start || b < run.End {
			sub := t.Text[run.Start:run.End]
			nRunes := max(1, utf8.RuneCountInString(sub))
			x := run.Bounds.Min.X
			minX, maxX := run.Bounds.Min.X, run.Bounds.Max.X
			i := 0
			for offset, r := range sub {
				pos := run.Start + offset
				var nextX int
				if i < len(run.CharX) {
					nextX = run.CharX[i]
				} else {
					nextX = run.Bounds.Min.X + (i+1)*run.Bounds.Dx()/nRunes
				}
				if pos == a {
					minX = x
				}
				if pos+utf8.RuneLen(r) == b {
					maxX = nextX
				}
				x = nextX
				i++
			}
			rect.Min.X = minX
			rect.Max.X = max(minX+1, maxX)
		}
		if !rect.Empty() {
			regions = append(regions, rect)
		}
	}
	return regions
}

// Hit returns the visible character range nearest an area in both Typeset and
// TTY modes.
func (t *TextIndex) Hit(area image.Rectangle) (Match, bool) {
	if t == nil || area.Empty() {
		return Match{}, false
	}
	best := -1
	distance := int(^uint(0) >> 1)
	for i, run := range t.Runs {
		dx := max(0, run.Bounds.Min.X-area.Max.X+1, area.Min.X-run.Bounds.Max.X+1)
		dy := max(0, run.Bounds.Min.Y-area.Max.Y+1, area.Min.Y-run.Bounds.Max.Y+1)
		d := dy*100000 + dx
		if d < distance {
			distance = d
			best = i
		}
	}
	if best < 0 {
		return Match{}, false
	}
	run := t.Runs[best]
	sub := t.Text[run.Start:run.End]
	nRunes := max(1, utf8.RuneCountInString(sub))
	x := run.Bounds.Min.X
	i := 0
	for offset, r := range sub {
		var nextX int
		if t.Cells {
			nextX = x + ansi.StringWidth(string(r))
		} else if i < len(run.CharX) {
			nextX = run.CharX[i]
		} else {
			nextX = run.Bounds.Min.X + (i+1)*run.Bounds.Dx()/nRunes
		}
		if max(x+1, nextX) > area.Min.X {
			return Match{run.Start + offset, run.Start + offset + utf8.RuneLen(r)}, true
		}
		x = nextX
		i++
	}
	return Match{run.End, run.End}, true
}
