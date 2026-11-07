package document

import (
	"image"
	"strings"
	"testing"
)

func TestHiddenPositionedLinksDoNotInterceptHits(t *testing.T) {
	r := renderCSS(t, `<a id="visible" href="/visible" style="display:block;width:100px;height:30px">visible</a><div style="position:fixed;top:0;left:0;visibility:hidden"><a id="hidden" href="/hidden" style="display:block;width:100px;height:30px">secret</a></div>`)
	if got, ok := r.LinkAt(image.Pt(5, 5)); !ok || got.Href != "/visible" {
		t.Fatalf("hit hidden overlay: %+v %v", got, ok)
	}
	if got, _ := r.SourceAt(image.Pt(5, 5)); got != "visible" {
		t.Fatalf("source hit: %q", got)
	}
	for _, link := range r.Links {
		if link.Href == "/hidden" {
			t.Fatal("hidden link remains keyboard focusable")
		}
	}
	if strings.Contains(r.TextIndex().Text, "secret") {
		t.Fatal("hidden text remains searchable")
	}
}

func TestVisibleDescendantAndTransparentLinkRemainInteractive(t *testing.T) {
	r := renderCSS(t, `<a href="/restored" style="visibility:hidden"><span style="visibility:visible">restored</span></a><a href="/transparent" style="display:block;opacity:0">transparent</a>`)
	for _, href := range []string{"/restored", "/transparent"} {
		found := false
		for _, link := range r.Links {
			if link.Href == href {
				found = true
			}
		}
		if !found {
			t.Fatalf("lost interactive link %s", href)
		}
	}
}

func TestClippedLinksHaveClippedHitRegions(t *testing.T) {
	r := renderCSS(t, `<div style="width:30px;height:20px;overflow:hidden"><a id="clipped" href="/clipped" style="display:block;width:100px;height:60px">clipped</a></div>`)
	if _, ok := r.LinkAt(image.Pt(5, 5)); !ok {
		t.Fatal("visible portion not clickable")
	}
	for _, p := range []image.Point{image.Pt(40, 5), image.Pt(5, 30)} {
		if link, ok := r.LinkAt(p); ok {
			t.Fatalf("clipped link hit at %v: %+v", p, link)
		}
	}
	for _, link := range r.Links {
		for _, region := range link.Regions {
			if !region.In(image.Rect(0, 0, 30, 20)) {
				t.Fatalf("unclipped focus region %v", region)
			}
		}
	}
}
