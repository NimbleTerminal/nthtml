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
func TestTypesetTableWeightChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium table weight explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	source := []byte(`<!doctype html><style>.normal{font-weight:normal}</style><table>
 <thead><tr><th id="default">default</th><th id="normal" class="normal">normal</th><td id="ordinary">ordinary</td></tr></thead>
 <tbody><tr><th id="number" style="font-weight:400">number</th><td style="font-weight:bold"><span id="bold">bold</span><span id="inner" style="font-weight:normal">inner</span></td><td id="body">body</td></tr></tbody>
 </table>`)
	suite := Suite{Manifest: Manifest{Viewport: Viewport{800, 600}, Files: map[string]string{"table.html": fmt.Sprintf("%x", sha256.Sum256(source))}}, Files: fstest.MapFS{"table.html": &fstest.MapFile{Data: source}}}
	session, err := startCDPSession(ctx, suite, browser)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	if _, _, err := session.renderCase(ctx, Case{Path: "table.html"}, nil); err != nil {
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
	if len(result.Result.Value) != 7 {
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
		var spans []nttypeset.Span
		for _, row := range b.Rows {
			for _, cell := range row {
				spans = append(spans, cell...)
			}
		}
		for _, span := range spans {
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
	if len(seen) != 7 {
		t.Fatal("missing table runs", seen)
	}
}
