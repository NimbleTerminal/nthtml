package document

import (
	"context"
	"slices"
	"strings"
)

// MediaPreferences describes host preferences and capabilities. Empty fields
// select standalone defaults: reduce, none, and none respectively.
// It never enables scripting or input; it only selects CSS media branches.
type MediaPreferences struct {
	ReducedMotion string // reduce or no-preference
	Hover         string // none or hover; also used for any-hover
	Scripting     string // none, initial-only, or enabled
}

func (m MediaPreferences) normalized() MediaPreferences {
	if m.ReducedMotion != "no-preference" {
		m.ReducedMotion = "reduce"
	}
	if m.Hover != "hover" {
		m.Hover = "none"
	}
	if m.Scripting != "enabled" && m.Scripting != "initial-only" {
		m.Scripting = "none"
	}
	return m
}

// RenderState supplies host-owned presentation state without author attributes.
// OpenPopovers contains HTML ids of shown popovers. Lists are copied and bounded
// by the document node/byte limits; duplicates and unknown ids are harmless.
// An open popover is rendered in flow, without top-layer or modal semantics.
type RenderState struct {
	Media        MediaPreferences
	OpenPopovers []string
}

func (s RenderState) normalized(l Limits) (RenderState, error) {
	s.Media = s.Media.normalized()
	if len(s.OpenPopovers) > l.Nodes {
		return RenderState{}, limitError("open popovers")
	}
	total := 0
	for _, id := range s.OpenPopovers {
		total += len(id)
		if total > l.Bytes {
			return RenderState{}, limitError("popover ids")
		}
	}
	s.OpenPopovers = slices.Clone(s.OpenPopovers)
	slices.Sort(s.OpenPopovers)
	s.OpenPopovers = slices.Compact(s.OpenPopovers)
	return s, nil
}
func (s RenderState) equal(other RenderState) bool {
	return s.Media == other.Media && slices.Equal(s.OpenPopovers, other.OpenPopovers)
}

// WithRenderState returns an immutable snapshot for standalone TTY callers.
// Render/ExtractTypeset also accept the same state through Options.State.
func (d *Document) WithRenderState(ctx context.Context, state RenderState) (*Document, error) {
	if d == nil {
		return nil, nil
	}
	return d.forInteraction(ctx, d.cascadedWidth, d.cascadedScheme, d.hoverSource, state)
}

func normalizeDetails(root *node) {
	seen := map[string]bool{}
	var walk func(*node)
	walk = func(n *node) {
		if n.tag == "details" && n.attrs["name"] != "" && hasAttr(n, "open") {
			name := n.attrs["name"]
			if seen[name] {
				delete(n.attrs, "open")
			} else {
				seen[name] = true
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(root)
}

func mediaPreferenceFeature(inner string, m MediaPreferences) (bool, bool, bool) {
	key, value, has := strings.Cut(inner, ":")
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.ToLower(strings.TrimSpace(value))
	m = m.normalized()
	var actual string
	switch key {
	case "prefers-reduced-motion":
		actual = m.ReducedMotion
	case "hover", "any-hover":
		actual = m.Hover
	case "scripting":
		actual = m.Scripting
	default:
		return false, false, false
	}
	if !has {
		return actual != "none" && actual != "no-preference", true, true
	}
	valid := false
	switch key {
	case "prefers-reduced-motion":
		valid = value == "reduce" || value == "no-preference"
	case "hover", "any-hover":
		valid = value == "none" || value == "hover"
	case "scripting":
		valid = value == "none" || value == "initial-only" || value == "enabled"
	}
	return valid && value == actual, valid, true
}
