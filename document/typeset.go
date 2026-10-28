package document

import (
	"context"
	"image"
	"image/color"
	"strings"
)

// Document returns the immutable parsed Document that produced r, or nil.
func (r *Result) Document() *Document {
	if r == nil {
		return nil
	}
	return r.document
}

// WithDocument associates d and o with a host-constructed Result so TTY reflow
// and Result.Document() remain available.
func (r *Result) WithDocument(d *Document, o Options) *Result {
	if r == nil {
		return nil
	}
	r.document = d
	r.opts = o
	if d != nil {
		r.limits = d.limits
	}
	if r.Image != nil {
		r.size = r.Image.Bounds().Max
	}
	return r
}

// Options returns a copy of the render Options used to produce r.
func (r *Result) Options() Options {
	if r == nil {
		return Options{}
	}
	o := r.opts
	if len(r.opts.Images) > 0 {
		o.Images = make(map[string]image.Image, len(r.opts.Images))
		for k, v := range r.opts.Images {
			o.Images[k] = v
		}
	}
	if r.opts.SVGImages != nil {
		o.SVGImages = make(map[string]image.Image, len(r.opts.SVGImages))
		for k, v := range r.opts.SVGImages {
			o.SVGImages[k] = v
		}
	}
	return o
}

// BlockKind identifies a logical document block for host typesetters such as nttypeset.
type BlockKind uint8

const (
	BlockParagraph BlockKind = iota
	BlockHeading
	BlockQuote
	BlockCode
	BlockRule
	BlockImage
	BlockTable
)

// Insets are nonnegative layout pixels on each edge (top, right, bottom, left).
type Insets struct {
	Top, Right, Bottom, Left int
}

// SectionBorder describes one solid rectangular border edge.
type SectionBorder struct {
	Width int
	Color color.RGBA
}

// BlockFormat supplies resolved block spacing, padding, and background in layout pixels.
type BlockFormat struct {
	LineHeight, SpaceBefore, SpaceAfter int
	Padding                             Insets
	Background                          color.RGBA
}

// Section decorates a nonempty half-open range of StyledBlocks [Start, End)
// and carries its HTML element ID (if any) and enclosing parent section ID.
type Section struct {
	Start, End               int
	SpaceBefore, SpaceAfter  int
	SourceID, ParentID       string
	Padding                  Insets
	Background               color.RGBA
	Top, Right, Bottom, Left SectionBorder
}

// Span is a styled inline text run extracted from the document tree.
type Span struct {
	Text                                       string
	Size                                       int
	Bold, Italic, Monospace, Strike, Underline bool
	SmallCaps, Accent, ExplicitColor           bool
	Color                                      color.RGBA
	Href                                       string
}

// QuoteBar describes a vertical blockquote bar beside a block.
type QuoteBar struct {
	Indent int
	Last   bool
}

// StyledBlock is a structured block in reading order, suitable for conversion
// to nttypeset.Block or other host layout engines.
type StyledBlock struct {
	SourceID, SectionID     string
	Format                  *BlockFormat
	Kind                    BlockKind
	Level, Size, Indent     int
	Language, Code          string
	Spans                   []Span
	Rows                    [][][]Span
	Quotes                  []QuoteBar
	ImageKey, ImageAlign    string
	Image                   image.Image
	ImageWidth, ImageHeight int
	Href                    string
}

// TypesetDocument holds reading-order StyledBlocks, nested container Sections,
// and resolved canvas/text colors extracted from a Document.
type TypesetDocument struct {
	Blocks     []StyledBlock
	Sections   []Section
	Background color.RGBA
	Foreground color.RGBA
}

const maxStyledBlocks = 32768

// Blocks extracts styled reading-order blocks from d using the CSS cascade,
// pseudo-elements, list markers, table structure, and optional image/diagram
// resources in o.
func (d *Document) Blocks(ctx context.Context, o Options) ([]StyledBlock, error) {
	td, err := d.ExtractTypeset(ctx, o)
	if err != nil {
		return nil, err
	}
	return td.Blocks, nil
}

// ExtractTypeset extracts styled reading-order blocks, nested container
// sections, source anchors, and resolved canvas colors from d.
func (d *Document) ExtractTypeset(ctx context.Context, o Options) (*TypesetDocument, error) {
	if d == nil || d.root == nil {
		return nil, limitError("nil document")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.Width == 0 {
		o.Width = 800
	}
	var err error
	d, err = d.forMedia(ctx, o.Width, o.ColorScheme)
	if err != nil {
		return nil, err
	}
	links, err := d.Links(ctx)
	if err != nil {
		return nil, err
	}
	hrefs := make(map[int]string, len(links))
	for _, l := range links {
		hrefs[l.ID] = l.Href
	}
	fg := color.RGBA{0, 0, 0, 255}
	var linkColor color.RGBA
	explicitFg := false
	if !d.hasAuthorColors {
		if o.Colors.Text.A > 0 {
			fg = o.Colors.Text
			explicitFg = true
		}
		linkColor = o.Colors.Link
	}
	base := style{rootSize: 16, size: 16, line: 22, spacing: [2]int{2, 2}, fg: fg, defaultFg: fg, defaultLink: linkColor, explicitFg: explicitFg, vw: o.Width, vh: viewportHeight(o)}
	probe := &layout{ctx: ctx, d: d, opts: o, lim: d.limits}
	canvasBg, _ := probe.resolveCanvasBackground(base)
	bodyFg := resolveDocumentForeground(d, base)

	w := &blockExtractor{
		ctx:     ctx,
		d:       d,
		opts:    o,
		hrefs:   hrefs,
		seenIDs: map[string]bool{},
	}
	if err := w.walk(d.root, base, 0, max(64, o.Width)); err != nil {
		return nil, err
	}
	if len(w.pendingIDs) > 0 {
		if len(w.blocks) == 0 {
			if err := w.addBlock(StyledBlock{Kind: BlockParagraph, Size: 16, Format: &BlockFormat{LineHeight: 1}}); err != nil {
				return nil, err
			}
		} else {
			w.attachPendingIDs(len(w.blocks) - 1)
		}
	}
	sections := make([]Section, 0, len(w.sections))
	for _, sec := range w.sections {
		if sec.Start >= 0 && sec.End > sec.Start && sec.End <= len(w.blocks) {
			sections = append(sections, sec)
		}
	}
	return &TypesetDocument{
		Blocks:     w.blocks,
		Sections:   sections,
		Background: canvasBg,
		Foreground: bodyFg,
	}, nil
}

func resolveDocumentForeground(d *Document, base style) color.RGBA {
	var find func(*node, style) (color.RGBA, bool)
	find = func(n *node, p style) (color.RGBA, bool) {
		if n == nil || n.tag == "" {
			return color.RGBA{}, false
		}
		s := d.styled(n, p)
		if s.display == "none" {
			return color.RGBA{}, false
		}
		if n.tag == "body" && s.explicitFg {
			return s.fg, true
		}
		for _, c := range n.children {
			if fg, ok := find(c, s); ok {
				return fg, true
			}
		}
		if n.tag == "html" && s.explicitFg {
			return s.fg, true
		}
		return color.RGBA{}, false
	}
	if fg, ok := find(d.root, base); ok {
		return fg
	}
	return base.fg
}

type pendingID struct {
	id, parentID string
}

type blockExtractor struct {
	ctx          context.Context
	d            *Document
	opts         Options
	hrefs        map[int]string
	seenIDs      map[string]bool
	parentID     string
	sectionDepth int
	pendingIDs   []pendingID
	blocks       []StyledBlock
	sections     []Section
	diagramCount int
	diagramPx    int
}

func (w *blockExtractor) claimID(id string) string {
	if id == "" || w.seenIDs[id] {
		return ""
	}
	w.seenIDs[id] = true
	return id
}

func (w *blockExtractor) attachPendingIDs(blockIdx int) {
	if len(w.pendingIDs) == 0 {
		return
	}
	pending := w.pendingIDs
	w.pendingIDs = nil
	for _, p := range pending {
		if len(w.sections) < maxStyledBlocks {
			w.sections = append(w.sections, Section{
				Start:    blockIdx,
				End:      blockIdx + 1,
				SourceID: p.id,
				ParentID: p.parentID,
			})
		}
	}
}

func (w *blockExtractor) addBlock(b StyledBlock) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if len(w.blocks) >= maxStyledBlocks {
		return limitError("typeset blocks")
	}
	if b.SectionID == "" {
		b.SectionID = w.parentID
	}
	w.blocks = append(w.blocks, b)
	w.attachPendingIDs(len(w.blocks) - 1)
	return nil
}

