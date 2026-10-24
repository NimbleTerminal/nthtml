package document

import (
	"math"
	"strconv"
	"strings"
)

type lengthUnit uint8

const (
	lengthUnset lengthUnit = iota
	lengthPX
	lengthPercent
	lengthAuto
	lengthNone
)

type cssLength struct {
	value float64
	unit  lengthUnit
}

func px(n int) cssLength         { return cssLength{float64(n), lengthPX} }
func uniform(n int) [4]cssLength { return [4]cssLength{px(n), px(n), px(n), px(n)} }
func parseLength(s string, auto, none bool) (cssLength, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if auto && s == "auto" {
		return cssLength{unit: lengthAuto}, true
	}
	if none && s == "none" {
		return cssLength{unit: lengthNone}, true
	}
	unit := lengthPX
	if strings.HasSuffix(s, "%") {
		unit = lengthPercent
		s = strings.TrimSuffix(s, "%")
	} else {
		s = strings.TrimSuffix(s, "px")
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 16384 {
		return cssLength{}, false
	}
	return cssLength{v, unit}, true
}
func (l cssLength) resolve(base, ceiling int) int {
	v := l.value
	if l.unit == lengthPercent {
		if base < 0 {
			return 0
		}
		v = v * float64(base) / 100
	}
	if l.unit != lengthPX && l.unit != lengthPercent {
		return 0
	}
	return min(ceiling, max(0, int(v)))
}
func (l cssLength) definite(base int) bool {
	return l.unit == lengthPX || l.unit == lengthPercent && base >= 0
}

// Percent margins and padding (including vertical sides) resolve against the
// containing block's content width. Each side retains the existing 256px cap.
func resolveEdges(edges [4]cssLength, width int) [4]int {
	var out [4]int
	for i, l := range edges {
		out[i] = l.resolve(width, 256)
	}
	return out
}
