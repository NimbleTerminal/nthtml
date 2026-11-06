package typeset

import (
	"context"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

func TestHeadingResolvedFontWeight(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<style>.normal {font-weight:normal} .number {font-weight:400}</style>
 <h1>default</h1><h2 class="normal">normal</h2><h3 class="number">number</h3>
 <h4 style="font-weight:normal">outer<strong>strong</strong></h4>
 <h5>bold<span style="font-weight:normal">inner</span></h5>
 <h6 style="font-weight:normal"><span style="font-weight:inherit">inherit</span></h6>`)
	want := map[string]bool{"default": true, "normal": false, "number": false, "outer": false, "strong": true, "bold": true, "inner": false, "inherit": false}
	complete, err := Build(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := BuildChunked(ctx, doc, ChunkOptions{Limits: document.TypesetChunkLimits{Nodes: 6}})
	if err != nil {
		t.Fatal(err)
	}
	if chunks.Chunks() < 2 {
		t.Fatal("test did not split")
	}
	var parts []*nttypeset.Document
	for i := 0; i < chunks.Chunks(); i++ {
		part, err := chunks.Chunk(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, part)
	}
	for mode, docs := range map[string][]*nttypeset.Document{"complete": {complete}, "chunked": parts} {
		seen := map[string]bool{}
		for _, d := range docs {
			for _, b := range d.Blocks {
				if b.Kind != nttypeset.KindHeading {
					t.Errorf("%s lost heading kind", mode)
				}
				for _, span := range b.Spans {
					text := strings.TrimSpace(span.Text)
					if text == "" {
						continue
					}
					bold, ok := want[text]
					if !ok {
						t.Fatalf("unexpected heading text %q", text)
					}
					seen[text] = true
					if got := span.Style&nttypeset.Bold != 0; got != bold {
						t.Errorf("%s %s bold=%v want %v", mode, text, got, bold)
					}
				}
			}
		}
		if len(seen) != len(want) {
			t.Errorf("%s lost text: %v", mode, seen)
		}
	}
}
