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
	case "summary":
		if pseudo == "before" && n.parent != nil && n.parent.tag == "details" && firstSummary(n.parent) == n {
			if hasAttr(n.parent, "open") {
				return "▾ "
			}
			return "▸ "
		}
	case "details":
		if pseudo == "before" && firstSummary(n) == nil {
			if hasAttr(n, "open") {
				return "▾ Details"
			}
			return "▸ Details"
		}

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

func firstSummary(n *node) *node {
	for _, c := range n.children {
		if c.tag == "summary" {
			return c
		}
	}
	return nil
}

func displayedChildren(n *node) []*node {
	if placeholderSizeKnown(n) {
		return nil
	}
	if n.tag == "progress" || n.tag == "meter" {
		return nil
	}
	if n.tag == "input" || n.tag == "select" {
		for i, c := range n.children {
			if c.pseudo == "control" {
				return n.children[i : i+1]
			}
		}
		return nil
	}
	if n.tag == "details" && !hasAttr(n, "open") {
		summary := firstSummary(n)
		for i, c := range n.children {
			if c == summary || summary == nil && c.pseudo == "before" {
				return n.children[i : i+1]
			}
		}
		return nil
	}
	return n.children
}
