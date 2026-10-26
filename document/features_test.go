package document

import (
	"context"
	"errors"
	"image"
	"image/color"
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

func TestFirstLineScopingCurrentColorVisibilityAndLogicalCSS(t *testing.T) {
	const src = `<html><head><style>
		body { margin: 0; }
		div.fl { color: red; }
		div.fl::first-line { color: green; }
		br { line-height: 0; }
		span.box {
			line-height: 40px;
			background-color: currentColor;
			border: 4px solid currentColor;
		}
		.hide { visibility: hidden; background: red; border: 4px solid red; }
		.show { visibility: visible; color: blue; }
		.logical {
			position: relative;
			width: 100px;
			height: 20px;
			margin-block: 6px 10px;
			margin-inline: 12px 8px;
			padding-block: 2px 4px;
			padding-inline: 5px 7px;
			text-align: end;
		}
		.abs {
			position: absolute;
			inset: 2px 4px 6px 8px;
			background: lime;
		}
		.kw { font: bolder x-large/1.5 sans-serif; }
		.kw-sub { font-size: smaller; font-weight: lighter; }
	</style></head><body>
		<div class="fl"><span class="box">First<br>Second</span></div>
		<div class="hide">Hidden<span class="show">Visible</span></div>
		<div class="logical"><div class="abs"></div>End</div>
		<div class="kw">Big <span class="kw-sub">Small</span></div>
	</body></html>`
	r, err := Render(context.Background(), strings.NewReader(src), Options{Width: 300})
	if err != nil {
		t.Fatal(err)
	}
	green := color.RGBA{0, 128, 0, 255}
	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	var firstBox, secondBox, hiddenBox, visibleBox, bigBox, smallBox Box
	for _, b := range r.Boxes {
		switch b.Text {
		case "First":
			firstBox = b
		case "Second":
			secondBox = b
		case "Hidden":
			hiddenBox = b
		case "Visible":
			visibleBox = b
		case "Big":
			bigBox = b
		case "Small":
			smallBox = b
		}
	}
	if firstBox.Color != green || secondBox.Color != red {
		t.Fatalf("expected First=green and Second=red after <br>, got First=%v Second=%v", firstBox.Color, secondBox.Color)
	}
	if got := r.Image.RGBAAt(2, 2); got != green {
		t.Fatalf("expected line 1 span border to be green at (2,2), got %v", got)
	}
	if got := r.Image.RGBAAt(2, 42); got != red {
		t.Fatalf("expected line 2 span border to be red at (2,42), got %v", got)
	}
	if hiddenBox.Color.A != 0 || visibleBox.Color != blue {
		t.Fatalf("expected Hidden alpha=0 and Visible=blue, got Hidden=%v Visible=%v", hiddenBox.Color, visibleBox.Color)
	}
	if visibleBox.Bounds.Min.X <= hiddenBox.Bounds.Min.X {
		t.Fatalf("expected hidden text to reserve horizontal advance before visible child: hidden=%v visible=%v", hiddenBox.Bounds, visibleBox.Bounds)
	}
	if bigBox.FontSize != 24 || !bigBox.Bold || smallBox.FontSize != 20 || smallBox.Bold {
		t.Fatalf("unexpected keyword font sizes/weights: big=%+v small=%+v", bigBox, smallBox)
	}
}

func TestAtRulesAndMediaQueries(t *testing.T) {
	ctx := context.Background()
	white := color.RGBA{255, 255, 255, 255}
	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	green := color.RGBA{0, 128, 0, 255}

	blocks := `<body style="margin:0"><div class="a" style="height:40px">A</div><div class="b" style="height:40px">B</div><div class="c" style="height:40px">C</div></body>`
	src := `<style>@media (max-width:750px){.a{background:red}.b{background:blue}} .c{background:green}</style>` + blocks

	doc, err := Parse(ctx, strings.NewReader(src), Limits{})
	if err != nil {
		t.Fatal(err)
	}

	// Width 800: only .c painted
	r800, err := doc.Render(ctx, Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	if gotA, gotB, gotC := r800.Image.RGBAAt(20, 20), r800.Image.RGBAAt(20, 60), r800.Image.RGBAAt(20, 100); gotA != white || gotB != white || gotC != green {
		t.Fatalf("Width 800: got A=%v B=%v C=%v, want white, white, green", gotA, gotB, gotC)
	}

	// Width 700: .a, .b, and .c painted
	r700, err := doc.Render(ctx, Options{Width: 700})
	if err != nil {
		t.Fatal(err)
	}
	if gotA, gotB, gotC := r700.Image.RGBAAt(20, 20), r700.Image.RGBAAt(20, 60), r700.Image.RGBAAt(20, 100); gotA != red || gotB != blue || gotC != green {
		t.Fatalf("Width 700: got A=%v B=%v C=%v, want red, blue, green", gotA, gotB, gotC)
	}

	// Statement and non-@media block at-rules are skipped cleanly
	for _, css := range []string{
		`@import url(x.css); .c{background:green}`,
		`@charset "utf-8"; @namespace svg url(http://www.w3.org/2000/svg); .c{background:green}`,
		`@font-face{font-family:Custom;src:url(x)} .c{background:green}`,
		`@supports (display:grid){.a{background:red}} .c{background:green}`,
		`@keyframes spin{0%{opacity:0}100%{opacity:1}} .c{background:green}`,
	} {
		r, err := Render(ctx, strings.NewReader(`<style>`+css+`</style>`+blocks), Options{Width: 800})
		if err != nil {
			t.Fatalf("%s: %v", css, err)
		}
		if gotA, gotC := r.Image.RGBAAt(20, 20), r.Image.RGBAAt(20, 100); gotA != white || gotC != green {
			t.Fatalf("%s: got A=%v C=%v, want white, green", css, gotA, gotC)
		}
	}

	// Nested @media ANDs; depth 8 works, depth 9 is skipped without error
	nestedOK := `@media (min-width: 600px) { @media (max-width: 750px) { .a { background: red } } }`
	rNested700, err := Render(ctx, strings.NewReader(`<style>`+nestedOK+`</style>`+blocks), Options{Width: 700})
	if err != nil {
		t.Fatal(err)
	}
	if got := rNested700.Image.RGBAAt(20, 20); got != red {
		t.Fatalf("nested @media at 700: got %v, want red", got)
	}
	rNested800, err := Render(ctx, strings.NewReader(`<style>`+nestedOK+`</style>`+blocks), Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	if got := rNested800.Image.RGBAAt(20, 20); got != white {
		t.Fatalf("nested @media at 800: got %v, want white", got)
	}

	depth9 := strings.Repeat("@media screen {", 9) + ".a{background:red}" + strings.Repeat("}", 9) + ".c{background:green}"
	rDepth9, err := Render(ctx, strings.NewReader(`<style>`+depth9+`</style>`+blocks), Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	if gotA, gotC := rDepth9.Image.RGBAAt(20, 20), rDepth9.Image.RGBAAt(20, 100); gotA != white || gotC != green {
		t.Fatalf("depth 9: got A=%v C=%v, want white, green", gotA, gotC)
	}

	// Media query syntax variants
	for _, tc := range []struct {
		query string
		width int
		match bool
	}{
		{`only screen and (min-width : 690px) and (max-width : 809px)`, 800, true},
		{`only screen and (min-width : 690px) and (max-width : 809px)`, 810, false},
		{`print`, 800, false},
		{`not print`, 800, true},
		{`screen, print`, 800, true},
		{`(-webkit)`, 800, false},
		{`(-webkit-min-device-pixel-ratio: 2)`, 800, false},
		{``, 800, true},
		{`(width: 50em)`, 800, true},
	} {
		r, err := Render(ctx, strings.NewReader(`<style>@media `+tc.query+`{.a{background:red}}</style>`+blocks), Options{Width: tc.width})
		if err != nil {
			t.Fatalf("%s @ %d: %v", tc.query, tc.width, err)
		}
		want := white
		if tc.match {
			want = red
		}
		if got := r.Image.RGBAAt(20, 20); got != want {
			t.Fatalf("@media %s at %d: got %v, want %v", tc.query, tc.width, got, want)
		}
	}

	// TTY mode evaluates media queries against columns * 10 px
	ttyDoc, err := Parse(ctx, strings.NewReader(`<style>@media (max-width: 500px){.a{display:none}}</style><div class="a">NarrowHidden</div><div>Always</div>`), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	tty40, err := ttyDoc.TTY(ctx, 40) // 400 px
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tty40, "NarrowHidden") {
		t.Fatalf("expected NarrowHidden hidden at 40 cols (400px), got %q", tty40)
	}
	tty80, err := ttyDoc.TTY(ctx, 80) // 800 px
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tty80, "NarrowHidden") {
		t.Fatalf("expected NarrowHidden visible at 80 cols (800px), got %q", tty80)
	}

	// Rule budget: rules inside @media count toward Limits.Rules at Parse time
	if _, err := Parse(ctx, strings.NewReader(`<style>@media print { .a{color:red} .b{color:blue} .c{color:green} }</style>`), Limits{Rules: 2}); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected ErrLimit when @media rules exceed Limits.Rules, got %v", err)
	}
}

func TestPresentationalAttributes(t *testing.T) {
	ctx := context.Background()
	orange := color.RGBA{0xff, 0x66, 0x00, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}

	// 1. <center><table width="85%" cellpadding="0"><tr><td bgcolor="#ff6600">hdr</td></tr></table></center>
	rCenter, err := Render(ctx, strings.NewReader(`<center><table width="85%" cellpadding="0"><tr><td bgcolor="#ff6600">hdr</td></tr></table></center>`), Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	var hdrBox Box
	for _, b := range rCenter.Boxes {
		if b.Text == "hdr" {
			hdrBox = b
			break
		}
	}
	if hdrBox.Text == "" {
		t.Fatal("missing hdr text box")
	}
	if hdrBox.Bounds.Min.X < 55 || hdrBox.Bounds.Min.X > 75 {
		t.Fatalf("expected hdr starting near x=60, got x=%d (bounds=%v)", hdrBox.Bounds.Min.X, hdrBox.Bounds)
	}
	if got := rCenter.Image.RGBAAt(400, hdrBox.Bounds.Min.Y+2); got != orange {
		t.Fatalf("expected orange band across centered cell at (400,%d), got %v", hdrBox.Bounds.Min.Y+2, got)
	}

	// 2. <td align="right"> and <td valign="top"> in a 60px row
	rAlign, err := Render(ctx, strings.NewReader(`<table width="400"><tr><td align="right" valign="top" style="height:60px">rightaligned</td><td>second</td><td valign="bottom">bottomaligned</td></tr></table>`), Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	var rightBox, secondBox, bottomBox Box
	for _, b := range rAlign.Boxes {
		switch b.Text {
		case "rightaligned":
			rightBox = b
		case "second":
			secondBox = b
		case "bottomaligned":
			bottomBox = b
		}
	}
	var firstCell Box
	for _, b := range rAlign.Boxes {
		if b.Tag == "td" {
			firstCell = b
			break
		}
	}
	if rightBox.Bounds.Max.X < firstCell.Bounds.Max.X-10 {
		t.Fatalf("expected <td align=right> text near right edge of cell (%v), got %v", firstCell.Bounds, rightBox.Bounds)
	}
	if !(rightBox.Bounds.Min.Y < secondBox.Bounds.Min.Y && secondBox.Bounds.Min.Y < bottomBox.Bounds.Min.Y) {
		t.Fatalf("expected valign top < default(middle) < bottom, got top=%v middle=%v bottom=%v", rightBox.Bounds, secondBox.Bounds, bottomBox.Bounds)
	}

	// 3. cellpadding="10" offsets cell text by 10 px
	rPad0, err := Render(ctx, strings.NewReader(`<body style="margin:0"><table cellpadding="0" cellspacing="0"><tr><td>txt</td></tr></table></body>`), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	rPad10, err := Render(ctx, strings.NewReader(`<body style="margin:0"><table cellpadding="10" cellspacing="0"><tr><td>txt</td></tr></table></body>`), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	var box0, box10 Box
	for _, b := range rPad0.Boxes {
		if b.Text == "txt" {
			box0 = b
		}
	}
	for _, b := range rPad10.Boxes {
		if b.Text == "txt" {
			box10 = b
		}
	}
	if dx, dy := box10.Bounds.Min.X-box0.Bounds.Min.X, box10.Bounds.Min.Y-box0.Bounds.Min.Y; dx != 10 || dy != 10 {
		t.Fatalf("expected cellpadding=10 to offset cell text by (10,10), got (%d,%d) (pad0=%v pad10=%v)", dx, dy, box0.Bounds, box10.Bounds)
	}

	// 4. Author rule (including specificity 0,0,0) beats bgcolor
	for _, css := range []string{`td { background-color: white }`, `* { background-color: white }`} {
		rBeat, err := Render(ctx, strings.NewReader(`<style>`+css+`</style><body style="margin:0"><table cellpadding="0" cellspacing="0"><tr><td bgcolor="#ff6600" width="40" height="40">x</td></tr></table></body>`), Options{Width: 200})
		if err != nil {
			t.Fatal(err)
		}
		if got := rBeat.Image.RGBAAt(30, 20); got != white {
			t.Fatalf("%s: expected white to beat bgcolor, got %v", css, got)
		}
	}

	// 5. Legacy 6-hex bgcolor without '#', table border="1", <font>, and <body bgcolor/text/link>
	rLegacy, err := Render(ctx, strings.NewReader(`<body bgcolor="f6f6ef" text="#222222" link="#ff6600"><table border="1" width="200" height="40"><tr><td bgcolor="ff6600"><font color="#008000" size="5" face="monospace">fonttxt</font><a href="https://example.com">lnk</a></td></tr></table></body>`), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	if got := rLegacy.Background(); got != (color.RGBA{0xf6, 0xf6, 0xef, 0xff}) {
		t.Fatalf("expected body bgcolor #f6f6ef as canvas background, got %v", got)
	}
	if len(boxesWithTag(rLegacy, "border")) < 8 {
		t.Fatalf("expected border=1 on table to paint table and cell borders, got %d border boxes", len(boxesWithTag(rLegacy, "border")))
	}
	var fontBox, linkBox Box
	for _, b := range rLegacy.Boxes {
		if b.Text == "fonttxt" {
			fontBox = b
		}
		if b.Text == "lnk" {
			linkBox = b
		}
	}
	if fontBox.Color != (color.RGBA{0, 128, 0, 255}) || fontBox.FontSize != 24 || !fontBox.Monospace {
		t.Fatalf("unexpected <font> styling: %+v", fontBox)
	}
	if linkBox.Color != orange {
		t.Fatalf("expected <body link=#ff6600> to color link orange, got %v", linkBox.Color)
	}
}

func TestImageEmptyAltAndSpacer(t *testing.T) {
	ctx := context.Background()

	// 1. <p>before<img src="s.gif" alt="" width="40" height="1">after</p>
	src := `<p>before<img src="s.gif" alt="" width="40" height="1">after</p>`
	r, err := Render(ctx, strings.NewReader(src), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	var beforeBox, afterBox Box
	for _, b := range r.Boxes {
		if strings.Contains(b.Text, "[image:") {
			t.Fatalf("expected no [image: text box for empty alt, got %+v", b)
		}
		switch b.Text {
		case "before":
			beforeBox = b
		case "after":
			afterBox = b
		}
	}
	if beforeBox.Text == "" || afterBox.Text == "" {
		t.Fatalf("missing before/after boxes: before=%+v after=%+v", beforeBox, afterBox)
	}
	if gap := afterBox.Bounds.Min.X - beforeBox.Bounds.Max.X; gap != 40 {
		t.Fatalf("expected after to start 40px right of before's end, got gap=%d (before=%v after=%v)", gap, beforeBox.Bounds, afterBox.Bounds)
	}

	// 2. Missing alt without width+height takes no space and renders no text
	rNoDim, err := Render(ctx, strings.NewReader(`<p>before<img src="s.gif">after</p>`), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	var b0, a0 Box
	for _, b := range rNoDim.Boxes {
		if strings.Contains(b.Text, "[image:") {
			t.Fatalf("expected no [image: text box for missing alt, got %+v", b)
		}
		switch b.Text {
		case "before":
			b0 = b
		case "after":
			a0 = b
		}
	}
	if gap := a0.Bounds.Min.X - b0.Bounds.Max.X; gap != 0 {
		t.Fatalf("expected missing alt without width+height to take 0px, got gap=%d", gap)
	}

	// 3. Non-empty alt still produces "[image: logo]" across Render, TTY, and ExtractTypeset
	withAlt := `<p><img src="logo.png" alt="logo"></p><p><img src="s.gif" alt=""></p>`
	doc, err := Parse(ctx, strings.NewReader(withAlt), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	rAlt, err := doc.Render(ctx, Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	if txt := rAlt.TextIndex().Text; !strings.Contains(txt, "[image: logo]") || strings.Contains(txt, "[image: ]") {
		t.Fatalf("unexpected Render text for alt: %q", txt)
	}
	ttyTxt, err := doc.TTY(ctx, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ttyTxt, "[image: logo]") || strings.Contains(ttyTxt, "[image: ]") {
		t.Fatalf("unexpected TTY text for alt: %q", ttyTxt)
	}
	td, err := doc.ExtractTypeset(ctx, Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	var typesetTexts []string
	for _, blk := range td.Blocks {
		for _, sp := range blk.Spans {
			typesetTexts = append(typesetTexts, sp.Text)
		}
	}
	joinedTypeset := strings.Join(typesetTexts, " ")
	if !strings.Contains(joinedTypeset, "logo") || strings.Contains(joinedTypeset, "s.gif") || strings.Contains(joinedTypeset, "[image: ]") {
		t.Fatalf("unexpected ExtractTypeset spans for alt: %q", joinedTypeset)
	}
}

func TestLinkAnchorAndEmptyImageLink(t *testing.T) {
	ctx := context.Background()
	src := `<!doctype html><body>
<a id="img-link" href="HTTP://example.test/A"><img alt=""></a>
<a id="dup-1" href="/dup">one</a> <a name="dup-2" href="/dup">two</a>
<a id="empty-anchor" href="/empty"></a>
</body>`
	doc, err := Parse(ctx, strings.NewReader(src), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := doc.Render(ctx, Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Links) != 4 {
		t.Fatalf("expected 4 links in Result.Links, got %+v", r.Links)
	}
	wantAnchors := []string{"img-link", "dup-1", "dup-2", "empty-anchor"}
	wantHrefs := []string{"http://example.test/A", "/dup", "/dup", "/empty"}
	wantTexts := []string{"http://example.test/A", "one", "two", "/empty"}
	for i := range wantAnchors {
		if r.Links[i].Anchor != wantAnchors[i] || r.Links[i].Href != wantHrefs[i] || r.Links[i].Text != wantTexts[i] {
			t.Fatalf("link[%d] = %+v, want anchor=%q href=%q text=%q", i, r.Links[i], wantAnchors[i], wantHrefs[i], wantTexts[i])
		}
		if len(r.Links[i].Regions) == 0 {
			t.Fatalf("link[%d] has no regions: %+v", i, r.Links[i])
		}
	}
	ttyRes, err := doc.TTYLayout(ctx, 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(ttyRes.Links) != 2 || ttyRes.Links[0].Anchor != "dup-1" || ttyRes.Links[1].Anchor != "dup-2" {
		t.Fatalf("unexpected TTYResult.Links: %+v", ttyRes.Links)
	}
}
