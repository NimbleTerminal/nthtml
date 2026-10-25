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

// Span is a styled inline text run extracted from the document tree.
type Span struct {
	Text                                       string
	Bold, Italic, Monospace, Strike, Underline bool
	SmallCaps, Accent                          bool
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

const maxStyledBlocks = 32768

// Blocks extracts styled reading-order blocks from d using the CSS cascade,
// pseudo-elements, list markers, table structure, and optional image/diagram
// resources in o.
func (d *Document) Blocks(ctx context.Context, o Options) ([]StyledBlock, error) {
	if d == nil || d.root == nil {
		return nil, limitError("nil document")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.Width == 0 {
		o.Width = 800
	}
	links, err := d.Links(ctx)
	if err != nil {
		return nil, err
	}
	hrefs := make(map[int]string, len(links))
	for _, l := range links {
		hrefs[l.ID] = l.Href
	}
	w := &blockExtractor{
		ctx:   ctx,
		d:     d,
		opts:  o,
		hrefs: hrefs,
	}
	base := style{rootSize: 16, size: 16, line: 22, spacing: [2]int{2, 2}, fg: color.RGBA{0, 0, 0, 255}}
	if err := w.walk(d.root, base, 0); err != nil {
		return nil, err
	}
	return w.blocks, nil
}

type blockExtractor struct {
	ctx          context.Context
	d            *Document
	opts         Options
	hrefs        map[int]string
	blocks       []StyledBlock
	diagramCount int
	diagramPx    int
}

func (w *blockExtractor) addBlock(b StyledBlock) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if len(w.blocks) >= maxStyledBlocks {
		return limitError("typeset blocks")
	}
	w.blocks = append(w.blocks, b)
	return nil
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

func (w *blockExtractor) inlineSpans(n *node, parent style) []Span {
	var out []Span
	var visit func(*node, style)
	visit = func(cur *node, p style) {
		if cur.tag == "" {
			txt := applyTextTransform(cur.text, p.textTransform)
			if !p.pre && p.whiteSpace != "pre-wrap" && p.whiteSpace != "pre-line" {
				txt = cleanText(txt)
			}
			if txt == "" {
				return
			}
			if p.smallCaps {
				txt = strings.ToUpper(txt)
			}
			out = append(out, Span{
				Text:      txt,
				Bold:      p.bold,
				Italic:    p.italic,
				Monospace: p.mono,
				Strike:    p.strike,
				Underline: p.underline,
				SmallCaps: p.smallCaps,
				Accent:    p.linkID != 0,
				Color:     p.fg,
				Href:      w.hrefs[p.linkID],
			})
			return
		}
		if cur.tag == "br" {
			out = append(out, Span{Text: "\n", Bold: p.bold, Italic: p.italic, Monospace: p.mono})
			return
		}
		s := w.d.styled(cur, p)
		if s.display == "none" {
			return
		}
		if cur.tag == "img" {
			alt := cleanText(cur.attrs["alt"])
			if alt == "" {
				alt = cleanText(cur.attrs["src"])
			}
			out = append(out, Span{
				Text:   "[image: " + alt + "]",
				Italic: true,
				Color:  s.fg,
				Href:   w.hrefs[s.linkID],
				Accent: s.linkID != 0,
			})
			return
		}
		for _, ch := range cur.children {
			visit(ch, s)
		}
	}
	for _, ch := range n.children {
		visit(ch, parent)
	}
	return mergeSpans(out)
}

func mergeSpans(spans []Span) []Span {
	if len(spans) == 0 {
		return nil
	}
	var out []Span
	for _, sp := range spans {
		if sp.Text == "" {
			continue
		}
		if len(out) > 0 {
			last := &out[len(out)-1]
			if !strings.HasSuffix(last.Text, " ") && !strings.HasSuffix(last.Text, "\n") && !strings.HasPrefix(sp.Text, " ") && !strings.HasPrefix(sp.Text, "\n") && !strings.HasPrefix(sp.Text, ".") && !strings.HasPrefix(sp.Text, ",") && !strings.HasPrefix(sp.Text, ":") && !strings.HasPrefix(sp.Text, ";") {
				last.Text += " "
			}
			if last.Bold == sp.Bold && last.Italic == sp.Italic && last.Monospace == sp.Monospace && last.Strike == sp.Strike && last.Underline == sp.Underline && last.SmallCaps == sp.SmallCaps && last.Accent == sp.Accent && last.Color == sp.Color && last.Href == sp.Href {
				last.Text += sp.Text
				continue
			}
		}
		out = append(out, sp)
	}
	return out
}

func (w *blockExtractor) walk(n *node, parent style, indent int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if n.tag == "" {
		txt := cleanText(applyTextTransform(n.text, parent.textTransform))
		if txt != "" {
			return w.addBlock(StyledBlock{
				Kind:   BlockParagraph,
				Indent: indent,
				Spans:  []Span{{Text: txt, Bold: parent.bold, Italic: parent.italic, Monospace: parent.mono, Strike: parent.strike, Underline: parent.underline, Color: parent.fg, Href: w.hrefs[parent.linkID], Accent: parent.linkID != 0}},
			})
		}
		return nil
	}
	s := w.d.styled(n, parent)
	if s.display == "none" {
		return nil
	}
	if kind, source := diagramSource(n); kind != "" {
		return w.extractDiagram(n, kind, source, s, indent)
	}
	if lvl := headingLevel(n.tag); lvl > 0 {
		spans := w.inlineSpans(n, s)
		for i := range spans {
			spans[i].Bold = true
			spans[i].Accent = true
		}
		sizes := []int{50, 46, 42, 38, 33, 29}
		return w.addBlock(StyledBlock{
			Kind:   BlockHeading,
			Level:  lvl,
			Size:   sizes[lvl-1],
			Indent: indent,
			Spans:  spans,
		})
	}
	switch n.tag {
	case "hr":
		return w.addBlock(StyledBlock{Kind: BlockRule, Indent: indent})
	case "img":
		return w.extractImage(n, s, indent)
	case "pre":
		code := strings.TrimSuffix(strings.TrimPrefix(collectRawText(n), "\n"), "\n")
		lang := codeLanguage(n)
		return w.addBlock(StyledBlock{
			Kind:     BlockCode,
			Size:     25,
			Indent:   indent,
			Language: lang,
			Code:     code,
			Spans:    []Span{{Text: code, Monospace: true, Color: s.fg}},
		})
	case "blockquote":
		start := len(w.blocks)
		if err := w.walkChildren(n, s, indent+24); err != nil {
			return err
		}
		for i := start; i < len(w.blocks); i++ {
			w.blocks[i].Quotes = append(w.blocks[i].Quotes, QuoteBar{Indent: indent + 24, Last: i == len(w.blocks)-1})
			if w.blocks[i].Kind == BlockParagraph {
				w.blocks[i].Kind = BlockQuote
			}
		}
		return nil
	case "ul", "ol":
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
			if n.tag == "ol" {
				idx = listNumber(c.attrs["value"], idx)
			}
			marker := formatListMarker(resolveListStyleType(n, c, cs), idx)
			idx += step
			start := len(w.blocks)
			if hasBlockChildren(c, w.d, cs) {
				if err := w.walkChildren(c, cs, indent+24); err != nil {
					return err
				}
			} else {
				spans := w.inlineSpans(c, cs)
				if len(spans) > 0 {
					if err := w.addBlock(StyledBlock{Kind: BlockParagraph, Indent: indent + 24, Spans: spans}); err != nil {
						return err
					}
				}
			}
			if start < len(w.blocks) && marker != "" {
				w.blocks[start].Spans = append([]Span{{Text: marker + " ", Bold: cs.bold, Color: cs.fg}}, w.blocks[start].Spans...)
			}
		}
		return nil
	case "table":
		return w.extractTable(n, s, indent)
	case "dd":
		indent += 24
	}
	if !hasBlockChildren(n, w.d, s) {
		if single := standaloneImgChild(n); single != nil {
			is := w.d.styled(single, s)
			return w.extractImage(single, is, indent)
		}
		spans := w.inlineSpans(n, s)
		if len(spans) > 0 {
			return w.addBlock(StyledBlock{
				Kind:   BlockParagraph,
				Indent: indent,
				Spans:  spans,
			})
		}
		return nil
	}
	return w.walkChildren(n, s, indent)
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
		case "p", "div", "section", "article", "main", "header", "footer", "nav", "aside", "blockquote", "ul", "ol", "li", "dl", "dt", "dd", "pre", "table", "hr", "h1", "h2", "h3", "h4", "h5", "h6":
			return true
		}
		cs := d.styled(c, parent)
		if cs.display == "block" || cs.display == "flex" || cs.display == "table" || cs.display == "list-item" {
			return true
		}
	}
	return false
}

