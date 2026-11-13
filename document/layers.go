package document

import "strings"

const maxCascadeLayers = 256

type cssLayer struct {
	name     string
	parent   *cssLayer
	path     []int
	children []*cssLayer
}
type cssLayers struct {
	root   cssLayer
	count  int
	events []layerEvent
}

// Names are scoped to their parent; anonymous blocks always create a new layer.
func (ls *cssLayers) register(parent *cssLayer, name string, media []string) (*cssLayer, error) {
	if parent == nil {
		parent = &ls.root
	}
	parts := strings.Split(name, "\x00")
	for _, part := range parts {
		var child *cssLayer
		for _, c := range parent.children {
			if part != "" && c.name == part {
				child = c
				break
			}
		}
		if child == nil {
			if ls.count >= maxCascadeLayers {
				return nil, limitError("cascade layers")
			}
			ls.count++
			child = &cssLayer{name: part, parent: parent, path: append(append([]int(nil), parent.path...), len(parent.children))}
			parent.children = append(parent.children, child)
		}
		parent = child
	}
	if len(ls.events) >= 4096 {
		return nil, limitError("cascade layer declarations")
	}
	ls.events = append(ls.events, layerEvent{parent, media})
	return parent, nil
}

func layerName(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	var parts []string
	for {
		id, n, ok := cssIdentifier(raw)
		if !ok {
			return "", false
		}
		switch strings.ToLower(id) {
		case "initial", "inherit", "unset", "revert", "revert-layer", "default":
			return "", false
		}
		parts = append(parts, id)
		raw = strings.TrimSpace(raw[n:])
		if raw == "" {
			return strings.Join(parts, "\x00"), true
		}
		if raw[0] != '.' {
			return "", false
		}
		raw = strings.TrimSpace(raw[1:])
	}
}

// Direct declarations in a parent layer follow its sublayers. Unlayered
// declarations follow every layer. Important declarations reverse this order.
func compareLayers(a, b *cssLayer) int {
	if a == b {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	for i := 0; i < len(a.path) && i < len(b.path); i++ {
		if a.path[i] < b.path[i] {
			return -1
		}
		if a.path[i] > b.path[i] {
			return 1
		}
	}
	if len(a.path) < len(b.path) {
		return 1
	}
	return -1
}

type cascadeScore struct {
	important, inline bool
	layer             *cssLayer
	specificity       int
}

func (s cascadeScore) atLeast(other cascadeScore) bool {
	if s.important != other.important {
		return s.important
	}
	if s.inline != other.inline {
		return s.inline
	}
	if order := compareLayers(s.layer, other.layer); order != 0 {
		if s.important {
			return order < 0
		}
		return order > 0
	}
	return s.specificity >= other.specificity
}

type layerEvent struct {
	layer *cssLayer
	media []string
}

func (ls *cssLayers) order(width int, scheme string, preferences ...MediaPreferences) map[*cssLayer]*cssLayer {
	out := map[*cssLayer]*cssLayer{}
	if ls == nil {
		return out
	}
	counts := map[*cssLayer]int{}
	var add func(*cssLayer) *cssLayer
	add = func(original *cssLayer) *cssLayer {
		if original == nil || original == &ls.root {
			return nil
		}
		if found := out[original]; found != nil {
			return found
		}
		parent := add(original.parent)
		var path []int
		if parent != nil {
			path = parent.path
		}
		next := &cssLayer{path: append(append([]int(nil), path...), counts[original.parent])}
		counts[original.parent]++
		out[original] = next
		return next
	}
	for _, event := range ls.events {
		if (rule{media: event.media}).matchesMedia(width, scheme, preferences...) {
			add(event.layer)
		}
	}
	return out
}
