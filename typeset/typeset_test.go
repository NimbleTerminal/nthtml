package typeset

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

func stripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

func TestBuildAndRasterWithNTTypeset(t *testing.T) {
	const src = `<html><body>
		<h1>Typeset Bridge</h1>
		<p>Hello <b>bold</b>, <i>italic</i>, <code>mono</code>, <s>strike</s>, and <a href="https://nimbleterminal.dev">link</a>.</p>
		<blockquote><p>A quoted observation.</p></blockquote>
		<ol start="3"><li>Third item</li><li>Fourth item</li></ol>
		<pre><code class="language-go">package main
func main() {}</code></pre>
		<hr>
		<table><thead><tr><th>Name</th><th>Value</th></tr></thead><tbody><tr><td>Alpha</td><td>42</td></tr></tbody></table>
	</body></html>`

	ctx := context.Background()
	doc, err := document.Parse(ctx, strings.NewReader(src), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	tdoc, err := Build(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tdoc.Blocks) < 7 {
		t.Fatalf("expected at least 7 blocks, got %d", len(tdoc.Blocks))
	}
	if tdoc.Blocks[0].Kind != nttypeset.KindHeading || tdoc.Blocks[0].Size != 32 {
		t.Fatalf("expected H1 heading block with UA size 32, got %+v", tdoc.Blocks[0])
	}

	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	img, lay, err := Raster(ctx, eng, doc, 600, nttypeset.View{Width: 600, Height: 400, Scale: 1}, nttypeset.RasterOptions{Palette: nttypeset.LightPalette()}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 400 {
		t.Fatalf("unexpected raster bounds %v", img.Bounds())
	}
	links := lay.Links(nttypeset.View{Width: 600, Height: 400, Scale: 1})
	if len(links) == 0 || links[0].URL != "https://nimbleterminal.dev" {
		t.Fatalf("expected link hit area from nttypeset layout, got %+v", links)
	}
}

func TestFromResultBoxes(t *testing.T) {
	ctx := context.Background()
	res, err := document.Render(ctx, strings.NewReader(`<h1>Heading</h1><p>Paragraph with <a href="#sec">anchor</a>.</p><hr>`), document.Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	tdoc := FromResult(res)
	if len(tdoc.Blocks) < 3 {
		t.Fatalf("expected heading, paragraph, and rule blocks, got %d", len(tdoc.Blocks))
	}
	if tdoc.Blocks[0].Kind != nttypeset.KindHeading {
		t.Fatalf("expected first block to be heading, got %v", tdoc.Blocks[0].Kind)
	}
	if tdoc.Blocks[len(tdoc.Blocks)-1].Kind != nttypeset.KindRule {
		t.Fatalf("expected last block to be rule, got %v", tdoc.Blocks[len(tdoc.Blocks)-1].Kind)
	}
}

func layoutText(t *testing.T, ctx context.Context, lay *nttypeset.Layout) string {
	t.Helper()
	sel, err := lay.Select(ctx, nttypeset.Selection{
		Anchor: nttypeset.Place{Point: image.Pt(0, 0)},
		Focus:  nttypeset.Place{Point: image.Pt(lay.Width(), lay.Extent())},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sel.Text()
}

func TestStructuredSectionsCrossRendererParity(t *testing.T) {
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("testdata", "structured_sections.html"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := document.Parse(ctx, strings.NewReader(string(raw)), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}

	// Render across Native, TTY, and Bridged nttypeset at wide (800px / 80 cols) and narrow (440px / 44 cols).
	nativeWide, err := doc.Render(ctx, document.Options{Width: 800})
	if err != nil {
		t.Fatal(err)
	}
	nativeNarrow, err := doc.Render(ctx, document.Options{Width: 440})
	if err != nil {
		t.Fatal(err)
	}
	ttyWide, err := doc.TTYLayout(ctx, 80)
	if err != nil {
		t.Fatal(err)
	}
	ttyNarrow, err := doc.TTYLayout(ctx, 44)
	if err != nil {
		t.Fatal(err)
	}
	tdoc, sm, err := BuildWithSources(ctx, doc, Options{Document: document.Options{Width: 800}})
	if err != nil {
		t.Fatal(err)
	}
	layWide, err := eng.Layout(tdoc, 800, nttypeset.LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := layWide.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	layNarrow, err := eng.Layout(tdoc, 440, nttypeset.LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := layNarrow.Complete(ctx); err != nil {
		t.Fatal(err)
	}

	// 1. Reading-order text & display:none parity across all three renderers.
	nativeText := nativeWide.TextIndex().Text
	ttyText := stripANSI(ttyWide.Text)
	bridgedText := layoutText(t, ctx, layWide)

	orderedPhrases := []string{
		"Structured feed in terminal and web",
		"Author",
		"Can you make the document viewer easier to read?",
		"Reviewer",
		"Reasoning",
		"Preserving section boxes and source anchors",
		"A shared document pipeline",
		"Stage",
		"Responsibility",
		"document.Parse(ctx, r, document.Limits{})",
		"Action: Inspect",
		"Outcome details",
		"Service",
		"Action error",
		"Permission denied: the target resource could not be opened.",
		"Originating action",
		"Summary",
		"The same semantic content should survive a change of rendering mode.",
		"Preserve section anchors.",
		"the renderer",
		"[Image: Architecture diagram]",
		"Attachment",
		"Image — image/png",
		"Collapsed note (summary only)",
		"Exported fixture",
	}
	for name, rawTxt := range map[string]string{
		"native":  nativeText,
		"tty":     ttyText,
		"bridged": bridgedText,
	} {
		txt := strings.Join(strings.Fields(rawTxt), " ")
		if strings.Contains(txt, "HIDDEN_COLLAPSED_PAYLOAD_SHOULD_NEVER_RENDER") {
			t.Fatalf("%s rendered display:none content", name)
		}
		lastIdx := -1
		for _, phrase := range orderedPhrases {
			idx := strings.Index(txt, phrase)
			if idx < 0 {
				t.Fatalf("%s missing expected phrase %q in:\n%s", name, phrase, txt)
			}
			if idx <= lastIdx {
				t.Fatalf("%s phrase %q out of reading order (idx %d <= %d)", name, phrase, idx, lastIdx)
			}
			lastIdx = idx
		}
	}

	// 2. Source IDs, parent/root hierarchy, and block ranges.
	expectedIDs := []string{
		"entry-alpha",
		"entry-alpha-text",
		"entry-beta",
		"entry-beta-note",
		"entry-beta-prose",
		"entry-beta-action",
		"entry-gamma",
		"entry-gamma-error",
		"entry-delta",
		"entry-delta-summary",
		"entry-delta-media",
		"entry-delta-collapsed",
	}
	gotIDs := sm.IDs()
	for _, want := range expectedIDs {
		if !containsString(gotIDs, want) {
			t.Fatalf("SourceMap missing ID %q; got %v", want, gotIDs)
		}
	}
	if containsString(gotIDs, "hidden-collapsed-body") {
		t.Fatalf("SourceMap must omit display:none IDs; got %v", gotIDs)
	}
	if p := sm.ParentID("entry-beta-action"); p != "entry-beta" {
		t.Fatalf("ParentID(entry-beta-action) = %q, want entry-beta", p)
	}
	if p := doc.ParentID("entry-beta-action"); p != "entry-beta" {
		t.Fatalf("doc.ParentID(entry-beta-action) = %q, want entry-beta", p)
	}
	if r := sm.RootID("#entry-beta-action"); r != "entry-beta" {
		t.Fatalf("RootID(#entry-beta-action) = %q, want entry-beta", r)
	}
	if r := doc.RootID("#entry-beta-action"); r != "entry-beta" {
		t.Fatalf("doc.RootID(#entry-beta-action) = %q, want entry-beta", r)
	}
	pStart, pEnd, ok := sm.BlockRange("entry-beta")
	if !ok {
		t.Fatal("missing BlockRange for entry-beta")
	}
	for _, child := range []string{"entry-beta-note", "entry-beta-prose", "entry-beta-action"} {
		cStart, cEnd, ok := sm.BlockRange(child)
		if !ok || cStart < pStart || cEnd > pEnd {
			t.Fatalf("child %s range [%d,%d) not within parent entry-beta [%d,%d)", child, cStart, cEnd, pStart, pEnd)
		}
	}

	// 3. Anchor bounds, hit-to-source resolution, and stability across wide/narrow reflows.
	if nativeNarrow.Image.Bounds().Dy() <= nativeWide.Image.Bounds().Dy() {
		t.Fatalf("expected narrow native height (%d) > wide (%d)", nativeNarrow.Image.Bounds().Dy(), nativeWide.Image.Bounds().Dy())
	}
	if strings.Count(ttyNarrow.Text, "\n") <= strings.Count(ttyWide.Text, "\n") {
		t.Fatalf("expected narrow TTY lines (%d) > wide (%d)", strings.Count(ttyNarrow.Text, "\n"), strings.Count(ttyWide.Text, "\n"))
	}
	if layNarrow.Extent() <= layWide.Extent() {
		t.Fatalf("expected narrow bridged extent (%d) > wide (%d)", layNarrow.Extent(), layWide.Extent())
	}

	var prevNativeY, prevTTYY, prevBridgedWideY, prevBridgedNarrowY int
	for _, id := range []string{"entry-alpha", "entry-beta", "entry-beta-note", "entry-beta-prose", "entry-beta-action", "entry-gamma", "entry-gamma-error", "entry-delta", "entry-delta-summary", "entry-delta-media", "entry-delta-collapsed"} {
		nr, ok := nativeWide.AnchorBounds("#" + id)
		if !ok || nr.Empty() {
			t.Fatalf("native missing anchor %s", id)
		}
		if nr.Min.Y < prevNativeY {
			t.Fatalf("native anchor %s out of vertical order (%d < %d)", id, nr.Min.Y, prevNativeY)
		}
		prevNativeY = nr.Min.Y

		if hitID, ok := nativeWide.SourceAt(image.Pt(nr.Min.X+4, nr.Min.Y+4)); !ok || (hitID != id && doc.RootID(hitID) != doc.RootID(id)) {
			t.Fatalf("native SourceAt(%s) = %q, %v", id, hitID, ok)
		}

		tr, ok := ttyWide.AnchorBounds("#" + id)
		if !ok || tr.Empty() {
			t.Fatalf("TTY missing anchor %s", id)
		}
		if tr.Min.Y < prevTTYY {
			t.Fatalf("TTY anchor %s out of vertical order (%d < %d)", id, tr.Min.Y, prevTTYY)
		}
		prevTTYY = tr.Min.Y
		if hitID, ok := ttyWide.SourceAt(image.Pt(1, tr.Min.Y)); !ok || (hitID != id && doc.RootID(hitID) != doc.RootID(id)) {
			t.Fatalf("TTY SourceAt(%s) = %q, %v", id, hitID, ok)
		}

		bw, ok, err := sm.AnchorBounds(ctx, layWide, "#"+id, true)
		if err != nil || !ok || !bw.Complete || bw.Rect.Empty() {
			t.Fatalf("bridged wide missing anchor %s: %+v, ok=%v, err=%v", id, bw, ok, err)
		}
		if bw.Rect.Min.Y < prevBridgedWideY {
			t.Fatalf("bridged wide anchor %s out of vertical order (%d < %d)", id, bw.Rect.Min.Y, prevBridgedWideY)
		}
		prevBridgedWideY = bw.Rect.Min.Y

		bn, ok, err := sm.AnchorBounds(ctx, layNarrow, id, true)
		if err != nil || !ok || !bn.Complete || bn.Rect.Empty() {
			t.Fatalf("bridged narrow missing anchor %s: %+v, ok=%v, err=%v", id, bn, ok, err)
		}
		if bn.Rect.Min.Y < prevBridgedNarrowY {
			t.Fatalf("bridged narrow anchor %s out of vertical order (%d < %d)", id, bn.Rect.Min.Y, prevBridgedNarrowY)
		}
		prevBridgedNarrowY = bn.Rect.Min.Y

		bStart, _, ok := sm.BlockRange(id)
		if !ok {
			t.Fatalf("missing block range for %s", id)
		}
		hitWide, ok, err := sm.SourceAt(ctx, layWide, nttypeset.Place{Block: bStart, Placed: true})
		if err != nil || !ok || (hitWide != id && sm.RootID(hitWide) != sm.RootID(id)) {
			t.Fatalf("bridged wide SourceAt(%s) = %q, ok=%v, err=%v", id, hitWide, ok, err)
		}
		hitNarrow, ok, err := sm.SourceAt(ctx, layNarrow, nttypeset.Place{Block: bStart, Placed: true})
		if err != nil || !ok || hitNarrow != hitWide {
			t.Fatalf("bridged narrow SourceAt(%s) = %q != wide %q", id, hitNarrow, hitWide)
		}
	}

	// 4. Link and #fragment target mapping across renderers.
	wantLinks := []string{"#entry-gamma-error", "#entry-beta-action", "https://nimbleterminal.dev/nthtml"}
	bridgedLinks := layWide.Links(nttypeset.View{Width: 800, Height: layWide.Extent(), Scale: 1})
	for _, href := range wantLinks {
		if !hasDocLink(nativeWide.Links, href) {
			t.Fatalf("native missing link %q", href)
		}
		if !hasDocLink(ttyWide.Links, href) {
			t.Fatalf("TTY missing link %q", href)
		}
		if !hasTypesetLink(bridgedLinks, href) {
			t.Fatalf("bridged missing link %q", href)
		}
	}
	if hasDocLink(nativeWide.Links, "#entry-alpha") || hasDocLink(ttyWide.Links, "#entry-alpha") || hasTypesetLink(bridgedLinks, "#entry-alpha") {
		t.Fatal("hidden link inside display:none must not be exposed")
	}

	// 5. Supported style preservation: font sizes, explicit ink colors, section backgrounds, and borders.
	var sawH128, sawH220, sawH316, sawMeta13 bool
	var sawTitleColor, sawHeaderColor, sawMetaColor, sawLinkColor bool
	for _, b := range tdoc.Blocks {
		if b.Kind == nttypeset.KindHeading && b.Size == 50 {
			t.Fatalf("heading size was replaced with hardcoded 50: %+v", b)
		}
		if b.Kind == nttypeset.KindHeading && b.Size == 28 {
			sawH128 = true
		}
		if b.Kind == nttypeset.KindHeading && b.Size == 20 {
			sawH220 = true
		}
		if b.Kind == nttypeset.KindHeading && b.Size == 16 {
			sawH316 = true
		}
		if b.Kind == nttypeset.KindParagraph && b.Size == 13 {
			sawMeta13 = true
		}
		for _, sp := range b.Spans {
			if sp.Ink == (color.RGBA{0xe3, 0xe8, 0xf0, 0xff}) && sp.Role == nttypeset.RoleText {
				sawTitleColor = true
			}
			if sp.Ink == (color.RGBA{0xc6, 0xd2, 0xe1, 0xff}) && sp.Role == nttypeset.RoleText {
				sawHeaderColor = true
			}
			if sp.Ink == (color.RGBA{0xaa, 0xb9, 0xcc, 0xff}) && sp.Role == nttypeset.RoleText {
				sawMetaColor = true
			}
			if sp.Href != "" && sp.Ink == (color.RGBA{0x9f, 0xc3, 0xff, 0xff}) && sp.Role == nttypeset.RoleText {
				sawLinkColor = true
			}
		}
	}
	if !sawH128 || !sawH220 || !sawH316 || !sawMeta13 {
		t.Fatalf("expected resolved CSS sizes (28, 20, 16, 13), got h1=%v h2=%v h3=%v meta=%v", sawH128, sawH220, sawH316, sawMeta13)
	}
	if !sawTitleColor || !sawHeaderColor || !sawMetaColor || !sawLinkColor {
		t.Fatalf("expected explicit CSS colors preserved without RoleAccent override: body=%v header=%v meta=%v link=%v", sawTitleColor, sawHeaderColor, sawMetaColor, sawLinkColor)
	}

	viewWide := nttypeset.View{Width: 800, Height: layWide.Extent(), Scale: 1}
	imgWide, err := layWide.Raster(ctx, viewWide, nttypeset.RasterOptions{Palette: sm.Palette()})
	if err != nil {
		t.Fatal(err)
	}
	viewNarrow := nttypeset.View{Width: 440, Height: layNarrow.Extent(), Scale: 1}
	imgNarrow, err := layNarrow.Raster(ctx, viewNarrow, nttypeset.RasterOptions{Palette: sm.Palette()})
	if err != nil {
		t.Fatal(err)
	}
	expectedColors := map[string]color.RGBA{
		"page-bg":          {0x10, 0x14, 0x1c, 0xff},
		"card-bg":          {0x19, 0x21, 0x2d, 0xff},
		"card-border":      {0x34, 0x41, 0x55, 0xff},
		"primary-border":   {0x90, 0xb4, 0xfa, 0xff},
		"secondary-border": {0xed, 0xa8, 0x79, 0xff},
		"service-border":   {0x70, 0xd6, 0xb4, 0xff},
		"note-bg":          {0x24, 0x28, 0x3c, 0xff},
		"note-border":      {0xb6, 0xa5, 0xf5, 0xff},
		"action-bg":        {0x14, 0x2c, 0x2b, 0xff},
		"error-border":     {0xfa, 0x94, 0x94, 0xff},
		"pre-bg":           {0x10, 0x18, 0x20, 0xff},
		"quote-border":     {0x75, 0x87, 0x9d, 0xff},
	}
	for label, want := range expectedColors {
		if !imageHasColor(imgWide, want) {
			t.Fatalf("bridged wide raster missing expected CSS color %s (%#v)", label, want)
		}
		if !imageHasColor(imgNarrow, want) {
			t.Fatalf("bridged narrow raster missing expected CSS color %s (%#v)", label, want)
		}
		if !imageHasColor(nativeWide.Image, want) {
			t.Fatalf("native wide raster missing expected CSS color %s (%#v)", label, want)
		}
		if !imageHasColor(nativeNarrow.Image, want) {
			t.Fatalf("native narrow raster missing expected CSS color %s (%#v)", label, want)
		}
	}

	// 6. Bounded viewport clipping & incremental layout check.
	layClip, err := eng.Layout(tdoc, 800, nttypeset.LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	clipView := nttypeset.View{Top: 140, Width: 800, Height: 220, Scale: 1}
	imgClip, err := layClip.Raster(ctx, clipView, nttypeset.RasterOptions{Palette: sm.Palette()})
	if err != nil {
		t.Fatal(err)
	}
	if layClip.Laid() >= len(tdoc.Blocks) {
		t.Fatalf("expected incremental viewport raster to lay out a prefix of blocks, laid %d of %d", layClip.Laid(), len(tdoc.Blocks))
	}
	if imgClip.Bounds().Dx() != 800 || imgClip.Bounds().Dy() != 220 {
		t.Fatalf("unexpected clipped viewport bounds %v", imgClip.Bounds())
	}
	for y := 0; y < 220; y += 17 {
		for x := 0; x < 800; x += 31 {
			if got, want := imgClip.RGBAAt(x, y), imgWide.RGBAAt(x, y+clipView.Top); got != want {
				t.Fatalf("clipped viewport mismatch at (%d,%d): got %v, want %v", x, y, got, want)
			}
		}
	}

	outDir := os.Getenv("NTHTML_ARTIFACTS_DIR")
	if outDir == "" {
		outDir = t.TempDir()
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, im := range map[string]image.Image{
		"structured-native-wide.png":    nativeWide.Image,
		"structured-native-narrow.png":  nativeNarrow.Image,
		"structured-bridged-wide.png":   imgWide,
		"structured-bridged-narrow.png": imgNarrow,
		"structured-bridged-clip.png":   imgClip,
	} {
		path := filepath.Join(outDir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, im)
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}

	// 7. Widget integration across Native, Bridged RenderResult, and TTY modes.
	bridgedRes, _, _, err := RenderResult(ctx, eng, doc, 800, Options{Document: document.Options{Width: 800}})
	if err != nil {
		t.Fatal(err)
	}
	m := nthtml.New(bridgedRes)
	m.SetSize(80, 24)
	if _, ok := m.NavigateSource("#entry-gamma-error"); !ok {
		t.Fatal("Model.NavigateSource(#entry-gamma-error) failed on bridged result")
	}
	if got, ok := m.ActiveSourceID(); !ok || (got != "entry-gamma-error" && doc.RootID(got) != "entry-gamma") {
		t.Fatalf("ActiveSourceID after NavigateSource = %q, %v", got, ok)
	}
	m.SetLayoutMode(nthtml.LayoutTTY)
	if _, ok := m.NavigateSource("entry-gamma-error"); !ok {
		t.Fatal("Model.NavigateSource(entry-gamma-error) failed in TTY mode")
	}
}

func TestCollapsedContainerRetainsAnchorOnRebuild(t *testing.T) {
	const tmpl = `<!DOCTYPE html><html><head><style>
		.card { padding: 12px; background: #19212d; border-left: 4px solid #eda879; }
		.is-collapsed .body { display: none; }
	</style></head><body>
		<section class="card" id="msg-1"><div class="body"><p>First message</p></div></section>
		<section class="card __COLLAPSED__" id="msg-2"><div class="body" id="msg-2-body"><p>Collapsible body text</p></div></section>
		<section class="card" id="msg-3"><div class="body"><p>Third message</p></div></section>
	</body></html>`

	ctx := context.Background()
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}

	for _, collapsed := range []bool{false, true} {
		cls := ""
		if collapsed {
			cls = "is-collapsed"
		}
		htmlSrc := strings.ReplaceAll(tmpl, "__COLLAPSED__", cls)
		doc, err := document.Parse(ctx, strings.NewReader(htmlSrc), document.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		res, err := doc.Render(ctx, document.Options{Width: 600})
		if err != nil {
			t.Fatal(err)
		}
		ttyRes, err := doc.TTYLayout(ctx, 72)
		if err != nil {
			t.Fatal(err)
		}
		tdoc, sm, err := BuildWithSources(ctx, doc, Options{Document: document.Options{Width: 600}})
		if err != nil {
			t.Fatal(err)
		}
		lay, err := eng.Layout(tdoc, 600, nttypeset.LayoutOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := lay.Complete(ctx); err != nil {
			t.Fatal(err)
		}

		// msg-2 must remain a valid anchor even when its entire .body is display:none.
		if r, ok := res.AnchorBounds("msg-2"); !ok || r.Empty() {
			t.Fatalf("collapsed=%v: native lost msg-2 anchor: %v, %v", collapsed, r, ok)
		}
		if r, ok := ttyRes.AnchorBounds("msg-2"); !ok || r.Empty() {
			t.Fatalf("collapsed=%v: TTY lost msg-2 anchor: %v, %v", collapsed, r, ok)
		}
		if a, ok, err := sm.AnchorBounds(ctx, lay, "msg-2", true); err != nil || !ok || a.Rect.Empty() {
			t.Fatalf("collapsed=%v: bridged lost msg-2 anchor: %+v, ok=%v, err=%v", collapsed, a, ok, err)
		}

		// msg-2-body must only exist when not collapsed.
		_, bodyInNative := res.AnchorBounds("msg-2-body")
		_, bodyInTTY := ttyRes.AnchorBounds("msg-2-body")
		_, bodyInBridged, _ := sm.AnchorBounds(ctx, lay, "msg-2-body", true)
		if collapsed && (bodyInNative || bodyInTTY || bodyInBridged || strings.Contains(layoutText(t, ctx, lay), "Collapsible body text")) {
			t.Fatalf("collapsed=true: display:none child leaked (native=%v tty=%v bridged=%v)", bodyInNative, bodyInTTY, bodyInBridged)
		}
		if !collapsed && (!bodyInNative || !bodyInTTY || !bodyInBridged) {
			t.Fatalf("collapsed=false: expected msg-2-body anchor (native=%v tty=%v bridged=%v)", bodyInNative, bodyInTTY, bodyInBridged)
		}
	}
}

func TestExplicitAssetAllowlistNoImplicitIO(t *testing.T) {
	const src = `<html><body>
		<p id="p1">Safe paragraph</p>
		<img id="remote-img" src="https://example.com/secret.png" alt="Remote diagram">
		<img id="file-img" src="/etc/passwd" alt="Local file">
	</body></html>`

	ctx := context.Background()
	doc, err := document.Parse(ctx, strings.NewReader(src), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	tdoc, sm, err := BuildWithSources(ctx, doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range tdoc.Blocks {
		if b.Kind == nttypeset.KindImage && b.Image != nil && b.Image.Still != nil {
			t.Fatalf("expected nil Still image when Options.Document.Images is empty, got %+v", b.Image)
		}
	}
	if _, _, ok := sm.BlockRange("remote-img"); !ok {
		t.Fatal("expected remote-img placeholder block to retain source ID")
	}
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func hasDocLink(links []document.Link, href string) bool {
	for _, l := range links {
		if l.Href == href && len(l.Regions) > 0 {
			return true
		}
	}
	return false
}

func hasTypesetLink(links []nttypeset.Link, href string) bool {
	for _, l := range links {
		if l.URL == href && !l.Rect.Empty() {
			return true
		}
	}
	return false
}

func imageHasColor(img *image.RGBA, target color.RGBA) bool {
	if img == nil {
		return false
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y) == target {
				return true
			}
		}
	}
	return false
}
