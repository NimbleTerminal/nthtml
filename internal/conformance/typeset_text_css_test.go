package conformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"
	"time"

	"nimbleterminal.dev/nthtml/document"
)

// Compare resolved CSS independently; raster line-spacing tests live in typeset.
func TestTypesetTextCSSChromium(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium text CSS explicitly skipped: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	source := []byte(`<!doctype html><body>
 <h1 id="plain" style="line-height:48px">plain</h1>
 <div style="line-height:2;color:#123456"><h1 id="unitless">unitless</h1></div>
 <div style="line-height:48px"><h2 id="length">length</h2></div>
 <div style="font-size:32px;line-height:150%"><h3 id="percent">percent</h3></div>
 <div style="color:#123456;line-height:48px"><h4 id="override" style="color:#abcdef;line-height:24px">override</h4></div>
 <pre id="pre" style="line-height:40px">pre</pre>
 <div style="line-height:40px"><pre id="inherited">inherited</pre></div></body>`)
	suite := Suite{Manifest: Manifest{Viewport: Viewport{800, 600}, Files: map[string]string{"text.html": fmt.Sprintf("%x", sha256.Sum256(source))}}, Files: fstest.MapFS{"text.html": &fstest.MapFile{Data: source}}}
	session, err := startCDPSession(ctx, suite, browser)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	if _, _, err := session.renderCase(ctx, Case{Path: "text.html"}, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := session.call(ctx, "Runtime.evaluate", map[string]any{"expression": `Object.fromEntries(Array.from(document.querySelectorAll('[id]'),e=>[e.id,{color:getComputedStyle(e).color,line:parseFloat(getComputedStyle(e).lineHeight)}]))`, "returnByValue": true}, session.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Result struct {
			Value map[string]struct {
				Color string
				Line  int
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Result.Value) != 7 {
		t.Fatalf("missing browser results: %s", raw)
	}
	doc, err := document.Parse(ctx, bytes.NewReader(source), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	td, err := doc.ExtractTypeset(ctx, document.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(td.Blocks) != 7 {
		t.Fatal("missing blocks")
	}
	for _, b := range td.Blocks {
		css, ok := result.Result.Value[b.SourceID]
		if !ok {
			t.Fatalf("missing measurement for %q", b.SourceID)
		}
		if b.Format == nil || b.Format.LineHeight != css.Line {
			t.Errorf("%s format=%+v, Chromium line height=%d", b.SourceID, b.Format, css.Line)
		}
		for _, span := range b.Spans {
			ink := fmt.Sprintf("rgb(%d, %d, %d)", span.Color.R, span.Color.G, span.Color.B)
			if ink != css.Color || span.Accent {
				t.Errorf("%s ink=%s accent=%v, Chromium color=%s", b.SourceID, ink, span.Accent, css.Color)
			}
		}
	}
}
