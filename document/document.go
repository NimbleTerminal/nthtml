// Package document renders a bounded subset of HTML and CSS without a browser,
// filesystem access, networking, or CGO.
package document

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Limits are resource ceilings; zero fields use DefaultLimits. Callers may
// lower any field or raise fields up to MaxLimits.
type Limits struct {
	Bytes, Nodes, Depth, Rules, Pixels, Height int
	// LayoutHeight bounds page height in CSS pixels when Options.NoImage is
	// true (where no whole-page image is allocated and Paint enforces Pixels
	// per call). Zero defaults to DefaultLimits.LayoutHeight (262,144 px).
	LayoutHeight int
}

var DefaultLimits = Limits{
	Bytes:        1 << 20,
	Nodes:        10000,
	Depth:        64,
	Rules:        256,
	Pixels:       16 << 20,
	Height:       16384,
	LayoutHeight: 262144,
}

var MaxLimits = Limits{
	Bytes:        16 << 20,
	Nodes:        200000,
	Depth:        256,
	Rules:        32768,
	Pixels:       64 << 20,
	Height:       262144,
	LayoutHeight: 1 << 20,
}

var ErrLimit = errors.New("nthtml: resource limit exceeded")

// FontFamilies maps the default document font and CSS generic font families
// (serif, sans-serif, monospace). Empty fields keep the built-in mappings
// (Go for Default, Literata for Serif, IBM Plex Sans for SansSerif, and
// IBM Plex Mono / Go Mono for Monospace).
type FontFamilies struct {
	Default   string
	Serif     string
	SansSerif string
	Monospace string
}

// Colors sets fallback canvas background, body text, and hyperlink colors used
// only when the document sets neither text nor background colors of its own.
// Zero-alpha fields keep the standard browser defaults (white canvas, black
// text, #285cba links).
type Colors struct {
	Background color.RGBA
	Text       color.RGBA
	Link       color.RGBA
}

type Options struct {
	// HoverSource is the host-hit-tested element ID; it and its ancestors match :hover.
	HoverSource string
	// Fonts defaults to ntfonts.Embedded; missing families fall back to Go fonts.
	Fonts FaceSource
	// FallbackFaces are tried in order after CSS families and before bundled
	// fallback fonts. Sources may serve Regular when a requested variant is absent.
	// Faces larger than 4 MiB are ignored and cached as misses for this render.
	// The shared 16 MiB total font budget stops further font loads on exhaustion.
	FallbackFaces []FallbackFace
	// FontFamilies overrides the default body font and generic CSS font families
	// (serif, sans-serif, monospace) where the page sets none or names generics.
	FontFamilies FontFamilies
	// Colors overrides the default canvas background, text, and link colors
	// where the page sets none.
	Colors Colors
	// ColorScheme selects "light" or "dark" for @media (prefers-color-scheme)
	// queries. Empty defaults to "light".
	ColorScheme string
	// Diagrams defaults to the pure-Go SVG/Mermaid renderer.
	Diagrams DiagramRenderer
	// DisableDiagrams keeps labels/source text without invoking a renderer.
	DisableDiagrams bool
	Width           int
	// ViewportHeight sets the CSS viewport height in pixels for vh/vmin/vmax
	// units. Zero defaults to 600.
	ViewportHeight int
	Limits         Limits
	// Images is an explicit allowlist of already decoded images, keyed by src.
	// The caller owns decoding and must keep these images immutable during Render.
	Images map[string]image.Image
	// SVGImages is an optional, separate allowlist of decoded SVG <image>
	// resources, keyed by href/xlink:href (or legacy src). Nil disables support.
	// Missing references reject the SVG. No network, file, or data URL loading
	// occurs. The default diagram renderer supports positioning, transforms,
	// opacity, and preserveAspectRatio (none, meet, and slice).
	// Custom Diagrams implementations own their resource policy instead.
	SVGImages map[string]image.Image
	// BackgroundImages is an optional allowlist of already decoded CSS
	// background-image url(...) resources, keyed by URL. Lookup falls back
	// to Images when an entry is absent here.
	BackgroundImages map[string]image.Image
	// NoImage lays the page out without painting it: Result.Image is nil,
	// and Result.Paint draws the parts wanted, at the scale wanted.
	NoImage bool
}

type node struct {
	hovered       bool
	decl          map[string]string
	firstLineDecl map[string]string
	parent        *node
	prevSib       *node
	nextSib       *node
	pseudo        string
	svg           string
	id            int
	tag, text     string
	attrs         map[string]string
	children      []*node
}

