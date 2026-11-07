package document

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestIndexedCascadeMatchesFullScan(t *testing.T) {
	source := `<style>
 *{color:black} section{color:red} #target{color:blue}
 .a{color:green} .a{color:purple} [data-x]{background:red}
 [data-x="yes"]{background:blue} section>.a{font-weight:bold}
 .a.a{font-style:italic} .a + p{color:orange} section p.b{color:cyan}
 :is(.a,.b){padding:2px} :not(.a){margin:1px}
 section:has(>.a){border:1px solid red}
 .a::before{content:"first"} [data-x]::after{content:"last"}
 p::first-line{color:red}.escaped\:x{background:yellow}
 @media(min-width:900px){.a{color:lime!important}}
 @media(prefers-color-scheme:dark){.b{color:white}}
 </style><section><p id="target" class="a a escaped:x" data-x="yes" style="color:pink">A</p><p class="b">B</p><div data-x="no">C</div></section>`
	d, err := Parse(context.Background(), strings.NewReader(source), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, scheme := range []string{"light", "dark"} {
		for _, width := range []int{400, 1000} {
			doc, err := d.forMedia(context.Background(), width, scheme)
			if err != nil {
				t.Fatal(err)
			}
			all := make([]int, len(doc.rules))
			for i := range all {
				all[i] = i
			}
			var visit func(*node)
			visit = func(n *node) {
				if n.tag != "" {
					for _, pseudo := range []string{"", "before", "after", "first-line"} {
						got := doc.cascadeCandidates(n, pseudo, doc.candidateRules(n))
						want := doc.cascadeCandidates(n, pseudo, all)
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("%s/%d %s %s: got %v want %v", scheme, width, n.tag, pseudo, got, want)
						}
					}
				}
				for _, child := range n.children {
					visit(child)
				}
			}
			visit(doc.root)
		}
	}
}

func TestIrrelevantRulesDoNotExhaustMatchingBudget(t *testing.T) {
	var source strings.Builder
	source.WriteString("<style>")
	for i := range 1000 {
		fmt.Fprintf(&source, ".class%d{color:red}", i)
	}
	source.WriteString("</style>")
	source.WriteString(strings.Repeat(`<p class="class999">text</p>`, 2000))
	d, err := Parse(context.Background(), strings.NewReader(source.String()), Limits{Rules: 1024})
	if err != nil {
		t.Fatal(err)
	}
	n := &node{tag: "p", attrs: map[string]string{"class": "class999 class999"}}
	if got := len(d.candidateRules(n)); got != 1 {
		t.Fatalf("duplicate/unrelated candidates: %d", got)
	}
}

func TestInheritedVariablesChargedOnce(t *testing.T) {
	var css strings.Builder
	css.WriteString("<html><head><style>:root{")
	for i := range 200 {
		fmt.Fprintf(&css, "--v%d:red;", i)
	}
	css.WriteString("}p{color:var(--v0)}</style></head><body>")
	body := strings.Repeat("<p>text</p>", 1000)
	if _, err := Parse(context.Background(), strings.NewReader(css.String()+body+"</body></html>"), Limits{}); err != nil {
		t.Fatalf("shared inherited variables: %v", err)
	}
	// Local overrides copy the environment; these must still consume the budget.
	body = strings.Repeat(`<p style="--local:blue">text</p>`, 1000)
	_, err := Parse(context.Background(), strings.NewReader(css.String()+body+"</body></html>"), Limits{})
	if !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "computed declarations") {
		t.Fatalf("copied environments escaped budget: %v", err)
	}
}
