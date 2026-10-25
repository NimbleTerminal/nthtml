package document

import (
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
type faceKey struct {
	size         int
	family       string
	bold, italic bool
}

func (l *layout) face(s style) font.Face {
	k := faceKey{s.size, s.family, s.bold, s.italic}
	if s.mono && k.family == "" {
		k.family = "Go Mono"
	}
	if f := l.faces[k]; f != nil {
		return f
	}
	if len(l.faces) >= 128 {
		l.err = limitError("font faces")
		return basicfont.Face7x13
	}
	variant := "Regular"
	if s.bold {
		variant = "Bold"
	}
	if s.italic {
		variant = "Italic"
		if s.bold {
			variant = "BoldItalic"
		}
	}
	source := l.opts.Fonts
	if source == nil {
		source = ntfonts.Embedded
	}
	var parsed *opentype.Font
	families := strings.Split(k.family, ",")
	if len(families) > 8 {
		families = families[:8]
	}
	families = append(families, "Go")
	for _, family := range families {
		family = strings.Trim(strings.TrimSpace(family), "'\"")
		switch strings.ToLower(family) {
		case "serif":
			family = "Literata"
		case "sans-serif":
			family = "IBM Plex Sans"
		case "monospace":
			family = "IBM Plex Mono"
		case "":
			continue
		}
		for _, known := range ntfonts.Families() {
			if strings.EqualFold(family, known) {
				family = known
				break
			}
		}
		key := family + "/" + variant
		if t, ok := l.fonts[key]; ok {
			if t != nil {
				parsed = t
				break
			}
			continue
		}
		if len(l.fonts) >= 64 {
			l.err = limitError("font families")
			return basicfont.Face7x13
		}
		var data []byte
		if family == "Go" || family == "Go Mono" {
			fonts := [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF}
			if family == "Go Mono" {
				fonts = [][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF}
			}
			i := 0
			if s.bold {
				i++
			}
			if s.italic {
				i += 2
			}
			data = fonts[i]
		} else {
			var err error
			data, err = source.Face(family, variant)
			if err != nil {
				l.fonts[key] = nil
				continue
			}
		}
		if len(data) > 4<<20 || l.fontBytes+len(data) > 16<<20 {
			l.err = limitError("font bytes")
			return basicfont.Face7x13
		}
		l.fontBytes += len(data)
		var err error
		parsed, err = opentype.Parse(data)
		if err != nil {
			l.err = fmt.Errorf("nthtml: font %s: %w", key, err)
			return basicfont.Face7x13
		}
		l.fonts[key] = parsed
		break
	}
	hinting := font.HintingFull
	if strings.Contains(strings.ToLower(k.family), "ahem") {
		hinting = font.HintingNone
	}
	f, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: float64(s.size), DPI: 72, Hinting: hinting})
	if err != nil {
		l.err = err
		return basicfont.Face7x13
	}
	l.faces[k] = f
	return f
}
