package conformance

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
)

func TestXHTMLInput(t *testing.T) {
	input := []byte(`<?xml version="1.0"?><!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "https://invalid.example/never-fetch.dtd"><html xmlns="http://www.w3.org/1999/xhtml"><head><style><![CDATA[p{color:green}]]></style></head><body><p id="a">A&nbsp;&amp;&lt;B</p><div/></body></html>`)
	output, err := fixtureInput(context.Background(), "test.xht", input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"p{color:green}", "A\u00a0&amp;&lt;B", "<div></div>"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("missing %q: %s", want, output)
		}
	}
	if bytes.Contains(output, []byte("CDATA")) || bytes.Contains(output, []byte("DOCTYPE")) {
		t.Fatal(string(output))
	}
	r, err := document.Render(context.Background(), bytes.NewReader(output), document.Options{Width: 200})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range r.Boxes {
		if b.Text != "" {
			found = true
			if b.Color.G != 128 || b.Color.R != 0 {
				t.Fatal("CDATA stylesheet lost", b)
			}
		}
	}
	if !found {
		t.Fatal("missing text")
	}
	unchanged, err := fixtureInput(context.Background(), "test.html", input)
	if err != nil || !bytes.Equal(unchanged, input) {
		t.Fatal("HTML input changed")
	}
}
func TestXHTMLRejectsUnsupportedXML(t *testing.T) {
	for _, input := range []string{
		`<html><p></html>`, `<html/><html/>`, `<html>&custom;</html>`,
		`<!DOCTYPE html [<!ENTITY custom "test">]><html>&custom;</html>`,
		`<html xmlns="urn:foreign"/>`, `<html ID="a"/>`, `<?xml-stylesheet href="https://invalid.example/a.css"?><html/>`, `<html><P/></html>`,
		`<html><style><![CDATA[</style><script>bad</script>]]></style></html>`,
	} {
		if _, err := fixtureInput(context.Background(), "test.xht", []byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixtureInput(ctx, "test.xht", []byte(`<html/>`)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := fixtureInput(context.Background(), "test.xht", []byte(strings.Repeat("x", (1<<20)+1))); !errors.Is(err, document.ErrLimit) {
		t.Fatal(err)
	}
}
