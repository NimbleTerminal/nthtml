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
