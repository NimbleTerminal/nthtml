package document

import (
	"context"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type ttyCell struct {
	text, align string
	header      bool
}
type ttyRow []ttyCell

func (d *Document) ttyTable(ctx context.Context, n *node, parent style, width int, cellText func(*node, style) (string, error)) (string, error) {
	var rows []ttyRow
	var captions []string
	columns := 0
	var collect func(*node, style) error
	collect = func(n *node, st style) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, child := range n.children {
			cs := d.styled(child, st)
			if cs.display == "none" {
				continue
			}
			switch child.tag {
			case "thead", "tbody", "tfoot":
				if err := collect(child, cs); err != nil {
					return err
				}
			case "caption":
				text, err := cellText(child, st)
				if err != nil {
					return err
				}
				captions = append(captions, ansi.Wrap(text, width, ""))
			case "tr":
				var row ttyRow
				for _, cell := range child.children {
					if cell.tag != "td" && cell.tag != "th" {
						continue
					}
					s := d.styled(cell, cs)
					if s.display == "none" {
						continue
					}
					text, err := cellText(cell, cs)
					if err != nil {
						return err
					}
					row = append(row, ttyCell{text, s.align, cell.tag == "th"})
				}
				if len(row) > 0 {
					rows = append(rows, row)
					columns = max(columns, len(row))
				}
			}
		}
		return nil
	}
	if err := collect(n, parent); err != nil {
		return "", err
	}
	var out strings.Builder
	write := func(s string) error {
		if out.Len()+len(s) > maxTTYBytes {
			return limitError("TTY table output")
		}
		out.WriteString(s)
		return nil
	}
	for _, caption := range captions {
		if err := write(caption + "\n"); err != nil {
			return "", err
		}
	}
	if columns == 0 {
		return strings.TrimRight(out.String(), "\n"), nil
	}
	// Two content cells per column allow wide Unicode glyphs without overflow.
	// Three additional cells account for padding and dividers.
	if width < 5*columns+1 {
		var headers []string
		if len(rows) > 0 {
			for _, c := range rows[0] {
				if c.header {
					headers = append(headers, strings.Join(strings.Fields(c.text), " "))
				} else {
					headers = append(headers, "")
				}
			}
		}
		for i, row := range rows {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if i > 0 {
				if err := write("\n"); err != nil {
					return "", err
				}
			}
			for j, c := range row {
				text := c.text
				if i > 0 && j < len(headers) && headers[j] != "" {
					text = headers[j] + ": " + text
				}
				if err := write(ansi.Wrap(text, width, "") + "\n"); err != nil {
					return "", err
				}
			}
		}
		return strings.TrimRight(out.String(), "\n"), nil
	}
	desired := make([]int, columns)
	sizes := make([]int, columns)
	for i := range sizes {
		sizes[i] = 2
		desired[i] = 2
	}
	for _, row := range rows {
		for j, c := range row {
			for _, line := range strings.Split(c.text, "\n") {
				desired[j] = max(desired[j], min(width, ansi.StringWidth(line)))
			}
		}
	}
	budget := width - 1 - 3*columns - 2*columns
	for budget > 0 {
		changed := false
		for i := range sizes {
			if sizes[i] < desired[i] && budget > 0 {
				sizes[i]++
				budget--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	border := func(left, mid, right, fill string) error {
		var parts []string
		for _, size := range sizes {
			parts = append(parts, strings.Repeat(fill, size+2))
		}
		return write(left + strings.Join(parts, mid) + right + "\n")
	}
	if err := border("┌", "┬", "┐", "─"); err != nil {
		return "", err
	}
	for i, row := range rows {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		wrapped := make([][]string, columns)
		height := 1
		header := false
		for j, c := range row {
			wrapped[j] = linkedLines(ansi.Wrap(c.text, sizes[j], ""))
			height = max(height, len(wrapped[j]))
			header = header || c.header
		}
		for line := 0; line < height; line++ {
			var cells []string
			for j, size := range sizes {
				text := ""
				if line < len(wrapped[j]) {
					text = wrapped[j][line]
				}
				padding := max(0, size-ansi.StringWidth(text))
				left := 0
				if j < len(row) {
					switch row[j].align {
					case "right":
						left = padding
					case "center":
						left = padding / 2
					}
				}
				cells = append(cells, " "+strings.Repeat(" ", left)+text+strings.Repeat(" ", padding-left)+" ")
			}
			if err := write("│" + strings.Join(cells, "│") + "│\n"); err != nil {
				return "", err
			}
		}
		var err error
		if i == len(rows)-1 {
			err = border("└", "┴", "┘", "─")
		} else if header {
			err = border("╞", "╪", "╡", "═")
		} else {
			err = border("├", "┼", "┤", "─")
		}
		if err != nil {
			return "", err
		}
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
