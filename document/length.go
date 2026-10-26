package document

import (
	"math"
	"regexp"
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
	lengthEM
	lengthREM
)

type cssLength struct {
	value float64
	unit  lengthUnit
}

func px(n int) cssLength         { return cssLength{float64(n), lengthPX} }
func uniform(n int) [4]cssLength { return [4]cssLength{px(n), px(n), px(n), px(n)} }

func htmlLength(s string) (cssLength, bool) {
	l, ok := parseLength(s, false, false)
	if !ok || (l.unit != lengthPX && l.unit != lengthPercent) {
		return cssLength{}, false
	}
	return l, true
}

func parseLength(s string, auto, none bool) (cssLength, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if auto && s == "auto" {
		return cssLength{unit: lengthAuto}, true
	}
	if none && s == "none" {
		return cssLength{unit: lengthNone}, true
	}
	unit := lengthPX
	factor := 1.0
	for _, u := range []struct {
		suffix string
		unit   lengthUnit
		factor float64
	}{
		{"rem", lengthREM, 1}, {"em", lengthEM, 1}, {"px", lengthPX, 1}, {"pt", lengthPX, 96.0 / 72}, {"pc", lengthPX, 16}, {"in", lengthPX, 96}, {"cm", lengthPX, 96.0 / 2.54}, {"mm", lengthPX, 96.0 / 25.4}, {"q", lengthPX, 96.0 / 101.6}, {"%", lengthPercent, 1},
	} {
		if strings.HasSuffix(s, u.suffix) {
			s = strings.TrimSuffix(s, u.suffix)
			unit, factor = u.unit, u.factor
			break
		}
	}
	if len(s) > 64 || !cssNumber.MatchString(s) {
		return cssLength{}, false
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 16384 {
		return cssLength{}, false
	}
	return cssLength{v * factor, unit}, true
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
	return min(ceiling, max(0, int(v+1e-9)))
}
func (l cssLength) definite(base int) bool {
	return l.unit == lengthPX || l.unit == lengthPercent && base >= 0
}

func (l cssLength) resolveSigned(base, ceiling int) int {
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
	if v < 0 {
		return max(-ceiling, int(v-1e-9))
	}
	return min(ceiling, int(v+1e-9))
}

// Percent margins and padding (including vertical sides) resolve against the
// containing block's content width. Each side retains the existing ±256px cap.
func resolveEdges(edges [4]cssLength, width int) [4]int {
	var out [4]int
	for i, l := range edges {
		out[i] = l.resolveSigned(width, 256)
	}
	return out
}

var cssNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// Convert font-relative units at computed-style time, leaving percentages for
// layout. Inherited absolute line heights must not scale with a child's font.
func (l cssLength) computed(fontSize, rootSize int) cssLength {
	switch l.unit {
	case lengthEM:
		l.value *= float64(fontSize)
		l.unit = lengthPX
	case lengthREM:
		l.value *= float64(rootSize)
		l.unit = lengthPX
	}
	return l
}
func (l cssLength) absolute() bool {
	return l.unit == lengthPX || l.unit == lengthEM || l.unit == lengthREM
}
