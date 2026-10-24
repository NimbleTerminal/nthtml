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
