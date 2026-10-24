package document

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	mermaid "github.com/zkrebbekx/go-mermaid"
	"nimbleterminal.dev/ntsvg"
)

const maxSVGBytes = 256 << 10

// DiagramRenderer renders a bounded diagram into an aspect-preserving image.
// kind is "svg" or "mermaid". Implementations must respect context and bounds,
// perform no implicit network access, and be safe for concurrent Render calls.
type DiagramRenderer interface {
	RenderDiagram(ctx context.Context, kind, source string, maxWidth, maxHeight int) (image.Image, error)
}

// DefaultDiagramRenderer uses go-mermaid for source-to-SVG and ntsvg for pixels.
// It uses no browser, JavaScript runtime, filesystem, or network resources.
type DefaultDiagramRenderer struct{}

func (DefaultDiagramRenderer) RenderDiagram(ctx context.Context, kind, source string, w, h int) (img image.Image, err error) {
	defer func() {
		if v := recover(); v != nil {
			img = nil
			err = fmt.Errorf("diagram renderer: %v", v)
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if w < 1 || h < 1 || w > 4096 || h > 2048 || w > (1<<20)/h {
		return nil, limitError("diagram pixels")
	}
	if kind == "mermaid" {
		if len(source) > 4096 || strings.Count(source, "\n") > 64 || len(strings.Fields(source)) > 512 || len(mermaidWords.FindAllString(source, 257)) > 256 {
			return nil, limitError("Mermaid source")
		}
		trimmed := strings.TrimSpace(source)
		allowed := false
		for _, prefix := range []string{"flowchart ", "graph ", "sequenceDiagram", "stateDiagram-v2", "classDiagram", "pie"} {
			if strings.HasPrefix(trimmed, prefix) {
				allowed = true
			}
		}
		if !allowed || strings.Contains(source, "%%{") {
			return nil, fmt.Errorf("unsupported Mermaid diagram or directive")
		}
		data, e := mermaid.Render(source)
		if e != nil {
			return nil, e
		}
		source = string(data)
	} else if kind != "svg" {
		return nil, fmt.Errorf("unknown diagram kind")
	}
	if err = validateSVG(ctx, source); err != nil {
		return nil, err
	}
	factory := ntsvg.DefaultRendererFactoryWithLimits(ntsvg.Limits{MaxRenderPixels: 1 << 20, MaxRenderEdge: 4096})
	renderer, e := factory("embedded", []byte(source))
	if renderer != nil {
		defer renderer.Close()
	}
	if e != nil {
		return nil, e
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	img, err = renderer.Render(w, h)
	if err == nil {
		err = ctx.Err()
	}
	return
}

func diagramSource(n *node) (kind, source string) {
	if n.svg != "" {
		return "svg", n.svg
	}
	isMermaid := false
	if n.tag == "pre" || n.tag == "div" {
		for _, class := range strings.Fields(n.attrs["class"]) {
			if class == "mermaid" {
				isMermaid = true
			}
		}
	}
	if n.tag == "pre" {
		for _, c := range n.children {
			if c.tag == "code" {
				for _, class := range strings.Fields(c.attrs["class"]) {
					if class == "language-mermaid" {
						isMermaid = true
					}
				}
			}
		}
	}
	if !isMermaid {
		return "", ""
	}
	var b strings.Builder
	var visit func(*node)
	visit = func(n *node) {
		b.WriteString(n.text)
		for _, c := range n.children {
			visit(c)
		}
	}
	visit(n)
	return "mermaid", b.String()
}
func diagramLabel(n *node) string {
	if label := n.attrs["aria-label"]; label != "" {
		return cleanText(label)
	}
	if n.svg != "" {
		dec := xml.NewDecoder(strings.NewReader(n.svg))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if start, ok := tok.(xml.StartElement); ok && (start.Name.Local == "title" || start.Name.Local == "desc") {
				var label string
				if dec.DecodeElement(&label, &start) == nil && strings.TrimSpace(label) != "" {
					return cleanText(label)
				}
			}
		}
	}
	return "diagram"
}
func (l *layout) diagram(n *node, kind, source string, x, y, w, height int, s style) int {
	fallback := func() int {
		text := "[SVG: " + diagramLabel(n) + "]"
		if kind == "mermaid" {
			text = source
			s.pre = true
			s.mono = true
			s.family = "Go Mono"
		}
		return l.flow([]*node{{text: text}}, x, y, w, s)
	}
	if l.opts.DisableDiagrams {
		return fallback()
	}
	l.diagramCount++
	if l.diagramCount > 32 {
		l.err = limitError("diagram count")
		return y
	}
	remaining := min(4<<20, l.d.limits.Pixels) - l.diagramPixels
	maxH := min(2048, l.d.limits.Height-y, (1<<20)/w, remaining/w)
	if height > 0 {
		maxH = min(maxH, height)
	}
	if maxH < 1 {
		l.err = limitError("diagram pixels")
		return y
	}
	renderer := l.opts.Diagrams
	if renderer == nil {
		renderer = DefaultDiagramRenderer{}
	}
	img, err := renderer.RenderDiagram(l.ctx, kind, source, w, maxH)
	if err != nil {
		if errors.Is(err, ErrLimit) || l.ctx.Err() != nil {
			l.err = err
			return y
		}
		message := cleanText(err.Error())
		if len(message) > 512 {
			message = message[:512]
		}
		l.warnings = append(l.warnings, kind+": "+message)
		return fallback()
	}
	if img == nil || img.Bounds().Empty() {
		l.warnings = append(l.warnings, kind+": empty image")
		return fallback()
	}
	b := img.Bounds()
	if b.Dx() > w || b.Dy() > maxH {
		l.err = limitError("diagram renderer dimensions")
		return y
	}
	l.diagramPixels += b.Dx() * b.Dy()
	l.add(Box{Tag: kind, Bounds: image.Rect(x, y, x+b.Dx(), y+b.Dy()), LinkID: s.linkID}, img)
	return y + b.Dy()
}

var mermaidWords = regexp.MustCompile(`[\p{L}\p{N}_]+`)
var svgLocalURL = regexp.MustCompile(`url\(#[a-zA-Z0-9_-]+\)`)
var svgNumber = regexp.MustCompile(`[-+]?(?:[0-9]*\.[0-9]+|[0-9]+\.?[0-9]*)(?:[eE][-+]?[0-9]+)?`)

func validateSVG(ctx context.Context, source string) error {
	if len(source) > maxSVGBytes {
		return limitError("SVG bytes")
	}
	dec := xml.NewDecoder(strings.NewReader(source))
	depth, elements, numbers := 0, 0, 0
	definitions := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.Directive, xml.ProcInst:
			return fmt.Errorf("SVG directives are unsupported")
		case xml.StartElement:
			depth++
			elements++
			if depth > 32 || elements > 2048 {
				return limitError("SVG complexity")
			}
			if elements == 1 && t.Name.Local != "svg" {
				return fmt.Errorf("SVG root required")
			}
			switch t.Name.Local {
			case "svg", "g", "path", "rect", "circle", "ellipse", "line", "polygon", "polyline", "text", "tspan", "defs", "marker", "linearGradient", "radialGradient", "stop", "title", "desc":
			default:
				return fmt.Errorf("unsupported SVG element %s", t.Name.Local)
			}
			if t.Name.Local == "defs" || t.Name.Local == "marker" {
				definitions++
			}
			for _, a := range t.Attr {
				key := strings.ToLower(a.Name.Local)
				v := strings.ToLower(a.Value)
				if strings.HasPrefix(key, "on") || key == "href" || strings.Contains(svgLocalURL.ReplaceAllString(v, ""), "url(") {
					return fmt.Errorf("SVG external/active references are unsupported")
				}
				if definitions > 0 && (key == "marker" || key == "marker-start" || key == "marker-mid" || key == "marker-end" || strings.Contains(v, "url(")) {
					return fmt.Errorf("SVG references inside definitions are unsupported")
				}
				properties := map[string]string{key: v}
				if key == "style" {
					properties = map[string]string{}
					parts := strings.Split(v, ";")
					if len(parts) > 128 {
						return limitError("SVG style declarations")
					}
					for _, part := range parts {
						if k, value, ok := strings.Cut(part, ":"); ok {
							properties[strings.TrimSpace(k)] = strings.TrimSpace(value)
						}
					}
				}
				for property, value := range properties {
					switch property {
					case "d", "points", "viewbox", "x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry", "width", "height", "transform", "gradienttransform", "stroke-width", "stroke-dasharray", "stroke-dashoffset", "font-size", "dx", "dy", "markerwidth", "markerheight", "refx", "refy":
						if strings.Contains(value, "nan") || strings.Contains(value, "inf") {
							return fmt.Errorf("non-finite SVG coordinate")
						}
						for _, number := range svgNumber.FindAllString(value, -1) {
							numbers++
							if numbers > 16384 {
								return limitError("SVG coordinates")
							}
							f, e := strconv.ParseFloat(number, 64)
							if e != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 32768 {
								return limitError("SVG coordinate range")
							}
							if (property == "stroke-dasharray" || property == "stroke-width") && f != 0 && math.Abs(f) < 0.1 {
								return fmt.Errorf("SVG stroke values below 0.1 are unsupported")
							}
						}
					}
				}
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "defs" || t.Name.Local == "marker" {
				definitions--
			}
		}
	}
	if depth != 0 || elements == 0 {
		return fmt.Errorf("incomplete SVG")
	}
	return nil
}
