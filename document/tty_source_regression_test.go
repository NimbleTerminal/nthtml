package document

import (
	"context"
	"strings"
	"testing"
)

func TestTTYAdjacentScopesKeepTextAndAnchorIdentity(t *testing.T) {
	d, err := Parse(context.Background(), strings.NewReader(`<section id="first"><div id="body"><p>earlier <strong>message</strong></p></div></section><section id="next"><p>later message</p></section>`), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := d.TTYLayout(context.Background(), 40)
	if err != nil {
		t.Fatal(err)
	}
	index := result.TextIndex()
	hits := index.Search("earlier message")
	if len(hits) != 1 {
		t.Fatal("missing first paragraph")
	}
	if id, _ := index.SourceAt(hits[0].Start); id != "body" {
		t.Fatalf("text belongs to %q", id)
	}
	bounds, ok := result.AnchorBounds("next")
	if !ok {
		t.Fatal("missing next anchor")
	}
	if id, ok := result.SourceAt(bounds.Min); !ok || id != "next" {
		t.Fatalf("anchor points to %q", id)
	}
}
