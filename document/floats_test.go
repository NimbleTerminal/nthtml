package document

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
)

func TestFloatBorderBoxPercentageWidths(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative;width:300px"><div style="float:left;box-sizing:border-box;width:50%;border:5px solid black">LEFT HALF</div><div style="float:left;box-sizing:border-box;width:50%;border:5px solid black">RIGHT HALF</div></article>`)
	cells := boxesWithTag(r, "div")
	parent := tagged(r, "article")
	if cells[0].Bounds != image.Rect(0, 0, 150, 32) || cells[1].Bounds != image.Rect(150, 0, 300, 32) {
		t.Fatal(cells)
	}
	// Relative positioning does not establish a BFC; the floats remain outside
	// its normal-flow height, while still extending the exported canvas.
	if parent.Bounds.Dy() != 0 || r.Image.Bounds().Dy() != 32 {
		t.Fatal(parent.Bounds, r.Image.Bounds())
	}
}
func TestFloatRowsRightAndClear(t *testing.T) {
	r := renderCSS(t, `<article style="width:200px"><div style="float:left;width:120px;height:30px"></div><div style="float:left;width:120px;height:20px"></div><aside style="float:right;width:40px;height:10px"></aside><section style="clear:both;height:5px"></section></article>`)
	cells := boxesWithTag(r, "div")
	right, clear := tagged(r, "aside"), tagged(r, "section")
	if cells[0].Bounds != image.Rect(0, 0, 120, 30) || cells[1].Bounds != image.Rect(0, 30, 120, 50) || right.Bounds != image.Rect(160, 30, 200, 40) || clear.Bounds.Min.Y != 50 {
		t.Fatal(cells, right.Bounds, clear.Bounds)
	}
}
func TestTextWrapsAroundFloatAndReturnsToFullWidth(t *testing.T) {
	r := renderCSS(t, `<article style="width:200px"><aside style="float:left;width:80px;height:44px"></aside><p style="margin:0">`+strings.Repeat("word ", 25)+`</p></article>`)
	float := tagged(r, "aside").Bounds
	below := false
	for _, b := range boxesWithTag(r, "text") {
		if b.Bounds.Overlaps(float) {
			t.Fatal("text overlaps float", b.Bounds, float)
		}
		if b.Bounds.Min.Y < float.Max.Y && b.Bounds.Min.X < 80 {
			t.Fatal("text did not use exclusion", b.Bounds)
		}
		if b.Bounds.Min.Y >= 44 && b.Bounds.Min.X == 0 {
			below = true
		}
	}
	if !below {
		t.Fatal("text never returned to full width")
	}
}
func TestFloatMarginsAndFormattingContexts(t *testing.T) {
	r := renderCSS(t, `<article style="position:absolute;left:0;top:0;width:200px"><aside style="float:left;width:50px;height:20px;margin:5px 10px"></aside></article>`)
	if b := tagged(r, "aside"); b.Bounds != image.Rect(10, 5, 60, 25) {
		t.Fatal(b.Bounds)
	}
	if b := tagged(r, "article"); b.Bounds.Dy() != 30 {
		t.Fatal("absolute container did not enclose float margin box", b.Bounds)
	}
	r = renderCSS(t, `<article style="width:200px"><aside style="float:left;width:100px;height:80px"></aside> <span> </span> </article>`)
	if b := tagged(r, "article"); b.Bounds.Dy() != 0 {
		t.Fatal("whitespace enclosed the float", b.Bounds)
	}
}
func TestStaticFloatDoesNotBecomeAbsoluteContainingBlock(t *testing.T) {
	r := renderCSS(t, `<article style="position:relative;width:200px;height:100px"><aside style="float:right;width:80px;height:40px"><section style="position:absolute;left:10px;top:5px;width:10px;height:10px"></section></aside></article>`)
	if b := tagged(r, "section"); b.Bounds != image.Rect(10, 5, 20, 15) {
		t.Fatal("static float became containing block", b.Bounds)
	}
}
func TestFloatShrinkToFitAndBudget(t *testing.T) {
	r := renderCSS(t, `<article style="width:200px"><aside style="float:left;padding:5px"><div style="width:50px;height:10px"></div></aside></article>`)
	if b := tagged(r, "aside"); b.Bounds.Dx() != 60 || b.Bounds.Dy() != 20 {
		t.Fatal(b.Bounds)
	}
	source := `<article>` + strings.Repeat(`<aside style="float:left;width:1px;height:1px"></aside>`, 1000) + `</article>`
	_, err := Render(context.Background(), strings.NewReader(source), Options{Width: 400})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("expected cumulative float-work limit, got %v", err)
	}
}

func TestFloatsRespectNegativePositionedOrigins(t *testing.T) {
	r := renderCSS(t, `<article style="position:absolute;left:0;top:-10px;width:100px"><aside style="float:left;width:20px;height:5px"></aside></article>`)
	if a, b := tagged(r, "article"), tagged(r, "aside"); a.Bounds != image.Rect(0, -10, 100, -5) || b.Bounds != image.Rect(0, -10, 20, -5) {
		t.Fatal(a.Bounds, b.Bounds)
	}
}

func TestTextAlignmentDoesNotMoveNestedFloat(t *testing.T) {
	r := renderCSS(t, `<article style="width:200px;text-align:right"><span><aside style="float:left;width:80px;height:44px;background:green">F</aside>TEXT</span></article>`)
	if b := tagged(r, "aside"); b.Bounds.Min.X != 0 {
		t.Fatal("inline text alignment shifted float", b.Bounds)
	}
	for _, b := range boxesWithTag(r, "text") {
		if b.Text == "TEXT" && b.Bounds.Overlaps(tagged(r, "aside").Bounds) {
			t.Fatal("aligned text overlaps float", b.Bounds)
		}
	}
}
