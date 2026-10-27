package document

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
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
	return imageSizeStyled(n, style{}, natural, cssWidth, box, cssHeight)
}

func imageSizeStyled(n *node, s style, natural image.Point, cssWidth bool, box, cssHeight int) (int, int) {
	attrW, attrH := pixelAttr(n.attrs["width"]), pixelAttr(n.attrs["height"])
	ratio := float64(natural.X) / float64(max(1, natural.Y))
	if s.aspectRatio > 0 {
		ratio = s.aspectRatio
	}
	w := box
	switch {
	case cssWidth:
	case attrW > 0:
		w = attrW
	case cssHeight > 0:
		w = int(math.Round(float64(cssHeight) * ratio))
	case attrH > 0:
		if s.aspectRatio > 0 {
			w = int(math.Round(float64(attrH) * ratio))
		} else {
			w = attrH * natural.X / natural.Y
		}
	default:
		w = natural.X
	}
	w = max(1, min(w, box))
	h := cssHeight
	if h <= 0 && s.aspectRatio > 0 && (cssWidth || attrH == 0) {
		h = int(math.Round(float64(w) / s.aspectRatio))
	}
	if h <= 0 {
		h = attrH
		if h > 0 && attrW > 0 && attrW != w {
			h = h * w / attrW // A capped width keeps the attributes' ratio.
		}
	}
	if h <= 0 {
		if s.aspectRatio > 0 {
			h = int(math.Round(float64(w) / s.aspectRatio))
		} else {
			h = w * natural.Y / natural.X
		}
	}
	return w, max(1, h)
}

func prepareImageSource(src image.Image, w, h int, s style) image.Image {
	if src == nil || s.hidden || w <= 0 || h <= 0 || w > 4096 || h > 4096 || w*h > 4<<20 {
		return nil
	}
	mode := s.objectFit
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}
	out := src
	if mode == "scale-down" {
		if sw <= w && sh <= h {
			mode = "none"
		} else {
			mode = "contain"
		}
	}
	switch mode {
	case "contain":
		scale := min(float64(w)/float64(sw), float64(h)/float64(sh))
		dw := max(1, min(w, int(math.Round(float64(sw)*scale))))
		dh := max(1, min(h, int(math.Round(float64(sh)*scale))))
		if dw != w || dh != h {
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			ox, oy := (w-dw)/2, (h-dh)/2
			xdraw.ApproxBiLinear.Scale(dst, image.Rect(ox, oy, ox+dw, oy+dh), src, b, draw.Over, nil)
			out = dst
		}
	case "cover":
		scale := max(float64(w)/float64(sw), float64(h)/float64(sh))
		cw := max(1, min(sw, int(math.Round(float64(w)/scale))))
		ch := max(1, min(sh, int(math.Round(float64(h)/scale))))
		if cw != sw || ch != sh {
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			ox := b.Min.X + (sw-cw)/2
			oy := b.Min.Y + (sh-ch)/2
			xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, image.Rect(ox, oy, ox+cw, oy+ch), draw.Over, nil)
			out = dst
		}
	case "none":
		if sw != w || sh != h {
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			ox, oy := (w-sw)/2, (h-sh)/2
			draw.Draw(dst, image.Rect(ox, oy, ox+sw, oy+sh), src, b.Min, draw.Over)
			out = dst
		}
	}
	eff := s.effectiveOpacity()
	if eff < 1 {
		ob := out.Bounds()
		if ob.Dx() > 0 && ob.Dy() > 0 && ob.Dx()*ob.Dy() <= 4<<20 {
			dst := image.NewRGBA(image.Rect(0, 0, ob.Dx(), ob.Dy()))
			for y := 0; y < ob.Dy(); y++ {
				for x := 0; x < ob.Dx(); x++ {
					r, g, bl, a := out.At(ob.Min.X+x, ob.Min.Y+y).RGBA()
					dst.SetRGBA(x, y, scaleColorAlpha(color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)}, eff))
				}
			}
			out = dst
		}
	}
	return out
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
	if s.width.definite(availW) {
		iw = s.width.resolve(availW, 16384)
		okW = true
	}
	if s.height.definite(parentH) {
		ih = s.height.resolve(parentH, maxH)
		okH = true
	}
	if s.aspectRatio > 0 {
		if okW && !okH {
			ih = int(math.Round(float64(iw) / s.aspectRatio))
			okH = true
		} else if okH && !okW {
			iw = int(math.Round(float64(ih) * s.aspectRatio))
			okW = true
		}
	}
	if !okW || !okH {
		return 0, 0, false
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
