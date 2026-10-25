package conformance

import (
	"fmt"
	"io/fs"
	"strings"

	"nimbleterminal.dev/ntfonts"
)

const ahemCSS = "fonts/ahem.css"
const ahemTTF = "fonts/Ahem.ttf"

type fixtureFonts struct{ ahem []byte }

func (f fixtureFonts) Face(family, variant string) ([]byte, error) {
	if strings.EqualFold(family, "Ahem") {
		return f.ahem, nil
	}
	return ntfonts.Embedded.Face(family, variant)
}
func (s Suite) fontSource(enabled bool) (fixtureFonts, error) {
	if !enabled {
		return fixtureFonts{}, nil
	}
	if s.Manifest.Files[ahemCSS] == "" || s.Manifest.Files[ahemTTF] == "" {
		return fixtureFonts{}, fmt.Errorf("ahem font and stylesheet must be pinned")
	}
	data, err := fs.ReadFile(s.Files, ahemTTF)
	if err != nil {
		return fixtureFonts{}, err
	}
	if len(data) > 1<<20 {
		return fixtureFonts{}, fmt.Errorf("ahem font too large")
	}
	return fixtureFonts{data}, nil
}
