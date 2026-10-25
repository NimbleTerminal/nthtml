package document

import (
	"context"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type ttyCell struct {
	text, align string
	header      bool
	row         int
	span        cellSpan
	lines       []string
}

func (d *Document) ttyTable(ctx context.Context, n *node, parent style, width int, cellText func(*node, style) (string, error)) (string, error) {
	work := 0
	rows, columns, err := d.collectTable(n, parent, func() bool { work++; return ctx.Err() == nil && work <= 100000 })
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	if work > 100000 {
		return "", limitError("TTY table grid")
	}
	var out strings.Builder
	write := func(s string) error {
		if out.Len()+len(s) > maxTTYBytes {
			return limitError("TTY table output")
		}
		out.WriteString(s)
		return nil
	}
	isBottomCaption := func(c *node) bool {
		cs := d.styled(c, parent)
		return cs.captionSide == "bottom" || (cs.captionSide == "" && parent.captionSide == "bottom")
	}
	for _, child := range n.children {
		if child.tag == "caption" && d.styled(child, parent).display != "none" && !isBottomCaption(child) {
			text, err := cellText(child, parent)
			if err != nil {
				return "", err
			}
			if err = write(ansi.Wrap(text, width, "") + "\n"); err != nil {
				return "", err
			}
		}
	}
	if columns == 0 {
		for _, child := range n.children {
			if child.tag == "caption" && d.styled(child, parent).display != "none" && isBottomCaption(child) {
				text, err := cellText(child, parent)
				if err != nil {
					return "", err
				}
				if err = write(ansi.Wrap(text, width, "") + "\n"); err != nil {
					return "", err
				}
			}
		}
		return strings.TrimRight(out.String(), "\n"), nil
	}
	var cells []ttyCell
	owners := make([][]int, len(rows))
	for r := range owners {
		owners[r] = make([]int, columns)
		for c := range owners[r] {
			owners[r][c] = -1
		}
	}
	textBytes := 0
	for r, row := range rows {
		for i, cell := range row.cells {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			text, err := cellText(cell, row.style)
			if err != nil {
				return "", err
			}
			if i == 0 {
				for _, id := range row.anchorIDs {
					text = anchorMarker(id) + text
				}
			}
			textBytes += len(text)
			if textBytes > maxTTYBytes {
				return "", limitError("TTY table text")
			}
			sp := row.spans[i]
			id := len(cells)
			cells = append(cells, ttyCell{text: text, align: row.styles[i].align, header: cell.tag == "th", row: r, span: sp})
			for rr := r; rr < r+sp.rows; rr++ {
				for c := sp.col; c < sp.col+sp.cols; c++ {
					owners[rr][c] = id
				}
			}
		}
	}
	// Narrow terminals use each source cell once, with logical-column headers.
	if width < 5*columns+1 {
		headers := make([]string, columns)
		for _, c := range cells {
			if c.row == 0 && c.header {
				for col := c.span.col; col < c.span.col+c.span.cols; col++ {
					headers[col] = strings.Join(strings.Fields(c.text), " ")
				}
			}
		}
		last := -1
		for _, c := range cells {
			if c.row != last && last >= 0 {
				if err := write("\n"); err != nil {
					return "", err
				}
			}
			last = c.row
			text := c.text
			if c.row > 0 && headers[c.span.col] != "" {
				text = headers[c.span.col] + ": " + text
			}
			if err := write(ansi.Wrap(text, width, "") + "\n"); err != nil {
				return "", err
			}
		}
		return strings.TrimRight(out.String(), "\n"), nil
	}
	desired, sizes := make([]int, columns), make([]int, columns)
	for i := range sizes {
		sizes[i] = 2
		desired[i] = 2
	}
	for _, c := range cells {
		need := 2
		for _, line := range strings.Split(c.text, "\n") {
			need = max(need, min(width, ansi.StringWidth(line)))
		}
		spreadRequirement(desired, c.span.col, c.span.cols, max(2, need-3*(c.span.cols-1)))
	}
	budget := width - 1 - 5*columns
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
	cellWidth := func(sp cellSpan) int {
		w := 3 * (sp.cols - 1)
		for _, v := range sizes[sp.col : sp.col+sp.cols] {
			w += v
		}
		return w
	}
	heights := make([]int, len(rows))
	for i := range heights {
		heights[i] = 1
	}
	for i := range cells {
		c := &cells[i]
		c.lines = linkedLines(ansi.Wrap(c.text, cellWidth(c.span), ""))
		if c.span.rows == 1 {
			heights[c.row] = max(heights[c.row], len(c.lines))
		}
	}
	for _, c := range cells {
		spreadRequirement(heights, c.row, c.span.rows, len(c.lines))
	}
	vertical := func(r, c int) bool {
		if r < 0 || r >= len(rows) {
			return false
		}
		return c == 0 || c == columns || owners[r][c-1] != owners[r][c] || owners[r][c] < 0
	}
	horizontal := func(r, c int) bool {
		if c < 0 || c >= columns {
			return false
		}
		return r == 0 || r == len(rows) || owners[r-1][c] != owners[r][c] || owners[r][c] < 0
	}
	separator := func(r int) error {
		heavy := false
		if r > 0 && r < len(rows) {
			for _, c := range cells {
				if c.row == r-1 && c.header {
					heavy = true
					break
				}
			}
		}
		var line strings.Builder
		for c := 0; c <= columns; c++ {
			mask := 0
			if horizontal(r, c-1) {
				mask |= 1
			}
			if horizontal(r, c) {
				mask |= 2
			}
			if vertical(r-1, c) {
				mask |= 4
			}
			if vertical(r, c) {
				mask |= 8
			}
			glyph := map[int]string{0: " ", 1: "─", 2: "─", 3: "─", 4: "│", 8: "│", 12: "│", 10: "┌", 9: "┐", 6: "└", 5: "┘", 11: "┬", 7: "┴", 14: "├", 13: "┤", 15: "┼"}[mask]
			if glyph == "" {
				glyph = "│"
			}
			if heavy {
				switch glyph {
				case "─":
					glyph = "═"
				case "├":
					glyph = "╞"
				case "┤":
					glyph = "╡"
				case "┼":
					glyph = "╪"
				}
			}
			line.WriteString(glyph)
			if c < columns {
				fill := " "
				if horizontal(r, c) {
					fill = "─"
					if heavy {
						fill = "═"
					}
				}
				line.WriteString(strings.Repeat(fill, sizes[c]+2))
			}
		}
		return write(line.String() + "\n")
	}
	if err := separator(0); err != nil {
		return "", err
	}
	for r := range rows {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		for line := 0; line < heights[r]; line++ {
			var text strings.Builder
			text.WriteString("│")
			for col := 0; col < columns; {
				id := owners[r][col]
				span := cellSpan{col: col, cols: 1}
				value, align := "", ""
				if id >= 0 {
					c := cells[id]
					span = c.span
					align = c.align
					offset := line
					for rr := c.row; rr < r; rr++ {
						offset += heights[rr]
					}
					if offset < len(c.lines) {
						value = c.lines[offset]
					}
				}
				size := cellWidth(span)
				padding := max(0, size-ansi.StringWidth(value))
				left := 0
				if align == "right" {
					left = padding
				} else if align == "center" {
					left = padding / 2
				}
				text.WriteString(" " + strings.Repeat(" ", left) + value + strings.Repeat(" ", padding-left) + " │")
				col += span.cols
			}
			if err := write(text.String() + "\n"); err != nil {
				return "", err
			}
		}
		if err := separator(r + 1); err != nil {
			return "", err
		}
	}
	for _, child := range n.children {
		if child.tag == "caption" && d.styled(child, parent).display != "none" && isBottomCaption(child) {
			text, err := cellText(child, parent)
			if err != nil {
				return "", err
			}
			if err = write(ansi.Wrap(text, width, "") + "\n"); err != nil {
				return "", err
			}
		}
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
