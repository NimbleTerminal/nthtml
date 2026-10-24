package document

import (
	"github.com/charmbracelet/x/ansi"
	"sort"
	"strconv"
	"strings"
)

// ANSIHighlights preserves hyperlinks and adds inverse-video byte ranges.
// Each segment resets its state, including at physical line boundaries.
func (r *TTYResult) ANSIHighlights(ranges []Match) string {
	if r == nil {
		return ""
	}
	if len(ranges) == 0 {
		return r.ANSI()
	}
	if len(ranges) > 1001 {
		ranges = ranges[:1001]
	}
	events := map[int]int{}
	points := []int{0, len(r.Text)}
	add := func(p int) { points = append(points, p) }
	index := r.TextIndex()
	for _, v := range ranges {
		a, b := index.Range(v.Start, v.End)
		if a == b {
			continue
		}
		events[a]++
		events[b]--
		add(a)
		add(b)
	}
	for _, s := range r.Spans {
		add(s.Start)
		add(s.End)
	}
	for i, c := range r.Text {
		if c == '\n' {
			add(i)
			add(i + 1)
		}
	}
	sort.Ints(points)
	hrefs := map[int]string{}
	for _, l := range r.Links {
		hrefs[l.ID] = SafeURL(l.Href)
	}
	var out strings.Builder
	active, link, pos := 0, 0, 0
	for i := 0; i < len(points)-1; i++ {
		a, b := points[i], points[i+1]
		if a == b {
			continue
		}
		if a < 0 || b > len(r.Text) {
			continue
		}
		active += events[a]
		for pos < len(r.Spans) && r.Spans[pos].End <= a {
			pos++
		}
		link = 0
		if pos < len(r.Spans) && r.Spans[pos].Start <= a && r.Spans[pos].End >= b {
			link = r.Spans[pos].LinkID
		}
		part := r.Text[a:b]
		href := hrefs[link]
		if part == "\n" {
			out.WriteString(part)
			continue
		}
		if href != "" {
			out.WriteString(ansi.SetHyperlink(href, "id=nthtml-"+strconv.Itoa(link)))
		}
		if active > 0 {
			out.WriteString("\x1b[7m")
		}
		out.WriteString(part)
		if active > 0 {
			out.WriteString("\x1b[0m")
		}
		if href != "" {
			out.WriteString(ansi.ResetHyperlink())
		}
		if out.Len() > maxTTYBytes {
			return r.Text
		}
	}
	return out.String()
}
