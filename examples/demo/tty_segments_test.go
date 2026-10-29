package main

import (
	"context"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
)

func TestTTYSegmentsPreserveStyledLinksAndText(t *testing.T) {
	d, err := document.Parse(context.Background(), strings.NewReader(`<p style="color:#123456">Wide 界 <a href="https://example.com"><b>bold</b> <i>italic</i></a> end</p>`), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := d.TTYLayout(context.Background(), 12)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	boldLink, italicLink := false, false
	for _, raw := range ttySegments(r) {
		s := raw.(map[string]any)
		text.WriteString(s["text"].(string))
		if s["href"] == "https://example.com" {
			css := s["style"].(map[string]any)
			boldLink = boldLink || css["fontWeight"] == "bold"
			italicLink = italicLink || css["fontStyle"] == "italic"
		}
	}
	if text.String() != r.Text {
		t.Fatal("segment text differs from plain TTY")
	}
	if !boldLink || !italicLink {
		t.Fatal("lost emphasis inside hyperlink")
	}
}
