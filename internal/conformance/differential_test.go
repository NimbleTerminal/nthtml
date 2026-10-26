package conformance

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nimbleterminal.dev/nthtml/document"
)

func TestMatchElementGeometryUnit(t *testing.T) {
	boxes := []document.Box{
		{Tag: "html", Bounds: image.Rect(0, 0, 800, 300)},
		{Tag: "body", Bounds: image.Rect(8, 16, 792, 292)},
		{Tag: "p", Bounds: image.Rect(8, 16, 792, 38)},
		{Tag: "div", Anchor: "box1", SourceID: "box1", Bounds: image.Rect(8, 54, 104, 150)},
		{Tag: "div", Anchor: "box2", SourceID: "box2", Bounds: image.Rect(8, 154, 208, 250)},
		{Tag: "anchor", Anchor: "inline1", SourceID: "inline1", Bounds: image.Rect(120, 54, 121, 70)},
		{Tag: "text", SourceID: "inline1", Bounds: image.Rect(120, 54, 152, 70)},
	}

	// 1. Exact ID match with intro <p> flow offset (+4px).
	g1 := matchElementGeometry(
		DifferentialTarget{Selector: "#box1"},
		chromiumElement{
			Selector:    "#box1",
			Found:       true,
			Tag:         "div",
			ID:          "box1",
			InIntroFlow: true,
			Rect:        Rect{X: 8, Y: 50, Width: 96, Height: 96},
		},
		boxes,
		4,
	)
	if g1.Status != "MATCH" || !g1.SizeMatch || g1.DW != 0 || g1.DH != 0 || g1.DY != 4 {
		t.Fatalf("expected MATCH with flow offset, got %+v", g1)
	}

	// 2. Mismatch in width on #box2.
	g2 := matchElementGeometry(
		DifferentialTarget{Selector: "#box2"},
		chromiumElement{
			Selector:    "#box2",
			Found:       true,
			Tag:         "div",
			ID:          "box2",
			InIntroFlow: true,
			Rect:        Rect{X: 8, Y: 150, Width: 96, Height: 96},
		},
		boxes,
		4,
	)
	if g2.Status != "MISMATCH" || g2.DW != 104 {
		t.Fatalf("expected MISMATCH on width, got %+v", g2)
	}

	// 3. Inline element with ID matched via SourceID union.
	g3 := matchElementGeometry(
		DifferentialTarget{Selector: "#inline1"},
		chromiumElement{
			Selector: "#inline1",
			Found:    true,
			Tag:      "span",
			ID:       "inline1",
			Rect:     Rect{X: 120, Y: 54, Width: 32, Height: 16},
		},
		boxes,
		0,
	)
	if g3.Status != "MATCH" || g3.NTHTML == nil || g3.NTHTML.Width != 32 {
		t.Fatalf("expected inline ID MATCH, got %+v", g3)
	}

	// 4. Inline element without ID -> UNAVAILABLE.
	g4 := matchElementGeometry(
		DifferentialTarget{Selector: "span"},
		chromiumElement{
			Selector: "span",
			Found:    true,
			Tag:      "span",
			TagIndex: 0,
			TagCount: 1,
			Rect:     Rect{X: 8, Y: 50, Width: 40, Height: 16},
		},
		boxes,
		0,
	)
	if g4.Status != "UNAVAILABLE" {
		t.Fatalf("expected UNAVAILABLE for un-ID'd span, got %+v", g4)
	}

	// 5. Ambiguous block tag count without ID -> UNAVAILABLE.
	g5 := matchElementGeometry(
		DifferentialTarget{Selector: "div"},
		chromiumElement{
			Selector: "div",
			Found:    true,
			Tag:      "div",
			TagIndex: 0,
			TagCount: 3, // 3 in DOM vs 2 in boxes
			Rect:     Rect{X: 8, Y: 50, Width: 96, Height: 96},
		},
		boxes,
		0,
	)
	if g5.Status != "UNAVAILABLE" {
		t.Fatalf("expected UNAVAILABLE for ambiguous div count, got %+v", g5)
	}

	// 6. display:none suppression -> MATCH.
	g6 := matchElementGeometry(
		DifferentialTarget{Selector: "#hidden"},
		chromiumElement{
			Selector:    "#hidden",
			Found:       true,
			Tag:         "div",
			ID:          "hidden",
			DisplayNone: true,
		},
		boxes,
		0,
	)
	if g6.Status != "MATCH" || g6.NTHTML == nil || g6.NTHTML.Width != 0 {
		t.Fatalf("expected MATCH for display:none, got %+v", g6)
	}
}

func TestFindChromiumUnavailableEnv(t *testing.T) {
	t.Setenv("NTHTML_CHROMIUM", "/nonexistent/path/to/chromium-binary")
	_, err := FindChromium()
	if !errors.Is(err, ErrChromiumUnavailable) {
		t.Fatalf("expected ErrChromiumUnavailable, got %v", err)
	}
}

func TestDifferentialSuite(t *testing.T) {
	chrPath, err := FindChromium()
	if err != nil {
		t.Skipf("skipping Chromium differential suite: %v", err)
	}
	suite, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	outDir := t.TempDir()
	report, err := suite.RunDifferential(ctx, DifferentialOptions{
		ArtifactsDir: outDir,
		ChromiumPath: chrPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteDifferentialReport(outDir, report); err != nil {
		t.Fatal(err)
	}
	if report.Counts["ERROR"] > 0 {
		for _, r := range report.Results {
			if r.Status == "ERROR" {
				t.Errorf("ERROR %s: %s", r.Path, r.Summary)
			}
		}
	}
	if len(report.Results) != len(DefaultDifferentialCases()) {
		t.Fatalf("got %d results, want %d", len(report.Results), len(DefaultDifferentialCases()))
	}
	for _, r := range report.Results {
		t.Logf("%-18s %-52s %s (doc nt=%dx%d chr=%dx%d)",
			r.Status, r.Path, r.Summary,
			r.Document.NTHTML.Width, r.Document.NTHTML.Height,
			r.Document.Chromium.Width, r.Document.Chromium.Height)
		for _, g := range r.Geometry {
			if g.NTHTML != nil && g.Chromium != nil {
				t.Logf("    %-12s %-16s nt=(%d,%d %dx%d) chr=(%d,%d %dx%d) d=(%+d,%+d,%+d,%+d) %s",
					g.Status, g.Selector,
					g.NTHTML.X, g.NTHTML.Y, g.NTHTML.Width, g.NTHTML.Height,
					g.Chromium.X, g.Chromium.Y, g.Chromium.Width, g.Chromium.Height,
					g.DX, g.DY, g.DW, g.DH, g.Note)
			} else {
				t.Logf("    %-12s %-16s %s", g.Status, g.Selector, g.Note)
			}
		}
	}
	for _, name := range []string{"index.html", "report.json"} {
		info, err := os.Stat(filepath.Join(outDir, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("missing or empty %s: %v", name, err)
		}
	}
	htmlBytes, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlBytes), "GEOMETRY_MATCH") {
		t.Fatal("index.html missing GEOMETRY_MATCH summary")
	}
}
