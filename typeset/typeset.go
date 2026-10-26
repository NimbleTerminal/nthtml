// Package typeset bridges parsed nthtml documents and rendered boxes into
// nimbleterminal.dev/nttypeset documents for UAX #14 line breaking, shaping,
// per-glyph font fallback, and viewport rasterization.
package typeset

import (
	"context"
	"image"
	"image/color"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nttypeset"
)

// Options configures HTML-to-nttypeset document conversion.
type Options struct {
	// Document supplies allowed images, diagram renderer, and reference width.
	Document document.Options
	// Syntax is an optional Chroma style for <pre><code> syntax highlighting.
	Syntax *chroma.Style
}

// SourceMap maps HTML element IDs across blocks and nested sections in an
// nttypeset.Document, preserving parent/section hierarchy and block ranges.
type SourceMap struct {
	ids        []string
	parents    map[string]string
	ranges     map[string][2]int
	background color.RGBA
	foreground color.RGBA
}

// IDs returns all visible source IDs in document order.
func (m *SourceMap) IDs() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.ids...)
}

// ParentID returns the nearest enclosing section's SourceID for id (or #fragment),
// or "" if id is top-level or unknown.
func (m *SourceMap) ParentID(id string) string {
	if m == nil {
		return ""
	}
	name, ok := normalizeID(id)
	if !ok {
		return ""
	}
	return m.parents[name]
}

// RootID returns the outermost enclosing section's SourceID for id (or id itself
// if top-level), or "" if id is unknown.
func (m *SourceMap) RootID(id string) string {
	if m == nil {
		return ""
	}
	name, ok := normalizeID(id)
	if !ok || name == "" {
		return ""
	}
	if _, exists := m.ranges[name]; !exists {
		return ""
	}
	cur := name
	for range len(m.ids) + 1 {
		p := m.parents[cur]
		if p == "" {
			return cur
		}
		cur = p
	}
	return cur
}

// BlockRange returns the half-open block index range [start, end) in
// nttypeset.Document.Blocks for id (or #fragment).
func (m *SourceMap) BlockRange(id string) (start, end int, ok bool) {
	if m == nil {
		return 0, 0, false
	}
	name, valid := normalizeID(id)
	if !valid || name == "" {
		return 0, 0, false
	}
	r, exists := m.ranges[name]
	if !exists {
		return 0, 0, false
	}
	return r[0], r[1], true
}

// AnchorBounds resolves idOrHref (either a raw element ID or a local #fragment)
// to its laid-out geometry in lay. With ensure=true, lay advances through the
// target block or section end.
func (m *SourceMap) AnchorBounds(ctx context.Context, lay *nttypeset.Layout, idOrHref string, ensure bool) (nttypeset.Anchor, bool, error) {
	if lay == nil {
		return nttypeset.Anchor{}, false, nil
	}
	if strings.HasPrefix(idOrHref, "#") {
		name, ok := document.Fragment(idOrHref)
		if !ok {
			return nttypeset.Anchor{}, false, nil
		}
		if name == "" {
			return nttypeset.Anchor{Rect: image.Rect(0, 0, lay.Width(), 1), Complete: true}, true, nil
		}
		return lay.AnchorBounds(ctx, name, ensure)
	}
	return lay.AnchorBounds(ctx, idOrHref, ensure)
}

// SourceAt resolves a selection or hit Place in lay back to its innermost block
// or section SourceID.
func (m *SourceMap) SourceAt(ctx context.Context, lay *nttypeset.Layout, place nttypeset.Place) (string, bool, error) {
	if lay == nil {
		return "", false, nil
	}
	return lay.SourceAt(ctx, place)
}

// Palette returns an nttypeset.Palette matching the document's resolved canvas
// background and body foreground colors.
func (m *SourceMap) Palette() nttypeset.Palette {
	if m == nil {
		return nttypeset.LightPalette()
	}
	return paletteFromColors(m.background, m.foreground)
}

func normalizeID(idOrHref string) (string, bool) {
	if strings.HasPrefix(idOrHref, "#") {
		return document.Fragment(idOrHref)
	}
	return idOrHref, idOrHref != ""
}

