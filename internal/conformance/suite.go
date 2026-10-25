// Package conformance runs an audited, pinned subset of WPT visual reference
// tests through nthtml. It is not the browser WPT runner or a certification.
package conformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"nimbleterminal.dev/nthtml/document"
)

//go:embed manifest.json testdata/wpt
var fixtures embed.FS

type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type Case struct {
	Assets   []string `json:"assets,omitempty"`
	Path     string   `json:"path"`
	Expected string   `json:"expected"`
	Reason   string   `json:"reason"`
}
type Manifest struct {
	Repository string            `json:"repository"`
	Revision   string            `json:"revision"`
	Viewport   Viewport          `json:"viewport"`
	Files      map[string]string `json:"files"`
	Cases      []Case            `json:"cases"`
}
type Suite struct {
	Manifest Manifest
	Files    fs.FS
}

func Load() (Suite, error) {
	var m Manifest
	data, e := fixtures.ReadFile("manifest.json")
	if e != nil {
		return Suite{}, e
	}
	if e = json.Unmarshal(data, &m); e != nil {
		return Suite{}, e
	}
	files, e := fs.Sub(fixtures, "testdata/wpt")
	if e != nil {
		return Suite{}, e
	}
	s := Suite{m, files}
	return s, s.Verify()
}

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (s Suite) Verify() error {
	m := s.Manifest
	if m.Repository != "https://github.com/web-platform-tests/wpt" || !revisionPattern.MatchString(m.Revision) {
		return fmt.Errorf("invalid WPT repository/revision")
	}
	if m.Viewport != (Viewport{800, 600}) {
		return fmt.Errorf("only the 800x600 viewport is supported")
	}
	if len(m.Cases) == 0 || len(m.Cases) > 256 || len(m.Files) > 1024 {
		return fmt.Errorf("invalid fixture count")
	}
	for name, want := range m.Files {
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return fmt.Errorf("unsafe fixture path %q", name)
		}
		data, e := fs.ReadFile(s.Files, name)
		if e != nil {
			return e
		}
		if len(data) > 1<<20 {
			return fmt.Errorf("fixture too large: %s", name)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			return fmt.Errorf("SHA-256 mismatch: %s", name)
		}
	}
	seen := map[string]bool{}
	for _, c := range m.Cases {
		if seen[c.Path] || m.Files[c.Path] == "" || strings.TrimSpace(c.Reason) == "" {
			return fmt.Errorf("invalid/duplicate case %q", c.Path)
		}
		seen[c.Path] = true
		if len(c.Assets) > 16 {
			return fmt.Errorf("too many assets: %s", c.Path)
		}
		assetSeen := map[string]bool{}
		for _, asset := range c.Assets {
			if m.Files[asset] == "" || assetSeen[asset] {
				return fmt.Errorf("invalid asset %q", asset)
			}
			assetSeen[asset] = true
		}
		if c.Expected != "PASS" && c.Expected != "FAIL" && c.Expected != "UNSUPPORTED" {
			return fmt.Errorf("invalid expectation: %s", c.Path)
		}
	}
	return nil
}

type Reference struct{ Path, Relation string }

func references(name string, data []byte, assets ...[]string) ([]Reference, error) {
	data, err := fixtureInput(context.Background(), name, data)
	if err != nil {
		return nil, err
	}
	z := html.NewTokenizer(bytes.NewReader(data))
	var refs []Reference
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				return nil, z.Err()
			}
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		t := z.Token()
		a := map[string]string{}
		for _, v := range t.Attr {
			a[v.Key] = v.Val
		}
		if t.Data == "script" || t.Data == "iframe" || t.Data == "object" {
			return nil, fmt.Errorf("unreviewed external/scripted prerequisite: %s", t.Data)
		}
		if t.Data == "img" {
			asset, err := fixturePath(name, a["src"])
			if err != nil {
				return nil, err
			}
			allowed := false
			if len(assets) > 0 {
				for _, candidate := range assets[0] {
					if asset == candidate {
						allowed = true
						break
					}
				}
			}
			if !allowed {
				return nil, fmt.Errorf("unreviewed image prerequisite: %s", asset)
			}
		}
		if t.Data == "meta" && (a["name"] == "fuzzy" || a["name"] == "viewport") {
			return nil, fmt.Errorf("unsupported comparison metadata: %s", a["name"])
		}
		for _, class := range strings.Fields(a["class"]) {
			if class == "reftest-wait" || class == "reftest-print" {
				return nil, fmt.Errorf("unsupported reftest mode: %s", class)
			}
		}
		if t.Data != "link" {
			continue
		}
		for _, rel := range strings.Fields(a["rel"]) {
			if rel == "stylesheet" {
				return nil, fmt.Errorf("external stylesheet prerequisite")
			}
			if rel != "match" && rel != "mismatch" {
				continue
			}
			resolved, e := fixturePath(name, a["href"])
			if e != nil {
				return nil, e
			}
			refs = append(refs, Reference{resolved, rel})
			if len(refs) > 16 {
				return nil, fmt.Errorf("too many references")
			}
		}
	}
	if bytes.Contains(data, []byte("<![CDATA[")) {
		return nil, fmt.Errorf("XHTML CDATA prerequisite")
	}
	return refs, nil
}

