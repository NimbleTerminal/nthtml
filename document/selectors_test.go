package document

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"
)

func TestSelectorCascade(t *testing.T) {
	for _, tc := range []struct {
		css, inline string
		want        color.RGBA
	}{
		{`article.report p.note {color:red} article > p.note {color:blue}`, "", color.RGBA{255, 0, 0, 255}},
		{`#target {color:red} article.report > section > p.note {color:blue}`, "", color.RGBA{255, 0, 0, 255}},
		{`p.note {color:red!important} #target {color:blue}`, "color:green", color.RGBA{255, 0, 0, 255}},
		{`#target {color:red!important}`, "color:blue ! IMPORTANT;color:green", color.RGBA{0, 0, 255, 255}},
		{`p.note {color:red} p.note {color:blue}`, "", color.RGBA{0, 0, 255, 255}},
		{`article > p {color:red} article section > p {color:blue}`, "", color.RGBA{0, 0, 255, 255}},
	} {
		r := renderCSS(t, `<style>`+tc.css+`</style><article class="report"><section><p id="target" class="note extra" style="`+tc.inline+`">Hello</p></section></article>`)
		if got := tagged(r, "text").Color; got != tc.want {
			t.Errorf("%s / %s: %v", tc.css, tc.inline, got)
		}
	}
	r := renderCSS(t, `<style>p{padding:3px!important;padding-left:9px} p.note{padding-left:7px!important}</style><p class="note" style="padding:1px">Hello</p>`)
	if got := tagged(r, "text").Bounds.Min.X; got != 7 {
		t.Fatalf("important shorthand: x=%d", got)
	}
}
func TestSelectorRejectionAndSpecificity(t *testing.T) {
	for _, s := range []string{"", ".", "p >", "p >> a", "p +", "p[attr", "p[attr~=]", "p:hover", "p*", ".123", "#123", strings.Repeat(".a", 65)} {
		if _, _, ok := parseSelector(s); ok {
			t.Errorf("accepted %q", s)
		}
	}
	_, id, _ := parseSelector("#x")
	_, classes, _ := parseSelector(strings.Repeat(".a", 64))
	if id <= classes {
		t.Fatal("specificity carries between components")
	}
	// A nearer matching ancestor must not prevent a valid earlier child chain.
	r := renderCSS(t, `<style>article > section p {color:red}</style><article><section><div><section><p>Hi</p></section></div></section></article>`)
	if tagged(r, "text").Color != (color.RGBA{255, 0, 0, 255}) {
		t.Fatal("ancestor backtracking")
	}
}

func TestSelectorBudget(t *testing.T) {
	source := "<style>" + strings.Repeat(strings.Repeat(".a ", 32)+"{color:red}", 128) + "</style>" + strings.Repeat("<div class=a>x</div>", 2500)
	if _, err := Parse(context.Background(), strings.NewReader(source), Limits{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected matching budget, got %v", err)
	}
}
func TestInvalidSelectorList(t *testing.T) {
	r := renderCSS(t, `<style>p{color:blue} p, p:hover {color:red}</style><p>Hello</p>`)
	if tagged(r, "text").Color != (color.RGBA{0, 0, 255, 255}) {
		t.Fatal("invalid selector list partially applied")
	}
}

func TestAttributeSiblingAndStructuralSelectors(t *testing.T) {
	for _, tc := range []struct {
		name, html string
		want       color.RGBA
	}{
		{
			name: "attribute presence and exact",
			html: `<style>p[data-role]{color:red} p[data-role="hero"]{color:green}</style><p data-role="hero">Hi</p>`,
			want: color.RGBA{0, 128, 0, 255},
		},
		{
			name: "attribute ~, |, ^, $, * and i flag",
			html: `<style>p[data-tags~="beta"][lang|="en"][data-url^="https://"][data-url$=".org"][data-code*="xyz" i]{color:blue}</style><p data-tags="alpha beta" lang="en-US" data-url="https://example.org" data-code="aXyZb">Hi</p>`,
			want: color.RGBA{0, 0, 255, 255},
		},
		{
			name: "sibling combinators + and ~",
			html: `<style>h1 + p {color:red} h1 ~ p + p {color:green}</style><div><h1>T</h1><p>One</p><p>Two</p></div>`,
			want: color.RGBA{0, 128, 0, 255},
		},
		{
			name: "structural pseudo-classes",
			html: `<html><style>:root ul > li:nth-child(2n+1):not(:first-child):last-of-type {color:purple}</style><ul><li>1</li><li>2</li><li>3</li></ul></html>`,
			want: color.RGBA{128, 0, 128, 255},
		},
	} {
		r := renderCSS(t, tc.html)
		var lastText Box
		for _, b := range r.Boxes {
			if b.Tag == "text" {
				lastText = b
			}
		}
		if lastText.Color != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, lastText.Color, tc.want)
		}
	}
}

func TestPseudoElementsBeforeAfterAndFirstLine(t *testing.T) {
	r := renderCSS(t, `<style>p::before { content: "["; color: red; } p::after { content: "]"; color: blue; } div:first-line { color: green; }</style><p>Body</p><div>First</div>`)
	var textBoxes []Box
	var texts []string
	for _, b := range r.Boxes {
		if b.Tag == "text" {
			textBoxes = append(textBoxes, b)
			texts = append(texts, b.Text)
		}
	}
	joined := strings.Join(texts, "")
	if joined != "[Body]First" {
		t.Fatalf("unexpected generated text sequence: %q (%v)", joined, texts)
	}
	if textBoxes[0].Color != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("expected red ::before, got %v", textBoxes[0].Color)
	}
	if textBoxes[len(textBoxes)-1].Color != (color.RGBA{0, 128, 0, 255}) {
		t.Fatalf("expected green :first-line, got %v", textBoxes[len(textBoxes)-1].Color)
	}
}