func paletteFromColors(bg, fg color.RGBA) nttypeset.Palette {
	pal := nttypeset.LightPalette()
	if bg.A != 0 {
		lum := (299*int(bg.R) + 587*int(bg.G) + 114*int(bg.B)) / 1000
		if lum < 128 {
			pal = nttypeset.DarkPalette()
		}
		pal.Background = bg
	}
	if fg.A != 0 && fg != (color.RGBA{0, 0, 0, 255}) {
		pal.Foreground = fg
	}
	return pal
}

// PaletteFor returns an nttypeset.Palette configured for doc's resolved canvas
// background and body text color.
func PaletteFor(ctx context.Context, doc *document.Document, opts Options) nttypeset.Palette {
	_, sm, err := BuildWithSources(ctx, doc, opts)
	if err != nil || sm == nil {
		return nttypeset.LightPalette()
	}
	return sm.Palette()
}

// Build converts a parsed *document.Document into an *nttypeset.Document,
// preserving headings, paragraphs, blockquotes, lists, tables, horizontal
// rules, code blocks (with optional Chroma syntax highlighting), nested
// container sections, source anchors, links, inline emphasis, and supplied
// images/diagrams.
func Build(ctx context.Context, doc *document.Document, opts Options) (*nttypeset.Document, error) {
	tdoc, _, err := BuildWithSources(ctx, doc, opts)
	return tdoc, err
}

// BuildWithSources converts doc into an *nttypeset.Document and returns its
// *SourceMap for anchor navigation, parent-section lookup, and hit resolution.
func BuildWithSources(ctx context.Context, doc *document.Document, opts Options) (*nttypeset.Document, *SourceMap, error) {
	extracted, err := doc.ExtractTypeset(ctx, opts.Document)
	if err != nil {
		return nil, nil, err
	}
	sm := &SourceMap{
		parents:    make(map[string]string),
		ranges:     make(map[string][2]int),
		background: extracted.Background,
		foreground: extracted.Foreground,
	}
	seenID := make(map[string]bool)
	recordID := func(id, parent string, start, end int) string {
		if id == "" || seenID[id] {
			return ""
		}
		seenID[id] = true
		sm.ids = append(sm.ids, id)
		if parent != "" && parent != id {
			sm.parents[id] = parent
		}
		sm.ranges[id] = [2]int{start, end}
		return id
	}

	sections := make([]nttypeset.Section, 0, len(extracted.Sections))
	for _, s := range extracted.Sections {
		if s.Start < 0 || s.End <= s.Start || s.End > len(extracted.Blocks) {
			continue
		}
		srcID := recordID(s.SourceID, s.ParentID, s.Start, s.End)
		sections = append(sections, nttypeset.Section{
			Start:       s.Start,
			End:         s.End,
			SpaceBefore: s.SpaceBefore,
			SpaceAfter:  s.SpaceAfter,
			SourceID:    srcID,
			Padding:     convertInsets(s.Padding),
			Background:  s.Background,
			Top:         convertBorder(s.Top),
			Right:       convertBorder(s.Right),
			Bottom:      convertBorder(s.Bottom),
			Left:        convertBorder(s.Left),
		})
	}

	blocks := make([]nttypeset.Block, 0, len(extracted.Blocks))
	for i, b := range extracted.Blocks {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		srcID := recordID(b.SourceID, b.SectionID, i, i+1)
		nb := nttypeset.Block{
			SourceID: srcID,
			Format:   convertFormat(b.Format),
			Size:     b.Size,
			Indent:   b.Indent,
			Href:     b.Href,
		}
		if len(b.Quotes) > 0 {
			nb.Quotes = make([]nttypeset.Quote, len(b.Quotes))
			for qIdx, q := range b.Quotes {
				nb.Quotes[qIdx] = nttypeset.Quote{Indent: q.Indent, Last: q.Last}
			}
		}
		switch b.Kind {
		case document.BlockHeading:
			nb.Kind = nttypeset.KindHeading
			nb.Spans = convertSpans(b.Spans)
		case document.BlockQuote:
			nb.Kind = nttypeset.KindQuote
			nb.Spans = convertSpans(b.Spans)
		case document.BlockCode:
			nb.Kind = nttypeset.KindCode
			if nb.Size == 0 {
				nb.Size = 16
			}
			if opts.Syntax != nil && b.Language != "" {
				nb.Spans = nttypeset.Highlight(ctx, b.Code, b.Language, opts.Syntax)
				if len(b.Spans) > 0 && b.Spans[0].ExplicitColor {
					baseInk := b.Spans[0].Color
					for sIdx := range nb.Spans {
						if nb.Spans[sIdx].Token == 0 && nb.Spans[sIdx].Ink == (color.RGBA{}) {
							nb.Spans[sIdx].Ink = baseInk
						}
					}
				}
			} else {
				nb.Spans = convertSpans(b.Spans)
			}
		case document.BlockRule:
			nb.Kind = nttypeset.KindRule
		case document.BlockImage:
			nb.Kind = nttypeset.KindImage
			nb.Image = &nttypeset.Image{
				Key:   b.ImageKey,
				Still: b.Image,
				Bounds: nttypeset.Bounds{
					Width:  b.ImageWidth,
					Height: b.ImageHeight,
					Align:  b.ImageAlign,
				},
			}
			nb.Spans = convertSpans(b.Spans)
		case document.BlockTable:
			nb.Kind = nttypeset.KindTable
			nb.Rows = make([][][]nttypeset.Span, len(b.Rows))
			for r, row := range b.Rows {
				nb.Rows[r] = make([][]nttypeset.Span, len(row))
				for c, cell := range row {
					nb.Rows[r][c] = convertSpans(cell)
				}
			}
		default:
			nb.Kind = nttypeset.KindParagraph
			nb.Spans = convertSpans(b.Spans)
		}
		blocks = append(blocks, nb)
	}
	return &nttypeset.Document{
		Sections: sections,
		Blocks:   blocks,
	}, sm, nil
}

