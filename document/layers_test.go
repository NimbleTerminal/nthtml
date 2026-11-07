package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestLayerCascadePrecedence(t *testing.T) {
	tests := []struct{ css, inline, want string }{
		{`@layer first,second; @layer second {p{color:blue}} @layer first {#target{color:red}}`, "", "blue"},
		{`@layer first,second; @layer second {p{color:blue!important}} @layer first {p{color:red!important}}`, "", "red"},
		{`p{color:green}@layer first{#target{color:red}}`, "", "green"},
		{`p{color:green!important}@layer first{p{color:red!important}}`, "", "red"},
		{`@layer first{p{color:red!important}}`, `color:blue!important`, "blue"},
		{`@layer first{p{color:red!important}}`, `color:blue`, "red"},
		{`@layer first{#target{color:red}}`, `color:blue`, "blue"},
		{`@layer first{p{color:red} @layer child{#target{color:blue}}}`, "", "red"},
		{`@layer first{p{color:red!important} @layer child{p{color:blue!important}}}`, "", "blue"},
		{`@layer{#target{color:red}} @layer{p{color:blue}}`, "", "blue"},
		{`@layer first.child,first.other; @layer first.other{p{color:blue}} @layer first.child{p{color:red}}`, "", "blue"},
	}
	for _, tt := range tests {
		t.Run(tt.css+tt.inline, func(t *testing.T) {
			d := parseFeature(t, `<style>`+tt.css+`</style><p id="target" style="`+tt.inline+`">text</p>`)
			var visit func(*node)
			visit = func(n *node) {
				if n.attrs["id"] == "target" {
					if got := d.cascade(n)["color"]; got != tt.want {
						t.Fatalf("color=%s want %s", got, tt.want)
					}
				}
				for _, c := range n.children {
					visit(c)
				}
			}
			visit(d.root)
		})
	}
}

func TestLayerOrderAcrossSheetsAndMedia(t *testing.T) {
	d := parseFeature(t, `<style>@media(min-width:600px){@layer a{p{color:red}}}</style><style>@layer b,a; @layer b{p{color:blue}} @layer a{p{color:red}}</style><p id="target">text</p>`)
	for _, tt := range []struct {
		width int
		want  string
	}{{800, "blue"}, {400, "red"}, {800, "blue"}} {
		doc, err := d.forMedia(context.Background(), tt.width, "light")
		if err != nil {
			t.Fatal(err)
		}
		var visit func(*node)
		visit = func(n *node) {
			if n.attrs["id"] == "target" {
				if got := doc.cascade(n)["color"]; got != tt.want {
					t.Fatalf("width=%d color=%s want %s", tt.width, got, tt.want)
				}
			}
			for _, c := range n.children {
				visit(c)
			}
		}
		visit(doc.root)
	}
}

func TestLayerBudgets(t *testing.T) {
	var css strings.Builder
	for i := 0; i <= maxCascadeLayers; i++ {
		fmt.Fprintf(&css, "@layer l%d;", i)
	}
	if _, err := parseCSS(css.String(), 1000); !errors.Is(err, ErrLimit) {
		t.Fatalf("layer limit: %v", err)
	}
	if _, err := parseCSS(`@layer a{p{color:red}b{color:blue}}`, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("rule limit: %v", err)
	}
	if _, err := parseCSS(strings.Repeat(`@layer a;`, 4097), 1000); !errors.Is(err, ErrLimit) {
		t.Fatalf("declaration limit: %v", err)
	}
}

func TestLayeredComponentLayout(t *testing.T) {
	r := renderCSS(t, `<style>@layer base,components;@layer components{.nav{display:flex;gap:8px;list-style:none;margin:0;padding:0}.sr{position:absolute;width:1px;height:1px;overflow:hidden}.nav a{display:block}}</style><h2 class="sr">Repository navigation</h2><ul class="nav"><li><a href="/code">Code</a></li><li><a href="/issues">Issues</a></li></ul>`)
	if len(r.Links) != 2 {
		t.Fatalf("links: %+v", r.Links)
	}
	a, b := r.Links[0].Regions[0], r.Links[1].Regions[0]
	if b.Min.X <= a.Min.X || b.Min.Y != a.Min.Y {
		t.Fatalf("layered navigation did not form a row: %v %v", a, b)
	}
}

func TestDeepFlexIntrinsicMeasurementStaysBounded(t *testing.T) {
	src := `<style>.row{display:flex;min-width:0}</style>` + strings.Repeat(`<div class="row">`, 16) + `<span>` + strings.Repeat("readable words ", 400) + `</span>` + strings.Repeat(`</div>`, 16)
	d := parseFeature(t, src)
	for _, width := range []int{400, 800} {
		r, err := d.Render(context.Background(), Options{Width: width, NoImage: true})
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		if !strings.Contains(r.TextIndex().Text, "readable") {
			t.Fatal("lost flex text")
		}
	}
}
