package document

import "strconv"

type cellSpan struct{ col, cols, rows int }

// collectTable builds the same bounded logical grid for raster and TTY layout.
// Row spans stop at row-group boundaries; rowspan=0 reaches that group's end.
func (d *Document) collectTable(n *node, s style, step func() bool) ([]tableRow, int, error) {
	var rows []tableRow
	var collect func(*node, style)
	collect = func(parent *node, ps style) {
		first := len(rows)
		for _, child := range parent.children {
			if !step() {
				return
			}
			cs := d.styled(child, ps)
			if cs.display == "none" {
				continue
			}
			switch child.tag {
			case "thead", "tbody", "tfoot":
				start := len(rows)
				for i := first; i < start; i++ {
					if rows[i].groupEnd == 0 {
						rows[i].groupEnd = start
					}
				}
				collect(child, cs)
				if name := anchorName(child); name != "" && start < len(rows) {
					rows[start].anchors = append(rows[start].anchors, name)
					rows[start].anchorIDs = append(rows[start].anchorIDs, child.id)
				}
			case "tr":
				if cs.bg.A == 0 {
					cs.bg = ps.bg
				}
				row := tableRow{style: cs}
				if name := anchorName(child); name != "" {
					row.anchors = append(row.anchors, name)
					row.anchorIDs = append(row.anchorIDs, child.id)
				}
				for _, cell := range child.children {
					if !step() {
						return
					}
					if cell.tag != "td" && cell.tag != "th" {
						continue
					}
					st := d.styled(cell, cs)
					if st.display == "none" {
						continue
					}
					row.cells = append(row.cells, cell)
					row.styles = append(row.styles, st)
				}
				rows = append(rows, row)
			}
		}
		for i := first; i < len(rows); i++ {
			if rows[i].groupEnd == 0 {
				rows[i].groupEnd = len(rows)
			}
		}
	}
	collect(n, s)
	occupied := make([]int, 256)
	cols := 0
	work := 0
	for ri := range rows {
		cursor := 0
		for _, cell := range rows[ri].cells {
			if !step() {
				return nil, 0, limitError("table grid")
			}
			cs, rs := spanValue(cell.attrs["colspan"], false), spanValue(cell.attrs["rowspan"], true)
			if cs > 256 {
				return nil, 0, limitError("table columns")
			}
			if rs == 0 {
				rs = rows[ri].groupEnd - ri
			} else {
				rs = min(rs, rows[ri].groupEnd-ri)
			}
			for {
				if cursor+cs > 256 {
					return nil, 0, limitError("table columns")
				}
				free := true
				for c := cursor; c < cursor+cs; c++ {
					if occupied[c] > ri {
						cursor = c + 1
						free = false
						break
					}
				}
				if free {
					break
				}
			}
			work += cs * rs
			if work > 100000 {
				return nil, 0, limitError("table grid slots")
			}
			rows[ri].spans = append(rows[ri].spans, cellSpan{cursor, cs, rs})
			for c := cursor; c < cursor+cs; c++ {
				occupied[c] = ri + rs
			}
			cursor += cs
			cols = max(cols, cursor)
		}
	}
	if len(rows)*cols > 100000 {
		return nil, 0, limitError("table grid slots")
	}
	return rows, cols, nil
}
func spanValue(s string, zero bool) int {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 1
	}
	if n == 0 && !zero {
		return 1
	}
	if s == "" {
		return 1
	}
	return int(min(n, 100000))
}
func spreadRequirement(widths []int, col, span, needed int) {
	total := 0
	for _, w := range widths[col : col+span] {
		total += w
	}
	if needed <= total {
		return
	}
	deficit := needed - total
	for i := 0; i < span; i++ {
		widths[col+i] += deficit / span
		if i < deficit%span {
			widths[col+i]++
		}
	}
}
