package document

import (
	"errors"
	"strings"
	"testing"
)

func TestCSSRuleCount(t *testing.T) {
	for _, tc := range []struct {
		css  string
		want int
	}{
		{`a,b { & p {} }`, 4},
		{`@layer base {a{}} @supports (display:grid){b,c{}} @supports (position-area:block-end){d{}}`, 3},
		{`@media print {a,b{}}`, 2},
		{"\uFEFFa{}", 1},
	} {
		got, err := CSSRuleCount(tc.css)
		if err != nil || got != tc.want {
			t.Errorf("%q: %d, %v; want %d", tc.css, got, err, tc.want)
		}
	}
	for _, css := range []string{strings.Repeat("a{}", MaxLimits.Rules+1), strings.Repeat(" ", MaxLimits.Bytes+1)} {
		if _, err := CSSRuleCount(css); !errors.Is(err, ErrLimit) {
			t.Errorf("limit: %v", err)
		}
	}
}
