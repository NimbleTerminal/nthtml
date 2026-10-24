package document

import (
	"context"
	"strings"
	"testing"
)

func TestWhiteSpaceModes(t *testing.T) {
	for _, tc := range []struct{ mode, input, want string }{
		{"normal", " a  b\nc ", "a b c"},
		{"nowrap", " a  b\nc ", "a b c"},
		{"pre", " a  b\nc ", " a  b\nc "},
		{"pre-wrap", " a  b\nc ", " a  b\nc "},
		{"break-spaces", " a  b\nc ", " a  b\nc "},
		{"pre-line", " a  b\nc ", "a b\nc"},
	} {
		r := renderCSS(t, `<div style="white-space:`+tc.mode+`">`+tc.input+`</div>`)
		got, err := r.TTY(context.Background(), 80)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("TTY %s: %q want %q", tc.mode, got, tc.want)
		}
		// Every explicit newline in preserving modes must move raster text down.
		if tc.mode != "normal" && tc.mode != "nowrap" {
			boxes := boxesWithTag(r, "text")
			if boxes[len(boxes)-1].Bounds.Min.Y <= boxes[0].Bounds.Min.Y {
				t.Errorf("%s newline lost", tc.mode)
			}
		}
	}
}
func TestOverflowWrapping(t *testing.T) {
	for _, tc := range []struct {
		css  string
		wrap bool
	}{
		{"white-space:nowrap", false}, {"white-space:pre", false},
		{"overflow-wrap:normal", false}, {"overflow-wrap:break-word", true}, {"overflow-wrap:anywhere", true},
		{"white-space:nowrap;overflow-wrap:anywhere", false},
	} {
		source := `<div style="width:40px;` + tc.css + `">abcdefghijklmnop</div>`
		r := renderCSS(t, source)
		boxes := boxesWithTag(r, "text")
		wrapped := boxes[len(boxes)-1].Bounds.Min.Y > boxes[0].Bounds.Min.Y
		if wrapped != tc.wrap {
			t.Errorf("raster %s: wrapped=%v", tc.css, wrapped)
		}
		text, err := r.TTY(context.Background(), 4)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "\n") != tc.wrap {
			t.Errorf("TTY %s: %q", tc.css, text)
		}
	}
}

func TestInlineWhiteSpaceAndLinks(t *testing.T) {
	r := renderCSS(t, `<p>Before <a href="https://example.com" style="white-space:nowrap">one two three</a> after</p><div style="white-space:pre">  <span>kept  spaces</span>
next</div>`)
	layout, err := r.document.TTYLayout(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(layout.Text, "one two three") || !strings.Contains(layout.Text, "  kept  spaces\nnext") {
		t.Fatal(layout.Text)
	}
	if len(layout.Spans) == 0 {
		t.Fatal("link spans lost")
	}
}

func TestPreLineAndInvalidDeclarations(t *testing.T) {
	r := renderCSS(t, "<div style='white-space:pre-line;white-space:invalid'>a\n\n\nb</div>")
	text, err := r.TTY(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if text != "a\n\n\nb" {
		t.Fatalf("blank lines: %q", text)
	}
	r = renderCSS(t, "<div style='white-space:pre'>\nfirst</div>")
	if tagged(r, "text").Bounds.Min.Y == 0 {
		t.Fatal("CSS pre stripped a non-HTML-pre newline")
	}
}

func TestAnchoredPreInitialNewline(t *testing.T) {
	r := renderCSS(t, "<pre id='code'>\n  code</pre>")
	layout, err := r.document.TTYLayout(context.Background(), 40)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Text != "  code" {
		t.Fatalf("%q", layout.Text)
	}
	if _, ok := layout.Anchors["code"]; !ok {
		t.Fatal("anchor lost")
	}
}