func convertInsets(in document.Insets) nttypeset.Insets {
	return nttypeset.Insets{
		Top:    in.Top,
		Right:  in.Right,
		Bottom: in.Bottom,
		Left:   in.Left,
	}
}

func convertBorder(in document.SectionBorder) nttypeset.Border {
	return nttypeset.Border{
		Width: in.Width,
		Color: in.Color,
	}
}

func convertFormat(in *document.BlockFormat) *nttypeset.BlockFormat {
	if in == nil {
		return nil
	}
	return &nttypeset.BlockFormat{
		LineHeight:  in.LineHeight,
		SpaceBefore: in.SpaceBefore,
		SpaceAfter:  in.SpaceAfter,
		Padding:     convertInsets(in.Padding),
		Background:  in.Background,
	}
}

// FromResult converts an already-rendered *document.Result (its Boxes and
// Links) into an *nttypeset.Document.
func FromResult(r *document.Result) *nttypeset.Document {
	if r == nil {
		return &nttypeset.Document{}
	}
	return FromBoxes(r.Boxes, r.Links)
}

// FromBoxes groups painted text boxes into reading-order paragraphs and spans
// for consumption by an nttypeset.Engine.
func FromBoxes(boxes []document.Box, links []document.Link) *nttypeset.Document {
	hrefs := make(map[int]string, len(links))
	for _, l := range links {
		hrefs[l.ID] = l.Href
	}
	type headingRegion struct {
		bounds image.Rectangle
		level  int
	}
	var headings []headingRegion
	for _, b := range boxes {
		lvl := boxHeadingLevel(b.Tag)
		if lvl > 0 && !b.Bounds.Empty() {
			headings = append(headings, headingRegion{bounds: b.Bounds, level: lvl})
		}
	}

	var blocks []nttypeset.Block
	var curSpans []nttypeset.Span
	var curHeading int
	var curSize int
	var curSourceID string
	seenIDs := make(map[string]bool)
	lastBottom := -1
	lastRight := -1
	lastLineY := -1

	claimID := func(id string) string {
		if id == "" || seenIDs[id] {
			return ""
		}
		seenIDs[id] = true
		return id
	}

	flush := func() {
		if len(curSpans) == 0 {
			return
		}
		kind := nttypeset.KindParagraph
		size := curSize
		if curHeading > 0 {
			kind = nttypeset.KindHeading
			if size == 0 {
				sizes := []int{32, 24, 20, 16, 14, 12}
				size = sizes[min(curHeading, len(sizes))-1]
			}
		}
		blocks = append(blocks, nttypeset.Block{
			SourceID: claimID(curSourceID),
			Kind:     kind,
			Size:     size,
			Spans:    curSpans,
		})
		curSpans = nil
		curHeading = 0
		curSize = 0
		curSourceID = ""
	}

	for _, b := range boxes {
		if b.Tag == "hr" {
			flush()
			blocks = append(blocks, nttypeset.Block{
				SourceID: claimID(b.SourceID),
				Kind:     nttypeset.KindRule,
			})
			lastBottom = b.Bounds.Max.Y
			continue
		}
		if b.Text == "" {
			continue
		}
		lvl := boxHeadingLevel(b.Tag)
		if lvl == 0 {
			for _, h := range headings {
				if b.Bounds.Overlaps(h.bounds) {
					lvl = h.level
					break
				}
			}
		}
		if lvl == 0 && b.Bold && b.FontSize >= 20 {
			switch {
			case b.FontSize >= 30:
				lvl = 1
			case b.FontSize >= 24:
				lvl = 2
			default:
				lvl = 3
			}
		}
		if len(curSpans) > 0 && (lvl != curHeading || (lastBottom >= 0 && b.Bounds.Min.Y-lastBottom > max(8, b.FontSize/2))) {
			flush()
		}
		curHeading = lvl
		if curSize == 0 && b.FontSize > 0 {
			curSize = b.FontSize
		}
		if curSourceID == "" && b.SourceID != "" {
			curSourceID = b.SourceID
		}
		var st nttypeset.Style
		if b.Bold || lvl > 0 {
			st |= nttypeset.Bold
		}
		if b.Italic {
			st |= nttypeset.Italic
		}
		if b.Monospace {
			st |= nttypeset.Mono
		}
		if b.Strike {
			st |= nttypeset.Strike
		}
		ink := b.Color
		role := nttypeset.RoleText
		if ink == (color.RGBA{0, 0, 0, 255}) {
			ink = color.RGBA{}
			if lvl > 0 || b.LinkID != 0 {
				role = nttypeset.RoleAccent
			}
		}
		text := b.Text
		if len(curSpans) > 0 {
			needSpace := b.Bounds.Min.Y != lastLineY || b.Bounds.Min.X > lastRight+2
			if needSpace && !strings.HasSuffix(curSpans[len(curSpans)-1].Text, " ") && !strings.HasPrefix(text, " ") {
				text = " " + text
			}
		}
		curSpans = append(curSpans, nttypeset.Span{
			Text:  text,
			Style: st,
			Role:  role,
			Ink:   ink,
			Href:  hrefs[b.LinkID],
		})
		lastBottom = max(lastBottom, b.Bounds.Max.Y)
		lastRight = b.Bounds.Max.X
		lastLineY = b.Bounds.Min.Y
	}
	flush()
	return &nttypeset.Document{Blocks: blocks}
}

