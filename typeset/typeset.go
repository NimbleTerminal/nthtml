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

// Build converts a parsed *document.Document into an *nttypeset.Document,
// preserving headings, paragraphs, blockquotes, lists, tables, horizontal
// rules, code blocks (with optional Chroma syntax highlighting), links, inline
// emphasis, and supplied images/diagrams.
func Build(ctx context.Context, doc *document.Document, opts Options) (*nttypeset.Document, error) {
	styled, err := doc.Blocks(ctx, opts.Document)
	if err != nil {
		return nil, err
	}
	blocks := make([]nttypeset.Block, 0, len(styled))
	for _, b := range styled {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nb := nttypeset.Block{
			Size:   b.Size,
			Indent: b.Indent,
			Href:   b.Href,
		}
		if len(b.Quotes) > 0 {
			nb.Quotes = make([]nttypeset.Quote, len(b.Quotes))
			for i, q := range b.Quotes {
				nb.Quotes[i] = nttypeset.Quote{Indent: q.Indent, Last: q.Last}
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
				nb.Size = 25
			}
			nb.Spans = nttypeset.Highlight(ctx, b.Code, b.Language, opts.Syntax)
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
	return &nttypeset.Document{Blocks: blocks}, nil
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
	lastBottom := -1
	lastRight := -1
	lastLineY := -1

	flush := func() {
		if len(curSpans) == 0 {
			return
		}
		kind := nttypeset.KindParagraph
		size := 0
		if curHeading > 0 {
			kind = nttypeset.KindHeading
			sizes := []int{50, 46, 42, 38, 33, 29}
			size = sizes[min(curHeading, len(sizes))-1]
		}
		blocks = append(blocks, nttypeset.Block{
			Kind:  kind,
			Size:  size,
			Spans: curSpans,
		})
		curSpans = nil
		curHeading = 0
	}

	for _, b := range boxes {
		if b.Tag == "hr" {
			flush()
			blocks = append(blocks, nttypeset.Block{Kind: nttypeset.KindRule})
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
		role := nttypeset.RoleText
		if lvl > 0 || b.LinkID != 0 {
			role = nttypeset.RoleAccent
		}
		ink := b.Color
		if ink == (color.RGBA{0, 0, 0, 255}) {
			ink = color.RGBA{}
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
// and rasters view with rasterOpts.
func Raster(ctx context.Context, eng *nttypeset.Engine, doc *document.Document, width int, view nttypeset.View, rasterOpts nttypeset.RasterOptions, opts Options) (*image.RGBA, *nttypeset.Layout, error) {
	tdoc, err := Build(ctx, doc, opts)
	if err != nil {
		return nil, nil, err
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
		if s.Accent {
			role = nttypeset.RoleAccent
		}
		ink := s.Color
		if ink == (color.RGBA{0, 0, 0, 255}) {
			// Let nttypeset resolve default body foreground from its active Palette.
			ink = color.RGBA{}
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
