package document

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
	"unicode"
)

func listStart(n *node) int {
	if s := n.attrs["start"]; s != "" {
		return listNumber(s, 1)
	}
	if _, ok := n.attrs["reversed"]; ok && n.tag == "ol" {
		count := 0
		for _, c := range n.children {
			if c.tag == "li" {
				count++
			}
		}
		return max(1, count)
	}
	return 1
}

func listStep(n *node) int {
	if _, ok := n.attrs["reversed"]; ok && n.tag == "ol" {
		return -1
	}
	return 1
}

func listNumber(s string, fallback int) int {
	v, e := strconv.Atoi(s)
	if e != nil || v < -1000000 || v > 1000000 {
		return fallback
	}
	return v
}

func htmlListType(attr string) string {
	switch attr {
	case "1":
		return "decimal"
	case "a":
		return "lower-alpha"
	case "A":
		return "upper-alpha"
	case "i":
		return "lower-roman"
	case "I":
		return "upper-roman"
	}
	switch strings.ToLower(strings.TrimSpace(attr)) {
	case "disc", "circle", "square", "none", "decimal":
		return strings.ToLower(strings.TrimSpace(attr))
	}
	return ""
}

func resolveListStyleType(listNode, itemNode *node, itemStyle style) string {
	if itemStyle.listStyleType != "" {
		return itemStyle.listStyleType
	}
	if itemNode != nil {
		if t := htmlListType(itemNode.attrs["type"]); t != "" {
			return t
		}
	}
	if listNode != nil {
		if t := htmlListType(listNode.attrs["type"]); t != "" {
			return t
		}
		if listNode.tag == "ol" {
			return "decimal"
		}
		// Resolve only the UA fallback here, so inherited author styles and
		// HTML type attributes above retain precedence over nesting defaults.
		depth := 0
		for p := listNode.parent; p != nil; p = p.parent {
			switch p.tag {
			case "ul", "ol", "menu", "dir":
				depth++
			}
			if depth >= 2 {
				return "square"
			}
		}
		if depth == 1 {
			return "circle"
		}
	}
	return "disc"
}

func alphaMarker(n int, upper bool) string {
	if n <= 0 {
		return strconv.Itoa(n)
	}
	var buf []byte
	for n > 0 && len(buf) < 8 {
		n--
		ch := byte('a' + (n % 26))
		if upper {
			ch = byte('A' + (n % 26))
		}
		buf = append([]byte{ch}, buf...)
		n /= 26
	}
	return string(buf)
}

func romanMarker(n int, upper bool) string {
	if n <= 0 || n >= 4000 {
		return strconv.Itoa(n)
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}
	var out strings.Builder
	for i, v := range vals {
		for n >= v {
			out.WriteString(syms[i])
			n -= v
		}
	}
	if upper {
		return strings.ToUpper(out.String())
	}
	return out.String()
}

func formatListMarker(kind string, index int) string {
	switch kind {
	case "none":
		return ""
	case "disc":
		return "•"
	case "circle":
		return "◦"
	case "square":
		return "▪"
	case "decimal":
		return strconv.Itoa(index) + "."
	case "decimal-leading-zero":
		if index >= 0 && index < 10 {
			return fmt.Sprintf("0%d.", index)
		}
		return strconv.Itoa(index) + "."
	case "lower-alpha", "lower-latin":
		return alphaMarker(index, false) + "."
	case "upper-alpha", "upper-latin":
		return alphaMarker(index, true) + "."
	case "lower-roman":
		return romanMarker(index, false) + "."
	case "upper-roman":
		return romanMarker(index, true) + "."
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
