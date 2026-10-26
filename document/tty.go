package document

import (
	"context"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

const maxTTYBytes = 4 << 20

type ttyFlow struct {
	policy       style
	pending      ttyBuilder
	parts        []string
	anchorPrefix string
	bytes        int
	width        int
	preColumn    int
}

func (f *ttyFlow) append(s string) error {
	if s == "" {
		return nil
	}
	if ansi.StringWidth(s) == 0 && !strings.Contains(s, "\n") {
		if f.bytes+len(s) > maxTTYBytes {
			return limitError("TTY output")
		}
		if len(f.parts) > 0 && !strings.Contains(s, "nthtml-anchor=") {
			f.parts[len(f.parts)-1] += s
		} else {
			f.anchorPrefix += s
		}
		f.bytes += len(s)
		return nil
	}
	if f.anchorPrefix != "" {
		s = f.anchorPrefix + s
		f.anchorPrefix = ""
	}
	overhead := 0
	if len(f.parts) > 0 {
		overhead = 2
	}
	if f.bytes+len(s)+overhead > maxTTYBytes {
		return limitError("TTY output")
	}
	f.parts = append(f.parts, s)
	f.bytes += len(s) + overhead
	return nil
}

func (f *ttyFlow) joined() string {
	if len(f.parts) == 0 {
		return f.anchorPrefix
	}
	out := strings.Join(f.parts, "\n\n")
	if f.anchorPrefix != "" {
		out += f.anchorPrefix
	}
	return out
}
func (f *ttyFlow) flush() error {
	if f.pending.special {
		text := wrapTTYRuns(f.pending.runs, f.width)
		f.pending.Reset()
		return f.append(text)
	}
	text := f.pending.String()
	if !f.policy.pre {
		text = normalizeTTY(text)
	}
	f.pending.Reset()
	if text == "" {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, wrapTTYLine(line, f.width, f.policy))
	}
	return f.append(strings.Join(lines, "\n"))
}

// TTY returns readable text reflowed to terminal cells. CSS visibility is
// respected. Tables have borders, wrapped cells and CSS text alignment; very
// narrow tables fall back to stacked records. Terminal controls are removed.
// Width is clamped to 2–1000 cells; generated output is capped at 4 MiB.
func (d *Document) TTY(ctx context.Context, width int) (string, error) {
	result, err := d.TTYLayout(ctx, width)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}
