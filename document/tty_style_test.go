package document

import (
	"context"
	"image"
	"image/color"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func styledTTY(t *testing.T, html string, width int) *TTYResult {
	t.Helper()
	d, err := Parse(context.Background(), strings.NewReader(html), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := d.TTYLayout(context.Background(), width)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := d.TTY(context.Background(), width)
	if err != nil || plain != r.Text || strings.ContainsAny(plain, "\x1b\x07") {
		t.Fatalf("plain output: %q %v", plain, err)
	}
	if got := ansi.Strip(r.ANSI()); got != r.Text {
		t.Fatalf("ANSI changes text: %q != %q", got, r.Text)
	}
	return r
}

func ttyStyleAt(t *testing.T, r *TTYResult, word string) TTYStyle {
	t.Helper()
	offset := strings.Index(r.Text, word)
	if offset < 0 {
		t.Fatalf("missing %q in %q", word, r.Text)
	}
	for _, s := range r.Styles {
		if s.Start <= offset && offset < s.End {
			return s.Style
		}
	}
	return TTYStyle{}
}

// Decode only SGR attributes emitted by this renderer, independently of its
// style-span metadata. This checks actual terminal state and reset behavior.
type ttyTestState struct {
	TTYStyle
	reverse bool
	href    string
}

func decodeTTYANSI(t *testing.T, text string) (string, map[int]ttyTestState, ttyTestState) {
	t.Helper()
	var plain strings.Builder
	states := map[int]ttyTestState{}
	st := ttyTestState{}
	for len(text) > 0 {
		seq, _, n, _ := ansi.DecodeSequence(text, 0, nil)
		if n == 0 {
			t.Fatal("invalid ANSI sequence")
		}
		text = text[n:]
		switch {
		case strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m"):
			args := strings.Split(seq[2:len(seq)-1], ";")
			for i := 0; i < len(args); i++ {
				v, _ := strconv.Atoi(args[i])
				switch v {
				case 0:
					href := st.href
					st = ttyTestState{href: href}
				case 1:
					st.Bold = true
				case 3:
					st.Italic = true
				case 4:
					st.Underline = true
				case 7:
					st.reverse = true
				case 9:
					st.Strikethrough = true
				case 38, 48:
					if i+4 >= len(args) || args[i+1] != "2" {
						t.Fatalf("unsupported test color %q", seq)
					}
					r, _ := strconv.Atoi(args[i+2])
					g, _ := strconv.Atoi(args[i+3])
					b, _ := strconv.Atoi(args[i+4])
					c := color.RGBA{uint8(r), uint8(g), uint8(b), 255}
					if v == 38 {
						st.Foreground = c
					} else {
						st.Background = c
					}
					i += 4
				default:
					t.Fatalf("unexpected SGR %d", v)
				}
			}
		case strings.HasPrefix(seq, "\x1b]8;"):
			payload := strings.TrimSuffix(strings.TrimSuffix(seq, "\x07"), "\x1b\\")
			_, href, _ := strings.Cut(payload[4:], ";")
			st.href = href
		case strings.HasPrefix(seq, "\x1b"):
			t.Fatalf("unexpected control %q", seq)
		default:
			if seq == "\n" && st != (ttyTestState{}) {
				t.Fatalf("state leaked across newline: %+v", st)
			}
			for i := 0; i < len(seq); i++ {
				states[plain.Len()+i] = st
			}
			plain.WriteString(seq)
		}
	}
	return plain.String(), states, st
}

func TestTTYCSSStylesAndRestoration(t *testing.T) {
	r := styledTTY(t, `<style>
 section {color:#123456;background-color:#203040}
 strong {font-weight:normal}
 .override {color:#abcdef;background:#765432;font-style:normal;text-decoration:none}
 </style><section id="body">outer <em>italic <span class="override">inner</span> again</em> tail <strong>normal</strong> <b>bold</b> <u>under</u> <del>gone</del></section>`, 80)
	fg := color.RGBA{0x12, 0x34, 0x56, 255}
	bg := color.RGBA{0x20, 0x30, 0x40, 255}
	for _, word := range []string{"outer", "italic", "again", "tail", "normal", "bold", "under", "gone"} {
		s := ttyStyleAt(t, r, word)
		if s.Foreground != fg || s.Background != bg {
			t.Errorf("%s: %+v", word, s)
		}
	}
	if !ttyStyleAt(t, r, "italic").Italic || !ttyStyleAt(t, r, "again").Italic || ttyStyleAt(t, r, "tail").Italic {
		t.Fatal("italic scope")
	}
	inner := ttyStyleAt(t, r, "inner")
	if inner.Italic || inner.Foreground != (color.RGBA{0xab, 0xcd, 0xef, 255}) || inner.Background != (color.RGBA{0x76, 0x54, 0x32, 255}) {
		t.Fatal(inner)
	}
	if ttyStyleAt(t, r, "normal").Bold || !ttyStyleAt(t, r, "bold").Bold || !ttyStyleAt(t, r, "under").Underline || !ttyStyleAt(t, r, "gone").Strikethrough {
		t.Fatal("emphasis or author override")
	}
	plain, states, last := decodeTTYANSI(t, r.ANSI())
	if plain != r.Text || last != (ttyTestState{}) {
		t.Fatal("unrestored terminal")
	}
	for _, span := range r.Styles {
		for i := span.Start; i < span.End; i++ {
			if states[i].TTYStyle != span.Style {
				t.Fatalf("state at %d: %+v != %+v", i, states[i], span.Style)
			}
		}
	}
}

func TestTTYStyledWrapTablesListsAndCode(t *testing.T) {
	html := `<style>h2{color:#123456}li{color:#aa0000}pre,code{color:#00aa00;background:#222222}table{background:#445566}th{color:#777777}td{color:#112233}em{color:#abcdef}</style>
 <h2>Heading</h2><ol><li>one <em>wide 世界 and é text</em> ending</li></ol>
 <table><tr><th>Key</th><th>Value</th></tr><tr><td><em>wrapped 世界 text</em></td><td>neighbor</td></tr></table>
 <pre><code>  first 世界
    second é line
last</code></pre><p>after</p>`
	for _, width := range []int{8, 22, 60} {
		r := styledTTY(t, html, width)
		_, states, last := decodeTTYANSI(t, r.ANSI())
		if last != (ttyTestState{}) {
			t.Fatal(last)
		}
		if !ttyStyleAt(t, r, "Heading"[:min(width, 7)]).Bold {
			t.Fatal("heading default")
		}
		if s := ttyStyleAt(t, r, "first"); s.Background != (color.RGBA{34, 34, 34, 255}) {
			t.Fatal("code background", s)
		}
		if s := ttyStyleAt(t, r, "after"); s != (TTYStyle{}) {
			t.Fatal("code style leak", s)
		}
		for _, span := range r.Styles {
			if span.Bounds.Dx() != ansi.StringWidth(r.Text[span.Start:span.End]) {
				t.Fatal("span width", span)
			}
			for i := span.Start; i < span.End; i++ {
				if states[i].TTYStyle != span.Style {
					t.Fatalf("bad wrapped style %d", i)
				}
			}
		}
		for _, line := range strings.Split(r.ANSI(), "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("wide line (%d): %q", width, line)
			}
			_, _, last := decodeTTYANSI(t, line)
			if last != (ttyTestState{}) {
				t.Fatal("line depends on surrounding state")
			}
		}
	}
	r := styledTTY(t, "<table style='background:#123456'><tbody><tr><td>A</td><td style='background:#abcdef'>B</td></tr></tbody></table>", 30)
	if ttyStyleAt(t, r, "A").Background != (color.RGBA{0x12, 0x34, 0x56, 255}) || ttyStyleAt(t, r, "B").Background != (color.RGBA{0xab, 0xcd, 0xef, 255}) {
		t.Fatal("table background inheritance")
	}
}

func TestTTYStylesLinksHighlightsAndCoordinates(t *testing.T) {
	html := `<section id="scope"><p>before <a id="link" href="https://example.com">Café 世界 é <em>wrapped words</em></a> after</p></section>`
	css := `<style>section{color:#123456;background:#abcdef}a{color:#ff8800}em{font-weight:bold}</style>`
	plain := styledTTY(t, html, 18)
	styled := styledTTY(t, css+html, 18)
	if plain.Text != styled.Text || !reflect.DeepEqual(plain.Anchors, styled.Anchors) || !reflect.DeepEqual(plain.AnchorRects, styled.AnchorRects) || !reflect.DeepEqual(plain.lineSources, styled.lineSources) {
		t.Fatal("styling changed navigation")
	}
	// CSS nodes shift numeric LinkIDs; compare byte offsets and cell bounds.
	if len(plain.Spans) != len(styled.Spans) {
		t.Fatal("link span count")
	}
	for i, a := range plain.Spans {
		b := styled.Spans[i]
		if a.Start != b.Start || a.End != b.End || a.Bounds != b.Bounds {
			t.Fatalf("link span changed: %+v %+v", a, b)
		}
	}
	idx := styled.TextIndex()
	hits := idx.Search("世界")
	if len(hits) != 1 || idx.Selected(hits[0].Start, hits[0].End) != "世界" {
		t.Fatal(hits)
	}
	ranges := append(hits, Match{Start: strings.Index(styled.Text, "Café"), End: hits[0].End})
	text, states, last := decodeTTYANSI(t, styled.ANSIHighlights(ranges))
	if text != styled.Text || last != (ttyTestState{}) {
		t.Fatal("highlight text/reset")
	}
	for i := hits[0].Start; i < hits[0].End; i++ {
		if !states[i].reverse || states[i].href != "https://example.com" {
			t.Fatal(states[i])
		}
	}
	after := strings.Index(styled.Text, "after")
	if states[after].reverse || states[after].href != "" || states[after].Foreground != (color.RGBA{0x12, 0x34, 0x56, 255}) {
		t.Fatal("highlight/link leaked", states[after])
	}
	for _, span := range styled.Spans {
		id, ok := styled.SourceAt(span.Bounds.Min)
		want, wantOK := plain.SourceAt(span.Bounds.Min)
		if id != want || ok != wantOK {
			t.Fatalf("source=%s %v want=%s %v", id, ok, want, wantOK)
		}
		hit, ok := idx.Hit(image.Rect(span.Bounds.Min.X, span.Bounds.Min.Y, span.Bounds.Min.X+1, span.Bounds.Min.Y+1))
		if !ok || hit.Start < 0 || hit.End > len(styled.Text) {
			t.Fatal(hit)
		}
	}
}

func TestTTYColorProfilesAndControlSanitization(t *testing.T) {
	r := styledTTY(t, "<p style='color:#123456;background:#654321'><b>bold</b> <a href='https://example.com'>link</a></p><pre>\x1b[31mBAD\x1b]52;c;abc\x07</pre>", 40)
	for _, profile := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI256, colorprofile.ANSI, colorprofile.ASCII, colorprofile.NoTTY, colorprofile.Unknown} {
		out := r.ANSIHighlightsWithProfile([]Match{{Start: 0, End: 4}}, profile)
		if ansi.Strip(out) != r.Text {
			t.Fatal("profile changed text", profile)
		}
		if strings.Contains(out, "\x1b]52;") || strings.Contains(out, "\x1b[31mBAD") {
			t.Fatal("source controls leaked")
		}
		switch profile {
		case colorprofile.TrueColor:
			if !strings.Contains(out, "38;2;18;52;86") {
				t.Fatal(out)
			}
		case colorprofile.ANSI256:
			if !strings.Contains(out, "38;5;") || strings.Contains(out, "38;2;") {
				t.Fatal(out)
			}
		case colorprofile.ANSI:
			if strings.Contains(out, "38;") || strings.Contains(out, "48;") {
				t.Fatal(out)
			}
		case colorprofile.ASCII:
			if strings.Contains(out, "38;") || strings.Contains(out, "48;") || !strings.Contains(out, "\x1b[7m") || !strings.Contains(out, "https://example.com") {
				t.Fatal(out)
			}
		default:
			if out != r.Text {
				t.Fatal(out)
			}
		}
	}
	// Equal foreground/background colors must not hide highlighted text.
	r = styledTTY(t, "<span style='color:#222222;background:#222222'>match</span>", 20)
	_, states, _ := decodeTTYANSI(t, r.ANSIHighlights([]Match{{Start: 0, End: 5}}))
	if !states[0].reverse || states[0].Foreground == states[0].Background {
		t.Fatal("invisible highlight")
	}
}

