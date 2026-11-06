package nthtml_test

import (
	"context"
	"fmt"
	"strings"

	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"
)

func ExamplePlanPaint() {
	// Run this entire pass on the host's layout/paint worker.
	ctx := context.Background()
	result, err := document.Render(ctx, strings.NewReader(strings.Repeat("<p>A CSS paragraph.</p>", 1000)), document.Options{Width: 800, NoImage: true, Fonts: document.MinimalFonts})
	if err != nil {
		panic(err)
	}
	painter, err := result.NewPainter(ctx)
	if err != nil {
		panic(err)
	}
	defer painter.Close() // In an application, retain until the next layout.
	plan, err := nthtml.PlanPaint(result, nthtml.PaintViewport{Cols: 80, Rows: 24, CellWidth: 10, CellHeight: 20})
	if err != nil {
		panic(err)
	}
	band, err := painter.Paint(ctx, plan.Area, plan.Scale)
	if err != nil {
		panic(err)
	}
	frame := nthtml.PaintedResult{Result: result, Revision: 1, Plan: plan, Image: band}

	// Deliver frame to the UI goroutine. Return cmd through Bubble Tea.
	viewer := nthtml.NewWithOptions(nil, nthtml.ModelOptions{HostPainted: true})
	defer viewer.Close()
	cmd, err := viewer.SetPaintedResult(frame, false)
	if err != nil {
		panic(err)
	}
	fmt.Println(cmd != nil, result.Image == nil, band.Bounds().Dy() < result.Bounds().Dy())
	// Output: true true true
}
