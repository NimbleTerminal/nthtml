package document

import (
	"context"
	"image"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func anchorMarker(id int) string { return "\x1b]1337;nthtml-anchor=" + strconv.Itoa(id) + "\x07" }
func anchorEndMarker(id int) string {
	return "\x1b]1337;nthtml-anchor-end=" + strconv.Itoa(id) + "\x07"
}

const linkEnd = "\x1b]8;;\x07"

func linkStart(id int) string { return "\x1b]8;;nthtml:" + strconv.Itoa(id) + "\x07" }

// TextSpan maps a link to UTF-8 byte offsets in TTYResult.Text and terminal cells.
type TextSpan struct {
	Start, End, LinkID int
	Bounds             image.Rectangle
}
type TTYResult struct {
	Anchors     map[string]image.Point
	AnchorRects map[string]image.Rectangle
	Text        string
	Links       []Link
	Spans       []TextSpan
	lineSources []string
	width       int
}

// TTYLayout returns plain text plus link spans, without terminal escape codes.
// @media width queries are evaluated at columns * 10 CSS pixels (80 cols = 800 px).
func (d *Document) TTYLayout(ctx context.Context, width int) (*TTYResult, error) {
	clampedW := max(2, min(1000, width))
	if d != nil && d.root != nil {
		var err error
		d, err = d.forWidth(ctx, clampedW*ttyWidthPxPerColumn)
		if err != nil {
			return nil, err
		}
	}
	raw, e := d.tty(ctx, clampedW)
	if e != nil {
		return nil, e
	}
	links, e := d.Links(ctx)
	if e != nil {
		return nil, e
	}
	byID := map[int]int{}
	for i, l := range links {
		byID[l.ID] = i
	}
	names := map[int]string{}
	var collect func(*node)
	collect = func(n *node) {
		if name := anchorName(n); name != "" {
			names[n.id] = name
		}
		for _, c := range n.children {
			collect(c)
		}
	}
	collect(d.root)
	result := &TTYResult{
		Anchors:     map[string]image.Point{},
		AnchorRects: map[string]image.Rectangle{},
		width:       clampedW,
	}
	var text strings.Builder
	var active []string
	startY := map[string]int{}
	pendingStart := map[string]bool{}
	curLineSource := ""
	x, y, current := 0, 0, 0
	for len(raw) > 0 {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if strings.HasPrefix(raw, "\x1b]1337;nthtml-anchor-end=") {
			end := strings.IndexByte(raw, '\x07')
			if end < 0 {
				break
			}
			id, _ := strconv.Atoi(strings.TrimPrefix(raw[:end], "\x1b]1337;nthtml-anchor-end="))
			if name := names[id]; name != "" {
				for i := len(active) - 1; i >= 0; i-- {
					if active[i] == name {
						active = append(active[:i], active[i+1:]...)
						break
					}
				}
				if sy, ok := startY[name]; ok {
					endLine := y + 1
					if x == 0 && y > sy {
						endLine = y
					}
					if _, exists := result.AnchorRects[name]; !exists {
						result.AnchorRects[name] = image.Rect(0, sy, clampedW, max(sy+1, endLine))
					}
				}
			}
			raw = raw[end+1:]
			continue
		}
		if strings.HasPrefix(raw, "\x1b]1337;nthtml-anchor=") {
			end := strings.IndexByte(raw, '\x07')
			if end < 0 {
				break
			}
			id, _ := strconv.Atoi(strings.TrimPrefix(raw[:end], "\x1b]1337;nthtml-anchor="))
			if name := names[id]; name != "" {
				if _, ok := result.Anchors[name]; !ok {
					result.Anchors[name] = image.Pt(x, y)
					startY[name] = y
					pendingStart[name] = true
				}
				active = append(active, name)
			}
			raw = raw[end+1:]
			continue
		}
		if strings.HasPrefix(raw, "\x1b]8;;") {
			end := strings.IndexByte(raw, '\x07')
			if end < 0 {
				break
			}
			value := raw[5:end]
			current = 0
			if strings.HasPrefix(value, "nthtml:") {
				current, _ = strconv.Atoi(strings.TrimPrefix(value, "nthtml:"))
			}
			raw = raw[end+1:]
			continue
		}
		if raw[0] == '\n' {
			result.lineSources = append(result.lineSources, curLineSource)
			curLineSource = ""
			text.WriteByte('\n')
			x = 0
			y++
			raw = raw[1:]
			continue
		}
		end := strings.IndexAny(raw, "\x1b\n")
		if end < 0 {
			end = len(raw)
		}
		if end == 0 {
			raw = raw[1:]
			continue
		}
		chunk := raw[:end]
		if curLineSource == "" && len(active) > 0 {
			curLineSource = active[len(active)-1]
		}
		// Structural markers may sit after the previous paragraph's text.
		// Anchor scopes start at their first visible content, not that marker.
		if ansi.StringWidth(chunk) > 0 {
			for _, name := range active {
				if pendingStart[name] {
					result.Anchors[name] = image.Pt(x, y)
					startY[name] = y
					delete(pendingStart, name)
				}
			}
		}
		start := text.Len()
		text.WriteString(chunk)
		w := ansi.StringWidth(chunk)
		if i, ok := byID[current]; ok && w > 0 {
			b := image.Rect(x, y, x+w, y+1)
			if len(result.Spans) >= 100000 {
				return nil, limitError("TTY link spans")
			}
			result.Spans = append(result.Spans, TextSpan{start, text.Len(), current, b})
			links[i].Regions = append(links[i].Regions, b)
		}
		x += w
		raw = raw[end:]
	}
	result.lineSources = append(result.lineSources, curLineSource)
	for name, sy := range startY {
		if _, exists := result.AnchorRects[name]; !exists {
			result.AnchorRects[name] = image.Rect(0, sy, clampedW, max(sy+1, y+1))
		}
	}
	result.Text = text.String()
	for _, link := range links {
		if len(link.Regions) > 0 {
			result.Links = append(result.Links, link)
		}
	}
	return result, nil
}
func (r *Result) TTYLayout(ctx context.Context, width int) (*TTYResult, error) {
	if r == nil {
		return &TTYResult{}, nil
	}
	return r.document.TTYLayout(ctx, width)
}

// ANSI adds OSC 8 hyperlinks. Plain TTY output never contains escape codes.
// Each line independently closes its links, so a viewport can safely crop lines.
func (r *TTYResult) ANSI() string {
	if r == nil {
		return ""
	}
	hrefs := map[int]string{}
	for _, l := range r.Links {
		hrefs[l.ID] = SafeURL(l.Href)
	}
	var out strings.Builder
	pos := 0
	for _, s := range r.Spans {
		if s.Start < pos || s.End > len(r.Text) || s.End < s.Start {
			continue
		}
		out.WriteString(r.Text[pos:s.Start])
		href := hrefs[s.LinkID]
		if out.Len()+(s.End-s.Start)+len(href)+64 > maxTTYBytes {
			return r.Text
		}
		if href != "" {
			out.WriteString(ansi.SetHyperlink(href, "id=nthtml-"+strconv.Itoa(s.LinkID)))
		}
		out.WriteString(r.Text[s.Start:s.End])
		if href != "" {
			out.WriteString(ansi.ResetHyperlink())
		}
		pos = s.End
	}
	if out.Len()+len(r.Text)-pos > maxTTYBytes {
		return r.Text
	}
	out.WriteString(r.Text[pos:])
	return out.String()
}

// Close/reopen internal markers at every physical line boundary, before table
// cells are interleaved. Otherwise a wrapped link could leak into another cell.
func linkedLines(text string) []string {
	var lines []string
	var line strings.Builder
	active := ""
	for len(text) > 0 {
		if strings.HasPrefix(text, "\x1b]8;;") {
			end := strings.IndexByte(text, '\x07')
			if end < 0 {
				break
			}
			seq := text[:end+1]
			if seq == linkEnd {
				active = ""
			} else {
				active = seq
			}
			line.WriteString(seq)
			text = text[end+1:]
			continue
		}
		if strings.HasPrefix(text, "\x1b]1337;nthtml-anchor=") || strings.HasPrefix(text, "\x1b]1337;nthtml-anchor-end=") {
			end := strings.IndexByte(text, '\x07')
			if end < 0 {
				break
			}
			line.WriteString(text[:end+1])
			text = text[end+1:]
			continue
		}
		if text[0] == '\n' {
			if active != "" {
				line.WriteString(linkEnd)
			}
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(active)
			text = text[1:]
			continue
		}
		end := strings.IndexAny(text, "\x1b\n")
		if end < 0 {
			end = len(text)
		}
		if end == 0 {
			text = text[1:]
			continue
		}
		line.WriteString(text[:end])
		text = text[end:]
	}
	if active != "" {
		line.WriteString(linkEnd)
	}
	lines = append(lines, line.String())
	return lines
}
