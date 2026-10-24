package document

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const spanExample = `<style>table{border-collapse:collapse}td,th{border:1px solid blue;padding:3px}</style><table><tr><th colspan="3">Report</th></tr><tbody><tr><td rowspan="2" id="span"><a href="https://example.com">Team</a></td><td>A</td><td>B</td></tr><tr><td colspan="2">Total</td></tr></tbody></table>`

func TestTableSpanGeometry(t *testing.T) {
	r := renderCSS(t, spanExample)
	cells := boxesWithTag(r, "td")
	heads := boxesWithTag(r, "th")
	rows := boxesWithTag(r, "tr")
	if len(cells) != 4 || len(heads) != 1 || len(rows) != 3 {
		t.Fatalf("missing cells")
	}
	if cells[0].Bounds.Min.Y != rows[1].Bounds.Min.Y || cells[0].Bounds.Max.Y != rows[2].Bounds.Max.Y {
		t.Fatal("rowspan", cells[0])
	}
	if cells[3].Bounds.Min.X != cells[1].Bounds.Min.X || cells[3].Bounds.Max.X != cells[2].Bounds.Max.X {
		t.Fatal("colspan", cells)
	}
	if heads[0].Bounds.Min.X != cells[0].Bounds.Min.X || heads[0].Bounds.Max.X != cells[2].Bounds.Max.X {
		t.Fatal("header colspan")
	}
	// No collapsed edge may cross a merged cell's interior.
	for _, border := range boxesWithTag(r, "collapsed-border") {
		for _, cell := range []Box{heads[0], cells[0], cells[3]} {
			inner := cell.Bounds.Inset(2)
			if border.Bounds.Overlaps(inner) {
				t.Fatalf("border %v crosses span %v", border.Bounds, cell.Bounds)
			}
		}
	}
	anchor, ok := r.Anchors["span"]
	if !ok || anchor != cells[0].Bounds.Min {
		t.Fatal("anchor displaced", anchor)
	}
	if len(r.Links) != 1 || len(r.Links[0].Regions) == 0 {
		t.Fatal("link lost")
	}
	for _, box := range r.Boxes {
		if box.Text == "Team" {
			if _, ok := r.LinkAt(box.Bounds.Min); !ok {
				t.Fatal("link region displaced")
			}
		}
	}
}
func TestTableSpacingAndGroupSpan(t *testing.T) {
	r := renderCSS(t, `<table style="border-spacing:7px 11px"><tbody><tr><td rowspan="0">A</td><td>B</td></tr><tr><td>C</td></tr></tbody><tbody><tr><td>D</td><td>E</td></tr></tbody></table>`)
	c := boxesWithTag(r, "td")
	if c[0].Bounds.Min != (image.Point{7, 11}) || c[1].Bounds.Min.X-c[0].Bounds.Max.X != 7 || c[2].Bounds.Min.Y-c[1].Bounds.Max.Y != 11 {
		t.Fatal("spacing", c)
	}
	if c[0].Bounds.Max.Y != c[2].Bounds.Max.Y || c[3].Bounds.Min.X != c[0].Bounds.Min.X || c[3].Bounds.Min.Y-c[0].Bounds.Max.Y != 11 {
		t.Fatal("row group boundaries", c)
	}
	if tagged(r, "table").Bounds.Max.Y-c[4].Bounds.Max.Y != 11 {
		t.Fatal("outer spacing")
	}
}
func TestTTYTableSpans(t *testing.T) {
	d, err := Parse(context.Background(), strings.NewReader(spanExample), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{8, 20, 80} {
		text, err := d.TTY(context.Background(), width)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(text, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("%d overflow %q", width, line)
			}
		}
		if width >= 20 {
			for _, word := range []string{"Report", "Team", "Total"} {
				if strings.Count(text, word) != 1 {
					t.Fatalf("lost or repeated %q:\n%s", word, text)
				}
			}
		}
		if width == 80 {
			want := "┌────────────────┐\n│ Report         │\n╞══════┬════┬════╡\n│ Team │ A  │ B  │\n│      ├────┴────┤\n│      │ Total   │\n└──────┴─────────┘"
			if text != want {
				t.Fatalf("merged table:\n%s", text)
			}
			lines := strings.Split(text, "\n")
			if strings.Count(lines[1], "│") != 2 {
				t.Fatalf("divider crosses colspan: %s", lines[1])
			}
		}
	}
}
func TestTableSpanBudget(t *testing.T) {
	for _, source := range []string{`<table><tr><td colspan="257">x</td></tr></table>`, `<table>` + strings.Repeat(`<tr><td colspan="256">x</td></tr>`, 400) + `</table>`} {
		d, err := Parse(context.Background(), strings.NewReader(source), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Render(context.Background(), Options{Width: 400}); !errors.Is(err, ErrLimit) {
			t.Fatalf("raster: %v", err)
		}
		if _, err := d.TTY(context.Background(), 80); !errors.Is(err, ErrLimit) {
			t.Fatalf("TTY: %v", err)
		}
	}
}

func TestSeparatedRowBackgroundGaps(t *testing.T) {
	r := renderCSS(t, `<table style="border-spacing:5px"><tr style="background:red"><td>A</td><td>B</td></tr></table>`)
	cells := boxesWithTag(r, "td")
	pixel := r.Image.RGBAAt(cells[0].Bounds.Max.X+1, cells[0].Bounds.Min.Y+1)
	if pixel.R != 255 || pixel.G != 255 || pixel.B != 255 {
		t.Fatal("row background painted spacing", pixel)
	}
}
