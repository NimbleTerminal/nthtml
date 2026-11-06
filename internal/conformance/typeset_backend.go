package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"os"
	"path/filepath"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml/document"
	htmltypeset "nimbleterminal.dev/nthtml/typeset"
	"nimbleterminal.dev/nttypeset"
)

func differentialBackend(name string) (string, error) {
	if name == "" {
		return "native", nil
	}
	if name != "native" && name != "typeset" {
		return "", fmt.Errorf("unknown differential backend %q (native or typeset)", name)
	}
	return name, nil
}

// Prevent a run from replacing another renderer's reviewed artifacts. Old
// reports without backend metadata belong to the native renderer.
func checkDifferentialDirectory(dir, backend string) error {
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var previous struct {
		Backend string `json:"backend"`
	}
	if err = json.Unmarshal(data, &previous); err != nil {
		return fmt.Errorf("existing report: %w", err)
	}
	old, err := differentialBackend(previous.Backend)
	if err != nil {
		return err
	}
	if old != backend {
		return fmt.Errorf("output directory contains %s results; use a separate directory for %s", old, backend)
	}
	return nil
}

// No CSS reset, HTML rewriting, default margin subtraction, or font substitution
// is applied to make the adapter match the reference. Source boxes are envelopes,
// not a claim that the adapter exposes CSS border boxes for every element.
func (s Suite) renderTypesetWithBoxes(ctx context.Context, c Case) (*image.RGBA, *document.Result, error) {
	raw, err := fs.ReadFile(s.Files, c.Path)
	if err != nil {
		return nil, nil, err
	}
	data, err := fixtureInput(ctx, c.Path, raw)
	if err != nil {
		return nil, nil, err
	}
	doc, err := document.Parse(ctx, bytes.NewReader(data), document.Limits{})
	if err != nil {
		return nil, nil, err
	}
	assets, err := s.images(ctx, c.Path, doc, c.Assets)
	if err != nil {
		return nil, nil, err
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		return nil, nil, err
	}
	v := s.Manifest.Viewport
	result, lay, _, err := htmltypeset.RenderResult(ctx, eng, doc, v.Width, htmltypeset.Options{Document: document.Options{Width: v.Width, Images: assets, NoImage: true}})
	if err != nil {
		return nil, nil, err
	}
	if lay.Extent() > result.Image.Bounds().Dy() {
		return nil, nil, fmt.Errorf("typeset document exceeds complete-raster height; refusing clipped document measurement")
	}
	img := image.NewRGBA(image.Rect(0, 0, v.Width, v.Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(img, result.Image.Bounds(), result.Image, image.Point{}, draw.Over)
	return img, result, nil
}

func matchTypesetGeometry(target DifferentialTarget, chr chromiumElement, boxes []document.Box) GeometryComparison {
	// No inferred tag-order matching: the bridge does not retain every DOM tag.
	if chr.Found && chr.ID == "" {
		return GeometryComparison{Selector: target.Selector, Label: target.Label, Status: "UNAVAILABLE", Note: "typeset geometry requires a source ID; DOM tag-order inference is disabled"}
	}
	chr.InIntroFlow, chr.DefaultFontAutoHeight = false, false
	got := matchElementGeometry(target, chr, boxes, 0)
	if got.NTHTML != nil && !chr.DisplayNone {
		got.Note = "adapter source envelope vs DOM border box; " + got.Note
	}
	return got
}

// Strict typeset runs require complete geometry, matching document dimensions,
// and exact pixels. A geometry-only MATCH is never a general conformance pass.
func DifferentialFailed(report DifferentialReport, strict bool) bool {
	if report.Counts["ERROR"] > 0 {
		return true
	}
	if !strict {
		return false
	}
	if report.Backend != "typeset" {
		return report.Counts["GEOMETRY_MISMATCH"] > 0
	}
	if len(report.Results) == 0 {
		return true
	}
	for _, r := range report.Results {
		if r.Status != "GEOMETRY_MATCH" || r.PixelStatus != "EXACT" || r.UnavailableElements > 0 || r.Document.DW != 0 || r.Document.DH != 0 || r.DifferentPixels != 0 {
			return true
		}
	}
	return false
}
