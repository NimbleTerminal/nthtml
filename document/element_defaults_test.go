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
