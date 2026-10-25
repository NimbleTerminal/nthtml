package document

import (
	"context"
	"image"
	"net/url"
	"strings"
	"unicode"
)

// Link describes one visible anchor. ID is stable across layouts of a parsed
// document. Regions are pixels in Result and terminal cells in TTYResult.
type Link struct {
	ID         int
	Href, Text string
	Regions    []image.Rectangle
}

// SafeURL allows HTTP(S), mailto, and relative references. It rejects controls,
// whitespace, backslashes, oversized URLs, and executable/local-file schemes.
// Relative references need a host-supplied base before external activation.
func SafeURL(href string) string {
	if len(href) > 2048 {
		return ""
	}
	for _, r := range href {
		if unicode.IsControl(r) || r == '\\' {
			return ""
		}
	}
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	for _, r := range href {
		if unicode.IsSpace(r) {
			return ""
		}
	}
	u, e := url.Parse(href)
	if e != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "":
	case "http", "https":
		if u.Host == "" {
			return ""
		}
	case "mailto":
		if u.Opaque == "" {
			return ""
		}
	default:
		return ""
	}
	return u.String()
}

// ResolveURL resolves a safe reference against an HTTP(S) base. An empty result
// means it cannot be opened externally. Fragment navigation belongs to the host.
func ResolveURL(base, href string) string {
	href = SafeURL(href)
	if href == "" {
		return ""
	}
	u, _ := url.Parse(href)
	if u.IsAbs() {
		return href
	}
	b, e := url.Parse(SafeURL(base))
	if e != nil || b == nil || (b.Scheme != "https" && b.Scheme != "http") {
		return ""
	}
	return SafeURL(b.ResolveReference(u).String())
}

// Links returns visible, safe anchors in document order. It does not open URLs.
func (d *Document) Links(ctx context.Context) ([]Link, error) {
	if d == nil {
		return nil, nil
	}
	var out []Link
	metadataBytes := 0
	var label func(*node, style, *strings.Builder) error
	label = func(n *node, p style, b *strings.Builder) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if n.tag == "" {
			b.WriteString(cleanText(n.text))
			return nil
		}
		s := d.styled(n, p)
		if s.display == "none" {
			return nil
		}
		if n.tag == "br" {
			b.WriteByte(' ')
			return nil
		}
		if s.display == "block" {
			b.WriteByte(' ')
		}
		if n.svg != "" {
			b.WriteString(diagramLabel(n))
			return nil
		}
		if n.tag == "img" {
			b.WriteString(cleanText(n.attrs["alt"]))
		}
		for _, c := range n.children {
			if e := label(c, s, b); e != nil {
				return e
			}
		}
		if s.display == "block" {
			b.WriteByte(' ')
		}
		return nil
	}
	var walk func(*node, style) error
	walk = func(n *node, p style) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		s := d.styled(n, p)
		if s.display == "none" {
			return nil
		}
		if n.tag == "a" {
			if href := SafeURL(n.attrs["href"]); href != "" {
				var b strings.Builder
				if e := label(n, p, &b); e != nil {
					return e
				}
				text := strings.Join(strings.Fields(b.String()), " ")
				if text == "" {
					text = href
				}
				metadataBytes += len(href) + len(text)
				if metadataBytes > maxTTYBytes {
					return limitError("link metadata")
				}
				out = append(out, Link{ID: n.id, Href: href, Text: text})
			}
		}
		for _, c := range n.children {
			if e := walk(c, s); e != nil {
				return e
			}
		}
		return nil
	}
	if e := walk(d.root, style{size: 16, line: 22}); e != nil {
		return nil, e
	}
	return out, nil
}
func (r *Result) LinkAt(p image.Point) (Link, bool) {
	if r != nil {
		if !p.In(r.Bounds()) {
			return Link{}, false
		}
		// Positioned subtrees can overlap; hit-test link boxes in paint order.
		for i := len(r.Boxes) - 1; i >= 0; i-- {
			box := r.Boxes[i]
			if box.LinkID == 0 || !p.In(box.Bounds) {
				continue
			}
			for _, link := range r.Links {
				if link.ID == box.LinkID {
					link.Regions = append([]image.Rectangle(nil), link.Regions...)
					return link, true
				}
			}
		}
		for _, l := range r.Links {
			for _, b := range l.Regions {
				if p.In(b) {
					l.Regions = append([]image.Rectangle(nil), l.Regions...)
					return l, true
				}
			}
		}
	}
	return Link{}, false
}

func appendLinkRegion(link *Link, r image.Rectangle) {
	if len(link.Regions) > 0 {
		last := &link.Regions[len(link.Regions)-1]
		if last.Min.Y == r.Min.Y && last.Max.Y == r.Max.Y && last.Max.X == r.Min.X {
			last.Max.X = r.Max.X
			return
		}
	}
	link.Regions = append(link.Regions, r)
}
