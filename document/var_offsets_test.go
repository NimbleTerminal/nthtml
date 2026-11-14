package document

import (
	"context"
	"strings"
	"testing"
)

func TestSupportsVariableByteOffsets(t *testing.T) {
	// Reconstructed inputs reproduce the exact slice bounds in crashes #25/#26.
	for _, query := range []string{"(:\xff\xffvar())", "(:\xff\xff var())", "(color:ȺȺȺvar())"} {
		if Supports(query) {
			t.Errorf("accepted malformed condition %q", query)
		}
		source := `<style>@supports ` + query + ` {p{color:red}}</style><p>safe</p>`
		if _, err := Parse(context.Background(), strings.NewReader(source), Limits{}); err != nil {
			t.Fatalf("parse %q: %v", query, err)
		}
		if _, err := CSSRuleCount(`@supports ` + query + ` {p{color:red}}`); err != nil {
			t.Fatalf("count %q: %v", query, err)
		}
	}
	if !Supports("(color:VAR(--x, red))") {
		t.Fatal("ASCII-insensitive var() rejected")
	}
}

func TestVariableSubstitutionPreservesOriginalBytes(t *testing.T) {
	for _, prefix := range []string{"\xff\xff", "\xff\xff ", "ȺȺȺ", "İ ", "xvar(--ignored) "} {
		value, ok := substituteVars(prefix+"VaR(--missing, red)", nil, 0)
		if !ok || value != prefix+"red" {
			t.Errorf("prefix %q: %q, %v", prefix, value, ok)
		}
	}
	if containsVar("xvar(--ignored)") {
		t.Fatal("matched var inside an identifier")
	}
}
