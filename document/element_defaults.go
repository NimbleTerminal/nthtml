package document

func hasAttr(n *node, key string) bool {
	_, ok := n.attrs[key]
	return ok
}

// UA generated content goes through the same cascade and node budget as
// author pseudo-elements.
func defaultPseudo(n *node, pseudo string) string {
	if pseudo != "before" && pseudo != "after" {
		return ""
	}
	switch n.tag {
	case "q":
		depth := 0
		for p := n.parent; p != nil; p = p.parent {
			if p.tag == "q" {
				depth++
			}
		}
		if pseudo == "before" {
			if depth%2 == 0 {
				return "“"
			}
			return "‘"
		}
		if depth%2 == 0 {
			return "”"
		}
		return "’"
	case "rt":
		if pseudo == "before" {
			return "("
		}
		return ")"
	}
	return ""
}