// Raster builds an *nttypeset.Document from doc, lays it out at width with eng,
// and rasters view with rasterOpts. When rasterOpts.Palette is zero, PaletteFor
// selects a palette matching doc's resolved canvas/text colors.
func Raster(ctx context.Context, eng *nttypeset.Engine, doc *document.Document, width int, view nttypeset.View, rasterOpts nttypeset.RasterOptions, opts Options) (*image.RGBA, *nttypeset.Layout, error) {
	tdoc, sm, err := BuildWithSources(ctx, doc, opts)
	if err != nil {
		return nil, nil, err
	}
	if rasterOpts.Palette.Background == (color.RGBA{}) && rasterOpts.Palette.Foreground == (color.RGBA{}) {
		rasterOpts.Palette = sm.Palette()
	}
	lay, err := eng.Layout(tdoc, width, nttypeset.LayoutOptions{})
	if err != nil {
		return nil, nil, err
	}
	img, err := lay.Raster(ctx, view, rasterOpts)
	if err != nil {
		return nil, nil, err
	}
	return img, lay, nil
}

// RenderResult lays out and rasters doc through nttypeset and returns a
// *document.Result populated with the raster image, laid-out anchor coordinates,
// hyperlink hit regions, and text/source boxes so an nthtml.Model viewer can
// display and navigate the bridged output.
func RenderResult(ctx context.Context, eng *nttypeset.Engine, doc *document.Document, width int, opts Options) (*document.Result, *nttypeset.Layout, *SourceMap, error) {
	if width <= 0 {
		width = opts.Document.Width
	}
	if width <= 0 {
		width = 800
	}
	opts.Document.Width = width
	tdoc, sm, err := BuildWithSources(ctx, doc, opts)
	if err != nil {
		return nil, nil, nil, err
	}
	lay, err := eng.Layout(tdoc, width, nttypeset.LayoutOptions{})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := lay.Complete(ctx); err != nil {
		return nil, nil, nil, err
	}
	height := max(64, min(16384, lay.Extent()))
	view := nttypeset.View{Width: width, Height: height, Scale: 1}
	img, err := lay.Raster(ctx, view, nttypeset.RasterOptions{Palette: sm.Palette()})
	if err != nil {
		return nil, nil, nil, err
	}
	anchors := make(map[string]image.Point, len(sm.ids))
	var boxes []document.Box
	for _, id := range sm.ids {
		a, ok, err := lay.AnchorBounds(ctx, id, false)
		if err == nil && ok && !a.Rect.Empty() {
			anchors[id] = a.Rect.Min
			boxes = append(boxes, document.Box{
				Bounds:   a.Rect,
				Tag:      "section",
				Anchor:   id,
				SourceID: id,
			})
		}
	}
	blockBounds := lay.Bounds()
	for i, b := range tdoc.Blocks {
		var rect image.Rectangle
		if i < len(blockBounds) {
			rect = blockBounds[i]
		}
		if rect.Empty() {
			continue
		}
		srcID, _, _ := lay.SourceAt(ctx, nttypeset.Place{Block: i, Placed: true})
		text := flattenBlockText(b)
		if text != "" {
			boxes = append(boxes, document.Box{
				Bounds:     rect,
				Tag:        "text",
				Text:       text,
				TextPrefix: "\n",
				FontSize:   max(1, b.Size),
				SourceID:   srcID,
			})
		}
	}
	tlinks := lay.Links(view)
	var links []document.Link
	linkByHref := make(map[string]int)
	for _, tl := range tlinks {
		if tl.URL == "" || tl.Rect.Empty() {
			continue
		}
		idx, ok := linkByHref[tl.URL]
		if !ok {
			idx = len(links)
			linkByHref[tl.URL] = idx
			links = append(links, document.Link{
				ID:   idx + 1,
				Href: tl.URL,
				Text: tl.URL,
			})
		}
		links[idx].Regions = append(links[idx].Regions, tl.Rect)
	}
	res := (&document.Result{
		Anchors: anchors,
		Image:   img,
		Boxes:   boxes,
		Links:   links,
	}).WithDocument(doc, opts.Document)
	return res, lay, sm, nil
}

