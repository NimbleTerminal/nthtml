// Package document renders a bounded subset of HTML and CSS without a browser,
// filesystem access, networking, or CGO.
package document

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Limits are hard ceilings; zero fields use defaults. Values above defaults are
// rejected so callers cannot accidentally disable the resource budget.
type Limits struct{ Bytes, Nodes, Depth, Rules, Pixels, Height int }

var DefaultLimits = Limits{1 << 20, 10000, 64, 256, 16 << 20, 16384}
var ErrLimit = errors.New("nthtml: resource limit exceeded")

type Options struct {
	// Fonts defaults to ntfonts.Embedded; missing families fall back to Go fonts.
	Fonts FaceSource
	// Diagrams defaults to the pure-Go SVG/Mermaid renderer.
	Diagrams DiagramRenderer
	// DisableDiagrams keeps labels/source text without invoking a renderer.
	DisableDiagrams bool
	Width           int
	Limits          Limits
	// Images is an explicit allowlist of already decoded images, keyed by src.
	// The caller owns decoding and must keep these images immutable during Render.
	Images map[string]image.Image
}

type node struct {
	svg       string
	id        int
	tag, text string
	attrs     map[string]string
	children  []*node
}

// Document is immutable and can be rendered concurrently.
type Document struct {
	root   *node
	rules  []rule
	limits Limits
}

func limits(l Limits) (Limits, error) {
	d := Limits{1 << 20, 10000, 64, 256, 16 << 20, 16384}
	a := []*int{&l.Bytes, &l.Nodes, &l.Depth, &l.Rules, &l.Pixels, &l.Height}
	b := []int{d.Bytes, d.Nodes, d.Depth, d.Rules, d.Pixels, d.Height}
	for i, p := range a {
		if *p == 0 {
			*p = b[i]
		}
		if *p < 1 || *p > b[i] {
			return l, limitError(fmt.Sprintf("limit %d outside 1..%d", *p, b[i]))
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
			n := &node{id: count, tag: t.Data, attrs: map[string]string{}}
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
			parent.children = append(parent.children, &node{text: t.Data})
		}
	}
	d := &Document{root: root, limits: l}
	var visit func(*node) error
	visit = func(n *node) error {
		if n.tag == "style" {
			var s strings.Builder
			for _, c := range n.children {
				s.WriteString(c.text)
			}
			rs, e := parseCSS(s.String(), l.Rules-len(d.rules))
			if e != nil {
				return e
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
	return d, nil
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
