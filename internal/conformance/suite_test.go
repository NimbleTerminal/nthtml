package conformance

import (
	"context"
	"errors"
	"image"
	"image/color"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPinnedSuite(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Run(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range report.Results {
		t.Logf("%s %s: %s", r.Status, r.Path, r.Reason)
		if r.Unexpected {
			t.Errorf("expected %s, got %s: %s", r.Expected, r.Status, r.Path)
		}
	}
	totals := map[string]int{}
	for area, counts := range report.Areas {
		if area == "uncategorized" {
			t.Error("uncategorized corpus case")
		}
		for status, n := range counts {
			totals[status] += n
		}
	}
	for status, n := range report.Counts {
		if totals[status] != n {
			t.Errorf("area totals %s: %d != %d", status, totals[status], n)
		}
	}
	if len(report.Results) != len(s.Manifest.Cases) {
		t.Fatal("missing results")
	}
}

func TestTamperedFixture(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	for name := range s.Manifest.Files {
		data, err := fs.ReadFile(s.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: data}
	}
	files[s.Manifest.Cases[0].Path].Data = []byte("tampered")
	s.Files = files
	if err := s.Verify(); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("got %v", err)
	}
}

func TestReferences(t *testing.T) {
	refs, err := references("css/a/test.html", []byte(`<link rel="match" href="../ref.html"><link rel="mismatch" href="/common/ref.html">`))
	if err != nil || len(refs) != 2 || refs[0] != (Reference{"css/ref.html", "match"}) || refs[1] != (Reference{"common/ref.html", "mismatch"}) {
		t.Fatalf("%v %v", refs, err)
	}
	for _, input := range []string{`<link rel="match" href="https://example.com/ref">`, `<link rel="match" href="../../../ref">`, `<link rel="match" href="ref?x">`, `<link rel="match" href="ref#x">`, `<script></script>`, `<link rel="stylesheet" href="a.css">`, `<meta name="fuzzy" content="1;1">`, `<html class="reftest-wait">`, `<![CDATA[test]]>`} {
		if _, err := references("css/a.html", []byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}

func TestReferenceLogic(t *testing.T) {
	for _, tc := range []struct {
		comparisons []Comparison
		want        bool
	}{
		{nil, false}, {[]Comparison{{Relation: "match", Equal: true}}, true},
		{[]Comparison{{Relation: "match"}, {Relation: "match", Equal: true}}, true},
		{[]Comparison{{Relation: "match"}, {Relation: "mismatch"}}, false},
		{[]Comparison{{Relation: "mismatch"}}, true},
		{[]Comparison{{Relation: "match", Equal: true}, {Relation: "mismatch", Equal: true}}, false},
		{[]Comparison{{Relation: "unknown"}}, false},
	} {
		if got := Satisfies(tc.comparisons); got != tc.want {
			t.Errorf("%+v: got %v", tc.comparisons, got)
		}
	}
}

func TestCompare(t *testing.T) {
	a, b := image.NewRGBA(image.Rect(0, 0, 2, 2)), image.NewRGBA(image.Rect(0, 0, 2, 2))
	if n, d, _ := Compare(a, b); n != 0 || d != 0 {
		t.Fatalf("equal: %d %d", n, d)
	}
	b.SetRGBA(1, 1, color.RGBA{R: 20, A: 255})
	n, d, diff := Compare(a, b)
	if n != 1 || d != 255 || diff.RGBAAt(1, 1) != (color.RGBA{255, 0, 255, 255}) {
		t.Fatalf("changed: %d %d", n, d)
	}
	if n, _, _ := Compare(a, image.NewRGBA(image.Rect(0, 0, 1, 2))); n != 2 {
		t.Fatalf("bounds: %d", n)
	}
}

func TestCanceled(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Run(ctx, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
