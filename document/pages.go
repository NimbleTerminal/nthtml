package document

// PageBreak gives where a page that starts at top and may run to limit
// should end: the lowest y no higher than limit that cuts through no line
// of text, list marker or image. When the only such place would leave the
// page less than half full (a picture or a cell taller than that), it
// gives limit and the cut goes through it.
func (r *Result) PageBreak(top, limit int) int {
	if r == nil || r.Image == nil || limit >= r.Image.Bounds().Max.Y {
		if r != nil && r.Image != nil {
			return min(limit, r.Image.Bounds().Max.Y)
		}
		return limit
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
			if b.Bounds.Min.Y < y && y < b.Bounds.Max.Y {
				if b.Bounds.Min.Y <= floor {
					return limit
				}
				y, moved = b.Bounds.Min.Y, true
			}
		}
	}
	return y
}
