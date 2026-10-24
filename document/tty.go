package document

import (
	"context"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

const maxTTYBytes = 4 << 20

type ttyFlow struct {
	pending strings.Builder
	parts   []string
	bytes   int
	width   int
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
			b.WriteString(cleanText(n.text))
			return nil
		}
		s := d.styled(n, parent)
		if s.display == "none" {
			return nil
		}
		if n.tag == "br" {
			b.WriteByte('\n')
			return nil
		}
		if n.tag == "img" {
			b.WriteString("[image: " + cleanText(n.attrs["alt"]) + "]")
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
				return normalizeTTY(cell.pending.String()), nil
			}
			text, err := d.ttyTable(ctx, n, s, f.width, cellText)
			if err != nil {
				return err
			}
			return f.append(text)
		}
		if n.tag == "tr" {
			b.WriteByte('\n')
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
				b.WriteString(strings.Join(strings.Fields(cell.pending.String()), " "))
				first = false
			}
			b.WriteByte('\n')
			return nil
		}
		block := s.display != "inline"
		if block {
			b.WriteByte('\n')
		}
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
	return strings.Join(f.parts, "\n\n"), ctx.Err()
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
