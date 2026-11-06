package document

import (
	"context"
	"fmt"
	"strings"
)

// TypesetChunkLimits bounds each independently extracted HTML subtree, including
// its ancestor chain. Zero values use 2048 nodes and 65536 text/attribute bytes (including serialized SVG).
// Values may only lower these ceilings. Parsing limits still apply to the input.
type TypesetChunkLimits struct{ Nodes, TextBytes int }

type typesetUnit struct {
	root                *node
	nodes, bytes, depth int
}

// TypesetChunks is a bounded index over an immutable parsed document. It retains
// no extracted blocks or generated images. Use through the typeset adapter.
type TypesetChunks struct {
	d      *Document
	units  []typesetUnit
	ranges map[string][2]int
	limits TypesetChunkLimits
}

// IndexTypesetChunks indexes semantic subtrees without extracting text runs,
// rendering diagrams, or highlighting code. Generic containers with only block
// children may split; inline runs, lists, quotes and tables remain atomic.
func (d *Document) IndexTypesetChunks(ctx context.Context, o Options, lim TypesetChunkLimits) (*TypesetChunks, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d == nil || d.root == nil {
		return nil, limitError("nil document")
	}
	if lim.Nodes == 0 {
		lim.Nodes = 2048
	}
	if lim.TextBytes == 0 {
		lim.TextBytes = 65536
	}
	if lim.Nodes < 1 || lim.Nodes > 2048 || lim.TextBytes < 1 || lim.TextBytes > 65536 {
		return nil, limitError("typeset chunk limits")
	}
	if o.Width == 0 {
		o.Width = 800
	}
	var err error
	d, err = d.forMedia(ctx, o.Width, o.ColorScheme)
	if err != nil {
		return nil, err
	}
	out := &TypesetChunks{d: d, limits: lim, ranges: make(map[string][2]int)}
	type info struct {
		nodes, bytes, depth, ownBytes int
		split, visible                bool
	}
	// Count hidden source too: raw code extraction intentionally includes text
	// below display:none, and all source costs must be bounded before extraction.
	ownBytes := func(n *node) int {
		total := len(n.text) + len(n.svg)
		for key, value := range n.attrs {
			total += len(key) + len(value)
		}
		return total
	}
	var rawCost func(*node) (info, error)
	rawCost = func(n *node) (info, error) {
		if err := ctx.Err(); err != nil {
			return info{}, err
		}
		v := info{nodes: 1, bytes: ownBytes(n), depth: 1}
		for _, child := range n.children {
			c, err := rawCost(child)
			if err != nil {
				return info{}, err
			}
			v.nodes += c.nodes
			v.bytes += c.bytes
			v.depth = max(v.depth, c.depth+1)
		}
		return v, nil
	}
	stats := make(map[*node]info)
	var scan func(*node, style) (info, error)
	scan = func(n *node, p style) (info, error) {
		if err := ctx.Err(); err != nil {
			return info{}, err
		}
		s := d.styled(n, p)
		if s.display == "none" {
			return rawCost(n)
		}
		v := info{nodes: 1, bytes: ownBytes(n), depth: 1, visible: true}
		v.ownBytes = v.bytes
		switch n.tag {
		case "document", "html", "body", "div", "main", "section", "article", "header", "footer", "nav", "aside", "center":
			v.split = true
		}
		visible := 0
		for _, c := range n.children {
			ci, err := scan(c, s)
			if err != nil {
				return info{}, err
			}
			v.nodes += ci.nodes
			v.bytes += ci.bytes
			v.depth = max(v.depth, ci.depth+1)
			if !ci.visible || c.tag == "" && strings.TrimSpace(c.text) == "" {
				continue
			}
			visible++
			cs := d.styled(c, s)
			if c.tag == "" || !(c.tag == "html" || c.tag == "body" || cs.display == "block" || cs.display == "flex" || cs.display == "grid" || cs.display == "table" || cs.display == "list-item") {
				v.split = false
			}
		}
		v.split = v.split && visible > 0
		stats[n] = v
		return v, nil
	}
	if _, err := scan(d.root, style{rootSize: 16, size: 16, line: 22}); err != nil {
		return nil, err
	}
	var partition func(*node, int, int) error
	partition = func(n *node, ancestors, ancestorBytes int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		v := stats[n]
		if v.nodes == 0 {
			return nil
		}
		if (v.nodes+ancestors > lim.Nodes || v.bytes+ancestorBytes > lim.TextBytes) && v.split {
			for _, c := range n.children {
				if c.tag == "" && strings.TrimSpace(c.text) == "" {
					continue
				}
				if err := partition(c, ancestors+1, ancestorBytes+v.ownBytes); err != nil {
					return err
				}
			}
		} else {
			out.units = append(out.units, typesetUnit{n, v.nodes + ancestors, v.bytes + ancestorBytes, v.depth + ancestors})
		}
		return nil
	}
	if err := partition(d.root, 0, 0); err != nil {
		return nil, err
	}
	if len(out.units) == 0 {
		out.units = append(out.units, typesetUnit{root: d.root})
	}
	// Assign IDs in source order, with the first visible occurrence winning.
	// Ancestor IDs cover every chunk containing their subtree.
	owners := make(map[string]*node)
	var index func(*node, int) error
	index = func(n *node, i int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if stats[n].nodes == 0 {
			return nil
		}
		if id := anchorName(n); id != "" {
			if owner, ok := owners[id]; !ok {
				owners[id] = n
				out.ranges[id] = [2]int{i, i + 1}
			} else if owner == n {
				r := out.ranges[id]
				r[1] = i + 1
				out.ranges[id] = r
			}
		}
		for _, c := range n.children {
			if err := index(c, i); err != nil {
				return err
			}
		}
		return nil
	}
	for i, u := range out.units {
		var path []*node
		for n := u.root.parent; n != nil; n = n.parent {
			path = append(path, n)
		}
		for j := len(path) - 1; j >= 0; j-- {
			n := path[j]
			if id := anchorName(n); id != "" {
				if owner, ok := owners[id]; !ok {
					owners[id] = n
					out.ranges[id] = [2]int{i, i + 1}
				} else if owner == n {
					r := out.ranges[id]
					r[1] = i + 1
					out.ranges[id] = r
				}
			}
		}
		if err := index(u.root, i); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Chunks returns the fixed number of semantic chunks (at least one).
func (s *TypesetChunks) Chunks() int { return len(s.units) }

// ChunkRange returns the half-open chunk range for a raw source ID. These are
// chunk indexes, never block indexes. Ancestor sections can span multiple chunks.
func (s *TypesetChunks) ChunkRange(id string) (int, int, bool) {
	r, ok := s.ranges[id]
	return r[0], r[1], ok
}

// Extract extracts just one chunk. Oversized atomic units fail without truncation.
// Ancestor sections are reopened, including their decoration, in each chunk.
func (s *TypesetChunks) Extract(ctx context.Context, i int, o Options) (*TypesetDocument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if i < 0 || i >= len(s.units) {
		return nil, fmt.Errorf("nthtml: chunk index %d out of range", i)
	}
	u := s.units[i]
	if u.nodes > s.limits.Nodes || u.bytes > s.limits.TextBytes || u.depth > 64 {
		return nil, limitError(fmt.Sprintf("typeset semantic unit in chunk %d", i))
	}
	root := u.root
	for p := root.parent; p != nil; p = p.parent {
		cp := *p
		cp.children = []*node{root}
		root = &cp
	}
	d := *s.d
	d.root = root
	d.hasMedia = false
	td, err := d.ExtractTypeset(ctx, o)
	if err != nil {
		return nil, err
	}
	// Suppress duplicate IDs belonging to a different source occurrence.
	allowed := func(id string) string {
		a, b, ok := s.ChunkRange(id)
		if ok && i >= a && i < b {
			return id
		}
		return ""
	}
	for j := range td.Blocks {
		td.Blocks[j].SourceID = allowed(td.Blocks[j].SourceID)
	}
	for j := range td.Sections {
		td.Sections[j].SourceID = allowed(td.Sections[j].SourceID)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return td, nil
}