type Comparison struct {
	Reference            string `json:"reference"`
	Relation             string `json:"relation"`
	DifferentPixels      int    `json:"different_pixels"`
	MaxChannelDifference int    `json:"max_channel_difference"`
	Equal                bool   `json:"equal"`
	TestPNG              string `json:"test_png,omitempty"`
	ReferencePNG         string `json:"reference_png,omitempty"`
	DiffPNG              string `json:"diff_png,omitempty"`
}
type Result struct {
	Path        string       `json:"path"`
	URL         string       `json:"url"`
	Status      string       `json:"status"`
	Expected    string       `json:"expected"`
	Unexpected  bool         `json:"unexpected"`
	Reason      string       `json:"reason"`
	Comparisons []Comparison `json:"comparisons,omitempty"`
}
type Report struct {
	Repository string         `json:"repository"`
	Revision   string         `json:"revision"`
	Viewport   Viewport       `json:"viewport"`
	Note       string         `json:"note"`
	Counts     map[string]int `json:"counts"`
	Unexpected int            `json:"unexpected"`
	Results    []Result       `json:"results"`
}
type Options struct{ ArtifactsDir string }

func (s Suite) Run(ctx context.Context, o Options) (Report, error) {
	if err := s.Verify(); err != nil {
		return Report{}, err
	}
	report := Report{Repository: s.Manifest.Repository, Revision: s.Manifest.Revision, Viewport: s.Manifest.Viewport, Note: "Audited subset only; both test and reference use nthtml defaults. Not full WPT/browser conformance. Unsupported cases are not passes.", Counts: map[string]int{"PASS": 0, "FAIL": 0, "UNSUPPORTED": 0, "ERROR": 0}}
	for _, c := range s.Manifest.Cases {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		r := s.runCase(ctx, c, o)
		r.Unexpected = r.Status != r.Expected || r.Status == "ERROR"
		if r.Unexpected {
			report.Unexpected++
		}
		report.Counts[r.Status]++
		report.Results = append(report.Results, r)
	}
	return report, nil
}
func (s Suite) runCase(ctx context.Context, c Case, o Options) (r Result) {
	r = Result{Path: c.Path, URL: s.Manifest.Repository + "/blob/" + s.Manifest.Revision + "/" + c.Path, Expected: c.Expected, Reason: c.Reason}
	fail := func(err error) Result { r.Status = "ERROR"; r.Reason = err.Error(); return r }
	if c.Expected == "UNSUPPORTED" {
		r.Status = "UNSUPPORTED"
		return r
	}
	data, e := fs.ReadFile(s.Files, c.Path)
	if e != nil {
		return fail(e)
	}
	refs, e := references(c.Path, data, c.Assets)
	if e != nil {
		return fail(e)
	}
	if len(refs) == 0 {
		return fail(fmt.Errorf("no upstream match/mismatch references"))
	}
	img, e := s.render(ctx, c.Path, data, s.Manifest.Viewport, c.Assets)
	if e != nil {
		return fail(e)
	}
	for i, ref := range refs {
		if s.Manifest.Files[ref.Path] == "" {
			return fail(fmt.Errorf("reference is not pinned: %s", ref.Path))
		}
		data, e := fs.ReadFile(s.Files, ref.Path)
		if e != nil {
			return fail(e)
		}
		if _, e = references(ref.Path, data, c.Assets); e != nil {
			return fail(e)
		}
		reference, e := s.render(ctx, ref.Path, data, s.Manifest.Viewport, c.Assets)
		if e != nil {
			return fail(e)
		}
		different, delta, diff := Compare(img, reference)
		cmp := Comparison{Reference: ref.Path, Relation: ref.Relation, DifferentPixels: different, MaxChannelDifference: delta, Equal: different == 0}
		if o.ArtifactsDir != "" && ((ref.Relation == "match" && !cmp.Equal) || (ref.Relation == "mismatch" && cmp.Equal)) {
			sum := sha256.Sum256([]byte(c.Path))
			prefix := fmt.Sprintf("%x-%d", sum[:6], i)
			cmp.TestPNG = prefix + "-test.png"
			cmp.ReferencePNG = prefix + "-reference.png"
			cmp.DiffPNG = prefix + "-diff.png"
			for name, image := range map[string]image.Image{cmp.TestPNG: img, cmp.ReferencePNG: reference, cmp.DiffPNG: diff} {
				if e := writePNG(filepath.Join(o.ArtifactsDir, name), image); e != nil {
					return fail(e)
				}
			}
		}
		r.Comparisons = append(r.Comparisons, cmp)
	}
	r.Status = "FAIL"
	if Satisfies(r.Comparisons) {
		r.Status = "PASS"
	}
	return r
}

