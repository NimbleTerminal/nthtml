// nthtml renders an HTML file (or stdin) to PNG without network access.
package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"io"
	"nimbleterminal.dev/nthtml/document"
	"os"
	"time"
)

func main() {
	width := flag.Int("width", 800, "document width in pixels")
	out := flag.String("o", "document.png", "output PNG")
	flag.Parse()
	var r io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, e := os.Open(flag.Arg(0))
		if e != nil {
			fail(e)
		}
		defer f.Close()
		r = f
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, e := document.Render(ctx, r, document.Options{Width: *width})
	if e != nil {
		fail(e)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	f, e := os.Create(*out)
	if e != nil {
		fail(e)
	}
	e = png.Encode(f, result.Image)
	closeErr := f.Close()
	if e != nil {
		fail(e)
	}
	if closeErr != nil {
		fail(closeErr)
	}
}
func fail(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
