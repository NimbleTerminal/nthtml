package conformance

import (
	"context"
	_ "embed"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nimbleterminal.dev/nthtml/document"
)

//go:embed typeset-baseline.json
var typesetBaseline []byte

func TestTypesetGeometryDoesNotNormalizeDifferences(t *testing.T) {
	boxes := []document.Box{{Tag: "section", SourceID: "p", Bounds: image.Rect(8, 16, 104, 38)}}
	chr := chromiumElement{Found: true, ID: "p", Tag: "p", Rect: Rect{X: 8, Y: 16, Width: 96, Height: 18}, DefaultFontAutoHeight: true, InIntroFlow: true}
	if got := matchTypesetGeometry(DifferentialTarget{Selector: "#p"}, chr, boxes); got.Status != "MISMATCH" || got.DH != 4 {
		t.Fatalf("normalized font difference: %+v", got)
	}
	chr.ID = ""
	if got := matchTypesetGeometry(DifferentialTarget{Selector: "p"}, chr, boxes); got.Status != "UNAVAILABLE" {
		t.Fatalf("guessed DOM tag identity: %+v", got)
	}
	chr.ID = "missing"
	if got := matchTypesetGeometry(DifferentialTarget{Selector: "#missing"}, chr, boxes); got.Status != "UNAVAILABLE" {
		t.Fatalf("invented source geometry: %+v", got)
	}
}

func TestTypesetStrictResults(t *testing.T) {
	good := DifferentialResult{Status: "GEOMETRY_MATCH", PixelStatus: "EXACT"}
	tests := []struct {
		name   string
		change func(*DifferentialResult)
	}{
		{"pixels", func(r *DifferentialResult) { r.DifferentPixels = 1 }},
		{"unmeasured pixels", func(r *DifferentialResult) { r.PixelStatus = "UNAVAILABLE" }},
		{"dimensions", func(r *DifferentialResult) { r.Document.DH = 1 }},
		{"partial geometry", func(r *DifferentialResult) { r.UnavailableElements = 1 }},
		{"unsupported", func(r *DifferentialResult) { r.Status = "UNSUPPORTED" }},
		{"missing geometry", func(r *DifferentialResult) { r.Status = "GEOMETRY_UNAVAILABLE" }},
	}
	if DifferentialFailed(DifferentialReport{Backend: "typeset", Results: []DifferentialResult{good}}, true) {
		t.Fatal("exact comparison failed")
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := good
			tc.change(&r)
			report := DifferentialReport{Backend: "typeset", Results: []DifferentialResult{r}}
			if !DifferentialFailed(report, true) {
				t.Fatal("false pass")
			}
			if DifferentialFailed(report, false) {
				t.Fatal("diagnostic mode must allow measured discrepancies")
			}
		})
	}
	if !DifferentialFailed(DifferentialReport{Backend: "typeset"}, true) {
		t.Fatal("empty run passed")
	}
	if !DifferentialFailed(DifferentialReport{Counts: map[string]int{"ERROR": 1}}, false) {
		t.Fatal("infrastructure error passed")
	}
}

func TestDifferentialBackendIsolation(t *testing.T) {
	dir := t.TempDir()
	if err := WriteDifferentialReport(dir, DifferentialReport{Backend: "native"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteDifferentialReport(dir, DifferentialReport{Backend: "typeset"}); err == nil {
		t.Fatal("overwrote native report")
	}
	after, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("native report changed")
	}
	if _, err := (Suite{}).RunDifferential(context.Background(), DifferentialOptions{Backend: "typeset", ArtifactsDir: dir}); err == nil || !strings.Contains(err.Error(), "separate directory") {
		t.Fatalf("run did not reject directory before rendering: %v", err)
	}
	if _, err := differentialBackend("unknown"); err == nil {
		t.Fatal("unknown renderer accepted")
	}
}

func TestTypesetDifferentialSuite(t *testing.T) {
	browser, err := FindChromium()
	if err != nil {
		t.Skipf("Chromium differential explicitly skipped: %v", err)
	}
	suite, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	report, err := suite.RunDifferential(ctx, DifferentialOptions{Backend: "typeset", ChromiumPath: browser, ArtifactsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var baseline struct {
		Revision string
		Cases    map[string]string
	}
	if err := json.Unmarshal(typesetBaseline, &baseline); err != nil {
		t.Fatal(err)
	}
	if report.Backend != "typeset" || report.Revision != baseline.Revision || len(report.Results) != len(baseline.Cases) {
		t.Fatal("wrong renderer/revision/case count")
	}
	for _, r := range report.Results {
		if r.Status != baseline.Cases[r.Path] {
			t.Errorf("review baseline change %s: %s -> %s (%s)", r.Path, baseline.Cases[r.Path], r.Status, r.Summary)
		}
		if r.Backend != "typeset" {
			t.Fatal("missing backend provenance")
		}
		if r.Status == "UNSUPPORTED" {
			if r.PixelStatus != "UNAVAILABLE" || r.NTHTMLPNG != "" {
				t.Fatal("unsupported prerequisite reported pixels")
			}
			continue
		}
		if r.PixelStatus == "UNAVAILABLE" || r.TotalPixels != 800*600 {
			t.Fatal("missing pixel comparison")
		}
		if !strings.Contains(r.NTHTMLPNG, "-typeset-") {
			t.Fatal("shared native artifact names")
		}
	}
	if !DifferentialFailed(report, true) {
		t.Fatal("known gaps passed strict mode")
	}
	if err := WriteDifferentialReport(dir, report); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "typeset") || !strings.Contains(string(page), "UNSUPPORTED") {
		t.Fatal("report hides backend/prerequisites")
	}
}