// Satisfies follows WPT's any-match AND all-mismatch rule.
func Satisfies(comparisons []Comparison) bool {
	if len(comparisons) == 0 {
		return false
	}
	hasMatch, matched := false, false
	for _, c := range comparisons {
		switch c.Relation {
		case "match":
			hasMatch = true
			matched = matched || c.Equal
		case "mismatch":
			if c.Equal {
				return false
			}
		default:
			return false
		}
	}
	return !hasMatch || matched
}
func (s Suite) render(ctx context.Context, name string, data []byte, v Viewport, assets []string) (*image.RGBA, error) {
	data, err := fixtureInput(ctx, name, data)
	if err != nil {
		return nil, err
	}
	d, e := document.Parse(ctx, bytes.NewReader(data), document.Limits{})
	if e != nil {
		return nil, e
	}
	images, e := s.images(ctx, name, d, assets)
	if e != nil {
		return nil, e
	}
	r, e := d.Render(ctx, document.Options{Width: v.Width, Images: images})
	if e != nil {
		return nil, e
	}
	if len(r.Warnings) > 0 {
		return nil, fmt.Errorf("render warnings: %s", strings.Join(r.Warnings, "; "))
	}
	// This subset has no scrolling assertions. Refuse an overflow rather than
	// hiding a difference below the comparison viewport.
	if r.Image.Bounds().Dy() > v.Height {
		return nil, fmt.Errorf("document exceeds reviewed viewport")
	}
	img := image.NewRGBA(image.Rect(0, 0, v.Width, v.Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(img, r.Image.Bounds(), r.Image, image.Point{}, draw.Over)
	return img, nil
}

// Compare returns exact differing-pixel count, maximum RGBA channel delta,
// and an opaque difference image (magenta where pixels differ).
func Compare(a, b *image.RGBA) (different, maxDelta int, diff *image.RGBA) {
	bounds := a.Bounds().Union(b.Bounds())
	diff = image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			aa, bb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			changed := aa != bb || !image.Pt(x, y).In(a.Bounds()) || !image.Pt(x, y).In(b.Bounds())
			if changed {
				different++
				diff.SetRGBA(x, y, color.RGBA{255, 0, 255, 255})
				for _, pair := range [][2]uint8{{aa.R, bb.R}, {aa.G, bb.G}, {aa.B, bb.B}, {aa.A, bb.A}} {
					d := int(pair[0]) - int(pair[1])
					if d < 0 {
						d = -d
					}
					maxDelta = max(maxDelta, d)
				}
			} else {
				diff.SetRGBA(x, y, color.RGBA{245, 245, 245, 255})
			}
		}
	}
	return
}
func writePNG(name string, img image.Image) error {
	if e := os.MkdirAll(filepath.Dir(name), 0755); e != nil {
		return e
	}
	f, e := os.Create(name)
	if e != nil {
		return e
	}
	e = png.Encode(f, img)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	return closeErr
}
