package document

import (
	"context"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

const maxTTYBytes = 4 << 20

type ttyFlow struct {
	pending   strings.Builder
	parts     []string
	bytes     int
	width     int
	preColumn int
}

func (f *ttyFlow) append(s string) error {
	if s == "" {
		return nil
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
func (f *ttyFlow) flush() error {
	text := normalizeTTY(f.pending.String())
	f.pending.Reset()
	if text == "" {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, ansi.Wrap(line, f.width, ""))
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
		if n.tag == "" {
			if parent.pre {
				text, col := preTextAt(n.text, f.preColumn)
				f.preColumn = col
				b.WriteString(text)
			} else {
				b.WriteString(cleanText(n.text))
			}
			return nil
		}
		s := d.styled(n, parent)
		if s.display == "none" {
			return nil
		}
		marker := ""
		if anchorName(n) != "" {
			marker = anchorMarker(n.id)
		}
		if kind, source := diagramSource(n); kind != "" {
			if kind == "svg" {
				b.WriteString(marker + "[SVG: " + diagramLabel(n) + "]")
				return nil
			}
			if err := f.flush(); err != nil {
				return err
			}
			return f.append(marker + ansi.Hardwrap(cleanText(source), f.width, true))
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
			if parent.linkID != 0 {
				b.WriteString(linkStart(parent.linkID))
			}
			return nil
		}
		if n.tag == "pre" {
			if err := f.flush(); err != nil {
				return err
			}
			code := &ttyFlow{width: f.width}
			for _, child := range n.children {
				if err := walk(child, s, code, true); err != nil {
					return err
				}
			}
			text := strings.TrimPrefix(code.pending.String(), "\n")
			// Preserve whitespace; soft-wrap long lines without collapsing indentation.
			return f.append(marker + strings.Join(linkedLines(ansi.Hardwrap(text, f.width, true)), "\n"))
		}
		if n.tag == "ul" || n.tag == "ol" {
			if err := f.flush(); err != nil {
				return err
			}
			index := listStart(n)
			var items []string
			listBytes := 0
			for _, child := range n.children {
				if child.tag != "li" {
					continue
				}
				if d.styled(child, s).display == "none" {
					continue
				}
				if n.tag == "ol" {
					index = listNumber(child.attrs["value"], index)
				}
				marker := listMarker(n, index) + " "
				index++
				indent := ansi.StringWidth(marker)
				innerWidth := max(2, f.width-indent)
				item := &ttyFlow{width: innerWidth}
				if err := walk(child, s, item, inCell); err != nil {
					return err
				}
				if err := item.flush(); err != nil {
					return err
				}
				lines := linkedLines(strings.Join(item.parts, "\n\n"))
				var formatted []string
				if f.width-indent < 2 {
					formatted = append(formatted, ansi.Hardwrap(strings.TrimSpace(marker), f.width, true))
					marker = ""
					indent = 0
				}
				for i, line := range lines {
					prefix := strings.Repeat(" ", indent)
					if i == 0 {
						prefix = marker
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
			return f.append(marker + strings.Join(items, "\n"))
		}
		if n.tag == "br" {
			b.WriteString(marker)
			b.WriteByte('\n')
			return nil
		}
		if n.tag == "img" {
			b.WriteString(marker + "[image: " + cleanText(n.attrs["alt"]) + "]")
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
				return strings.Join(cell.parts, "\n\n"), nil
			}
			text, err := d.ttyTable(ctx, n, s, f.width, cellText)
			if err != nil {
				return err
			}
			return f.append(marker + text)
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
			b.WriteByte('\n')
			return nil
		}
		block := s.display != "inline"
		if block {
			b.WriteByte('\n')
		}
		b.WriteString(marker)
		for _, c := range n.children {
			if err := walk(c, s, f, inCell); err != nil {
				return err
			}
		}
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
	return strings.Join(linkedLines(strings.Join(f.parts, "\n\n")), "\n"), ctx.Err()
}
func normalizeTTY(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
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