func TestTTYStyledWhitespaceUnchanged(t *testing.T) {
	for _, html := range []string{
		"<p>a <span> b </span> c</p>",
		"<p style='white-space:nowrap'> a <span> b </span> c </p>",
		"<pre>\n  a <span> b </span> c \n next</pre>",
		"<p>a<span> </span>b<br>c</p>",
	} {
		baseline := styledTTY(t, html, 8)
		colored := styledTTY(t, "<style>p,pre{color:red}span{color:blue}</style>"+html, 8)
		if colored.Text != baseline.Text {
			t.Fatalf("%s: %q != %q", html, colored.Text, baseline.Text)
		}
	}
}

func TestTTYStylesDoNotSplitLinkRegions(t *testing.T) {
	plain := styledTTY(t, "<a href='https://example.com'>A<span>B</span>C</a>", 20)
	styled := styledTTY(t, "<style>span{color:red;background:blue}</style><a href='https://example.com'>A<span>B</span>C</a>", 20)
	if plain.Text != styled.Text || len(styled.Spans) != 1 || plain.Spans[0].Bounds != styled.Spans[0].Bounds {
		t.Fatalf("style boundaries changed link geometry: %+v %+v", plain.Spans, styled.Spans)
	}
}

func TestTTYInheritedBackgroundOnCollapsedSpaces(t *testing.T) {
	r := styledTTY(t, "<p style='background:#123456'>one <em>two</em> three</p>", 30)
	_, states, _ := decodeTTYANSI(t, r.ANSI())
	for i, c := range r.Text {
		if c == ' ' && states[i].Background != (color.RGBA{0x12, 0x34, 0x56, 255}) {
			t.Fatalf("background lost at space %d: %+v", i, states[i])
		}
	}
}

func TestNilTTYLayout(t *testing.T) {
	var d *Document
	r, err := d.TTYLayout(context.Background(), 80)
	if err != nil || r.Text != "" || r.ANSI() != "" {
		t.Fatalf("%+v %v", r, err)
	}
}
