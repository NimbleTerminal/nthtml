//go:build js && wasm

package main

import (
	"nimbleterminal.dev/nthtml/document"
	"syscall/js"
)

func run(r *document.Result) {
	d := js.Global().Get("document")
	canvas := d.Call("getElementById", "document")
	b := r.Image.Bounds()
	canvas.Set("width", b.Dx())
	canvas.Set("height", b.Dy())
	ctx := canvas.Call("getContext", "2d")
	data := ctx.Call("createImageData", b.Dx(), b.Dy())
	js.CopyBytesToJS(data.Get("data"), r.Image.Pix)
	ctx.Call("putImageData", data, 0, 0)
	d.Call("getElementById", "status").Set("textContent", "Rendered by nthtml · Go WebAssembly")
}
