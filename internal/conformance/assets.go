package conformance

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io/fs"
	"net/url"
	"path"
	"strings"

	"nimbleterminal.dev/nthtml/document"
)

func fixturePath(name, href string) (string, error) {
	u, err := url.Parse(href)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || u.Path == "" {
		return "", fmt.Errorf("unsupported fixture URL %q", href)
	}
	resolved := path.Clean(path.Join(path.Dir(name), u.Path))
	if strings.HasPrefix(u.Path, "/") {
		resolved = path.Clean(strings.TrimPrefix(u.Path, "/"))
	}
	if !fs.ValidPath(resolved) || strings.Contains(resolved, "\\") {
		return "", fmt.Errorf("unsafe fixture path %q", resolved)
	}
	return resolved, nil
}
func (s Suite) images(ctx context.Context, name string, d *document.Document, assets []string) (map[string]image.Image, error) {
	allowed := map[string]bool{}
	for _, name := range assets {
		allowed[name] = true
	}
	images := map[string]image.Image{}
	pixels, encoded := 0, 0
	for _, src := range d.ImageSources() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		asset, err := fixturePath(name, src)
		if err != nil {
			return nil, err
		}
		if !allowed[asset] || s.Manifest.Files[asset] == "" {
			return nil, fmt.Errorf("image asset is not allowlisted and pinned: %s", asset)
		}
		data, err := fs.ReadFile(s.Files, asset)
		if err != nil {
			return nil, err
		}
		encoded += len(data)
		if encoded > 4<<20 || len(images) >= 16 {
			return nil, document.ErrLimit
		}
		img, err := document.DecodeImage(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode fixture image %s: %w", asset, err)
		}
		pixels += img.Bounds().Dx() * img.Bounds().Dy()
		if pixels > 4<<20 {
			return nil, document.ErrLimit
		}
		images[src] = img
	}
	return images, nil
}
