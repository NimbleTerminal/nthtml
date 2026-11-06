package typeset

import (
	"context"
	"testing"

	"nimbleterminal.dev/nttypeset"
)

func TestExplicitVerticalMargins(t *testing.T) {
	for _, tc := range []struct {
		name, html    string
		before, after int
		section       bool
	}{
		{"pre", `<pre style="margin-top:23px;margin-bottom:31px">code</pre>`, 23, 31, false},
		{"pre zero", `<pre style="margin:0">code</pre>`, 0, 0, false},
		{"list item", `<ul><li style="margin-top:13px;margin-bottom:19px">item</li></ul>`, 13, 19, false},
		{"list item zero", `<ul><li style="margin:0;line-height:40px">item</li></ul>`, 0, 0, false},
		{"list item small", `<ol><li style="margin-bottom:2px">item</li></ol>`, 0, 2, false},
		{"list container", `<ul style="margin-top:17px;margin-bottom:29px"><li>item</li></ul>`, 17, 29, true},
		{"list container zero", `<ol style="margin:0"><li>item</li></ol>`, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			doc := parseChunks(t, tc.html)
			complete, err := Build(ctx, doc, Options{})
			if err != nil {
				t.Fatal(err)
			}
			chunks, err := BuildChunked(ctx, doc, ChunkOptions{})
			if err != nil {
				t.Fatal(err)
			}
			chunk, err := chunks.Chunk(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			for mode, d := range map[string]*nttypeset.Document{"complete": complete, "chunked": chunk} {
				if len(d.Blocks) != 1 {
					t.Fatal("unexpected blocks", len(d.Blocks))
				}
				if tc.section {
					if len(d.Sections) != 1 {
						t.Errorf("%s sections=%d want 1", mode, len(d.Sections))
						continue
					}
					s := d.Sections[0]
					if s.SpaceBefore != tc.before || s.SpaceAfter != tc.after {
						t.Errorf("%s spacing=%d/%d want %d/%d", mode, s.SpaceBefore, s.SpaceAfter, tc.before, tc.after)
					}
				} else {
					f := d.Blocks[0].Format
					if f == nil || f.SpaceBefore != tc.before || f.SpaceAfter != tc.after {
						t.Errorf("%s format=%+v want spacing %d/%d", mode, f, tc.before, tc.after)
					}
				}
			}
		})
	}
}
