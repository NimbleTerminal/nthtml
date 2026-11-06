package typeset

import (
	"context"
	"strings"
	"testing"

	"nimbleterminal.dev/nttypeset"
)

func TestTableResolvedFontWeight(t *testing.T) {
	ctx := context.Background()
	doc := parseChunks(t, `<style>.normal{font-weight:normal}</style><table>
 <thead><tr><th>default</th><th class="normal">normal</th><td>ordinary</td></tr></thead>
 <tbody><tr><th style="font-weight:400">number</th><td style="font-weight:bold">bold<span style="font-weight:normal">inner</span></td><td>body</td></tr></tbody>
 </table>`)
	want := map[string]bool{"default": true, "normal": false, "ordinary": false, "number": false, "bold": true, "inner": false, "body": false}
	complete, err := Build(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := BuildChunked(ctx, doc, ChunkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	part, err := chunks.Chunk(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for mode, d := range map[string]*nttypeset.Document{"complete": complete, "chunked": part} {
		if len(d.Blocks) != 1 || d.Blocks[0].Kind != nttypeset.KindTable {
			t.Fatal("lost table structure")
		}
		seen := map[string]bool{}
		for _, row := range d.Blocks[0].Rows {
			for _, cell := range row {
				for _, span := range cell {
					text := strings.TrimSpace(span.Text)
					if text == "" {
						continue
					}
					bold, ok := want[text]
					if !ok {
						t.Fatal("unexpected text", text)
					}
					seen[text] = true
					if got := span.Style&nttypeset.Bold != 0; got != bold {
						t.Errorf("%s %s bold=%v want=%v", mode, text, got, bold)
					}
				}
			}
		}
		if len(seen) != len(want) {
			t.Fatal("missing cells", seen)
		}
	}
}
