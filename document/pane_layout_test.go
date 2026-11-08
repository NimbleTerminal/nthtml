package document

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// A fixed-width side pane wrapping long text must contribute its fixed width
// to the row, not the text's max-content width, so a wrapping flex row keeps
// the pane beside the main column (GitHub repository sidebar).
func TestFixedWidthPaneStaysBesideFlexContent(t *testing.T) {
	r := renderWide(t, `<style>.row{display:flex;flex-wrap:wrap}.main{flex:1 1 0;min-width:1px;width:100%}.pane{display:flex}.side{width:120px}</style><div class="row"><div class="main"><div><a href="/readme">Readme</a> body text</div></div><div class="pane"><div class="side"><a href="/about">About</a> Visual pager for the terminal: like less, for images, PDFs, SVG, meshes, Markdown, notebooks, and spreadsheets.</div></div></div>`)
	assertPaneBesideMain(t, r)
}

// Pages that declare more than two thousand root custom properties must still
// resolve element-level variables declared further down the tree.
func TestManyRootCustomPropertiesKeepElementVariables(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<style>`)
	for rule := 0; rule < 3; rule++ {
		sb.WriteString(`:root{`)
		for i := 0; i < 800; i++ {
			fmt.Fprintf(&sb, "--token-%d-%d:#%06x;", rule, i, i)
		}
		sb.WriteString(`}`)
	}
	sb.WriteString(`.row{display:flex;flex-wrap:wrap}.main{flex:1 1 0;min-width:1px;width:100%}.pane{display:flex}.root{--pane-large:120px}.side{--size:var(--pane-large);width:var(--size)}</style>`)
	sb.WriteString(`<div class="root"><div class="row"><div class="main"><div><a href="/readme">Readme</a> body text</div></div><div class="pane"><div class="side"><a href="/about">About</a> Visual pager for the terminal: like less, for images, PDFs, SVG, meshes, Markdown, notebooks, and spreadsheets.</div></div></div></div>`)
	r := renderWide(t, sb.String())
	assertPaneBesideMain(t, r)
}

func assertPaneBesideMain(t *testing.T, r *Result) {
	t.Helper()
	if len(r.Links) != 2 {
		t.Fatalf("links: %+v", r.Links)
	}
	main, pane := r.Links[0].Regions[0], r.Links[1].Regions[0]
	if pane.Min.X <= main.Min.X || pane.Min.Y != main.Min.Y {
		t.Fatalf("pane not beside main column: main %v pane %v", main, pane)
	}
	if pane.Min.X < 600 {
		t.Fatalf("main column did not take the remaining width: pane at %v", pane)
	}
}

func renderWide(t *testing.T, source string) *Result {
	t.Helper()
	r, err := Render(context.Background(), strings.NewReader(source), Options{Width: 800, NoImage: true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A missing image laid out as a box (flex item or block) with a fixed width
// narrower than its alt text keeps the box and clips one line of alt text to
// it, rather than stacking characters and growing the line (GitHub avatar when
// the image is unavailable). Inline images keep flowing their alt text.
func TestNarrowSizedImageClipsAltTextInsteadOfStacking(t *testing.T) {
	for _, src := range []string{
		`<div style="display:flex"><img alt="NimbleTerminal" src="missing.png" style="width:24px;height:24px;display:inline-block"><b>gloss</b></div>`,
		`<img alt="NimbleTerminal" src="missing.png" style="width:24px;display:block"><p style="margin:0"><b>gloss</b></p>`,
	} {
		r := renderWide(t, src)
		gloss, alt := -1, 0
		for _, b := range r.Boxes {
			if b.Tag != "text" {
				continue
			}
			if strings.Contains(b.Text, "gloss") {
				gloss = b.Bounds.Min.Y
				continue
			}
			alt++
			if b.Bounds.Min.Y > 24 {
				t.Fatalf("%s: alt text stacked below the image box: %v %q", src, b.Bounds, b.Text)
			}
			if b.Clip.Empty() || b.Clip.Dx() > 24 {
				t.Fatalf("%s: alt text not clipped to the image box: clip %v", src, b.Clip)
			}
		}
		if alt == 0 {
			t.Fatalf("%s: alt text missing", src)
		}
		if gloss < 0 || gloss > 60 {
			t.Fatalf("%s: following text pushed down to y=%d", src, gloss)
		}
	}
}

// In a column flex container with indefinite height, `flex: 1 1 0` must not
// collapse an item below its content height (min-height: auto); following
// items start below the content instead of overlapping it.
func TestColumnFlexBasisZeroKeepsContentHeight(t *testing.T) {
	r := renderWide(t, `<style>.col{display:flex;flex-direction:column}.main{flex:1 1 0}.pane{margin-top:8px}</style><div class="col"><div class="main"><p>first</p><p>second</p><p>third</p></div><div class="pane"><a href="/about">About</a></div></div>`)
	if len(r.Links) != 1 {
		t.Fatalf("links: %+v", r.Links)
	}
	about := r.Links[0].Regions[0]
	for _, b := range r.Boxes {
		if b.Tag == "text" && b.Text == "third" && about.Min.Y < b.Bounds.Max.Y {
			t.Fatalf("pane overlaps collapsed main column: third %v about %v", b.Bounds, about)
		}
	}
}

// TestGridButtonIntrinsicWidthSumsColumns: a Primer-style button lays out
// icon, label and icon in a three-column grid; its shrink-to-fit width is the
// sum of the columns plus margins, so the label is neither clipped nor
// overlapped by the next button.
func TestGridButtonIntrinsicWidthSumsColumns(t *testing.T) {
	icon := `<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16"><path d="M0 0h16v16H0z"/></svg>`
	button := `<div style="display:inline-grid;grid-template-columns:min-content minmax(0,auto) min-content;align-items:center"><span style="margin-right:8px;display:flex">` + icon + `</span><span style="margin-right:8px;white-space:nowrap">Notifications</span><span style="display:flex">` + icon + `</span></div>`
	r := renderWide(t, `<div style="display:flex"><div style="display:flex">`+button+`</div><div style="display:flex"><div style="display:inline-grid;grid-template-columns:min-content minmax(0,auto)"><span style="margin-right:8px;display:flex">`+icon+`</span><span style="white-space:nowrap">Fork</span></div></div></div>`)
	var label, fork *Box
	var firstIcon *Box
	for i := range r.Boxes {
		b := &r.Boxes[i]
		switch {
		case b.Text == "Notifications":
			label = b
		case b.Text == "Fork":
			fork = b
		case b.Tag == "svg" && b.Bounds.Dx() == 16 && firstIcon == nil:
			firstIcon = b
		}
	}
	if label == nil || fork == nil || firstIcon == nil {
		t.Fatal("boxes missing")
	}
	if label.Bounds.Min.X < firstIcon.Bounds.Min.X+16+8 {
		t.Fatalf("label overlaps the leading icon: label x=%d icon x=%d", label.Bounds.Min.X, firstIcon.Bounds.Min.X)
	}
	// Fork's icon starts after Notifications' label, its margin and the trailing icon.
	if fork.Bounds.Min.X-16-8 < label.Bounds.Max.X+8+16 {
		t.Fatalf("next button overlaps: fork x=%d label end=%d", fork.Bounds.Min.X, label.Bounds.Max.X)
	}
	if label.Bounds.Min.Y != fork.Bounds.Min.Y {
		t.Fatalf("buttons left the row: %v vs %v", label.Bounds, fork.Bounds)
	}
}
