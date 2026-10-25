package document

// PageBreak gives where a page that starts at top and may run to limit
// should end: the lowest y no higher than limit that cuts through no line
// of text, list marker or image. When the only such place would leave the
// page less than half full (a picture or a cell taller than that), it
// gives limit and the cut goes through it.
func (r *Result) PageBreak(top, limit int) int {
	if r == nil || r.size.Y == 0 {
		return limit
	}
	if limit >= r.size.Y {
		return min(limit, r.size.Y)
	}
	floor := top + (limit-top)/2
	y := limit
	for moved := true; moved; {
		moved = false
		for _, b := range r.Boxes {
			switch b.Tag {
			case "text", "marker", "img":
			default:
				continue
			}
			bounds := b.Bounds.Union(b.InkBounds)
			if bounds.Min.Y < y && y < bounds.Max.Y {
				if bounds.Min.Y <= floor {
					return limit
				}
				y, moved = bounds.Min.Y, true
			}
		}
	}
	return y
}
