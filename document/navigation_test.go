package document

import (
	"context"
	"github.com/charmbracelet/x/ansi"
	"image"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFragmentTargets(t *testing.T) {
	r := renderCSS(t, `<p><a href="#caf%C3%A9">Jump</a></p><h2 id="café">Destination</h2><a name="legacy"></a><div id="hidden" style="display:none">Hidden</div><table><tbody id="group"><tr id="row"><td id="cell">Cell</td></tr></tbody></table>`)
	for _, href := range []string{"#", "#caf%C3%A9", "#legacy", "#group", "#row", "#cell"} {
		if _, ok := r.Fragment(href); !ok {
			t.Fatal(href)
		}
		tty, e := r.TTYLayout(context.Background(), 30)
		if e != nil {
			t.Fatal(e)
		}
		if _, ok := tty.Fragment(href); !ok {
			t.Fatal("TTY", href)
		}
		if strings.Contains(tty.Text, "nthtml-anchor") || strings.Contains(tty.Text, "\x1b") {
			t.Fatal("marker leaked", tty.Text)
		}
	}
	if _, ok := r.Fragment("#hidden"); ok {
		t.Fatal("hidden anchor")
	}
	if _, ok := r.Fragment("#missing"); ok {
		t.Fatal("missing anchor")
	}
	if _, ok := r.Fragment("#%XX"); ok {
		t.Fatal("invalid escape")
	}
}
func TestSearchAndTextExtraction(t *testing.T) {
	r, e := Render(context.Background(), strings.NewReader(`<p>Super<span>califragilistic</span> café multi word phrase</p><pre> a
 b</pre>`), Options{Width: 100})
	if e != nil {
		t.Fatal(e)
	}
	index := r.TextIndex()
	for _, query := range []string{"supercalifragilistic", "CAFÉ", "multi   word phrase", "a b"} {
		hits := index.Search(query)
		if len(hits) != 1 {
			t.Fatalf("%q in %q: %v", query, index.Text, hits)
		}
		if len(index.Regions(hits[0].Start, hits[0].End)) == 0 {
			t.Fatal("missing hit geometry")
		}
	}
	if !strings.Contains(index.Text, "Supercalifragilistic") {
		t.Fatal(index.Text)
	}
	for i := 0; i < len(index.Text); i++ {
		if !utf8.ValidString(index.Selected(i, i+1)) {
			t.Fatal("split UTF-8")
		}
	}
	if len(index.Search(strings.Repeat("a", 257))) != 0 {
		t.Fatal("query budget")
	}
	if hits := (&TextIndex{Text: strings.Repeat("a ", 2000)}).Search("a"); len(hits) != 1000 {
		t.Fatal(len(hits))
	}
}
func TestTTYHighlightsAndHit(t *testing.T) {
	r := renderCSS(t, `<p><a href="https://example.com">Café 世界</a> more</p>`)
	tty, e := r.TTYLayout(context.Background(), 40)
	if e != nil {
		t.Fatal(e)
	}
	index := tty.TextIndex()
	hits := index.Search("café")
	marked := tty.ANSIHighlights(hits)
	if ansi.Strip(marked) != tty.Text {
		t.Fatal(marked, tty.Text)
	}
	if !strings.Contains(marked, "\x1b[7m") || !strings.Contains(marked, "https://example.com") {
		t.Fatal("missing highlight/link")
	}
	hit, ok := index.Hit(image.Rect(5, 0, 6, 1))
	if !ok || !utf8.ValidString(index.Selected(hit.Start, hit.End)) {
		t.Fatal(hit)
	}
}

func TestAnchorMarkersDoNotChangeTTYText(t *testing.T) {
	src := `<h2>Title</h2><p>Text <a>inline</a></p><pre> code</pre><ul><li>Item</li></ul><table><tr><td>Cell</td></tr></table>`
	anchored := strings.NewReplacer("<h2>", "<h2 id='title'>", "<p>", "<p id='text'>", "<a>", "<a name='inline'>", "<pre>", "<pre id='code'>", "<ul>", "<ul id='list'>", "<table>", "<table id='table'>").Replace(src)
	a, e := parseFeature(t, src).TTY(context.Background(), 30)
	if e != nil {
		t.Fatal(e)
	}
	b, e := parseFeature(t, anchored).TTY(context.Background(), 30)
	if e != nil {
		t.Fatal(e)
	}
	if a != b {
		t.Fatalf("%q != %q", a, b)
	}
	index := (&TTYResult{Text: "first\nsecond"}).TextIndex()
	hit, ok := index.Hit(image.Rect(0, 1, 1, 2))
	if !ok || index.Selected(hit.Start, hit.End) != "s" {
		t.Fatal(hit)
	}
}