func (d *Document) tty(ctx context.Context, width int) (string, error) {
	if d == nil || d.root == nil {
		return "", nil
	}
	f := &ttyFlow{width: max(2, min(1000, width))}
	var walk func(*node, style, *ttyFlow, bool) error
	walk = func(n *node, parent style, f *ttyFlow, inCell bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b := &f.pending
		b.policy = parent
		if n.tag == "" {
			raw := applyTextTransform(n.text, parent.textTransform)
			if parent.pre {
				text, col := preTextAt(raw, f.preColumn)
				f.preColumn = col
				b.WriteString(text)
			} else if parent.whiteSpace == "pre-line" {
				text := strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
				for i, line := range strings.Split(text, "\n") {
					if i > 0 {
						b.WriteString("\n")
					}
					b.WriteString(cleanText(line))
				}
			} else {
				b.WriteString(cleanText(raw))
			}
			return nil
		}
		s := d.styled(n, parent)
		b.policy = s
		if s.display == "none" {
			return nil
		}
		marker, endMarker := "", ""
		if anchorName(n) != "" {
			marker = anchorMarker(n.id)
			endMarker = anchorEndMarker(n.id)
		}
		if kind, source := diagramSource(n); kind != "" {
			if kind == "svg" {
				b.WriteString(marker + "[SVG: " + diagramLabel(n) + "]" + endMarker)
				return nil
			}
			if err := f.flush(); err != nil {
				return err
			}
			return f.append(marker + ansi.Hardwrap(cleanText(source), f.width, true) + endMarker)
		}
		if n.tag == "a" && s.linkID != 0 {
			b.WriteString(marker)
			b.WriteString(linkStart(s.linkID))
			for _, child := range n.children {
				if err := walk(child, s, f, inCell); err != nil {
					return err
				}
			}
			b.WriteString(linkEnd)
			b.WriteString(endMarker)
			if parent.linkID != 0 {
				b.WriteString(linkStart(parent.linkID))
			}
			return nil
		}
		if n.tag == "pre" || s.display != "inline" && s.display != "inline-block" && (s.whiteSpace != parent.whiteSpace || s.overflowWrap != parent.overflowWrap) && n.tag != "table" {
			if err := f.flush(); err != nil {
				return err
			}
			code := &ttyFlow{width: f.width, policy: s}
			code.pending.policy = s
			code.pending.WriteString(marker)
			for _, child := range n.children {
				if err := walk(child, s, code, inCell); err != nil {
					return err
				}
			}
			if n.tag == "pre" {
				for i := range code.pending.runs {
					if ansi.StringWidth(code.pending.runs[i].text) == 0 && !strings.Contains(code.pending.runs[i].text, "\n") {
						continue
					}
					if code.pending.runs[i].text != "" {
						code.pending.runs[i].text = strings.TrimPrefix(code.pending.runs[i].text, "\n")
						break
					}
				}
			}
			code.pending.WriteString(endMarker)
			if err := code.flush(); err != nil {
				return err
			}
			return f.append(code.joined())
		}
		if n.tag == "ul" || n.tag == "ol" {
			if err := f.flush(); err != nil {
				return err
			}
			index := listStart(n)
			step := listStep(n)
			var items []string
			listBytes := 0
			for _, child := range n.children {
				if child.tag != "li" {
					continue
				}
				cs := d.styled(child, s)
				if cs.display == "none" {
					continue
				}
				if n.tag == "ol" {
					index = listNumber(child.attrs["value"], index)
				}
				mStr := formatListMarker(resolveListStyleType(n, child, cs), index)
				liMarker := ""
				if mStr != "" {
					liMarker = mStr + " "
				}
				index += step
				indent := ansi.StringWidth(liMarker)
				innerWidth := max(2, f.width-indent)
				item := &ttyFlow{width: innerWidth}
				if err := walk(child, s, item, inCell); err != nil {
					return err
				}
				if err := item.flush(); err != nil {
					return err
				}
				lines := linkedLines(item.joined())
				var formatted []string
				if f.width-indent < 2 {
					formatted = append(formatted, ansi.Hardwrap(strings.TrimSpace(liMarker), f.width, true))
					liMarker = ""
					indent = 0
				}
				for i, line := range lines {
					prefix := strings.Repeat(" ", indent)
					if i == 0 {
						prefix = liMarker
					}
					formatted = append(formatted, prefix+line)
				}
				itemText := strings.Join(formatted, "\n")
				listBytes += len(itemText) + 1
				if listBytes > maxTTYBytes {
					return limitError("TTY list output")
				}
				items = append(items, itemText)
			}
			return f.append(marker + strings.Join(items, "\n") + endMarker)
		}
		if n.tag == "hr" {
			if err := f.flush(); err != nil {
				return err
			}
			return f.append(marker + strings.Repeat("─", f.width) + endMarker)
		}
		if n.tag == "br" {
			b.WriteString(marker + endMarker)
			b.WriteByte('\n')
			return nil
		}
		if n.tag == "img" {
			b.WriteString(marker + "[image: " + cleanText(n.attrs["alt"]) + "]" + endMarker)
			return nil
		}
		if n.tag == "table" && !inCell {
			if err := f.flush(); err != nil {
				return err
			}
			cellText := func(c *node, st style) (string, error) {
				cell := &ttyFlow{width: f.width}
				if err := walk(c, st, cell, true); err != nil {
					return "", err
				}
				if err := cell.flush(); err != nil {
					return "", err
				}
				return cell.joined(), nil
			}
			text, err := d.ttyTable(ctx, n, s, f.width, cellText)
			if err != nil {
				return err
			}
			return f.append(marker + text + endMarker)
		}
		if n.tag == "tr" {
			b.WriteByte('\n')
			b.WriteString(marker)
			first := true
			for _, c := range n.children {
				if c.tag != "td" && c.tag != "th" {
					continue
				}
				if d.styled(c, s).display == "none" {
					continue
				}
				if !first {
					b.WriteString(" | ")
				}
				cell := &ttyFlow{width: f.width}
				if err := walk(c, s, cell, true); err != nil {
					return err
				}
				if err := cell.flush(); err != nil {
					return err
				}
				b.WriteString(strings.Join(strings.Fields(strings.Join(cell.parts, " ")), " "))
				first = false
			}
			b.WriteString(endMarker)
			b.WriteByte('\n')
			return nil
		}
		block := s.display != "inline" && s.display != "inline-block" && s.display != "inline-flex"
		if block {
			b.WriteByte('\n')
		}
		b.WriteString(marker)
		for _, c := range n.children {
			if err := walk(c, s, f, inCell); err != nil {
				return err
			}
		}
		b.WriteString(endMarker)
		if block {
			b.WriteByte('\n')
		}
		return nil
	}
	if err := walk(d.root, style{size: 16, line: 22}, f, false); err != nil {
		return "", err
	}
	if err := f.flush(); err != nil {
		return "", err
	}
	return strings.Join(linkedLines(f.joined()), "\n"), ctx.Err()
}
func normalizeTTY(text string) string {
	var lines []string
	var prefix string
	lastVisible := -1
	for _, line := range strings.Split(text, "\n") {
		line = collapseTTYSpaces(line)
		if line == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		if ansi.StringWidth(line) == 0 {
			if lastVisible >= 0 {
				lines[lastVisible] += line
			} else {
				prefix += line
			}
			continue
		}
		if prefix != "" {
			line = prefix + line
			prefix = ""
		}
		lastVisible = len(lines)
		lines = append(lines, line)
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return prefix
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func collapseTTYSpaces(line string) string {
	var out strings.Builder
	const (
		stateStart = iota
		stateWord
		statePendingSpace
		stateSpaceEmitted
	)
	state := stateStart
	for len(line) > 0 {
		if strings.HasPrefix(line, "\x1b]") {
			if end := strings.IndexByte(line, '\x07'); end >= 0 {
				seq := line[:end+1]
				isEnd := seq == linkEnd || strings.HasPrefix(seq, "\x1b]1337;nthtml-anchor-end=")
				if !isEnd && state == statePendingSpace {
					out.WriteByte(' ')
					state = stateSpaceEmitted
				}
				out.WriteString(seq)
				line = line[end+1:]
				continue
			}
		}
		if cssSpace(rune(line[0])) {
			if state == stateWord {
				state = statePendingSpace
			}
			line = line[1:]
			continue
		}
		if state == statePendingSpace {
			out.WriteByte(' ')
		}
		state = stateWord
		out.WriteByte(line[0])
		line = line[1:]
	}
	return out.String()
}
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if zeroWidth(r) {
			return -1
		}
		if cssSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// TTY reflows the parsed document retained by Render. Manually constructed
// Results without a parsed document return an empty string.
func (r *Result) TTY(ctx context.Context, width int) (string, error) {
	if r == nil {
		return "", nil
	}
	return r.document.TTY(ctx, width)
}

// TTY preserves explicit no-wrap modes; the widget can pan long lines.
func wrapTTYLine(text string, width int, s style) string {
	if s.whiteSpace == "nowrap" || s.whiteSpace == "pre" {
		return text
	}
	if s.pre {
		return ansi.Hardwrap(text, width, true)
	}
	if s.overflowWrap == "normal" {
		return ansi.Wordwrap(text, width, "")
	}
	return ansi.Wrap(text, width, "")
}
