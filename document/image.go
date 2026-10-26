package document

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strconv"
	"strings"
)

// DecodeImage bounds encoded bytes and decoded dimensions before allocation.
// Use its result in Options.Images to explicitly permit a local resource.
func DecodeImage(r io.Reader) (image.Image, error) {
	data, e := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(data) > 1<<20 {
		return nil, ErrLimit
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width > (4<<20)/cfg.Height {
		return nil, ErrLimit
	}
	img, _, e := image.Decode(bytes.NewReader(data))
	return img, e
}

// DecodeDataURL decodes base64 PNG/JPEG/GIF data URLs for an explicit Images entry.
func DecodeDataURL(src string) (image.Image, error) {
	if len(src) > 2<<20 {
		return nil, ErrLimit
	}
	header, payload, ok := strings.Cut(src, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
		return nil, fmt.Errorf("nthtml: expected base64 image data URL")
	}
	return DecodeImage(base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload)))
}

// ImageSources lists the distinct img src values in document order, so a host
// can decode exactly the resources it chooses to allow before Render. Hidden
// images are included; the list never exceeds the parsed node budget.
func (d *Document) ImageSources() []string {
	var out []string
	seen := map[string]bool{}
	var visit func(*node)
	visit = func(n *node) {
		if src := n.attrs["src"]; n.tag == "img" && src != "" && !seen[src] {
			seen[src] = true
			out = append(out, src)
		}
		for _, c := range n.children {
			visit(c)
		}
	}
	visit(d.root)
	return out
}

// imageSize places an image as a replaced element: a CSS width fills the
// box, else the width attribute, else the image's own size, never wider than
// the box. A CSS or attribute height is kept; a missing one follows the
// image's aspect ratio.
func imageSize(n *node, natural image.Point, cssWidth bool, box, cssHeight int) (int, int) {
	attrW, attrH := pixelAttr(n.attrs["width"]), pixelAttr(n.attrs["height"])
	w := box
	switch {
	case cssWidth:
	case attrW > 0:
		w = attrW
	case cssHeight > 0:
		w = cssHeight * natural.X / natural.Y
	case attrH > 0:
		w = attrH * natural.X / natural.Y
	default:
		w = natural.X
	}
	w = max(1, min(w, box))
	h := cssHeight
	if h <= 0 {
		h = attrH
		if h > 0 && attrW > 0 && attrW != w {
			h = h * w / attrW // A capped width keeps the attributes' ratio.
		}
	}
	if h <= 0 {
		h = w * natural.Y / natural.X
	}
	return w, max(1, h)
}

// pixelAttr reads a nonnegative integer width/height attribute, as in
// width="320" or width="320px"; anything else is absent.
func pixelAttr(v string) int {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(v), "px"))
	if err != nil || n < 1 || n > 16384 {
		return 0
	}
	return n
}

func nonnegPixelAttr(v string) (int, bool) {
	v = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), "px")
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 16384 {
		return 0, false
	}
	return n, true
}

func missingImageSpacer(n *node, s style, availW, parentH, maxH int) (int, int, bool) {
	iw, okW := nonnegPixelAttr(n.attrs["width"])
	ih, okH := nonnegPixelAttr(n.attrs["height"])
	if !okW || !okH {
		return 0, 0, false
	}
	if s.width.definite(availW) {
		iw = s.width.resolve(availW, 16384)
	}
	if s.height.definite(parentH) {
		ih = s.height.resolve(parentH, maxH)
	}
	iw = min(iw, max(0, availW))
	ih = min(ih, max(0, maxH))
	return iw, ih, true
}

func (l *layout) imgHasBox(n *node, s style) bool {
	if src := l.opts.Images[n.attrs["src"]]; src != nil && !src.Bounds().Empty() {
		return true
	}
	if cleanText(n.attrs["alt"]) != "" {
		return true
	}
	_, _, ok := missingImageSpacer(n, s, 4096, -1, l.d.limits.Height)
	return ok
}
