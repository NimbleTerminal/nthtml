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
	paint := func(r *document.Result) {
		b := r.Image.Bounds()
		canvas.Set("width", b.Dx())
		canvas.Set("height", b.Dy())
		dc := canvas.Call("getContext", "2d")
		data := dc.Call("createImageData", b.Dx(), b.Dy())
		js.CopyBytesToJS(data.Get("data"), r.Image.Pix)
		dc.Call("putImageData", data, 0, 0)
	}
	paint(initial)
	text, err := doc.TTY(ctx, 80)
	if err != nil {
		panic(err)
	}
	d.Call("getElementById", "tty").Set("textContent", text)
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
		text, err := doc.TTY(ctx, nextColumns)
		if err != nil {
			return err.Error()
		}
		if result != nil {
			paint(result)
		}
		d.Call("getElementById", "tty").Set("textContent", text)
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
	api.Set("reflow", reflow)
	api.Set("close", closeDemo)
	js.Global().Set("nthtmlDemo", api)
	d.Call("getElementById", "status").Set("textContent", "Rendered by nthtml · Go WebAssembly")
	<-done
	js.Global().Set("nthtmlDemo", js.Undefined())
}
