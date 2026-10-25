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

func TestPhase5ElementsAndTypography(t *testing.T) {
	d := parseFeature(t, `<h3>H3</h3><h4>H4</h4><h5>H5</h5><h6>H6</h6>`+
		`<blockquote>Quote</blockquote><hr><dl><dt>Term</dt><dd>Def</dd></dl>`+
		`<p><small>Small</small> <sub>Sub</sub> <sup>Sup</sup> <mark>Mark</mark></p>`+
		`<div style="text-indent:24px;letter-spacing:4px;text-transform:uppercase;font-variant:small-caps">ab cd</div>`)
	r, err := d.Render(context.Background(), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	tty, err := d.TTY(context.Background(), 40)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tty, "────────") || !strings.Contains(tty, "AB CD") {
		t.Fatalf("expected horizontal rule and uppercase text in TTY, got:\n%s", tty)
	}
	var h3Size, h6Size, smallSize int
	var indentedX int
	for _, b := range r.Boxes {
		switch b.Text {
		case "H3":
			h3Size = b.FontSize
		case "H6":
			h6Size = b.FontSize
		case "Small":
			smallSize = b.FontSize
		case "AB":
			indentedX = b.Bounds.Min.X
		}
	}
	if h3Size != 20 || h6Size != 12 || smallSize >= 16 || indentedX < 24 {
		t.Fatalf("unexpected element/typography metrics: h3=%d h6=%d small=%d indentX=%d", h3Size, h6Size, smallSize, indentedX)
	}
}

func TestPhase5ListsAndTables(t *testing.T) {
	d := parseFeature(t, `<ol reversed><li>First</li><li>Second</li></ol>`+
		`<ol type="I"><li>Roman1</li><li>Roman2</li></ol>`+
		`<ul style="list-style-type:square"><li>Sq</li><li style="list-style:none">HiddenMarker</li></ul>`+
		`<table style="caption-side:bottom"><caption>BottomCap</caption><colgroup><col width="120" style="background:rgb(255, 0, 0)"><col width="80"></colgroup><tr><td>A</td><td>B</td></tr></table>`)
	tty, err := d.TTY(context.Background(), 60)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2. First", "1. Second", "I. Roman1", "II. Roman2", "▪ Sq"} {
		if !strings.Contains(tty, want) {
			t.Fatalf("missing %q in TTY:\n%s", want, tty)
		}
	}
	if strings.Index(tty, "BottomCap") < strings.Index(tty, "│ A") {
		t.Fatalf("expected bottom caption after table rows in TTY:\n%s", tty)
	}
	r, err := d.Render(context.Background(), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	var tdA, capBox Box
	for _, b := range r.Boxes {
		if b.Tag == "td" && tdA.Tag == "" {
			tdA = b
		}
		if b.Text == "BottomCap" {
			capBox = b
		}
	}
	if tdA.Color.R != 255 || tdA.Bounds.Dx() < 120 || capBox.Bounds.Min.Y < tdA.Bounds.Max.Y {
		t.Fatalf("unexpected colgroup/caption layout: td=%+v cap=%+v", tdA, capBox)
	}
}

func TestPhase5FlexboxAndOverflow(t *testing.T) {
	d := parseFeature(t, `<div style="display:flex;gap:10px;width:310px">`+
		`<div id="f1" style="flex:1;height:20px;background:red"></div>`+
		`<div id="f2" style="flex:2;height:40px;background:blue"></div>`+
		`</div>`+
		`<div style="width:60px;height:20px;overflow:hidden;white-space:nowrap;text-overflow:ellipsis;font:16px monospace">LongUnbrokenIdentifierText</div>`+
		`<div style="width:40px;height:20px;overflow:hidden"><div style="width:40px;height:80px;background:rgb(255,0,0)"></div></div>`)
	r, err := d.Render(context.Background(), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	f1 := r.Anchors["f1"]
	f2 := r.Anchors["f2"]
	if f1.X != 0 || f2.X != 110 {
		t.Fatalf("unexpected flex item positions: f1=%v f2=%v", f1, f2)
	}
	hasEllipsis := false
	for _, b := range r.Boxes {
		if strings.Contains(b.Text, "…") {
			hasEllipsis = true
		}
	}
	if !hasEllipsis {
		t.Fatalf("expected ellipsis truncation, boxes=%+v", r.Boxes)
	}
	if r.Image.Bounds().Dy() > 90 {
		t.Fatalf("expected overflow:hidden child not to enlarge canvas beyond clipped box, got height %d", r.Image.Bounds().Dy())
	}
}

func TestHeadingAlignmentTablePaddingAndSpaceUnderlines(t *testing.T) {
	const src = `<html><body style="margin:0">
		<h1>Field notes</h1>
		<p>Paragraph text aligned with heading.</p>
		<h2>The ridge</h2>
		<p>The <a href="https://example.com">gallery's own readme</a> says more.</p>
		<table><tr><th>Day</th><th>Species</th><th>Count</th></tr><tr><td>Wed</td><td>Kingfisher</td><td>3</td></tr></table>
	</body></html>`
	r, err := Render(context.Background(), strings.NewReader(src), Options{Width: 600})
	if err != nil {
		t.Fatal(err)
	}
	var h1X, h2X, pX int
	var galleryBox, ownBox Box
	var kingfisherBox, count3Box Box
	for _, b := range r.Boxes {
		switch b.Text {
		case "Field":
			h1X = b.Bounds.Min.X
		case "Paragraph":
			pX = b.Bounds.Min.X
		case "The":
			if b.Bold && h2X == 0 {
				h2X = b.Bounds.Min.X
			}
		case "gallery's":
			galleryBox = b
		case "own":
			ownBox = b
		case "Kingfisher":
			kingfisherBox = b
		case "3":
			count3Box = b
		}
	}
	if h1X != pX || h2X != pX {
		t.Fatalf("headings should align with paragraph left edge: h1=%d h2=%d p=%d", h1X, h2X, pX)
	}
	if count3Box.Bounds.Min.X-kingfisherBox.Bounds.Max.X < 8 {
		t.Fatalf("expected horizontal gap between table cells, got %dpx (%v to %v)", count3Box.Bounds.Min.X-kingfisherBox.Bounds.Max.X, kingfisherBox.Bounds, count3Box.Bounds)
	}
	spaceX := (galleryBox.Bounds.Max.X + ownBox.Bounds.Min.X) / 2
	underY := galleryBox.Bounds.Min.Y + galleryBox.Baseline + 1
	px := r.Image.RGBAAt(spaceX, underY)
	if px.R == 255 && px.G == 255 && px.B == 255 {
		t.Fatalf("expected underline across space between link words at (%d,%d), got white %v", spaceX, underY, px)
	}
}
