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

func TestDisclosureDefaults(t *testing.T) {
	for _, tc := range []struct{ html, want string }{
		{"<details><summary>S</summary>BODY</details>", "▸ S"},
		{"<details>BODY</details>", "▸ Details"},
		{"<details open><summary>S</summary>BODY</details>", "▾ S\nBODY"},
		{"<dialog>X</dialog>", ""},
		{"<dialog open>X</dialog>", "X"},
	} {
		if got := defaultTTY(t, tc.html); got != tc.want {
			t.Errorf("%q != %q", got, tc.want)
		}
	}
}

func TestControlDefaults(t *testing.T) {
	if got := defaultTTY(t, "<p>x<input value='V'>y<my-elem>CUSTOM</my-elem>z</p>"); got != "xVyCUSTOMz" {
		t.Fatal(got)
	}
	for _, tc := range []struct{ html, want string }{
		{"<input type='password' value='abc'>", "•••"},
		{"<input type='hidden' value='z'>", ""},
		{"<select><option>A</option><option selected>B</option></select>", "B ▾"},
		{"<input type='submit'><input type='reset'>", "SubmitReset"},
	} {
		if got := defaultTTY(t, tc.html); got != tc.want {
			t.Errorf("%q != %q", got, tc.want)
		}
	}
	r := render(t, "<input id='q' value='hello'>")
	var control, text Box
	for _, b := range r.Boxes {
		if b.Tag == "input" {
			control = b
		}
		if b.Text == "hello" {
			text = b
		}
	}
	if control.SourceID != "q" || control.Bounds.Dx() <= text.Bounds.Dx() || !text.Bounds.In(control.Bounds) {
		t.Fatalf("control=%+v text=%+v", control, text)
	}
	ph := render(t, "<input placeholder='ph'>")
	for _, b := range ph.Boxes {
		if b.Text == "ph" && b.Color == text.Color {
			t.Fatal("placeholder not grey")
		}
	}
	for _, typ := range []string{"checkbox", "radio"} {
		a := render(t, "<input type='"+typ+"'>")
		b := render(t, "<input type='"+typ+"' checked>")
		if string(a.Image.Pix) == string(b.Image.Pix) {
			t.Errorf("%s checked pixels identical", typ)
		}
	}
	d, err := Parse(context.Background(), strings.NewReader("<form><input name='q'></form>"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := d.Sources(context.Background())
	if err != nil || len(scopes) != 1 || scopes[0].Tag != "input" {
		t.Fatalf("%+v %v", scopes, err)
	}
	rr, err := d.Render(context.Background(), Options{Width: 400})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range rr.Boxes {
		if b.Tag == "input" && b.SourceID == scopes[0].ID && !b.Bounds.Empty() {
			found = true
		}
	}
	if !found {
		t.Fatal("missing control source box")
	}
}

func TestControlAuthorStyles(t *testing.T) {
	r := render(t, "<textarea style='width:80px;height:30px;border:0;padding:0;background:red;font-size:12px'>hello</textarea>")
	for _, b := range r.Boxes {
		if b.Tag == "textarea" && (b.Bounds.Dx() != 80 || b.Bounds.Dy() != 30) {
			t.Fatalf("%+v", b)
		}
		if b.Tag == "border" {
			t.Fatal("author border:0 ignored")
		}
	}
	r = render(t, "<fieldset><legend>Name</legend><input></fieldset>")
	var field, legend Box
	for _, b := range r.Boxes {
		if b.Tag == "fieldset" {
			field = b
		}
		if b.Tag == "legend" {
			legend = b
		}
	}
	if legend.Bounds.Min.Y > field.Bounds.Min.Y || legend.Bounds.Max.Y < field.Bounds.Min.Y {
		t.Fatalf("legend=%v field=%v", legend.Bounds, field.Bounds)
	}
}

func TestReplacedDefaults(t *testing.T) {
	for _, html := range []string{
		"<iframe width='200' height='100'>FALLBACK</iframe>",
		"<video style='width:200px;height:100px'>FALLBACK</video>",
		"<canvas width='200' height='100'>FALLBACK</canvas>",
		"<object width='200' height='100'>FALLBACK</object>",
	} {
		r := render(t, html)
		found := false
		for _, b := range r.Boxes {
			if strings.Contains(b.Text, "FALLBACK") {
				t.Fatal("fallback rendered")
			}
			if b.Bounds.Dx() == 200 && b.Bounds.Dy() == 100 {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing 200x100 placeholder: %s", html)
		}
		if got := defaultTTY(t, html); got != "" {
			t.Fatal(got)
		}
	}
	if got := defaultTTY(t, "a<object>FALLBACK</object>b"); got != "aFALLBACKb" {
		t.Fatal(got)
	}
	if got := defaultTTY(t, "a<audio>FALLBACK</audio>b"); got != "ab" {
		t.Fatal(got)
	}
	for _, tc := range []struct {
		html, tag string
		w, h      int
	}{
		{"<iframe></iframe>", "iframe", 300, 150},
		{"<audio controls></audio>", "audio", 300, 32},
	} {
		r := render(t, tc.html)
		found := false
		for _, b := range r.Boxes {
			if b.Tag == tc.tag && b.Bounds.Dx() == tc.w && b.Bounds.Dy() == tc.h {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing default %s size", tc.tag)
		}
	}
}

func TestControlRasterContentAndBars(t *testing.T) {
	r := render(t, "<input type='password' value='abc'><input type='hidden' value='z'><select><option>A</option><option selected>B</option></select>")
	var text strings.Builder
	for _, b := range r.Boxes {
		text.WriteString(b.Text)
	}
	if got := text.String(); strings.Contains(got, "abc") || strings.Contains(got, "z") || strings.Contains(got, "A") || !strings.Contains(got, "•••") || !strings.Contains(got, "B") {
		t.Fatal(got)
	}
	r = render(t, "<progress value='0.5'>FALLBACK</progress><meter min='2' max='6' value='3'>FALLBACK</meter>")
	widths := []int{}
	for _, b := range r.Boxes {
		if strings.Contains(b.Text, "FALLBACK") {
			t.Fatal("bar fallback rendered")
		}
		if b.Tag == "control-fill" {
			widths = append(widths, b.Bounds.Dx())
		}
	}
	if len(widths) != 2 || widths[0] != 80 || widths[1] != 40 {
		t.Fatalf("bar fills: %v", widths)
	}
}

func TestAnonymousInlineBlockSiblingsRaster(t *testing.T) {
	r := render(t, "a<my-elem>x<div>b</div>y</my-elem>c")
	ys := map[string]int{}
	for _, b := range r.Boxes {
		if b.Text != "" {
			ys[b.Text] = b.Bounds.Min.Y
		}
	}
	if ys["a"] != ys["x"] || ys["b"] <= ys["a"] || ys["y"] <= ys["b"] || ys["c"] != ys["y"] {
		t.Fatalf("flow: %v", ys)
	}
}

func TestClosedDetailsLayoutModes(t *testing.T) {
	for _, display := range []string{"block", "flex", "grid", "inline"} {
		r := render(t, "<details style='display:"+display+"'><summary>S</summary><div>BODY</div></details>")
		for _, b := range r.Boxes {
			if strings.Contains(b.Text, "BODY") {
				t.Errorf("%s rendered closed body", display)
			}
		}
	}
	if got := defaultTTY(t, "<details open>BODY</details>"); got != "▾ Details\nBODY" {
		t.Fatal(got)
	}
}