// Document is immutable and can be rendered concurrently.
type Document struct {
	hasHover        bool
	hoverSource     string
	root            *node
	rules           []rule
	ruleIndex       *ruleIndex
	layers          *cssLayers
	layerOrder      map[*cssLayer]*cssLayer
	limits          Limits
	hasAuthorColors bool
	hasMedia        bool
	cascadedWidth   int
	cascadedScheme  string
	nodeSeq         int
}

func normalizeColorScheme(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "dark") {
		return "dark"
	}
	return "light"
}

func viewportHeight(o Options) int {
	if o.ViewportHeight <= 0 {
		return 600
	}
	return min(16384, max(32, o.ViewportHeight))
}

func hasAuthorColorDecl(decl map[string]string) bool {
	for _, prop := range []string{"color", "background-color"} {
		v, ok := decl[prop]
		if !ok || v == "" {
			continue
		}
		if isCSSKeyword(v) || strings.HasPrefix(v, "\x00") || containsVar(v) {
			return true
		}
		if _, ok := cssColor(v, color.RGBA{0, 0, 0, 255}); ok {
			return true
		}
	}
	if v, ok := decl["background-image"]; ok && v != "" && !strings.EqualFold(strings.TrimSpace(v), "none") {
		return true
	}
	return false
}

func hasPresentationalColorAttr(n *node) bool {
	if n == nil {
		return false
	}
	switch n.tag {
	case "body":
		for _, attr := range []string{"bgcolor", "text", "link"} {
			if _, ok := htmlColor(n.attrs[attr]); ok {
				return true
			}
		}
	case "table", "thead", "tbody", "tfoot", "tr", "td", "th":
		if _, ok := htmlColor(n.attrs["bgcolor"]); ok {
			return true
		}
	case "font":
		if _, ok := htmlColor(n.attrs["color"]); ok {
			return true
		}
	}
	return false
}

func limits(l Limits) (Limits, error) {
	d := DefaultLimits
	m := MaxLimits
	a := []*int{&l.Bytes, &l.Nodes, &l.Depth, &l.Rules, &l.Pixels, &l.Height, &l.LayoutHeight}
	b := []int{d.Bytes, d.Nodes, d.Depth, d.Rules, d.Pixels, d.Height, d.LayoutHeight}
	c := []int{m.Bytes, m.Nodes, m.Depth, m.Rules, m.Pixels, m.Height, m.LayoutHeight}
	for i, p := range a {
		if *p == 0 {
			*p = b[i]
		}
		if *p < 1 || *p > c[i] {
			return l, limitError(fmt.Sprintf("limit %d outside 1..%d", *p, c[i]))
		}
	}
	return l, nil
}

