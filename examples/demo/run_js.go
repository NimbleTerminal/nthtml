//go:build js && wasm

package main

import (
	"context"
	"math"
	"strings"
	"syscall/js"

	"nimbleterminal.dev/nthtml/document"
)

func run(initial *document.Result, source string) {
	ctx := context.Background()
	doc, err := document.Parse(ctx, strings.NewReader(source), document.Limits{})
	if err != nil {
		panic(err)
	}
	d := js.Global().Get("document")
	canvas := d.Call("getElementById", "document")
	linkValues := func(links []document.Link) []any {
		out := make([]any, 0, len(links))
		for _, link := range links {
			regions := make([]any, 0, len(link.Regions))
			for _, b := range link.Regions {
				regions = append(regions, map[string]any{"x": b.Min.X, "y": b.Min.Y, "width": b.Dx(), "height": b.Dy()})
			}
			out = append(out, map[string]any{"id": link.ID, "href": link.Href, "text": link.Text, "regions": regions})
		}
		return out
	}
	var currentText, currentTTY *document.TextIndex
	currentResult := initial
	var currentTTYResult *document.TTYResult
	paint := func(r *document.Result) {
		currentResult = r
		currentText = r.TextIndex()
		runs := []any{}
		previous := 0
		for _, run := range currentText.Runs {
			b := run.Bounds
			runs = append(runs, map[string]any{"text": currentText.Text[run.Start:run.End], "before": currentText.Text[previous:run.Start], "x": b.Min.X, "y": b.Min.Y, "width": b.Dx(), "height": b.Dy()})
			previous = run.End
		}
		canvas.Set("nthtmlTextRuns", js.ValueOf(runs))
		b := r.Image.Bounds()
		canvas.Set("width", b.Dx())
		canvas.Set("height", b.Dy())
		dc := canvas.Call("getContext", "2d")
		data := dc.Call("createImageData", b.Dx(), b.Dy())
		js.CopyBytesToJS(data.Get("data"), r.Image.Pix)
		dc.Call("putImageData", data, 0, 0)
		canvas.Set("nthtmlLinks", js.ValueOf(linkValues(r.Links)))
		warnings := []any{}
		for _, warning := range r.Warnings {
			warnings = append(warnings, warning)
		}
		canvas.Set("nthtmlWarnings", js.ValueOf(warnings))
		status := "Rendered by nthtml · Go WebAssembly"
		if len(r.Warnings) > 0 {
			status += " · " + strings.Join(r.Warnings, "; ")
		}
		d.Call("getElementById", "status").Set("textContent", status)
	}
	paint(initial)
	setTTY := func(result *document.TTYResult) {
		currentTTYResult = result
		currentTTY = result.TextIndex()
		tty := d.Call("getElementById", "tty")
		tty.Set("textContent", result.Text)
		segments := ttySegments(result)
		tty.Set("nthtmlSegments", js.ValueOf(segments))
	}
	tty, err := doc.TTYLayout(ctx, 80)
	if err != nil {
		panic(err)
	}
	setTTY(tty)

	d.Call("getElementById", "browser-reference").Set("srcdoc", "<!doctype html><meta charset=\"utf-8\">\n"+source)
	width, columns := 800, 80
	reflow := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 {
			return "expected pixel width and terminal columns"
		}
		for _, arg := range args {
			if arg.Type() != js.TypeNumber || math.IsNaN(arg.Float()) || math.IsInf(arg.Float(), 0) {
				return "invalid reflow dimensions"
			}
		}
		nextWidth := max(32, min(4096, args[0].Int()))
		nextColumns := max(2, min(1000, args[1].Int()))
		if nextWidth == width && nextColumns == columns {
			return ""
		}
		var result *document.Result
		if nextWidth != width {
			var err error
			result, err = doc.Render(ctx, document.Options{Width: nextWidth})
			if err != nil {
				return err.Error()
			}
		}
		tty, err := doc.TTYLayout(ctx, nextColumns)
		if err != nil {
			return err.Error()
		}
		if result != nil {
			paint(result)
		}
		setTTY(tty)
		width, columns = nextWidth, nextColumns
		return ""
	})
	done := make(chan struct{})
	closed := false
	closeDemo := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		if !closed {
			closed = true
			close(done)
		}
		return nil
	})
	defer reflow.Release()
	defer closeDemo.Release()
	api := js.Global().Get("Object").New()
	indexFor := func(tty bool) *document.TextIndex {
		if tty {
			return currentTTY
		}
		return currentText
	}
	search := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeBoolean {
			return []any{}
		}
		out := []any{}
		for _, m := range indexFor(args[1].Bool()).Search(args[0].String()) {
			out = append(out, map[string]any{"start": m.Start, "end": m.End})
		}
		return out
	})
	regions := js.FuncOf(func(_ js.Value, args []js.Value) any {
		out := []any{}
		if len(args) != 3 || args[0].Type() != js.TypeNumber || args[1].Type() != js.TypeNumber || args[2].Type() != js.TypeBoolean {
			return out
		}
		for _, r := range indexFor(args[2].Bool()).Regions(args[0].Int(), args[1].Int()) {
			out = append(out, map[string]any{"x": r.Min.X, "y": r.Min.Y, "width": r.Dx(), "height": r.Dy()})
		}
		return out
	})
	fragment := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeBoolean {
			return map[string]any{"found": false}
		}
		p, ok := currentResult.Fragment(args[0].String())
		if args[1].Bool() {
			p, ok = currentTTYResult.Fragment(args[0].String())
		}
		return map[string]any{"found": ok, "x": p.X, "y": p.Y}
	})
	defer search.Release()
	defer regions.Release()
	defer fragment.Release()
	api.Set("search", search)
	api.Set("regions", regions)
	api.Set("fragment", fragment)
	api.Set("reflow", reflow)
	api.Set("close", closeDemo)
	js.Global().Set("nthtmlDemo", api)
	<-done
	js.Global().Set("nthtmlDemo", js.Undefined())
}
