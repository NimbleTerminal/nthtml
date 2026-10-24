package document

import (
	"github.com/charmbracelet/x/ansi"
	"image"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TextRun maps UTF-8 byte offsets to a visible text rectangle.
type TextRun struct {
	Start, End int
	Bounds     image.Rectangle
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
	var text strings.Builder
	for _, b := range r.Boxes {
		if b.Tag != "text" || b.Text == "" {
			continue
		}
		if text.Len() > 0 {
			text.WriteString(b.TextPrefix)
		}
		start := text.Len()
		text.WriteString(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, b.Text))
		out.Runs = append(out.Runs, TextRun{start, text.Len(), b.Bounds})
	}
	out.Text = text.String()
	return out
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
			out.Runs = append(out.Runs, TextRun{offset, offset + len(line), image.Rect(0, y, ansi.StringWidth(line), y+1)})
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
		if t.Cells {
			a, b := max(start, run.Start), min(end, run.End)
			rect.Min.X += ansi.StringWidth(t.Text[run.Start:a])
			rect.Max.X = run.Bounds.Min.X + ansi.StringWidth(t.Text[run.Start:b])
		}
		if !rect.Empty() {
			regions = append(regions, rect)
		}
	}
	return regions
}

// Hit returns the visible text range nearest an area. Typeset selection snaps
// to painted runs; TTY selection snaps to UTF-8 runes and terminal cell widths.
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
	if !t.Cells {
		return Match{run.Start, run.End}, true
	}
	x := run.Bounds.Min.X
	for offset, r := range t.Text[run.Start:run.End] {
		width := ansi.StringWidth(string(r))
		if x+max(1, width) > area.Min.X {
			return Match{run.Start + offset, run.Start + offset + utf8.RuneLen(r)}, true
		}
		x += width
	}
	return Match{run.End, run.End}, true
}
