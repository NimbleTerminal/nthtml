package conformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"nimbleterminal.dev/nthtml/document"
	htmltypeset "nimbleterminal.dev/nthtml/typeset"
	"nimbleterminal.dev/nttypeset"
)

// This authored probe checks resolved weight, not full font metrics or pixels.
func TestTypesetHeadingWeightChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium heading weight explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	source := []byte(`<!doctype html><style>.normal{font-weight:normal}</style><body>
 <h1 id="default">default</h1><h2 id="normal" class="normal">normal</h2>
 <h3 id="number" style="font-weight:400">number</h3>
 <h4 style="font-weight:normal"><span id="inherit">inherit</span><strong id="strong">strong</strong></h4>
 <h5><span id="inner" style="font-weight:normal">inner</span></h5></body>`)
	suite := Suite{Manifest: Manifest{Viewport: Viewport{800, 600}, Files: map[string]string{"heading.html": fmt.Sprintf("%x", sha256.Sum256(source))}}, Files: fstest.MapFS{"heading.html": &fstest.MapFile{Data: source}}}
	session, err := startCDPSession(ctx, suite, browser)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	if _, _, err := session.renderCase(ctx, Case{Path: "heading.html"}, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := session.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    `Object.fromEntries(Array.from(document.querySelectorAll('[id]'), e => [e.id, getComputedStyle(e).fontWeight]))`,
		"returnByValue": true,
	}, session.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Result struct {
			Value map[string]string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Result.Value) != 6 {
		t.Fatalf("missing browser measurements: %s", raw)
	}
	doc, err := document.Parse(ctx, bytes.NewReader(source), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	ast, err := htmltypeset.Build(ctx, doc, htmltypeset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, b := range ast.Blocks {
		for _, span := range b.Spans {
			text := strings.TrimSpace(span.Text)
			if text == "" {
				continue
			}
			weight, ok := result.Result.Value[text]
			if !ok || (weight != "400" && weight != "700") {
				t.Fatalf("unexpected browser weight for %q: %q", text, weight)
			}
			if bold := span.Style&nttypeset.Bold != 0; bold != (weight == "700") {
				t.Errorf("%s: adapter bold=%v, Chromium weight=%s", text, bold, weight)
			}
			seen[text] = true
		}
	}
	if len(seen) != 6 {
		t.Fatal("missing heading runs", seen)
	}
}
