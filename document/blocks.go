package document

import (
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
	"unicode"
)

func listStart(n *node) int { return listNumber(n.attrs["start"], 1) }
func listNumber(s string, fallback int) int {
	v, e := strconv.Atoi(s)
	if e != nil || v < -1000000 || v > 1000000 {
		return fallback
	}
	return v
}
func listMarker(n *node, index int) string {
	if n.tag == "ol" {
		return strconv.Itoa(index) + "."
	}
	return "•"
}

// Preserve line breaks and spaces, normalize CRLF, expand tabs to four columns,
// and remove terminal controls. A leading newline follows HTML pre conventions.
func preTextAt(s string, col int) (string, int) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var out strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			out.WriteRune(r)
			col = 0
		case '\t':
			spaces := 4 - col%4
			out.WriteString(strings.Repeat(" ", spaces))
			col += spaces
		default:
			if !unicode.IsControl(r) {
				out.WriteRune(r)
				col += ansi.StringWidth(string(r))
			}
		}
	}
	return out.String(), col
}