func (w *blockExtractor) walkChildren(n *node, s style, indent int) error {
	var inlineBuf []*node
	flushInline := func() error {
		if len(inlineBuf) == 0 {
			return nil
		}
		tmp := &node{tag: "div", children: inlineBuf}
		inlineBuf = nil
		spans := w.inlineSpans(tmp, s)
		if len(spans) == 0 {
			return nil
		}
		return w.addBlock(StyledBlock{Kind: BlockParagraph, Indent: indent, Spans: spans})
	}
	for _, c := range n.children {
		if c.tag == "" {
			if strings.TrimSpace(c.text) != "" {
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
			case "p", "div", "section", "article", "main", "header", "footer", "nav", "aside", "blockquote", "ul", "ol", "li", "dl", "dt", "dd", "pre", "table", "hr":
				isBlock = true
			default:
				if cs.display == "block" || cs.display == "flex" || cs.display == "table" || cs.display == "list-item" {
					isBlock = true
				}
			}
		}
		if isBlock {
			if err := flushInline(); err != nil {
				return err
			}
			if err := w.walk(c, s, indent); err != nil {
				return err
			}
		} else {
			inlineBuf = append(inlineBuf, c)
		}
	}
	return flushInline()
}

func (w *blockExtractor) extractImage(n *node, s style, indent int) error {
	src := n.attrs["src"]
	alt := cleanText(n.attrs["alt"])
	if alt == "" {
		alt = src
	}
	img := w.opts.Images[src]
	iw := min(max(64, w.opts.Width-indent), pixelAttr(n.attrs["width"]))
	ih := min(1024, pixelAttr(n.attrs["height"]))
	align := ""
	if s.align == "center" || s.align == "right" {
		align = s.align
	}
	return w.addBlock(StyledBlock{
		Kind:        BlockImage,
		Indent:      indent,
		ImageKey:    src,
		Image:       img,
		ImageWidth:  iw,
		ImageHeight: ih,
		ImageAlign:  align,
		Href:        w.hrefs[s.linkID],
		Spans:       []Span{{Text: "[image unavailable: " + alt + "]", Italic: true, Color: s.fg}},
	})
}

