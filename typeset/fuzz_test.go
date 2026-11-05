package typeset

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
)

func FuzzBuildWithSources(f *testing.F) {
	for _, source := range []string{
		`<article id="root"><h1 id="title">Hello</h1><p id="body">Text <a href="#title">link</a></p></article>`,
		`<section id="a"><section id="a"><p id="b">duplicate</p></section></section>`,
		`<table id="t"><tr><th>Header</th></tr><tr><td colspan="2"><b>cell</b></td></tr></table>`,
		`<ul><li id="item">one<ul><li>two</li></ul></li></ul><pre><code>code</code></pre>`,
		`<style>#hidden{display:none}</style><div id="hidden">hidden</div><div style="padding:2px;border:1px solid red">box</div>`,
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			t.Skip()
		}
		ctx := context.Background()
		d, err := document.Parse(ctx, strings.NewReader(source), document.Limits{Bytes: 4096, Nodes: 100, Depth: 16, Rules: 16, Height: 1000, Pixels: 200000})
		if err != nil {
			return
		}
		result, sources, err := BuildWithSources(ctx, d, Options{Document: document.Options{Width: 200, DisableDiagrams: true}})
		if err != nil {
			return
		}
		for _, section := range result.Sections {
			if section.Start < 0 || section.End <= section.Start || section.End > len(result.Blocks) {
				t.Fatalf("invalid section range: %+v", section)
			}
		}
		seen := map[string]bool{}
		for _, id := range sources.IDs() {
			if id == "" || seen[id] {
				t.Fatalf("invalid or duplicate source ID %q", id)
			}
			seen[id] = true
			// Encode IDs as fragments so literal leading '#' and '%' remain IDs.
			start, end, ok := sources.BlockRange("#" + url.PathEscape(id))
			if !ok || start < 0 || end < start || end > len(result.Blocks) {
				t.Fatalf("invalid block range for %q: %d:%d", id, start, end)
			}
			// Walk the actual parent chain to detect cycles rather than relying on RootID's guard.
			parents := map[string]bool{id: true}
			for p := sources.ParentID("#" + url.PathEscape(id)); p != ""; p = sources.ParentID("#" + url.PathEscape(p)) {
				if parents[p] {
					t.Fatalf("cyclic parent chain for %q", id)
				}
				parents[p] = true
			}
		}
	})
}
