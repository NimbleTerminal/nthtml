package document

import (
	"context"
	"encoding/xml"
	"fmt"
	"image/color"
	"io"
	"regexp"
	"strings"
)

var svgAttribute = regexp.MustCompile(`([^\s=<>/]+)\s*=\s*(?:"[^"]*"|'[^']*')`)

// Resolve inherited HTML color before handing an isolated SVG to the rasterizer.
// Only paint attributes and paint declarations are changed: identifiers, URLs,
// text, namespace prefixes and all other source remain untouched. The normal
// SVG sanitizer still runs in the default renderer afterwards. Custom renderers
// retain their existing input contract. This helper adds no resource access.
func resolveSVGCurrentColor(ctx context.Context, source string, root color.RGBA) (string, error) {
	if !strings.Contains(strings.ToLower(source), "currentcolor") {
		return source, nil
	}
	if len(source) > maxSVGBytes {
		return "", limitError("SVG source")
	}
	dec := xml.NewDecoder(strings.NewReader(source))
	var colors []color.RGBA
	var out strings.Builder
	last, elements := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		start := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			elements++
			if len(colors) >= 32 || elements > 2048 {
				return "", limitError("SVG complexity")
			}
			fg := root
			if len(colors) > 0 {
				fg = colors[len(colors)-1]
				for _, a := range t.Attr {
					if a.Name.Space == "" && a.Name.Local == "color" {
						if c, ok := cssColor(a.Value, fg); ok {
							fg = c
						}
					}
				}
				for _, a := range t.Attr {
					if a.Name.Space == "" && a.Name.Local == "style" {
						if v, ok := declarations(a.Value)["color"]; ok {
							if c, ok := cssColor(v, fg); ok {
								fg = c
							}
						}
					}
				}
			}
			colors = append(colors, fg)
			c := color.NRGBAModel.Convert(fg).(color.NRGBA)
			concrete := fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
			if c.A != 255 {
				concrete = fmt.Sprintf("rgba(%d,%d,%d,%.6f)", c.R, c.G, c.B, float64(c.A)/255)
			}
			raw := source[start:int(dec.InputOffset())]
			changed := svgAttribute.ReplaceAllStringFunc(raw, func(attr string) string {
				key, _, ok := strings.Cut(attr, "=")
				if !ok {
					return attr
				}
				key = strings.TrimSpace(key)
				value := ""
				found := false
				for _, a := range t.Attr {
					if a.Name.Space == "" && a.Name.Local == key {
						value = a.Value
						found = true
						break
					}
				}
				if !found {
					return attr
				}
				replacement := value
				if svgPaintProperty(key) && strings.EqualFold(strings.TrimSpace(value), "currentcolor") {
					replacement = concrete
				}
				if key == "style" {
					parts := splitDeclarations(value)
					for i, part := range parts {
						k, v, ok := strings.Cut(part, ":")
						if ok && svgPaintProperty(strings.ToLower(strings.TrimSpace(k))) && strings.EqualFold(strings.TrimSpace(v), "currentcolor") {
							parts[i] = k + ":" + concrete
						}
					}
					replacement = strings.Join(parts, ";")
				}
				if replacement == value {
					return attr
				}
				var escaped strings.Builder
				_ = xml.EscapeText(&escaped, []byte(replacement))
				return key + `="` + escaped.String() + `"`
			})
			out.WriteString(source[last:start])
			out.WriteString(changed)
			last = int(dec.InputOffset())
		case xml.EndElement:
			if len(colors) > 0 {
				colors = colors[:len(colors)-1]
			}
		}
	}
	out.WriteString(source[last:])
	return out.String(), nil
}

func svgPaintProperty(name string) bool {
	switch name {
	case "fill", "stroke", "stop-color", "flood-color", "lighting-color":
		return true
	}
	return false
}
