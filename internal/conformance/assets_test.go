package conformance

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"nimbleterminal.dev/nthtml/document"
)

func TestPinnedImageAssets(t *testing.T) {
	const name = "css/CSS2/normal-flow/max-height-025-ref.xht"
	const asset = "css/CSS2/normal-flow/support/black96x96.png"
	const source = `<html><body><img src="support/black96x96.png"/></body></html>`
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	d, err := document.Parse(context.Background(), strings.NewReader(source), document.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := references(name, []byte(source)); err == nil {
		t.Fatal("unlisted image accepted by preflight")
	}
	if _, err := references(name, []byte(source), []string{asset}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.images(context.Background(), name, d, nil); err == nil {
		t.Fatal("unlisted image loaded")
	}
	images, err := s.images(context.Background(), name, d, []string{asset})
	if err != nil {
		t.Fatal(err)
	}
	if images["support/black96x96.png"].Bounds().Dx() != 96 {
		t.Fatal("wrong decoded asset")
	}
	// Missing/corrupt assets must fail, never silently render alt text.
	files := fstest.MapFS{asset: &fstest.MapFile{Data: []byte("not a PNG")}}
	s.Files = files
	if _, err := s.images(context.Background(), name, d, []string{asset}); err == nil {
		t.Fatal("corrupt image accepted")
	}
	delete(files, asset)
	if _, err := s.images(context.Background(), name, d, []string{asset}); err == nil {
		t.Fatal("missing image accepted")
	}
	original, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(original.Files, asset)
	if err != nil || len(data) == 0 {
		t.Fatal(err)
	}
}
func TestAssetPathsRejectNetworkAndEscape(t *testing.T) {
	for _, src := range []string{"https://example.com/a.png", "//example.com/a.png", "data:image/png;base64,a", "../../../escape.png", "a.png?x", "a.png#x", "..%2f..%2f..%2fescape.png"} {
		if _, err := fixturePath("css/ref.html", src); err == nil {
			t.Errorf("accepted %q", src)
		}
	}
}