func flattenBlockText(b nttypeset.Block) string {
	var out strings.Builder
	for _, sp := range b.Spans {
		out.WriteString(sp.Text)
	}
	for _, row := range b.Rows {
		for c, cell := range row {
			if c > 0 {
				out.WriteString(" ")
			}
			for _, sp := range cell {
				out.WriteString(sp.Text)
			}
		}
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String())
}

func boxHeadingLevel(tag string) int {
	switch tag {
	case "h1":
		return 1
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	case "h5":
		return 5
	case "h6":
		return 6
	}
	return 0
}

func convertSpans(in []document.Span) []nttypeset.Span {
	if len(in) == 0 {
		return nil
	}
	out := make([]nttypeset.Span, 0, len(in))
	for _, s := range in {
		var st nttypeset.Style
		if s.Bold {
			st |= nttypeset.Bold
		}
		if s.Italic {
			st |= nttypeset.Italic
		}
		if s.Monospace {
			st |= nttypeset.Mono
		}
		if s.Strike {
			st |= nttypeset.Strike
		}
		role := nttypeset.RoleText
		ink := s.Color
		if !s.ExplicitColor {
			if s.Accent {
				role = nttypeset.RoleAccent
			}
			if ink == (color.RGBA{0, 0, 0, 255}) {
				// Let nttypeset resolve default body foreground from its active Palette.
				ink = color.RGBA{}
			}
		}
		out = append(out, nttypeset.Span{
			Text:  s.Text,
			Style: st,
			Role:  role,
			Ink:   ink,
			Href:  s.Href,
		})
	}
	return out
}