func (w *blockExtractor) extractDiagram(n *node, kind, source string, s style, indent int) error {
	label := diagramLabel(n)
	if w.opts.DisableDiagrams {
		return w.addBlock(StyledBlock{
			Kind:   BlockImage,
			Indent: indent,
			Spans:  []Span{{Text: "[" + strings.ToUpper(kind) + ": " + label + "]", Italic: true, Color: s.fg}},
		})
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
	return w.addBlock(StyledBlock{
		Kind:     BlockImage,
		Indent:   indent,
		ImageKey: kind + ":" + label,
		Image:    img,
		Href:     w.hrefs[s.linkID],
		Spans:    []Span{{Text: "[" + strings.ToUpper(kind) + ": " + label + "]", Italic: true, Color: s.fg}},
	})
}

func (w *blockExtractor) extractTable(n *node, s style, indent int) error {
	var rows [][][]Span
	collectRow := func(tr *node, rowStyle style, forceBold bool) {
		var cells [][]Span
		for _, cell := range tr.children {
			if cell.tag != "td" && cell.tag != "th" {
				continue
			}
			cs := w.d.styled(cell, rowStyle)
			if cs.display == "none" {
				continue
			}
			if cell.tag == "th" || forceBold {
				cs.bold = true
			}
			cells = append(cells, w.inlineSpans(cell, cs))
		}
		if len(cells) > 0 {
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
			for _, tr := range child.children {
				if tr.tag == "tr" {
					rs := w.d.styled(tr, gs)
					if rs.display != "none" {
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
	if len(rows) == 0 {
		return nil
	}
	return w.addBlock(StyledBlock{
		Kind:   BlockTable,
		Indent: indent,
		Rows:   rows,
	})
}