// Parse accepts HTML fragments and tolerates unmatched tags. It uses a bounded
// token stack rather than browser tree repair. Script contents are never run.
func Parse(ctx context.Context, r io.Reader, budget Limits) (*Document, error) {
	l, err := limits(budget)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(r, int64(l.Bytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > l.Bytes {
		return nil, limitError(fmt.Sprintf("input over %d bytes", l.Bytes))
	}
	root := &node{tag: "document"}
	stack := []*node{root}
	count := 0
	z := html.NewTokenizer(strings.NewReader(string(data)))
	z.SetMaxBuf(l.Bytes)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				return nil, z.Err()
			}
			break
		}
		raw := string(z.Raw())
		t := z.Token()
		parent := stack[len(stack)-1]
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			count++
			if count > l.Nodes {
				return nil, limitError(fmt.Sprintf("more than %d nodes", l.Nodes))
			}
			if closesParagraph(t.Data) {
				// HTML's implied </p>: <p>text<h2>… ends the paragraph before
				// the heading instead of nesting the heading inside it.
				for i := len(stack) - 1; i > 0; i-- {
					if stack[i].tag == "p" {
						stack = stack[:i]
						parent = stack[len(stack)-1]
						break
					}
					if paragraphScopeBoundary(stack[i].tag) {
						break
					}
				}
			}
			n := &node{parent: parent, id: count, tag: t.Data, attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[a.Key] = a.Val
			}
			parent.children = append(parent.children, n)
			if n.tag == "svg" {
				var source strings.Builder
				source.WriteString(raw)
				depth := 0
				if tt == html.StartTagToken {
					depth = 1
				}
				for depth > 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					kind := z.Next()
					if kind == html.ErrorToken {
						return nil, fmt.Errorf("nthtml: unclosed SVG")
					}
					source.Write(z.Raw())
					_ = z.Token()
					count++
					if count > l.Nodes || source.Len() > maxSVGBytes {
						return nil, limitError("SVG input")
					}
					if kind == html.StartTagToken {
						depth++
					}
					if kind == html.EndTagToken {
						depth--
					}
					if depth+len(stack) > l.Depth {
						return nil, limitError("SVG depth")
					}
				}
				n.svg = source.String()
				continue
			}

			if tt != html.SelfClosingTagToken && !strings.Contains(" area base br col embed hr img input link meta param source track wbr ", " "+t.Data+" ") {
				if len(stack) >= l.Depth {
					return nil, limitError(fmt.Sprintf("nesting deeper than %d", l.Depth))
				}
				stack = append(stack, n)
			}
		case html.EndTagToken:
			for i := len(stack) - 1; i > 0; i-- {
				if stack[i].tag == t.Data {
					stack = stack[:i]
					break
				}
			}
		case html.TextToken:
			count++
			if count > l.Nodes {
				return nil, limitError(fmt.Sprintf("more than %d nodes", l.Nodes))
			}
			parent.children = append(parent.children, &node{parent: parent, text: t.Data})
		}
	}
	d := &Document{root: root, limits: l}
	layers := &cssLayers{}
	d.layers = layers
	var visit func(*node) error
	visit = func(n *node) error {
		if n.tag == "style" {
			var s strings.Builder
			for _, c := range n.children {
				s.WriteString(c.text)
			}
			rs, e := parseCSSLayers(s.String(), l.Rules-len(d.rules), layers)
			if e != nil {
				return e
			}
			for _, r := range rs {
				d.hasHover = d.hasHover || selectorHasHover(r.selector)
			}
			d.rules = append(d.rules, rs...)
		}
		for _, c := range n.children {
			if e := visit(c); e != nil {
				return e
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	for _, event := range layers.events {
		if len(event.media) > 0 {
			d.hasMedia = true
		}
	}
	d.nodeSeq = count
	for _, r := range d.rules {
		if len(r.media) > 0 {
			d.hasMedia = true
			break
		}
		// light-dark() and color-scheme make the cascade depend on the
		// preferred scheme the same way a media query does.
		for k, v := range r.decl {
			if k == "color-scheme" || strings.Contains(v, lightDarkFn) {
				d.hasMedia = true
				break
			}
		}
		if d.hasMedia {
			break
		}
	}
	if err := d.applyCascade(ctx, 800, "light", true); err != nil {
		return nil, err
	}
	return d, nil
}

func linkSiblings(n *node) {
	var prev *node
	for _, c := range n.children {
		if c.tag != "" && c.pseudo == "" {
			c.prevSib = prev
			if prev != nil {
				prev.nextSib = c
			}
			prev = c
		}
		linkSiblings(c)
	}
}

func (d *Document) applyCascade(ctx context.Context, width int, scheme string, checkWork bool) error {
	l := d.limits
	d.ruleIndex = indexRules(d.rules, d.root)
	count := d.nodeSeq
	scheme = normalizeColorScheme(scheme)
	d.cascadedWidth = width
	d.cascadedScheme = scheme
	d.layerOrder = d.layers.order(width, scheme)
	d.hasAuthorColors = false
	linkSiblings(d.root)
	hasBefore, hasAfter, hasFirstLine := false, false, false
	for _, r := range d.rules {
		if !r.matchesMedia(width, scheme) {
			continue
		}
		switch r.pseudo {
		case "before":
			hasBefore = true
		case "after":
			hasAfter = true
		case "first-line":
			hasFirstLine = true
		}
	}
	// Resolve author declarations once per target width. Parent-dependent
	// computed values still resolve during layout, but selector work cannot
	// multiply during reflow or nested table measurement.
	work, entries := d.ruleIndex.scanWork, 0
	var cascade func(*node, int, map[string]string, string) error
	cascade = func(n *node, depth int, parentVars map[string]string, scheme string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		vars := parentVars
		var candidates []int
		if n.tag != "" && n.pseudo == "" {
			candidates = d.candidateRules(n)
			if checkWork {
				// Charge subject-key scanning and conservative selector work for
				// every candidate, including inactive media/pseudo rules.
				work += 1 + len(n.attrs) + len(n.attrs["class"])
				if work > max(8000000, l.Nodes*800) {
					return limitError("CSS matching work")
				}
				for _, ri := range candidates {
					r := d.rules[ri]
					work += 1 + depth*d.ruleIndex.terms[ri] + len(r.decl) + len(n.attrs["class"])/16
					if work > max(8000000, l.Nodes*800) {
						return limitError("CSS matching work")
					}
				}
			}
			rawDecl := d.cascadeCandidates(n, "", candidates)
			vars = resolveElementVars(rawDecl, parentVars)
			scheme = declaredScheme(rawDecl, scheme)
			n.decl = resolveDeclaredValues(rawDecl, vars, d.usedScheme(scheme))
			entries += len(n.decl)
			// Inheritance reuses the parent's map. Charge only maps actually
			// copied for local custom-property declarations, not every reader.
			entries += newVariableEntries(rawDecl, vars)
			if !strings.Contains(" document script style head meta link title template ", " "+n.tag+" ") && (hasAuthorColorDecl(n.decl) || hasPresentationalColorAttr(n)) {
				d.hasAuthorColors = true
			}
			if hasFirstLine && n.tag != "document" {
				if fl := d.cascadeCandidates(n, "first-line", candidates); len(fl) > 0 {
					flVars := resolveElementVars(fl, vars)
					fl = resolveDeclaredValues(fl, flVars, d.usedScheme(scheme))
					n.firstLineDecl = fl
					entries += len(fl)
					if hasAuthorColorDecl(fl) {
						d.hasAuthorColors = true
					}
				}
			}
			if entries > max(100000, l.Nodes*10) {
				return limitError("computed declarations")
			}
		}
		origChildren := n.children
		for _, child := range origChildren {
			if err := cascade(child, depth+1, vars, scheme); err != nil {
				return err
			}
		}
		if n.tag != "" && n.tag != "document" && n.pseudo == "" {
			if hasBefore || defaultPseudo(n, "before") != "" {
				if bDecl := d.cascadeCandidates(n, "before", candidates); len(bDecl) > 0 {
					bVars := resolveElementVars(bDecl, vars)
					bDecl = resolveDeclaredValues(bDecl, bVars, d.usedScheme(scheme))
					if n.tag == "summary" && strings.HasPrefix(bDecl["content"], "\x01") && markerHidden(n.decl) {
						delete(bDecl, "content") // summary { list-style: none } hides the disclosure triangle, as in browsers
					}
					if text, ok := resolveContent(bDecl["content"]); ok {
						count++
						if count > l.Nodes {
							return limitError(fmt.Sprintf("more than %d nodes", l.Nodes))
						}
						bNode := &node{parent: n, id: count, tag: "span", pseudo: "before", decl: bDecl, attrs: map[string]string{}}
						if hasAuthorColorDecl(bDecl) {
							d.hasAuthorColors = true
						}
						if text != "" {
							bNode.children = []*node{{parent: bNode, text: text}}
						}
						n.children = append([]*node{bNode}, n.children...)
					}
				}
			}
			if hasAfter || defaultPseudo(n, "after") != "" {
				if aDecl := d.cascadeCandidates(n, "after", candidates); len(aDecl) > 0 {
					aVars := resolveElementVars(aDecl, vars)
					aDecl = resolveDeclaredValues(aDecl, aVars, d.usedScheme(scheme))
					if text, ok := resolveContent(aDecl["content"]); ok {
						count++
						if count > l.Nodes {
							return limitError(fmt.Sprintf("more than %d nodes", l.Nodes))
						}
						aNode := &node{parent: n, id: count, tag: "span", pseudo: "after", decl: aDecl, attrs: map[string]string{}}
						if hasAuthorColorDecl(aDecl) {
							d.hasAuthorColors = true
						}
						if text != "" {
							aNode.children = []*node{{parent: aNode, text: text}}
						}
						n.children = append(n.children, aNode)
					}
				}
			}
		}
		if text, ok := controlText(n); ok {
			count++
			if count > l.Nodes {
				return limitError(fmt.Sprintf("more than %d nodes", l.Nodes))
			}
			child := &node{parent: n, id: count, tag: "span", pseudo: "control", children: []*node{{text: text}}}
			child.children[0].parent = child
			n.children = append(n.children, child)
		}
		return nil
	}
	hasHTML := false
	for _, c := range d.root.children {
		if c.tag == "html" {
			hasHTML = true
			break
		}
	}
	var rootVars map[string]string
	if !hasHTML {
		rootVars = resolveElementVars(d.cascade(&node{tag: "html", parent: d.root}), nil)
	}
	return cascade(d.root, 0, rootVars, "")
}

func (d *Document) forWidth(ctx context.Context, width int) (*Document, error) {
	if d == nil {
		return nil, nil
	}
	return d.forMedia(ctx, width, d.cascadedScheme)
}

func (d *Document) forMedia(ctx context.Context, width int, scheme string) (*Document, error) {
	if d == nil {
		return nil, nil
	}
	return d.forInteraction(ctx, width, scheme, d.hoverSource)
}

func (d *Document) forInteraction(ctx context.Context, width int, scheme, hover string) (*Document, error) {
	scheme = normalizeColorScheme(scheme)
	if d == nil || (d.hoverSource == hover && (!d.hasMedia || (d.cascadedWidth == width && normalizeColorScheme(d.cascadedScheme) == scheme))) {
		return d, nil
	}
	var clone func(*node, *node) *node
	clone = func(n, parent *node) *node {
		if n == nil {
			return nil
		}
		c := &node{
			parent: parent,
			id:     n.id,
			tag:    n.tag,
			text:   n.text,
			svg:    n.svg,
			attrs:  n.attrs,
		}
		for _, ch := range n.children {
			if ch.pseudo != "" {
				continue
			}
			c.children = append(c.children, clone(ch, c))
		}
		return c
	}
	cp := &Document{
		root:   clone(d.root, nil),
		layers: d.layers, hoverSource: hover,
		rules:    d.rules,
		limits:   d.limits,
		hasMedia: d.hasMedia,
		nodeSeq:  d.nodeSeq,
	}
	if hover != "" {
		var mark func(*node)
		mark = func(n *node) {
			if n.attrs["id"] == hover {
				for p := n; p != nil; p = p.parent {
					p.hovered = true
				}
			}
			for _, c := range n.children {
				mark(c)
			}
		}
		mark(cp.root)
	}
	if err := cp.applyCascade(ctx, width, scheme, true); err != nil {
		return nil, err
	}
	return cp, nil
}

// Render is the convenience parse + layout + raster entry point.
func Render(ctx context.Context, r io.Reader, o Options) (*Result, error) {
	d, e := Parse(ctx, r, o.Limits)
	if e != nil {
		return nil, e
	}
	return d.Render(ctx, o)
}
func limitError(what string) error { return fmt.Errorf("%w: %s", ErrLimit, what) }

// declaredScheme is the color scheme an element's color-scheme declaration
// pins, "light" or "dark", else the inherited one. "light dark", "normal" and
// unknown values leave the choice to the rendering preference.
func declaredScheme(decl map[string]string, inherited string) string {
	v, ok := decl["color-scheme"]
	if !ok {
		return inherited
	}
	light, dark := false, false
	for _, f := range strings.Fields(strings.ToLower(v)) {
		switch f {
		case "light":
			light = true
		case "dark":
			dark = true
		case "inherit":
			return inherited
		case "initial", "normal":
			return ""
		}
	}
	if light != dark {
		if light {
			return "light"
		}
		return "dark"
	}
	return ""
}

// usedScheme is the scheme light-dark() picks from: the pinned one, else the
// preference the document was cascaded for.
func (d *Document) usedScheme(pinned string) string {
	if pinned == "light" || pinned == "dark" {
		return pinned
	}
	return normalizeColorScheme(d.cascadedScheme)
}

const lightDarkFn = "light-dark("

// resolveLightDarkValue replaces light-dark(light, dark) in a declared value
// with the argument for the used scheme, innermost call first, so tokens such
// as --ink: light-dark(#1c2421, #e9eee9) render the page's dark theme.
func resolveLightDarkValue(v, scheme string) string {
	for pass := 0; pass < 8 && strings.Contains(v, lightDarkFn); pass++ {
		i := strings.LastIndex(v, lightDarkFn)
		end := closingParen(v, i+len(lightDarkFn)-1)
		if end < 0 {
			break
		}
		a, b, ok := splitVarArgs(v[i+len(lightDarkFn) : end])
		if !ok {
			break
		}
		pick := strings.TrimSpace(a)
		if scheme == "dark" {
			pick = strings.TrimSpace(b)
		}
		v = v[:i] + pick + v[end+1:]
	}
	return v
}

// markerHidden reports a summary whose own list-style-type is none.
func markerHidden(decl map[string]string) bool {
	v, _ := importance(decl["list-style-type"])
	return strings.EqualFold(strings.TrimSpace(v), "none")
}

// closingParen returns the index of the parenthesis closing the one at open,
// skipping quoted text, or -1.
func closingParen(s string, open int) int {
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// closesParagraph reports a start tag that ends an open <p>, the block-level
// tags of HTML's "close a p element" rule.
func closesParagraph(tag string) bool {
	return strings.Contains(" address article aside blockquote details dialog div dl fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header hgroup hr main menu nav ol p pre search section table ul ", " "+tag+" ")
}

// paragraphScopeBoundary bounds the search for an open <p> to its button
// scope, so a paragraph outside a table cell or button is left alone.
func paragraphScopeBoundary(tag string) bool {
	return strings.Contains(" html table td th caption applet marquee object template button ", " "+tag+" ")
}
