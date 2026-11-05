package document

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

// Keep mutations small enough to explore structure rather than exhaust layout budgets.
func FuzzTTYNavigation(f *testing.F) {
	f.Add(`<p id="target">Hello <b>世界</b> é</p><a href="#target">jump</a>`, "世界", 20, -1, 100)
	f.Add("<pre>one\n\ttwo\x1b[31m</pre>", "two", 2, 0, 4)
	f.Add(`<table><tr><td rowspan="2">a</td><td>b</td></tr><tr><td>c</td></tr></table>`, "a", 80, 5, 0)
	f.Fuzz(func(t *testing.T, source, query string, width, start, end int) {
		if len(source) > 4096 || len(query) > 1024 {
			t.Skip()
		}
		ctx := context.Background()
		d, err := Parse(ctx, strings.NewReader(source), Limits{Bytes: 4096, Nodes: 100, Depth: 16, Rules: 16})
		if err != nil {
			return
		}
		r, err := d.TTYLayout(ctx, width)
		if err != nil {
			return
		}
		index := r.TextIndex()
		a, b := index.Range(start, end)
		if a < 0 || a > b || b > len(index.Text) {
			t.Fatalf("invalid selection range %d:%d", a, b)
		}
		if selected := index.Selected(start, end); selected != index.Text[a:b] || !utf8.ValidString(selected) {
			t.Fatalf("invalid selected text %q", selected)
		}
		_ = index.Regions(start, end)
		_, _ = index.SourceAt(start)
		matches := index.Search(query)
		for _, m := range matches {
			if m.Start < 0 || m.End <= m.Start || m.End > len(index.Text) {
				t.Fatalf("invalid search match %+v in %q", m, index.Text)
			}
			_ = index.Regions(m.Start, m.End)
		}
		_ = r.ANSIHighlights(matches)
		_, _ = r.Fragment(query)
	})
}

func FuzzSVGRewrite(f *testing.F) {
	for _, s := range []string{
		`<svg viewBox="0 0 10 10"><path d="M0 0L10 10"/></svg>`,
		`<svg><script>bad</script><rect width="10" height="10"/></svg>`,
		`<svg><defs><linearGradient id="a"><stop offset="0"/></linearGradient></defs><rect fill="url(#a)"/></svg>`,
		`<svg><image href="https://example.com/a.png"/></svg>`,
		`<svg><g transform="translate(1e99,NaN)"><path d="M0"/></g></svg>`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 8192 {
			t.Skip()
		}
		out, _, err := rewriteSVG(context.Background(), source)
		if err != nil {
			return
		}
		// Sanitization should be stable and must not expand attacker-controlled input.
		if len(out) > len(source) {
			t.Fatal("SVG rewrite expanded input")
		}
		again, skipped, err := rewriteSVG(context.Background(), out)
		if err != nil || again != out || len(skipped) != 0 {
			t.Fatalf("rewrite is not stable: %v, skipped %v", err, skipped)
		}
	})
}