func sectionBorders(s style) (SectionBorder, SectionBorder, SectionBorder, SectionBorder) {
	b := func(side int) SectionBorder {
		if s.bw[side] <= 0 {
			return SectionBorder{}
		}
		return SectionBorder{Width: s.bw[side], Color: s.borderColor(side)}
	}
	return b(0), b(1), b(2), b(3)
}

func (w *blockExtractor) wrapLeaf(idx int, id, parentID string, s style, margin [4]int, pad Insets, inlineIDs []string) {
	if s.hasBorder() && len(w.sections) < maxStyledBlocks {
		top, right, bottom, left := sectionBorders(s)
		w.sections = append(w.sections, Section{
			Start:       idx,
			End:         idx + 1,
			SpaceBefore: max(0, margin[0]),
			SpaceAfter:  max(0, margin[2]),
			SourceID:    id,
			ParentID:    parentID,
			Padding:     pad,
			Background:  s.bg,
			Top:         top,
			Right:       right,
			Bottom:      bottom,
			Left:        left,
		})
		if w.blocks[idx].Format != nil {
			w.blocks[idx].Format = &BlockFormat{LineHeight: w.blocks[idx].Format.LineHeight}
		}
		if id != "" {
			w.blocks[idx].SectionID = id
		}
	} else if id != "" {
		w.blocks[idx].SourceID = id
	}
	innerParent := parentID
	if id != "" {
		innerParent = id
	}
	for _, inl := range inlineIDs {
		uid := w.claimID(inl)
		if uid == "" {
			continue
		}
		if w.blocks[idx].SourceID == "" && !s.hasBorder() {
			w.blocks[idx].SourceID = uid
		} else if len(w.sections) < maxStyledBlocks {
			w.sections = append(w.sections, Section{
				Start:    idx,
				End:      idx + 1,
				SourceID: uid,
				ParentID: innerParent,
			})
		}
	}
}

