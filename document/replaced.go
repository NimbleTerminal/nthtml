package document

import "image/color"

func placeholderSizeKnown(n *node) bool {
	switch n.tag {
	case "iframe":
		return true
	case "audio":
		return hasAttr(n, "controls")
	case "video", "canvas", "object", "embed":
		for _, key := range []string{"width", "height"} {
			for _, value := range []string{n.attrs[key], n.decl[key]} {
				if v, ok := parseLength(value, false, false); ok && v.unit != lengthAuto && v.unit != lengthNone {
					return true
				}
			}
		}
	}
	return false
}

func replacedDefaults(n *node, s *style, bw *[4]int) {
	if n.tag == "audio" && !hasAttr(n, "controls") {
		s.display = "none"
		return
	}
	if !placeholderSizeKnown(n) {
		return
	}
	s.display = "inline-block"
	s.width, s.height = px(300), px(150)
	s.borderBox = true
	if n.tag == "audio" {
		s.height = px(32)
	}
	for key, target := range map[string]*cssLength{"width": &s.width, "height": &s.height} {
		if v, ok := parseLength(n.attrs[key], false, false); ok {
			*target = v
		}
	}
	*bw = [4]int{1, 1, 1, 1}
	s.borderStyle = [4]string{"solid", "solid", "solid", "solid"}
	for i := range s.border {
		s.border[i] = color.RGBA{200, 200, 200, 255}
		s.borderSet[i] = true
	}
}
