package document

import (
	"sort"
	"strings"
)

// Each rule lives in exactly one bucket selected from a necessary condition on
// its subject (rightmost compound). Ancestors and functional pseudo arguments
// are deliberately not indexed: they need not describe the subject itself.
type ruleKey struct {
	kind  byte
	value string
}
type ruleIndex struct {
	buckets   map[ruleKey][]int
	universal []int
	terms     []int
}

func indexRules(rules []rule) *ruleIndex {
	idx := &ruleIndex{buckets: map[ruleKey][]int{}, terms: make([]int, len(rules))}
	for i, r := range rules {
		for _, p := range r.selector {
			idx.terms[i] += p.termCount()
		}
		var key ruleKey
		if len(r.selector) > 0 {
			subject := r.selector[len(r.selector)-1]
			// IDs are generally most selective, followed by classes, attributes, tags.
			for _, kind := range []byte{'#', '.'} {
				for _, term := range subject.terms {
					if term[0] == kind {
						key = ruleKey{kind, term[1:]}
						break
					}
				}
				if key.kind != 0 {
					break
				}
			}
			if key.kind == 0 && len(subject.attrs) > 0 {
				key = ruleKey{'[', subject.attrs[0].name}
			}
			if key.kind == 0 {
				for _, term := range subject.terms {
					if term != "*" && term[0] != '#' && term[0] != '.' {
						key = ruleKey{'t', strings.ToLower(strings.TrimPrefix(term, "\x00"))}
						break
					}
				}
			}
		}
		if key.kind == 0 {
			idx.universal = append(idx.universal, i)
		} else {
			idx.buckets[key] = append(idx.buckets[key], i)
		}
	}
	return idx
}

func (d *Document) candidateRules(n *node) []int {
	idx := d.ruleIndex
	if idx == nil {
		all := make([]int, len(d.rules))
		for i := range all {
			all[i] = i
		}
		return all
	}
	result := append([]int(nil), idx.universal...)
	add := func(k ruleKey) { result = append(result, idx.buckets[k]...) }
	add(ruleKey{'t', strings.ToLower(n.tag)})
	if id, ok := n.attrs["id"]; ok {
		add(ruleKey{'#', id})
	}
	seen := map[string]bool{}
	for _, class := range strings.Fields(n.attrs["class"]) {
		if !seen[class] {
			seen[class] = true
			add(ruleKey{'.', class})
		}
	}
	for attr := range n.attrs {
		add(ruleKey{'[', attr})
	}
	// Preserve stylesheet source order for equal specificity and !important.
	sort.Ints(result)
	return result
}

func newVariableEntries(decl, vars map[string]string) int {
	for k := range decl {
		if strings.HasPrefix(k, "--") {
			return len(vars)
		}
	}
	return 0
}
