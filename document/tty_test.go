package document

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

func TestTTYFlow(t *testing.T) {
	d, e := Parse(context.Background(), strings.NewReader(`<style>.secret{display:none}</style><h1>Title</h1><p>Hello <strong>styled</strong> world.<br>Next line.</p><p class="secret">hidden</p><script>bad()</script><table><tr><th>Name</th><th>Value</th></tr><tr><td>Alpha</td><td>42</td></tr></table><img alt="diagram">`), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	text, e := d.TTY(context.Background(), 80)
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"Title", "Hello styled world.", "Next line.", "│ Name  │ Value │", "│ Alpha │ 42    │", "[image: diagram]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	for _, hidden := range []string{"hidden", "bad()", "display:none"} {
		if strings.Contains(text, hidden) {
			t.Fatalf("hidden text: %q", text)
		}
	}
}
func TestTTYWrappingAndControls(t *testing.T) {
	r := render(t, "<p>Unicode 世界 and averylongwordwithoutspaces &amp; text.</p><p>\x1b]52;c;payload\x07 End</p>")
	text, e := r.TTY(context.Background(), 12)
	if e != nil {
		t.Fatal(e)
	}
	if strings.ContainsAny(text, "\x1b\x07") {
		t.Fatal("terminal controls leaked")
	}
	for _, line := range strings.Split(text, "\n") {
		if ansi.StringWidth(line) > 12 {
			t.Fatalf("wide line: %q", line)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := r.TTY(ctx, 20); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestTTYTableBordersAndAlignment(t *testing.T) {
	d, e := Parse(context.Background(), strings.NewReader(`<table><thead><tr><th>Item</th><th style="text-align:right">Cost</th></tr></thead><tbody><tr><td>Tea</td><td style="text-align:right">$3</td></tr><tr><td>Cake</td><td style="text-align:center">$12</td></tr></tbody></table>`), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	got, e := d.TTY(context.Background(), 40)
	if e != nil {
		t.Fatal(e)
	}
	want := "┌──────┬──────┐\n│ Item │ Cost │\n╞══════╪══════╡\n│ Tea  │   $3 │\n├──────┼──────┤\n│ Cake │ $12  │\n└──────┴──────┘"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTTYTablesFitAndKeepContent(t *testing.T) {
	src := `<table><caption>Inventory</caption><tr><th>Item</th><th>Notes</th></tr><tr><td>世界</td><td>Wrap these words across lines</td></tr><tr><td>Solo</td></tr><tr style="display:none"><td>HIDDEN</td></tr></table>`
	d, e := Parse(context.Background(), strings.NewReader(src), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	for _, width := range []int{2, 8, 11, 16, 30, 80} {
		text, e := d.TTY(context.Background(), width)
		if e != nil {
			t.Fatal(e)
		}
		for _, line := range strings.Split(text, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow: %q", width, line)
			}
		}
		if strings.Contains(text, "HIDDEN") {
			t.Fatal("hidden row leaked")
		}
		// Ignore layout whitespace and border glyphs to check content survives wrapping.
		content := text
		if strings.Contains(text, "┌") {
			var cols [2]strings.Builder
			for _, line := range strings.Split(text, "\n") {
				if strings.HasPrefix(line, "│") {
					cells := strings.Split(line, "│")
					for j := 0; j < 2; j++ {
						cols[j].WriteString(cells[j+1])
					}
				}
			}
			content = "Inventory " + cols[0].String() + " " + cols[1].String()
		}
		flat := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) || (r >= '─' && r <= '╿') {
				return -1
			}
			return r
		}, content)
		for _, word := range []string{"Inventory", "世界", "Wrapthesewordsacrosslines", "Solo"} {
			if !strings.Contains(flat, word) {
				t.Fatalf("width %d lost %q:\n%s", width, word, text)
			}
		}
		if width >= 11 && !strings.Contains(text, "┌") {
			t.Fatalf("missing table border at width %d", width)
		}
		if width < 11 && strings.Contains(text, "┌") {
			t.Fatal("narrow fallback not used")
		}
	}
}

func TestTTYTableNestedAndHiddenSections(t *testing.T) {
	d, e := Parse(context.Background(), strings.NewReader(`<p>Before</p><table><tbody style="display:none"><tr><td>secret</td></tr></tbody><tr><td><p>Outer</p><table><tr><td>Nested</td><td>Value</td></tr></table></td><td>Other</td></tr></table><p>After</p>`), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	text, e := d.TTY(context.Background(), 50)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Count(text, "┌") != 1 || strings.Count(text, "Nested") != 1 || strings.Contains(text, "secret") {
		t.Fatal(text)
	}
	if !strings.HasPrefix(text, "Before\n\n┌") || !strings.HasSuffix(text, "\n\nAfter") {
		t.Fatal("table/prose boundaries lost", text)
	}
}

func TestTTYTableOutputBudget(t *testing.T) {
	src := "<table><tr><th>" + strings.Repeat("w", 995) + "</th></tr>" + strings.Repeat("<tr><td>x</td></tr>", 1500) + "</table>"
	d, e := Parse(context.Background(), strings.NewReader(src), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := d.TTY(context.Background(), 1000); !errors.Is(e, ErrLimit) {
		t.Fatalf("expected bounded table output, got %v", e)
	}
}
