package document

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
)

func renderCSS(t *testing.T, source string) *Result {
	t.Helper()
	r, e := Render(context.Background(), strings.NewReader(source), Options{Width: 400})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func tagged(r *Result, tag string) Box {
	for _, b := range r.Boxes {
		if b.Tag == tag {
			return b
		}
	}
	return Box{}
}
func TestSpacingAndSizing(t *testing.T) {
	r := renderCSS(t, `<article style="width:80%;max-width:200px;margin:10px auto;padding:1px 2px 3px 4px;padding-left:6px"><section style="height:20px"></section></article>`)
	if b := tagged(r, "article"); b.Bounds != image.Rect(96, 10, 304, 34) {
		t.Fatalf("article: %v", b.Bounds)
	}
	if b := tagged(r, "section"); b.Bounds != image.Rect(102, 11, 302, 31) {
		t.Fatalf("section: %v", b.Bounds)
	}
	r = renderCSS(t, `<article style="width:50%;box-sizing:border-box;padding:10%;height:100px"><section style="height:50%"></section></article>`)
	if b := tagged(r, "article"); b.Bounds.Dx() != 200 || b.Bounds.Dy() != 100 {
		t.Fatalf("border box: %v", b.Bounds)
	}
	if b := tagged(r, "section"); b.Bounds.Dy() != 10 {
		t.Fatalf("percent height: %v", b.Bounds)
	}
}
func TestSpacingCascade(t *testing.T) {
	d, e := Parse(context.Background(), strings.NewReader(`<style>#x {padding-left:7px} article{padding:2px}</style><article id="x" style="margin-left:9px;margin:1px 2px 3px"></article>`), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	s := d.styled(d.root.children[1], style{size: 16, line: 22})
	if p := resolveEdges(s.padding, 400); p != [4]int{2, 2, 2, 7} {
		t.Fatal(p)
	}
	if m := resolveEdges(s.margin, 400); m != [4]int{1, 2, 3, 2} {
		t.Fatal(m)
	}
	for _, bad := range []string{"NaN", "Inf", "-1px", "1e300%", "calc(1px)"} {
		if _, ok := parseLength(bad, true, true); ok {
			t.Fatal(bad)
		}
	}
}
func TestFontVariantsAndDecorations(t *testing.T) {
	r := renderCSS(t, `<div style="font-family:'Missing', 'Literata', serif;font-size:20px;line-height:1.5"><em>Italic</em><strong><i>Both</i></strong><u>Under</u><del>Strike</del><a href="https://example.com" style="text-decoration:none">Link</a></div>`)
	found := map[string]Box{}
	for _, b := range r.Boxes {
		if b.Text != "" {
			found[b.Text] = b
		}
	}
	if !found["Italic"].Italic || !found["Both"].Bold || !found["Both"].Italic || !found["Under"].Underline || !found["Strike"].Strike || found["Link"].Underline {
		t.Fatal(found)
	}
	if found["Italic"].Bounds.Dy() != 30 {
		t.Fatal(found["Italic"])
	}
	for _, family := range []string{"serif", "sans-serif", "monospace", "Newsreader", "Source Serif 4", "Iosevka", "JetBrains Mono"} {
		renderCSS(t, `<p style="font-family:'`+family+`'"><b><i>Variants</i></b></p>`)
	}
}

type fontSource func(string, string) ([]byte, error)

func (f fontSource) Face(a, b string) ([]byte, error) { return f(a, b) }
func TestFontSourceLimits(t *testing.T) {
	for _, tc := range []struct {
		data  []byte
		limit bool
	}{{[]byte("invalid"), false}, {make([]byte, (4<<20)+1), true}} {
		_, e := Render(context.Background(), strings.NewReader(`<p style="font-family:Custom">Hello</p>`), Options{Width: 400, Fonts: fontSource(func(string, string) ([]byte, error) { return tc.data, nil })})
		if e == nil || errors.Is(e, ErrLimit) != tc.limit {
			t.Fatal(e)
		}
	}
	// Missing source faces safely fall through to the built-in Go font.
	_, e := Render(context.Background(), strings.NewReader(`<p style="font-family:Missing">Hello</p>`), Options{Width: 400, Fonts: fontSource(func(string, string) ([]byte, error) { return nil, errors.New("missing") })})
	if e != nil {
		t.Fatal(e)
	}
}

func TestFontLookupBudget(t *testing.T) {
	var source strings.Builder
	for i := 0; i < 65; i++ {
		fmt.Fprintf(&source, `<span style="font-family:Missing%d">x</span>`, i)
	}
	_, err := Render(context.Background(), strings.NewReader(source.String()), Options{Width: 400})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("expected font lookup limit, got %v", err)
	}
}
func TestIndefiniteHeightAndRightMargin(t *testing.T) {
	r := renderCSS(t, `<article style="width:100px;margin-left:auto"><section style="height:50%">hello</section></article>`)
	if b := tagged(r, "article"); b.Bounds.Min.X != 300 {
		t.Fatal(b.Bounds)
	}
	if b := tagged(r, "section"); b.Bounds.Dy() != 22 {
		t.Fatal(b.Bounds)
	}
}
