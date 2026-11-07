package document

import (
	"sort"
	"strings"
)

// Each rule is indexed by necessary conditions on its rightmost subject.
// Positive functional alternatives contribute a union of subject keys;
// ancestor keys only eliminate rules when absent from the entire document.
type ruleKey struct {
	kind  byte
	value string
}
type ruleIndex struct {
	buckets   map[ruleKey][]int
	universal []int
	terms     []int
	scanWork  int
}

func indexRules(rules []rule, root *node) *ruleIndex {
	idx := &ruleIndex{buckets: map[ruleKey][]int{}, terms: make([]int, len(rules))}
	present := map[ruleKey]bool{{'t', "html"}: true, {'r', ""}: true}
	var scan func(*node)
	scan = func(n *node) {
		if n == nil {
			return
		}
		if n.tag != "" && n.pseudo == "" {
			idx.scanWork += 1 + len(n.attrs) + len(n.attrs["class"])
			present[ruleKey{'t', strings.ToLower(n.tag)}] = true
			if id, ok := n.attrs["id"]; ok {
				present[ruleKey{'#', id}] = true
			}
			for _, class := range strings.Fields(n.attrs["class"]) {
				present[ruleKey{'.', class}] = true
			}
			for attr := range n.attrs {
				present[ruleKey{'[', attr}] = true
			}
		}
		for _, c := range n.children {
			scan(c)
		}
	}
	scan(root)
	for i, r := range rules {
		for _, p := range r.selector {
			idx.terms[i] += p.termCount()
		}
		// A necessary key absent everywhere also rules out ancestor/sibling
		// compounds. This is a conservative prefilter, not an ancestor match.
		possible := true
		for _, part := range r.selector {
			keys := selectorKeys(selector{part})
			if len(keys) > 0 {
				found := false
				for _, key := range keys {
					if present[key] {
						found = true
						break
					}
				}
				if !found {
					possible = false
					break
				}
			}
		}
		if !possible {
			continue
		}
		keys := selectorKeys(r.selector)
		if len(keys) == 0 {
			idx.universal = append(idx.universal, i)
		} else {
			seen := map[ruleKey]bool{}
			for _, key := range keys {
				if !seen[key] {
					idx.buckets[key] = append(idx.buckets[key], i)
					seen[key] = true
				}
			}
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
	seenRules := make([]bool, len(d.rules))
	for _, i := range result {
		seenRules[i] = true
	}
	add := func(k ruleKey) {
		for _, i := range idx.buckets[k] {
			if !seenRules[i] {
				result = append(result, i)
				seenRules[i] = true
			}
		}
	}
	add(ruleKey{'t', strings.ToLower(n.tag)})
	if n.tag == "html" || n.parent != nil && n.parent.tag == "document" {
		add(ruleKey{'r', ""})
	}
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

// Positive functional pseudos describe a union of possible subjects. Index the
// union only when every alternative has a necessary key; otherwise keep the
// rule universal. Never infer subject keys from :not or :has.
func selectorKeys(sel selector) []ruleKey {
	if len(sel) == 0 {
		return nil
	}
	subject := sel[len(sel)-1]
	for _, kind := range []byte{'#', '.'} {
		for _, term := range subject.terms {
			if term[0] == kind {
				return []ruleKey{{kind, term[1:]}}
			}
		}
	}
	for _, ps := range subject.pseudos {
		if ps.name == "root" || ps.name == "scope" {
			return []ruleKey{{'r', ""}}
		}
	}
	if len(subject.attrs) > 0 {
		return []ruleKey{{'[', subject.attrs[0].name}}
	}
	for _, term := range subject.terms {
		if term != "*" && term[0] != '#' && term[0] != '.' {
			return []ruleKey{{'t', strings.ToLower(strings.TrimPrefix(term, "\x00"))}}
		}
	}
	for _, ps := range subject.pseudos {
		switch ps.name {
		case "is", "where", "matches", "-webkit-any", "-moz-any":
			var keys []ruleKey
			complete := true
			for _, sub := range ps.subs {
				k := selectorKeys(sub)
				if len(k) == 0 {
					complete = false
					break
				}
				keys = append(keys, k...)
			}
			if complete && len(keys) > 0 {
				return keys
			}
		}
	}
	return nil
}
