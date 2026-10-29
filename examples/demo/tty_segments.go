package main

import (
	"fmt"
	"nimbleterminal.dev/nthtml/document"
	"sort"
)

// Split only at renderer-provided UTF-8 boundaries, preserving links and styles.
func ttySegments(r *document.TTYResult) []any {
	bounds := []int{0, len(r.Text)}
	for _, s := range r.Spans {
		bounds = append(bounds, s.Start, s.End)
	}
	for _, s := range r.Styles {
		bounds = append(bounds, s.Start, s.End)
	}
	sort.Ints(bounds)
	hrefs := map[int]string{}
	for _, l := range r.Links {
		hrefs[l.ID] = l.Href
	}
	out := []any{}
	si, li := 0, 0
	for i := 1; i < len(bounds); i++ {
		start, end := bounds[i-1], bounds[i]
		if start == end {
			continue
		}
		segment := map[string]any{"text": r.Text[start:end]}
		for si < len(r.Styles) && r.Styles[si].End <= start {
			si++
		}
		if si < len(r.Styles) && r.Styles[si].Start <= start {
			s := r.Styles[si].Style
			css := map[string]any{"fontWeight": "normal", "fontStyle": "normal", "textDecoration": "none", "color": "inherit"}
			if s.Foreground.A != 0 {
				css["color"] = fmt.Sprintf("#%02x%02x%02x", s.Foreground.R, s.Foreground.G, s.Foreground.B)
			}
			if s.Background.A != 0 {
				css["backgroundColor"] = fmt.Sprintf("#%02x%02x%02x", s.Background.R, s.Background.G, s.Background.B)
			}
			if s.Bold {
				css["fontWeight"] = "bold"
			}
			if s.Italic {
				css["fontStyle"] = "italic"
			}
			decoration := ""
			if s.Underline {
				decoration = "underline "
			}
			if s.Strikethrough {
				decoration += "line-through"
			}
			if decoration != "" {
				css["textDecoration"] = decoration
			}
			segment["style"] = css
		}
		for li < len(r.Spans) && r.Spans[li].End <= start {
			li++
		}
		if li < len(r.Spans) && r.Spans[li].Start <= start {
			segment["href"] = hrefs[r.Spans[li].LinkID]
			segment["id"] = r.Spans[li].LinkID
		}
		out = append(out, segment)
	}
	return out
}
