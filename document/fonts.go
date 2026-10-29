package document

import (
	"cmp"
	"fmt"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"nimbleterminal.dev/ntfonts"
	"strings"
)

// FaceSource supplies immutable OpenType font bytes. Implementations must bound
// their own work and be safe for concurrent use when a Document is shared.
type FaceSource interface {
	Face(family, variant string) ([]byte, error)
}

// MinimalFonts is an opt-in FaceSource backed solely by the Go standard
// library's embedded Go and Go Mono fonts, avoiding ntfonts family lookups on
// memory-constrained or embedded targets.
var MinimalFonts FaceSource = minimalFontSource{}

type minimalFontSource struct{}

func (minimalFontSource) Face(family, variant string) ([]byte, error) {
	mono := strings.EqualFold(family, "Go Mono") || strings.EqualFold(family, "IBM Plex Mono") || strings.EqualFold(family, "monospace")
	fonts := [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF}
	if mono {
		fonts = [][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF}
	}
	switch variant {
	case "", "Regular":
		return fonts[0], nil
	case "Bold":
		return fonts[1], nil
	case "Italic":
		return fonts[2], nil
	case "BoldItalic":
		return fonts[3], nil
	default:
		return nil, fmt.Errorf("unknown font variant %q", variant)
	}
}

// FallbackFace registers a host font without requiring a dependency on its module.
type FallbackFace struct {
	Family string
	Source FaceSource
}

type faceKey struct {
	size         int
	family       string
	bold, italic bool
	source       int // 0: page source; -1: Go fonts; -2: ntfonts; positive: host fallback index + 1.
}

func (l *layout) fontKey(s style) faceKey {
	k := faceKey{size: s.size, family: s.family, bold: s.bold, italic: s.italic}
	if s.mono && (k.family == "" || k.family == "Go Mono") {
		k.family = cmp.Or(l.opts.FontFamilies.Monospace, "Go Mono")
	} else if k.family == "" && l.opts.FontFamilies.Default != "" {
		k.family = l.opts.FontFamilies.Default
	}
	return k
}
func (l *layout) cssFamilies(k faceKey) []faceKey {
	families := strings.Split(k.family, ",")
	if len(families) > 8 {
		families = families[:8]
	}
	out := make([]faceKey, 0, len(families))
	for _, family := range families {
		family = strings.Trim(strings.TrimSpace(family), "'\"")
		switch strings.ToLower(family) {
		case "serif":
			family = cmp.Or(l.opts.FontFamilies.Serif, "Literata")
		case "sans-serif":
			family = cmp.Or(l.opts.FontFamilies.SansSerif, "IBM Plex Sans")
		case "monospace":
			family = cmp.Or(l.opts.FontFamilies.Monospace, "IBM Plex Mono")
		case "":
			continue
		}
		for _, known := range ntfonts.Families() {
			if strings.EqualFold(family, known) {
				family = known
				break
			}
		}
		f := k
		f.family = family
		if family == "Go" || family == "Go Mono" {
			f.source = -1
		}
		out = append(out, f)
	}
	return out
}
func (l *layout) face(s style) font.Face {
	k := l.fontKey(s)
	if l.primary == nil {
		l.primary = map[faceKey]font.Face{}
	}
	if f := l.primary[k]; f != nil {
		return f
	}
	candidates := l.cssFamilies(k)
	goKey := k
	goKey.family = "Go"
	goKey.source = -1
	candidates = append(candidates, goKey)
	for _, candidate := range candidates {
		if f := l.loadFace(candidate, false); f != nil {
			if len(l.primary) < 128 {
				l.primary[k] = f
			}
			return f
		}
	}
	return basicfont.Face7x13
}

// loadFace shares the existing family, face and byte budgets with fallback.
// Exhaustion degrades to .notdef instead of failing an otherwise renderable page.
func (l *layout) loadFace(k faceKey, fallback bool) font.Face {
	if l.fonts == nil {
		l.fonts = map[string]*opentype.Font{}
	}
	if f := l.faces[k]; f != nil {
		return f
	}
	if l.fontStopped {
		return nil
	}
	if len(l.faces) >= 128 {
		if fallback {
			l.fontStopped = true
		} else {
			l.err = limitError("font faces")
		}
		return nil
	}
	variant := "Regular"
	if k.bold {
		variant = "Bold"
	}
	if k.italic {
		variant = "Italic"
		if k.bold {
			variant = "BoldItalic"
		}
	}
	key := fmt.Sprintf("%d/%s/%s", k.source, k.family, variant)
	parsed, known := l.fonts[key]
	if !known {
		if len(l.fonts) >= 64 {
			if fallback {
				l.fontStopped = true
			} else {
				l.err = limitError("font families")
			}
			return nil
		}
		source := l.opts.Fonts
		if source == nil {
			source = ntfonts.Embedded
		}
		if k.source == -1 {
			source = MinimalFonts
		}
		if k.source == -2 {
			source = ntfonts.Embedded
		}
		if k.source > 0 {
			if k.source > len(l.opts.FallbackFaces) {
				return nil
			}
			source = l.opts.FallbackFaces[k.source-1].Source
		}
		if source == nil {
			l.fonts[key] = nil
			return nil
		}
		data, err := source.Face(k.family, variant)
		if err != nil && variant != "Regular" {
			data, err = source.Face(k.family, "Regular")
		}
		if err != nil {
			l.fonts[key] = nil
			return nil
		}
		if len(data) > 4<<20 || l.fontBytes+len(data) > 16<<20 {
			if !fallback {
				l.err = limitError("font bytes")
			}
			l.fontStopped = true
			return nil
		}
		l.fontBytes += len(data)
		parsed, err = opentype.Parse(data)
		if err != nil {
			l.fonts[key] = nil
			if !fallback {
				l.err = fmt.Errorf("nthtml: font %s: %w", key, err)
			}
			return nil
		}
		l.fonts[key] = parsed
	}
	if parsed == nil {
		return nil
	}
	hinting := font.HintingFull
	if strings.Contains(strings.ToLower(k.family), "ahem") {
		hinting = font.HintingNone
	}
	f, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: float64(k.size), DPI: 72 * cmp.Or(l.scale, 1), Hinting: hinting})
	if err != nil {
		if !fallback {
			l.err = err
		}
		return nil
	}
	if l.faces == nil {
		l.faces = map[faceKey]font.Face{}
	}
	l.faces[k] = f
	if l.faceInfo == nil {
		l.faceInfo = map[font.Face]faceRecord{}
	}
	l.faceInfo[f] = faceRecord{key: k, parsed: parsed}
	return f
}
