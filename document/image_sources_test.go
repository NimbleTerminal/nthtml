package document

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestImageSources(t *testing.T) {
	d, err := Parse(context.Background(), strings.NewReader(`<p><img src="a.png"><img src=""><img src="b.png" style="display:none"></p><table><tr><td><img src="a.png"><img alt="none"></td></tr></table>`), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := d.ImageSources(); !slices.Equal(got, []string{"a.png", "b.png"}) {
		t.Fatalf("ImageSources = %q", got)
	}
}
