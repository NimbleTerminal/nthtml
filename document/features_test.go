package document

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func parseFeature(t *testing.T, src string) *Document {
	t.Helper()
	d, e := Parse(context.Background(), strings.NewReader(src), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestSafeLinkURLs(t *testing.T) {
	for _, href := range []string{"javascript:alert(1)", "data:text/html,bad", "file:///tmp/a", "vbscript:evil", "https://a/\x1b]52;bad", "https://a/\npath", "https:\\evil", "https:missing-host", "https://a/has space", strings.Repeat("x", 2049)} {
		if got := SafeURL(href); got != "" {
			t.Errorf("accepted %q as %q", href, got)
		}
	}
	for _, href := range []string{"https://example.com/a?x=1&y=2", "http://example.com/", "mailto:hello@example.com", "../guide.html", "#start"} {
		if SafeURL(href) == "" {
			t.Errorf("rejected %q", href)
		}
	}
	if got := ResolveURL("https://example.com/docs/index.html", "../guide.html"); got != "https://example.com/guide.html" {
		t.Fatal(got)
	}
	if ResolveURL("", "../guide.html") != "" || ResolveURL("file:///tmp/", "a.html") != "" {
		t.Fatal("unsafe relative resolution")
	}
}
func TestRasterLinkRegionsAndMonospace(t *testing.T) {
	d := parseFeature(t, `<p><a href="https://example.com">A linked phrase which wraps over several lines</a> plain <code>x := 1</code></p><a style="display:none" href="https://hidden.example">Hidden</a><a href="javascript:evil">Bad</a>`)
	r, e := d.Render(context.Background(), Options{Width: 160})
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Links) != 1 || len(r.Links[0].Regions) < 2 {
		t.Fatal(r.Links)
	}
	id := r.Links[0].ID
	mono := false
	for _, b := range r.Boxes {
		if b.Monospace {
			mono = true
		}
		if b.LinkID != 0 {
			l, ok := r.LinkAt(b.Bounds.Min)
			if !ok || l.ID != id {
				t.Fatal("link hit test failed", b)
			}
		}
	}
	if !mono {
		t.Fatal("inline code did not select monospace")
	}
	if _, ok := r.LinkAt(image.Pt(-1, -1)); ok {
		t.Fatal("outside hit")
	}
	wider, e := d.Render(context.Background(), Options{Width: 400})
	if e != nil {
		t.Fatal(e)
	}
	if wider.Links[0].ID != id {
		t.Fatal("link ID changed on reflow")
	}
}
func TestTTYLinkSpansAcrossTableWrapping(t *testing.T) {
	d := parseFeature(t, `<p>before <a href="https://example.com">世界 readable link</a> after</p><table><tr><td><a href="https://a.example">a long table link that wraps</a></td><td>UNLINKED</td></tr></table><a href="javascript:bad">unsafe text</a>`)
	r, e := d.TTYLayout(context.Background(), 24)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Links) != 2 || strings.ContainsRune(r.Text, '\x1b') {
		t.Fatal(r)
	}
	for _, span := range r.Spans {
		part := r.Text[span.Start:span.End]
		if strings.ContainsAny(part, "│┌─\n") || strings.Contains(part, "UNLINKED") {
			t.Fatalf("link leaked into adjacent cells: %q", part)
		}
		if ansi.StringWidth(part) != span.Bounds.Dx() {
			t.Fatalf("bad cells: %+v %q", span, part)
		}
	}
	for _, l := range r.Links {
		var text strings.Builder
		for _, span := range r.Spans {
			if span.LinkID == l.ID {
				text.WriteString(r.Text[span.Start:span.End])
			}
		}
		flat := strings.Join(strings.Fields(text.String()), "")
		want := strings.Join(strings.Fields(l.Text), "")
		if flat != want {
			t.Fatalf("lost linked text: %q != %q\n%s", flat, want, r.Text)
		}
	}
	encoded := r.ANSI()
	if !strings.Contains(encoded, "\x1b]8;") {
		t.Fatal("no OSC 8")
	}
	if ansi.Strip(encoded) != r.Text {
		t.Fatal("OSC encoding changed visible text")
	}
	for _, line := range strings.Split(encoded, "\n") {
		if strings.Count(line, "\x1b]8;")%2 != 0 {
			t.Fatalf("unbalanced line: %q", line)
		}
	}
}
func TestListsAndPreformattedCode(t *testing.T) {
	d := parseFeature(t, "<ol start='3'><li>First item<ul><li>Nested item</li></ul></li><li value='9'>Last item</li></ol><pre><code>\nfunc main() {\n\tprintln(\"x\")\n\n    done()\n}</code></pre>")
	text, e := d.TTY(context.Background(), 60)
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"3. First item", "   • Nested item", "9. Last item", "func main() {\n    println(\"x\")\n\n    done()\n}"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
	r, e := d.Render(context.Background(), Options{Width: 500})
	if e != nil {
		t.Fatal(e)
	}
	var markers []string
	lines := map[int]string{}
	for _, b := range r.Boxes {
		if b.Tag == "marker" {
			markers = append(markers, b.Text)
		}
		if b.Monospace {
			lines[b.Bounds.Min.Y] += b.Text
		}
	}
	if len(markers) != 3 {
		t.Fatal(markers)
	}
	found := false
	for _, line := range lines {
		if line == "    println(\"x\")" {
			found = true
		}
	}
	if !found {
		t.Fatal("lost code indentation", lines)
	}
	narrow, e := d.TTY(context.Background(), 8)
	if e != nil {
		t.Fatal(e)
	}
	for _, line := range strings.Split(narrow, "\n") {
		if ansi.StringWidth(line) > 8 {
			t.Fatalf("narrow overflow: %q", line)
		}
	}
}
func TestLinksCancellation(t *testing.T) {
	d := parseFeature(t, "<a href='https://example.com'>link</a>")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := d.Links(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := d.TTYLayout(ctx, 80); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
