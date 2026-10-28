package document

import (
	"context"
	"strings"
	"testing"
)

func defaultTTY(t *testing.T, html string) string {
	t.Helper()
	d := render(t, html)
	text, err := d.TTY(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(text)
}

func TestElementDisplayDefaults(t *testing.T) {
	for _, tag := range strings.Fields("my-elem kbd abbr cite var samp dfn ins time bdi label output") {
		if got := defaultTTY(t, "<p>a<"+tag+">b</"+tag+">c</p>"); got != "abc" {
			t.Errorf("%s: %q", tag, got)
		}
	}
	for _, html := range []string{"a<my-elem style='display:block'>b</my-elem>c", "a<div>b</div>c", "a<my-elem>x<div>b</div>y</my-elem>c"} {
		if got := defaultTTY(t, html); !strings.Contains(got, "\n") {
			t.Errorf("no block break: %q", got)
		}
	}
	if got := defaultTTY(t, "a<div style='display:inline'>b</div>c"); got != "abc" {
		t.Fatal(got)
	}
}

func TestHiddenDefault(t *testing.T) {
	for _, tc := range []struct{ html, want string }{
		{"A<span hidden>HIDDEN</span>B", "AB"},
		{"A<span hidden style='display:inline'>H</span>B", "AHB"},
		{"A<span hidden='UNTIL-FOUND'>H</span>B", "AHB"},
	} {
		if got := defaultTTY(t, tc.html); got != tc.want {
			t.Errorf("%q != %q", got, tc.want)
		}
	}
}

func TestPhrasingDefaults(t *testing.T) {
	for _, tc := range []struct{ html, want string }{
		{"<q>a<q>b</q></q>", "“a‘b’”"},
		{"<ruby>漢<rp>(</rp><rt>kan</rt><rp>)</rp></ruby>", "漢(kan)"},
		{"<style>q::before{content:'['}q::after{content:']'}</style><q>x</q>", "[x]"},
	} {
		if got := defaultTTY(t, tc.html); got != tc.want {
			t.Errorf("%q != %q", got, tc.want)
		}
	}
	for _, tag := range strings.Fields("kbd samp tt code") {
		r := render(t, "<"+tag+">x</"+tag+">")
		for _, b := range r.Boxes {
			if b.Text == "x" && !b.Monospace {
				t.Errorf("%s not mono", tag)
			}
		}
	}
}