func headingLevel(tag string) int {
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

func codeLanguage(n *node) string {
	for _, attr := range []string{n.attrs["class"]} {
		for _, cl := range strings.Fields(attr) {
			if strings.HasPrefix(cl, "language-") {
				return strings.TrimPrefix(cl, "language-")
			}
			if strings.HasPrefix(cl, "lang-") {
				return strings.TrimPrefix(cl, "lang-")
			}
		}
	}
	for _, c := range n.children {
		if c.tag == "code" {
			if lang := codeLanguage(c); lang != "" {
				return lang
			}
		}
	}
	return ""
}

func collectRawText(n *node) string {
	var b strings.Builder
	var visit func(*node)
	visit = func(cur *node) {
		if cur.tag == "br" {
			b.WriteByte('\n')
			return
		}
		b.WriteString(cur.text)
		for _, c := range cur.children {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}

func uniformSpanSize(spans []Span) int {
	sz := 0
	for _, sp := range spans {
		if sp.Size <= 0 {
			continue
		}
		if sz == 0 {
			sz = sp.Size
		} else if sz != sp.Size {
			return 0
		}
	}
	return sz
}

func (w *blockExtractor) inlineSpansWithIDs(n *node, parent style) ([]Span, []string) {
	var out []Span
	var ids []string
	var visit func(*node, style)
	visit = func(cur *node, p style) {
		if cur.tag == "" {
			txt := applyTextTransform(cur.text, p.textTransform)
			if !p.pre && p.whiteSpace != "pre-wrap" && p.whiteSpace != "pre-line" {
				txt = collapseSpaces(cleanText(txt))
			}
			if txt == "" {
				return
			}
			if p.smallCaps {
				txt = strings.ToUpper(txt)
			}
			out = append(out, Span{
				Text:          txt,
				Size:          max(1, p.size),
				Bold:          p.bold,
				Italic:        p.italic,
				Monospace:     p.mono,
				Strike:        p.strike,
				Underline:     p.underline,
				SmallCaps:     p.smallCaps,
				Accent:        p.linkID != 0 && !p.explicitFg,
				ExplicitColor: p.explicitFg,
				Color:         p.fg,
				Href:          w.hrefs[p.linkID],
			})
			return
		}
		if cur.tag == "br" {
			out = append(out, Span{Text: "\n", Size: max(1, p.size), Bold: p.bold, Italic: p.italic, Monospace: p.mono, Color: p.fg, ExplicitColor: p.explicitFg})
			return
		}
		s := w.d.styled(cur, p)
		if s.display == "none" {
			return
		}
		if id := anchorName(cur); id != "" {
			ids = append(ids, id)
		}
		if cur.tag == "img" {
			if alt := cleanText(cur.attrs["alt"]); alt != "" {
				out = append(out, Span{
					Text:          "[image: " + alt + "]",
					Size:          max(1, s.size),
					Italic:        true,
					Color:         s.fg,
					ExplicitColor: s.explicitFg,
					Href:          w.hrefs[s.linkID],
					Accent:        s.linkID != 0 && !s.explicitFg,
				})
			}
			return
		}
		startLen := len(out)
		for _, ch := range cur.children {
			visit(ch, s)
		}
		if !s.pre && s.whiteSpace != "pre-wrap" && len(out) > startLen && ((s.underline && !p.underline) || (s.strike && !p.strike) || (s.linkID != p.linkID)) {
			last := &out[len(out)-1]
			if strings.HasSuffix(last.Text, " ") {
				last.Text = strings.TrimSuffix(last.Text, " ")
				out = append(out, Span{
					Text:          " ",
					Size:          max(1, p.size),
					Bold:          p.bold,
					Italic:        p.italic,
					Monospace:     p.mono,
					Strike:        p.strike,
					Underline:     p.underline,
					SmallCaps:     p.smallCaps,
					Accent:        p.linkID != 0 && !p.explicitFg,
					ExplicitColor: p.explicitFg,
					Color:         p.fg,
					Href:          w.hrefs[p.linkID],
				})
			}
		}
	}
	for _, ch := range n.children {
		visit(ch, parent)
	}
	return mergeSpans(out), ids
}

func collapseSpaces(s string) string {
	if !strings.Contains(s, "  ") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sameSpanStyle(a, b Span) bool {
	return a.Size == b.Size && a.Bold == b.Bold && a.Italic == b.Italic && a.Monospace == b.Monospace &&
		a.Strike == b.Strike && a.Underline == b.Underline && a.SmallCaps == b.SmallCaps &&
		a.Accent == b.Accent && a.ExplicitColor == b.ExplicitColor && a.Color == b.Color && a.Href == b.Href
}

func mergeSpans(spans []Span) []Span {
	if len(spans) == 0 {
		return nil
	}
	var out []Span
	atLineStart := true
	for _, sp := range spans {
		if atLineStart {
			sp.Text = strings.TrimLeft(sp.Text, " ")
		}
		if sp.Text == "" {
			continue
		}
		if len(out) > 0 {
			last := &out[len(out)-1]
			if strings.HasPrefix(sp.Text, "\n") {
				last.Text = strings.TrimRight(last.Text, " ")
			} else if strings.HasSuffix(last.Text, " ") && strings.HasPrefix(sp.Text, " ") {
				sp.Text = strings.TrimLeft(sp.Text, " ")
				if sp.Text == "" {
					continue
				}
			}
			if last.Text == "" {
				out = out[:len(out)-1]
			}
		}
		if len(out) > 0 {
			last := &out[len(out)-1]
			if sameSpanStyle(*last, sp) {
				last.Text += sp.Text
				atLineStart = strings.HasSuffix(last.Text, "\n")
				continue
			}
		}
		atLineStart = strings.HasSuffix(sp.Text, "\n")
		out = append(out, sp)
	}
	for len(out) > 0 {
		last := &out[len(out)-1]
		last.Text = strings.TrimRight(last.Text, " ")
		if last.Text != "" {
			break
		}
		out = out[:len(out)-1]
	}
	return out
}

func hasExplicitBoxDecl(n *node) (hasMarginTop, hasMarginBottom, hasPadding, hasBg bool) {
	if n == nil {
		return false, false, false, false
	}
	if len(n.decl) > 0 {
		_, hasMarginTop = n.decl["margin-top"]
		_, hasMarginBottom = n.decl["margin-bottom"]
		for _, side := range borderSides {
			if _, ok := n.decl["padding-"+side]; ok {
				hasPadding = true
				break
			}
		}
		_, hasBg = n.decl["background-color"]
	}
	if !hasBg {
		switch n.tag {
		case "body", "table", "thead", "tbody", "tfoot", "tr", "td", "th":
			_, hasBg = htmlColor(n.attrs["bgcolor"])
		}
	}
	return
}

func (w *blockExtractor) walk(n *node, parent style, indent, availW int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if n.tag == "" {
		txt := cleanText(applyTextTransform(n.text, parent.textTransform))
		if txt != "" {
			return w.addBlock(StyledBlock{
				Kind:   BlockParagraph,
				Size:   max(1, parent.size),
				Indent: indent,
				Spans:  []Span{{Text: txt, Size: max(1, parent.size), Bold: parent.bold, Italic: parent.italic, Monospace: parent.mono, Strike: parent.strike, Underline: parent.underline, Color: parent.fg, ExplicitColor: parent.explicitFg, Href: w.hrefs[parent.linkID], Accent: parent.linkID != 0 && !parent.explicitFg}},
			})
		}
		return nil
	}
	s := w.d.styled(n, parent)
	if s.display == "none" {
		return nil
	}
	id := w.claimID(anchorName(n))
	prevParentID := w.parentID
	if id != "" {
		w.parentID = id
		defer func() { w.parentID = prevParentID }()
	}

	margin, padding := resolveEdges(s.margin, availW), resolveEdges(s.padding, availW)
	padInsets := Insets{Top: padding[0], Right: padding[1], Bottom: padding[2], Left: padding[3]}
	hasMarginTop, hasMarginBottom, hasPadding, hasBg := hasExplicitBoxDecl(n)

	if kind, source := diagramSource(n); kind != "" {
		return w.extractDiagram(n, kind, source, s, indent, id, prevParentID, margin, padInsets)
	}
	if lvl := headingLevel(n.tag); lvl > 0 {
		spans, inlineIDs := w.inlineSpansWithIDs(n, s)
		for i := range spans {
			spans[i].Bold = true
			if !spans[i].ExplicitColor {
				spans[i].Accent = true
			}
		}
		sz := max(1, s.size)
		if singleSz := uniformSpanSize(spans); singleSz > 0 && n.decl["font-size"] == "" {
			sz = singleSz
		}
		var fmt *BlockFormat
		if hasMarginTop || hasMarginBottom || hasPadding || hasBg || s.lineMode != 0 {
			fmt = &BlockFormat{
				LineHeight:  s.line,
				SpaceBefore: max(0, margin[0]),
				SpaceAfter:  max(0, margin[2]),
				Padding:     padInsets,
				Background:  s.bg,
			}
		}
		if err := w.addBlock(StyledBlock{
			Kind:   BlockHeading,
			Level:  lvl,
			Size:   sz,
			Indent: indent,
			Spans:  spans,
			Format: fmt,
		}); err != nil {
			return err
		}
		w.wrapLeaf(len(w.blocks)-1, id, prevParentID, s, margin, padInsets, inlineIDs)
		return nil
	}
	switch n.tag {
	case "hr":
		if err := w.addBlock(StyledBlock{Kind: BlockRule, Indent: indent}); err != nil {
			return err
		}
		w.wrapLeaf(len(w.blocks)-1, id, prevParentID, s, margin, padInsets, nil)
		return nil
	case "img":
		return w.extractImage(n, s, indent, id, prevParentID, margin, padInsets)
	case "pre":
		code := strings.TrimSuffix(strings.TrimPrefix(collectRawText(n), "\n"), "\n")
		lang := codeLanguage(n)
		var fmt *BlockFormat
		if hasPadding || hasBg || s.hasBorder() {
			fmt = &BlockFormat{
				LineHeight:  s.line,
				SpaceBefore: max(0, margin[0]),
				SpaceAfter:  max(0, margin[2]),
				Padding:     padInsets,
				Background:  s.bg,
			}
		}
		var inlineIDs []string
		var collectIDs func(*node)
		collectIDs = func(cur *node) {
			for _, c := range cur.children {
				if c.tag != "" {
					if cs := w.d.styled(c, s); cs.display != "none" {
						if cid := anchorName(c); cid != "" {
							inlineIDs = append(inlineIDs, cid)
						}
						collectIDs(c)
					}
				}
			}
		}
		collectIDs(n)
		if err := w.addBlock(StyledBlock{
			Kind:     BlockCode,
			Size:     max(1, s.size),
			Indent:   indent,
			Language: lang,
			Code:     code,
			Spans:    []Span{{Text: code, Size: max(1, s.size), Monospace: true, Color: s.fg, ExplicitColor: s.explicitFg}},
			Format:   fmt,
		}); err != nil {
			return err
		}
		w.wrapLeaf(len(w.blocks)-1, id, prevParentID, s, margin, padInsets, inlineIDs)
		return nil
	case "blockquote":
		if s.hasBorder() || hasPadding || hasBg {
			return w.walkContainer(n, s, indent, availW, id, prevParentID, margin, padInsets, true)
		}
		start := len(w.blocks)
		if err := w.walkChildren(n, s, indent+24, max(32, availW-24)); err != nil {
			return err
		}
		for i := start; i < len(w.blocks); i++ {
			w.blocks[i].Quotes = append(w.blocks[i].Quotes, QuoteBar{Indent: indent + 24, Last: i == len(w.blocks)-1})
			if w.blocks[i].Kind == BlockParagraph {
				w.blocks[i].Kind = BlockQuote
			}
		}
		if id != "" {
			if len(w.blocks) == start {
				w.pendingIDs = append(w.pendingIDs, pendingID{id: id, parentID: prevParentID})
			} else if len(w.sections) < maxStyledBlocks {
				w.sections = append(w.sections, Section{Start: start, End: len(w.blocks), SourceID: id, ParentID: prevParentID})
			}
		}
		return nil
	case "ul", "ol":
		return w.extractList(n, s, indent, availW, id, prevParentID, margin, padInsets)
	case "table":
		return w.extractTable(n, s, indent, availW, id, prevParentID, margin, padInsets)
	case "dd":
		indent += 24
		availW = max(32, availW-24)
	}
	if !hasBlockChildren(n, w.d, s) {
		if single := standaloneImgChild(n); single != nil {
			is := w.d.styled(single, s)
			imgID := id
			if imgID == "" {
				imgID = w.claimID(anchorName(single))
			}
			return w.extractImage(single, is, indent, imgID, prevParentID, margin, padInsets)
		}
		spans, inlineIDs := w.inlineSpansWithIDs(n, s)
		if len(spans) > 0 {
			sz := max(1, s.size)
			if singleSz := uniformSpanSize(spans); singleSz > 0 && n.decl["font-size"] == "" {
				sz = singleSz
			}
			var fmt *BlockFormat
			if hasMarginTop || hasMarginBottom || hasPadding || hasBg || s.lineMode != 0 {
				fmt = &BlockFormat{
					LineHeight:  s.line,
					SpaceBefore: max(0, margin[0]),
					SpaceAfter:  max(0, margin[2]),
					Padding:     padInsets,
					Background:  s.bg,
				}
			}
			if err := w.addBlock(StyledBlock{
				Kind:   BlockParagraph,
				Size:   sz,
				Indent: indent,
				Spans:  spans,
				Format: fmt,
			}); err != nil {
				return err
			}
			w.wrapLeaf(len(w.blocks)-1, id, prevParentID, s, margin, padInsets, inlineIDs)
			return nil
		}
		if id != "" {
			if s.hasBorder() || hasPadding || hasBg {
				if err := w.addBlock(StyledBlock{Kind: BlockParagraph, Size: max(1, s.size), Indent: indent, Format: &BlockFormat{LineHeight: 1}}); err != nil {
					return err
				}
				w.wrapLeaf(len(w.blocks)-1, id, prevParentID, s, margin, padInsets, inlineIDs)
			} else {
				w.pendingIDs = append(w.pendingIDs, pendingID{id: id, parentID: prevParentID})
			}
		}
		for _, inl := range inlineIDs {
			if uid := w.claimID(inl); uid != "" {
				w.pendingIDs = append(w.pendingIDs, pendingID{id: uid, parentID: w.parentID})
			}
		}
		return nil
	}
	wantSection := false
	if n != w.d.root && n.tag != "html" && w.sectionDepth < 64 && len(w.sections) < maxStyledBlocks {
		if n.tag == "body" {
			wantSection = id != "" || s.hasBorder() || hasPadding || hasBg
			if !hasMarginTop {
				margin[0] = 0
			}
			if !hasMarginBottom {
				margin[2] = 0
			}
		} else {
			wantSection = id != "" || s.hasBorder() || padInsets != (Insets{}) || s.bg.A != 0 || margin[0] > 0 || margin[2] > 0
		}
	}
	return w.walkContainer(n, s, indent, availW, id, prevParentID, margin, padInsets, wantSection)
}

func (w *blockExtractor) walkContainer(n *node, s style, indent, availW int, id, parentID string, margin [4]int, pad Insets, wantSection bool) error {
	start := len(w.blocks)
	pendingStart := len(w.pendingIDs)
	secIdx := -1
	if wantSection && w.sectionDepth < 64 && len(w.sections) < maxStyledBlocks {
		top, right, bottom, left := sectionBorders(s)
		secIdx = len(w.sections)
		w.sections = append(w.sections, Section{
			Start:       start,
			SpaceBefore: max(0, margin[0]),
			SpaceAfter:  max(0, margin[2]),
			SourceID:    id,
			ParentID:    parentID,
			Padding:     pad,
			Background:  s.bg,
			Top:         top,
			Right:       right,
			Bottom:      bottom,
			Left:        left,
		})
		w.sectionDepth++
	}
	childAvailW := max(32, availW-pad.Left-pad.Right-s.borderH())
	err := w.walkChildren(n, s, indent, childAvailW)
	if secIdx >= 0 {
		w.sectionDepth--
	}
	if err != nil {
		return err
	}
	if len(w.blocks) == start && (id != "" || len(w.pendingIDs) > pendingStart || (wantSection && s.bg.A != 0)) {
		if err := w.addBlock(StyledBlock{
			Kind:   BlockParagraph,
			Size:   max(1, s.size),
			Indent: indent,
			Format: &BlockFormat{LineHeight: 1},
		}); err != nil {
			return err
		}
	} else if len(w.blocks) > start && len(w.pendingIDs) > 0 {
		w.attachPendingIDs(len(w.blocks) - 1)
	}
	if secIdx >= 0 {
		w.sections[secIdx].End = len(w.blocks)
	}
	return nil
}

func (w *blockExtractor) extractList(n *node, s style, indent, availW int, id, parentID string, margin [4]int, pad Insets) error {
	hasMarginTop, hasMarginBottom, hasPadding, hasBg := hasExplicitBoxDecl(n)
	wantSection := id != "" || s.hasBorder() || hasPadding || hasBg
	startList := len(w.blocks)
	secIdx := -1
	if wantSection && w.sectionDepth < 64 && len(w.sections) < maxStyledBlocks {
		top, right, bottom, left := sectionBorders(s)
		spaceBefore, spaceAfter := 0, 0
		if hasMarginTop {
			spaceBefore = max(0, margin[0])
		}
		if hasMarginBottom {
			spaceAfter = max(0, margin[2])
		}
		secIdx = len(w.sections)
		w.sections = append(w.sections, Section{
			Start:       startList,
			SpaceBefore: spaceBefore,
			SpaceAfter:  spaceAfter,
			SourceID:    id,
			ParentID:    parentID,
			Padding:     pad,
			Background:  s.bg,
			Top:         top,
			Right:       right,
			Bottom:      bottom,
			Left:        left,
		})
		w.sectionDepth++
	}
	idx := listStart(n)
	step := listStep(n)
	for _, c := range n.children {
		if c.tag != "li" {
			continue
		}
		cs := w.d.styled(c, s)
		if cs.display == "none" {
			continue
		}
		liID := w.claimID(anchorName(c))
		prevLiParent := w.parentID
		if liID != "" {
			w.parentID = liID
		}
		if n.tag == "ol" {
			idx = listNumber(c.attrs["value"], idx)
		}
		marker := formatListMarker(resolveListStyleType(n, c, cs), idx)
		idx += step
		start := len(w.blocks)
		if hasBlockChildren(c, w.d, cs) {
			if err := w.walkChildren(c, cs, indent+24, max(32, availW-24)); err != nil {
				w.parentID = prevLiParent
				if secIdx >= 0 {
					w.sectionDepth--
				}
				return err
			}
			if liID != "" && len(w.blocks) > start && len(w.sections) < maxStyledBlocks {
				w.sections = append(w.sections, Section{Start: start, End: len(w.blocks), SourceID: liID, ParentID: prevLiParent})
			}
		} else {
			spans, inlineIDs := w.inlineSpansWithIDs(c, cs)
			if len(spans) > 0 {
				liMargin, liPad := resolveEdges(cs.margin, availW), resolveEdges(cs.padding, availW)
				liPadInsets := Insets{Top: liPad[0], Right: liPad[1], Bottom: liPad[2], Left: liPad[3]}
				var liFmt *BlockFormat
				if cs.lineMode != 0 || liPadInsets != (Insets{}) || cs.bg.A != 0 {
					liFmt = &BlockFormat{
						LineHeight:  cs.line,
						SpaceBefore: max(0, liMargin[0]),
						SpaceAfter:  max(4, liMargin[2]),
						Padding:     liPadInsets,
						Background:  cs.bg,
					}
				}
				if err := w.addBlock(StyledBlock{Kind: BlockParagraph, Size: max(1, cs.size), Indent: indent + 24, Spans: spans, Format: liFmt}); err != nil {
					w.parentID = prevLiParent
					if secIdx >= 0 {
						w.sectionDepth--
					}
					return err
				}
				w.wrapLeaf(len(w.blocks)-1, liID, prevLiParent, cs, liMargin, liPadInsets, inlineIDs)
			}
		}
		if start < len(w.blocks) && marker != "" {
			w.blocks[start].Spans = append([]Span{{Text: marker + " ", Size: max(1, cs.size), Bold: cs.bold, Color: cs.fg, ExplicitColor: cs.explicitFg}}, w.blocks[start].Spans...)
		}
		w.parentID = prevLiParent
	}
	if secIdx >= 0 {
		w.sectionDepth--
		w.sections[secIdx].End = len(w.blocks)
	}
	return nil
}

func standaloneImgChild(n *node) *node {
	var found *node
	for _, c := range n.children {
		if c.tag == "" && strings.TrimSpace(c.text) == "" {
			continue
		}
		if c.tag == "img" && found == nil {
			found = c
			continue
		}
		if c.tag == "a" && found == nil {
			if sub := standaloneImgChild(c); sub != nil {
				found = sub
				continue
			}
		}
		return nil
	}
	return found
}

func hasBlockChildren(n *node, d *Document, parent style) bool {
	for _, c := range n.children {
		if c.tag == "" {
			continue
		}
		if kind, _ := diagramSource(c); kind != "" {
			return true
		}
		switch c.tag {
		case "p", "div", "center", "section", "article", "main", "header", "footer", "nav", "aside", "blockquote", "ul", "ol", "li", "dl", "dt", "dd", "pre", "table", "hr", "h1", "h2", "h3", "h4", "h5", "h6":
			return true
		}
		cs := d.styled(c, parent)
		if cs.display == "block" || cs.display == "flex" || cs.display == "grid" || cs.display == "table" || cs.display == "list-item" {
			return true
		}
	}
	return false
}

func (w *blockExtractor) walkChildren(n *node, s style, indent, availW int) error {
	var inlineBuf []*node
	flushInline := func() error {
		if len(inlineBuf) == 0 {
			return nil
		}
		tmp := &node{tag: "div", children: inlineBuf}
		inlineBuf = nil
		spans, inlineIDs := w.inlineSpansWithIDs(tmp, s)
		if len(spans) == 0 {
			for _, inl := range inlineIDs {
				if uid := w.claimID(inl); uid != "" {
					w.pendingIDs = append(w.pendingIDs, pendingID{id: uid, parentID: w.parentID})
				}
			}
			return nil
		}
		sz := max(1, s.size)
		if singleSz := uniformSpanSize(spans); singleSz > 0 {
			sz = singleSz
		}
		if err := w.addBlock(StyledBlock{Kind: BlockParagraph, Size: sz, Indent: indent, Spans: spans}); err != nil {
			return err
		}
		w.wrapLeaf(len(w.blocks)-1, "", w.parentID, style{}, [4]int{}, Insets{}, inlineIDs)
		return nil
	}
	for _, c := range n.children {
		if c.tag == "" {
			if strings.TrimSpace(c.text) != "" || len(inlineBuf) > 0 {
				inlineBuf = append(inlineBuf, c)
			}
			continue
		}
		cs := w.d.styled(c, s)
		if cs.display == "none" {
			continue
		}
		_, isDiag := diagramSource(c)
		isBlock := isDiag != "" || headingLevel(c.tag) > 0
		if !isBlock {
			switch c.tag {
			case "p", "div", "center", "section", "article", "main", "header", "footer", "nav", "aside", "blockquote", "ul", "ol", "li", "dl", "dt", "dd", "pre", "table", "hr":
				isBlock = true
			default:
				if cs.display == "block" || cs.display == "flex" || cs.display == "grid" || cs.display == "table" || cs.display == "list-item" {
					isBlock = true
				}
			}
		}
		if isBlock {
			if err := flushInline(); err != nil {
				return err
			}
			if err := w.walk(c, s, indent, availW); err != nil {
				return err
			}
		} else {
			inlineBuf = append(inlineBuf, c)
		}
	}
	return flushInline()
}

func (w *blockExtractor) extractImage(n *node, s style, indent int, id, parentID string, margin [4]int, pad Insets) error {
	src := n.attrs["src"]
	alt := cleanText(n.attrs["alt"])
	img := w.opts.Images[src]
	if img == nil && alt == "" {
		if id != "" {
			w.pendingIDs = append(w.pendingIDs, pendingID{id: id, parentID: parentID})
		}
		return nil
	}
	availW := max(64, w.opts.Width-indent)
	rawW := pixelAttr(n.attrs["width"])
	if s.width.definite(availW) {
		rawW = s.width.resolve(availW, availW)
	}
	rawH := pixelAttr(n.attrs["height"])
	if s.height.absolute() {
		rawH = s.height.resolve(0, 1024)
	}
	iw := min(availW, rawW)
	ih := min(1024, rawH)
	align := ""
	if s.align == "center" || s.align == "right" {
		align = s.align
	}
	var spans []Span
	if alt != "" {
		spans = []Span{{Text: "[image: " + alt + "]", Size: max(1, s.size), Italic: true, Color: s.fg, ExplicitColor: s.explicitFg}}
	}
	if err := w.addBlock(StyledBlock{
		Kind:        BlockImage,
		Size:        max(1, s.size),
		Indent:      indent,
		ImageKey:    src,
		Image:       img,
		ImageWidth:  iw,
		ImageHeight: ih,
		ImageAlign:  align,
		Href:        w.hrefs[s.linkID],
		Spans:       spans,
	}); err != nil {
		return err
	}
	w.wrapLeaf(len(w.blocks)-1, id, parentID, s, margin, pad, nil)
	return nil
}

func (w *blockExtractor) extractDiagram(n *node, kind, source string, s style, indent int, id, parentID string, margin [4]int, pad Insets) error {
	label := diagramLabel(n)
	if w.opts.DisableDiagrams {
		if err := w.addBlock(StyledBlock{
			Kind:   BlockImage,
			Size:   max(1, s.size),
			Indent: indent,
			Spans:  []Span{{Text: "[" + strings.ToUpper(kind) + ": " + label + "]", Size: max(1, s.size), Italic: true, Color: s.fg, ExplicitColor: s.explicitFg}},
		}); err != nil {
			return err
		}
		w.wrapLeaf(len(w.blocks)-1, id, parentID, s, margin, pad, nil)
		return nil
	}
	w.diagramCount++
	if w.diagramCount > 32 {
		return limitError("diagram count")
	}
	availW := max(64, min(4096, w.opts.Width-indent))
	remaining := min(4<<20, w.d.limits.Pixels) - w.diagramPx
	maxH := min(2048, (1<<20)/availW, remaining/availW)
	if maxH < 1 {
		return limitError("diagram pixels")
	}
	renderer := w.opts.Diagrams
	if renderer == nil {
		renderer = DefaultDiagramRenderer{}
	}
	img, err := renderer.RenderDiagram(w.ctx, kind, source, availW, maxH)
	if err != nil {
		if err == w.ctx.Err() && err != nil {
			return err
		}
		img = nil
	}
	if img != nil && !img.Bounds().Empty() {
		w.diagramPx += img.Bounds().Dx() * img.Bounds().Dy()
	}
	if err := w.addBlock(StyledBlock{
		Kind:     BlockImage,
		Size:     max(1, s.size),
		Indent:   indent,
		ImageKey: kind + ":" + label,
		Image:    img,
		Href:     w.hrefs[s.linkID],
		Spans:    []Span{{Text: "[" + strings.ToUpper(kind) + ": " + label + "]", Size: max(1, s.size), Italic: true, Color: s.fg, ExplicitColor: s.explicitFg}},
	}); err != nil {
		return err
	}
	w.wrapLeaf(len(w.blocks)-1, id, parentID, s, margin, pad, nil)
	return nil
}

func cellHasNestedStructure(n *node, d *Document, parent style) bool {
	for _, c := range n.children {
		if c.tag == "" {
			continue
		}
		if kind, _ := diagramSource(c); kind != "" {
			return true
		}
		cs := d.styled(c, parent)
		if cs.display == "none" {
			continue
		}
		switch c.tag {
		case "table", "ul", "ol", "dl", "pre", "blockquote", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "section", "article", "main", "header", "footer", "nav", "aside":
			return true
		}
		if cs.display == "table" || cs.display == "list-item" {
			return true
		}
		if cellHasNestedStructure(c, d, cs) {
			return true
		}
	}
	return false
}

func tableHasNestedStructure(n *node, d *Document, tableStyle style) bool {
	checkRow := func(tr *node, rowStyle style) bool {
		for _, cell := range tr.children {
			if cell.tag != "td" && cell.tag != "th" {
				continue
			}
			cs := d.styled(cell, rowStyle)
			if cs.display == "none" {
				continue
			}
			if cellHasNestedStructure(cell, d, cs) {
				return true
			}
		}
		return false
	}
	for _, child := range n.children {
		switch child.tag {
		case "thead", "tbody", "tfoot":
			gs := d.styled(child, tableStyle)
			if gs.display == "none" {
				continue
			}
			for _, tr := range child.children {
				if tr.tag == "tr" {
					rs := d.styled(tr, gs)
					if rs.display != "none" && checkRow(tr, rs) {
						return true
					}
				}
			}
		case "tr":
			rs := d.styled(child, tableStyle)
			if rs.display != "none" && checkRow(child, rs) {
				return true
			}
		}
	}
	return false
}

func (w *blockExtractor) extractLayoutTable(n *node, s style, indent, availW int, id, parentID string, margin [4]int, pad Insets) error {
	hasMarginTop, hasMarginBottom, hasPadding, hasBg := hasExplicitBoxDecl(n)
	wantSection := id != "" || hasPadding || hasBg || s.bg.A != 0 || hasMarginTop || hasMarginBottom
	start := len(w.blocks)
	pendingStart := len(w.pendingIDs)
	secIdx := -1
	if wantSection && w.sectionDepth < 64 && len(w.sections) < maxStyledBlocks {
		var secPad Insets
		if hasPadding {
			secPad = pad
		}
		spaceBefore, spaceAfter := 0, 0
		if hasMarginTop {
			spaceBefore = max(0, margin[0])
		}
		if hasMarginBottom {
			spaceAfter = max(0, margin[2])
		}
		secIdx = len(w.sections)
		w.sections = append(w.sections, Section{
			Start:       start,
			SpaceBefore: spaceBefore,
			SpaceAfter:  spaceAfter,
			SourceID:    id,
			ParentID:    parentID,
			Padding:     secPad,
			Background:  s.bg,
		})
		w.sectionDepth++
	}
	walkRow := func(tr *node, rowStyle style) error {
		rid := w.claimID(anchorName(tr))
		prevRowParent := w.parentID
		if rid != "" {
			w.parentID = rid
		}
		defer func() { w.parentID = prevRowParent }()

		_, _, rowHasPad, rowHasBg := hasExplicitBoxDecl(tr)
		wantRowSec := rid != "" || rowHasPad || rowHasBg || rowStyle.bg.A != 0
		rowStart := len(w.blocks)
		rowSecIdx := -1
		if wantRowSec && w.sectionDepth < 64 && len(w.sections) < maxStyledBlocks {
			rowSecIdx = len(w.sections)
			w.sections = append(w.sections, Section{
				Start:      rowStart,
				SourceID:   rid,
				ParentID:   prevRowParent,
				Background: rowStyle.bg,
			})
			w.sectionDepth++
		}
		for _, cell := range tr.children {
			if cell.tag != "td" && cell.tag != "th" {
				continue
			}
			cs := w.d.styled(cell, rowStyle)
			if cs.display == "none" {
				continue
			}
			cid := w.claimID(anchorName(cell))
			prevCellParent := w.parentID
			if cid != "" {
				w.parentID = cid
			}
			cellMargin, cellPad := resolveEdges(cs.margin, availW), resolveEdges(cs.padding, availW)
			_, _, cellHasPad, cellHasBg := hasExplicitBoxDecl(cell)
			var cellPadInsets Insets
			if cellHasPad || (rowStyle.hasCellPadding && rowStyle.cellPadding > 0) {
				cellPadInsets = Insets{Top: cellPad[0], Right: cellPad[1], Bottom: cellPad[2], Left: cellPad[3]}
			}
			cellStyle := cs
			cellStyle.bw = [4]int{}
			wantCellSec := cid != "" || cellHasPad || cellHasBg || cs.bg.A != 0
			if err := w.walkContainer(cell, cellStyle, indent, availW, cid, prevCellParent, cellMargin, cellPadInsets, wantCellSec); err != nil {
				w.parentID = prevCellParent
				if rowSecIdx >= 0 {
					w.sectionDepth--
				}
				return err
			}
			w.parentID = prevCellParent
		}
		if rowSecIdx >= 0 {
			w.sectionDepth--
			if len(w.blocks) > rowStart {
				w.sections[rowSecIdx].End = len(w.blocks)
			} else if rid != "" {
				w.pendingIDs = append(w.pendingIDs, pendingID{id: rid, parentID: prevRowParent})
			}
		} else if rid != "" && len(w.blocks) == rowStart {
			w.pendingIDs = append(w.pendingIDs, pendingID{id: rid, parentID: prevRowParent})
		}
		return nil
	}
	for _, child := range n.children {
		switch child.tag {
		case "caption":
			if err := w.walk(child, s, indent, availW); err != nil {
				if secIdx >= 0 {
					w.sectionDepth--
				}
				return err
			}
		case "thead", "tbody", "tfoot":
			gs := w.d.styled(child, s)
			if gs.display == "none" {
				continue
			}
			gid := w.claimID(anchorName(child))
			prevGroupParent := w.parentID
			if gid != "" {
				w.parentID = gid
			}
			gStart := len(w.blocks)
			for _, tr := range child.children {
				if tr.tag == "tr" {
					rs := w.d.styled(tr, gs)
					if rs.display != "none" {
						if err := walkRow(tr, rs); err != nil {
							w.parentID = prevGroupParent
							if secIdx >= 0 {
								w.sectionDepth--
							}
							return err
						}
					}
				}
			}
			if gid != "" {
				if len(w.blocks) > gStart && len(w.sections) < maxStyledBlocks {
					w.sections = append(w.sections, Section{Start: gStart, End: len(w.blocks), SourceID: gid, ParentID: prevGroupParent, Background: gs.bg})
				} else {
					w.pendingIDs = append(w.pendingIDs, pendingID{id: gid, parentID: prevGroupParent})
				}
			}
			w.parentID = prevGroupParent
		case "tr":
			rs := w.d.styled(child, s)
			if rs.display != "none" {
				if err := walkRow(child, rs); err != nil {
					if secIdx >= 0 {
						w.sectionDepth--
					}
					return err
				}
			}
		}
	}
	if secIdx >= 0 {
		w.sectionDepth--
	}
	if len(w.blocks) == start && (id != "" || len(w.pendingIDs) > pendingStart || (wantSection && s.bg.A != 0)) {
		if err := w.addBlock(StyledBlock{
			Kind:   BlockParagraph,
			Size:   max(1, s.size),
			Indent: indent,
			Format: &BlockFormat{LineHeight: 1},
		}); err != nil {
			return err
		}
	} else if len(w.blocks) > start && len(w.pendingIDs) > 0 {
		w.attachPendingIDs(len(w.blocks) - 1)
	}
	if secIdx >= 0 {
		w.sections[secIdx].End = len(w.blocks)
	}
	return nil
}

func (w *blockExtractor) extractTable(n *node, s style, indent, availW int, id, parentID string, margin [4]int, pad Insets) error {
	if tableHasNestedStructure(n, w.d, s) {
		return w.extractLayoutTable(n, s, indent, availW, id, parentID, margin, pad)
	}
	var rows [][][]Span
	var innerIDs []string
	hasHeader := false
	var uniformCellBg color.RGBA
	allCellsSameBg := true
	sawAnyCell := false
	collectRow := func(tr *node, rowStyle style, forceBold bool) {
		if forceBold {
			hasHeader = true
		}
		if rid := anchorName(tr); rid != "" {
			innerIDs = append(innerIDs, rid)
		}
		var cells [][]Span
		rowHasSpans := false
		for _, cell := range tr.children {
			if cell.tag != "td" && cell.tag != "th" {
				continue
			}
			cs := w.d.styled(cell, rowStyle)
			if cs.display == "none" {
				continue
			}
			sawAnyCell = true
			if cid := anchorName(cell); cid != "" {
				innerIDs = append(innerIDs, cid)
			}
			if cell.tag == "th" || forceBold {
				cs.bold = true
				hasHeader = true
			}
			cellBg := cs.bg
			if cellBg.A == 0 {
				cellBg = rowStyle.bg
			}
			if cellBg.A == 0 {
				allCellsSameBg = false
			} else if uniformCellBg.A == 0 {
				uniformCellBg = cellBg
			} else if uniformCellBg != cellBg {
				allCellsSameBg = false
			}
			spans, inlIDs := w.inlineSpansWithIDs(cell, cs)
			innerIDs = append(innerIDs, inlIDs...)
			if len(spans) > 0 {
				rowHasSpans = true
			}
			cells = append(cells, spans)
			colSpan := min(16, spanValue(cell.attrs["colspan"], false))
			for k := 1; k < colSpan; k++ {
				cells = append(cells, nil)
			}
		}
		if rowHasSpans {
			rows = append(rows, cells)
		}
	}
	for _, child := range n.children {
		switch child.tag {
		case "thead", "tbody", "tfoot":
			gs := w.d.styled(child, s)
			if gs.display == "none" {
				continue
			}
			if gid := anchorName(child); gid != "" {
				innerIDs = append(innerIDs, gid)
			}
			for _, tr := range child.children {
				if tr.tag == "tr" {
					rs := w.d.styled(tr, gs)
					if rs.display != "none" {
						if rs.bg.A == 0 {
							rs.bg = gs.bg
						}
						collectRow(tr, rs, child.tag == "thead")
					}
				}
			}
		case "tr":
			rs := w.d.styled(child, s)
			if rs.display != "none" {
				collectRow(child, rs, false)
			}
		}
	}
	hasMarginTop, hasMarginBottom, hasPadding, hasBg := hasExplicitBoxDecl(n)
	tableBg := s.bg
	if tableBg.A == 0 && sawAnyCell && allCellsSameBg && uniformCellBg.A != 0 {
		tableBg = uniformCellBg
		hasBg = true
	}
	if len(rows) == 0 {
		if hasBg && tableBg.A != 0 {
			tableStyle := s
			tableStyle.bw = [4]int{}
			tableStyle.bg = tableBg
			if err := w.addBlock(StyledBlock{
				Kind:   BlockParagraph,
				Size:   max(1, s.size),
				Indent: indent,
				Format: &BlockFormat{LineHeight: 2, Padding: pad, Background: tableBg},
			}); err != nil {
				return err
			}
			w.wrapLeaf(len(w.blocks)-1, id, parentID, tableStyle, margin, pad, innerIDs)
		}
		return nil
	}
	maxCols := 0
	for _, r := range rows {
		maxCols = max(maxCols, len(r))
	}
	if maxCols > 1 {
		var kept []int
		for c := 0; c < maxCols; c++ {
			for _, r := range rows {
				if c < len(r) && len(r[c]) > 0 {
					kept = append(kept, c)
					break
				}
			}
		}
		if len(kept) > 0 {
			for i, r := range rows {
				norm := make([][]Span, len(kept))
				for j, c := range kept {
					if c < len(r) {
						norm[j] = r[c]
					}
				}
				rows[i] = norm
			}
		}
	}
	var fmt *BlockFormat
	if hasMarginTop || hasMarginBottom || hasPadding || hasBg || s.lineMode != 0 {
		spaceBefore, spaceAfter := 8, 8
		if hasMarginTop {
			spaceBefore = max(0, margin[0])
		}
		if hasMarginBottom {
			spaceAfter = max(0, margin[2])
		}
		fmt = &BlockFormat{
			LineHeight:  s.line,
			SpaceBefore: spaceBefore,
			SpaceAfter:  spaceAfter,
			Padding:     pad,
			Background:  tableBg,
		}
	}
	tableStyle := s
	tableStyle.bw = [4]int{}
	tableStyle.bg = tableBg
	if len(rows) == 1 && !hasHeader {
		var merged []Span
		for _, cell := range rows[0] {
			if len(cell) == 0 {
				continue
			}
			if len(merged) > 0 {
				last := merged[len(merged)-1]
				merged = append(merged, Span{Text: "  ", Size: max(1, last.Size), Color: s.fg, ExplicitColor: s.explicitFg})
			}
			merged = append(merged, cell...)
		}
		merged = mergeSpans(merged)
		if err := w.addBlock(StyledBlock{
			Kind:   BlockParagraph,
			Size:   max(1, s.size),
			Indent: indent,
			Spans:  merged,
			Format: fmt,
		}); err != nil {
			return err
		}
		w.wrapLeaf(len(w.blocks)-1, id, parentID, tableStyle, margin, pad, innerIDs)
		return nil
	}
	// Table CSS borders apply to the table grid; don't wrap the whole table in a Section border.
	if err := w.addBlock(StyledBlock{
		Kind:   BlockTable,
		Size:   max(1, s.size),
		Indent: indent,
		Rows:   rows,
		Format: fmt,
	}); err != nil {
		return err
	}
	w.wrapLeaf(len(w.blocks)-1, id, parentID, tableStyle, margin, pad, innerIDs)
	return nil
}
