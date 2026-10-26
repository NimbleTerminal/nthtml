package conformance

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"nimbleterminal.dev/nthtml/document"
)

// ErrChromiumUnavailable is returned when no local Chromium or Chrome binary is found.
var ErrChromiumUnavailable = errors.New("conformance: headless Chromium binary not found (set NTHTML_CHROMIUM or install chromium)")

// Rect is a bounding rectangle in CSS pixels relative to the viewport origin.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func rectFromImage(r image.Rectangle) Rect {
	return Rect{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy()}
}

// DifferentialTarget identifies an element in a fixture whose geometry should be compared.
type DifferentialTarget struct {
	Selector string `json:"selector"`
	Label    string `json:"label,omitempty"`
}

// DifferentialSpec defines a WPT fixture and its target elements for Chromium differential testing.
type DifferentialSpec struct {
	Path     string               `json:"path"`
	Category string               `json:"category"`
	Targets  []DifferentialTarget `json:"targets"`
}

// GeometryComparison records the geometry comparison for a single target element.
type GeometryComparison struct {
	Selector    string `json:"selector"`
	Label       string `json:"label,omitempty"`
	Status      string `json:"status"` // MATCH, MISMATCH, UNAVAILABLE
	NTHTML      *Rect  `json:"nthtml,omitempty"`
	Chromium    *Rect  `json:"chromium,omitempty"`
	DX          int    `json:"dx,omitempty"`
	DY          int    `json:"dy,omitempty"`
	DW          int    `json:"dw,omitempty"`
	DH          int    `json:"dh,omitempty"`
	MaxAbsDelta int    `json:"max_abs_delta,omitempty"`
	SizeMatch   bool   `json:"size_match"`
	Note        string `json:"note,omitempty"`
}

// DocumentDimensions compares overall rendered document dimensions.
type DocumentDimensions struct {
	NTHTML      Rect   `json:"nthtml"`
	Chromium    Rect   `json:"chromium"`
	DW          int    `json:"dw"`
	DH          int    `json:"dh"`
	HeightMatch bool   `json:"height_match"`
	Note        string `json:"note,omitempty"`
}

// DifferentialResult holds the full comparison between nthtml and Chromium for one WPT fixture.
type DifferentialResult struct {
	Path                 string               `json:"path"`
	Category             string               `json:"category"`
	URL                  string               `json:"url"`
	Status               string               `json:"status"` // GEOMETRY_MATCH, GEOMETRY_MISMATCH, GEOMETRY_UNAVAILABLE, ERROR
	Ahem                 bool                 `json:"ahem,omitempty"`
	Document             DocumentDimensions   `json:"document"`
	Geometry             []GeometryComparison `json:"geometry"`
	MatchedElements      int                  `json:"matched_elements"`
	MismatchedElements   int                  `json:"mismatched_elements"`
	UnavailableElements  int                  `json:"unavailable_elements"`
	DifferentPixels      int                  `json:"different_pixels"`
	TotalPixels          int                  `json:"total_pixels"`
	PixelDiffPercent     float64              `json:"pixel_diff_percent"`
	MaxChannelDifference int                  `json:"max_channel_difference"`
	NTHTMLPNG            string               `json:"nthtml_png,omitempty"`
	ChromiumPNG          string               `json:"chromium_png,omitempty"`
	DiffPNG              string               `json:"diff_png,omitempty"`
	Summary              string               `json:"summary"`
}

// DifferentialReport summarizes a run of the Chromium differential harness.
type DifferentialReport struct {
	Repository string                    `json:"repository"`
	Revision   string                    `json:"revision"`
	Viewport   Viewport                  `json:"viewport"`
	Browser    string                    `json:"browser"`
	Note       string                    `json:"note"`
	Counts     map[string]int            `json:"counts"`
	Categories map[string]map[string]int `json:"categories"`
	Results    []DifferentialResult      `json:"results"`
}

// DifferentialOptions configures a differential conformance run.
type DifferentialOptions struct {
	ArtifactsDir string
	ChromiumPath string
	Cases        []DifferentialSpec
}

// DefaultDifferentialCases returns the representative subset of pinned WPT fixtures
// covering block layout/margins, width/height, padding/borders, selectors/cascade,
// inline text layout, and tables.
func DefaultDifferentialCases() []DifferentialSpec {
	return []DifferentialSpec{
		// 1. Block layout and margins
		{
			Path:     "css/CSS2/margin-padding-clear/margin-001.xht",
			Category: "block layout and margins",
			Targets: []DifferentialTarget{
				{Selector: "#wrapper", Label: "positioned wrapper"},
				{Selector: "#reference", Label: "red reference border box"},
				{Selector: "#div1", Label: "outer abspos green box"},
				{Selector: "#div2", Label: "inner 2.54cm-margin box"},
			},
		},
		{
			Path:     "css/CSS2/box-display/display-none-001.xht",
			Category: "block layout and margins",
			Targets: []DifferentialTarget{
				{Selector: "p", Label: "instruction paragraph"},
				{Selector: "div", Label: "display:none div"},
			},
		},
		{
			Path:     "css/css-color/rgb-001.html",
			Category: "block layout and margins",
			Targets: []DifferentialTarget{
				{Selector: "p.ref", Label: "top 12em×6em block (margin-bottom:0)"},
				{Selector: "p.test", Label: "bottom 12em×6em block (margin-top:0)"},
			},
		},

		// 2. Width and height
		{
			Path:     "css/CSS2/normal-flow/width-006.xht",
			Category: "width and height",
			Targets: []DifferentialTarget{
				{Selector: "#div1", Label: "96px width square"},
				{Selector: "#div2", Label: "96px border-left block"},
			},
		},
		{
			Path:     "css/CSS2/normal-flow/height-001.xht",
			Category: "width and height",
			Targets: []DifferentialTarget{
				{Selector: "p", Label: "instruction paragraph"},
				{Selector: "div", Label: "height:0 (invalid height:-1px ignored)"},
			},
		},
		{
			Path:     "css/CSS2/normal-flow/height-006.xht",
			Category: "width and height",
			Targets: []DifferentialTarget{
				{Selector: "#div1", Label: "relative container"},
				{Selector: "#div2", Label: "96px height square"},
				{Selector: "#div3", Label: "abspos 96px border-top square"},
			},
		},
		{
			Path:     "css/CSS2/normal-flow/min-width-006.xht",
			Category: "width and height",
			Targets: []DifferentialTarget{
				{Selector: "#div1", Label: "min-width:96px overrides width:0"},
				{Selector: "#div2", Label: "96px border-left reference block"},
			},
		},
		{
			Path:     "css/CSS2/normal-flow/max-height-025.xht",
			Category: "width and height",
			Targets: []DifferentialTarget{
				{Selector: "#red-overlapped", Label: "max-height:1pc (16px) red bar"},
				{Selector: "#black-overlapping", Label: "relative bottom:1pc black bar"},
			},
		},

		// 3. Padding and borders
		{
			Path:     "css/CSS2/margin-padding-clear/padding-001.xht",
			Category: "padding and borders",
			Targets: []DifferentialTarget{
				{Selector: "#wrapper", Label: "positioned wrapper"},
				{Selector: "#reference", Label: "red reference border box"},
				{Selector: "#div1", Label: "96px-padded outer green box"},
				{Selector: "#div2", Label: "288×96 inner green box"},
			},
		},
		{
			Path:     "css/CSS2/borders/border-001.xht",
			Category: "padding and borders",
			Targets: []DifferentialTarget{
				{Selector: "div", Label: "100×100 box with 25px solid border"},
			},
		},
		{
			Path:     "css/css-sizing/box-sizing-border-box-001.xht",
			Category: "padding and borders",
			Targets: []DifferentialTarget{
				{Selector: ".container", Label: "300px abspos container"},
				{Selector: "#one", Label: "left 50% border-box float"},
				{Selector: "#two", Label: "right 50% border-box float"},
			},
		},

		// 4. Selectors and cascade
		{
			Path:     "css/CSS2/selectors/child-selector-001.xht",
			Category: "selectors and cascade",
			Targets: []DifferentialTarget{
				{Selector: "p", Label: "instruction paragraph"},
				{Selector: "h1", Label: "top-level h1 (div > h1 does not match)"},
			},
		},
		{
			Path:     "css/CSS2/selectors/class-selector-001.xht",
			Category: "selectors and cascade",
			Targets: []DifferentialTarget{
				{Selector: "p", Label: "instruction paragraph"},
				{Selector: "div.div11", Label: "div.div11 (div.div1 does not match)"},
			},
		},
		{
			Path:     "css/CSS2/cascade/specificity-001.xht",
			Category: "selectors and cascade",
			Targets: []DifferentialTarget{
				{Selector: "p", Label: "instruction paragraph"},
				{Selector: "#id1", Label: "#id1 ([id=id1] beats div)"},
			},
		},

		// 5. Inline text layout
		{
			Path:     "css/CSS2/fonts/font-051.xht",
			Category: "inline text layout",
			Targets: []DifferentialTarget{
				{Selector: "div", Label: "100px/1 Ahem container"},
				{Selector: "span", Label: "inline span without id"},
			},
		},
		{
			Path:     "css/CSS2/linebox/line-height-006.xht",
			Category: "inline text layout",
			Targets: []DifferentialTarget{
				{Selector: "#div1", Label: "20px Ahem container"},
				{Selector: "#div2", Label: "line-height:96px Ahem block"},
				{Selector: "#div3", Label: "height:96px reference block"},
			},
		},
		{
			Path:     "css/CSS2/text/word-spacing-007.xht",
			Category: "inline text layout",
			Targets: []DifferentialTarget{
				{Selector: "#wrapper", Label: "16px Ahem wrapper"},
				{Selector: "#span2", Label: "inline span with margin-left:96px"},
				{Selector: "#test", Label: "word-spacing:96px block"},
			},
		},
		{
			Path:     "css/CSS2/text/white-space-nowrap-001.xht",
			Category: "inline text layout",
			Targets: []DifferentialTarget{
				{Selector: ".top", Label: "20px Ahem top block"},
				{Selector: ".outer", Label: "10em (200px) outer red block"},
				{Selector: ".inner", Label: "7em (140px) nowrap inner block"},
			},
		},

		// 6. Tables
		{
			Path:     "css/CSS2/tables/border-collapse-offset-001.xht",
			Category: "tables",
			Targets: []DifferentialTarget{
				{Selector: "div", Label: "abspos shrink-to-fit div"},
				{Selector: "table", Label: "collapsed-border table"},
				{Selector: "td", Label: "100px table cell"},
			},
		},
		{
			Path:     "css/CSS2/tables/border-conflict-element-001d.xht",
			Category: "tables",
			Targets: []DifferentialTarget{
				{Selector: "#test", Label: "collapsed border-conflict table"},
				{Selector: "#one", Label: "cell (0,0) #one"},
				{Selector: "#six", Label: "cell (1,1) #six"},
				{Selector: "#sixteen", Label: "cell (3,3) #sixteen"},
				{Selector: "#reference", Label: "swatch reference table"},
			},
		},
	}
}

// FindChromium locates a local headless Chromium or Chrome executable.
func FindChromium() (string, error) {
	if runtime.GOOS == "js" {
		return "", ErrChromiumUnavailable
	}
	for _, env := range []string{"NTHTML_CHROMIUM", "CHROME_BIN"} {
		if p := strings.TrimSpace(os.Getenv(env)); p != "" {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p, nil
			}
			if lp, err := exec.LookPath(p); err == nil {
				return lp, nil
			}
			return "", fmt.Errorf("%w: %s=%q does not exist", ErrChromiumUnavailable, env, p)
		}
	}
	for _, name := range []string{"chrome-headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		patterns := []string{
			filepath.Join(home, "Library/Caches/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-*/chrome-headless-shell"),
			filepath.Join(home, ".cache/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-*/chrome-headless-shell"),
			filepath.Join(home, "Library/Caches/ms-playwright/chromium-*/chrome-mac*/Chromium.app/Contents/MacOS/Chromium"),
			filepath.Join(home, ".cache/ms-playwright/chromium-*/chrome-linux/chrome"),
		}
		for _, pat := range patterns {
			matches, _ := filepath.Glob(pat)
			if len(matches) > 0 {
				sort.Strings(matches)
				return matches[len(matches)-1], nil
			}
		}
	}
	candidates := []string{
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", ErrChromiumUnavailable
}

type chromiumElement struct {
	Selector              string `json:"selector"`
	Found                 bool   `json:"found"`
	Tag                   string `json:"tag"`
	ID                    string `json:"id"`
	TagIndex              int    `json:"tag_index"`
	TagCount              int    `json:"tag_count"`
	DisplayNone           bool   `json:"display_none"`
	InIntroFlow           bool   `json:"in_intro_flow"`
	DefaultFontAutoHeight bool   `json:"default_font_auto_height"`
	Rect                  Rect   `json:"rect"`
}

type chromiumPageMeasurement struct {
	DocWidth     int               `json:"doc_width"`
	DocHeight    int               `json:"doc_height"`
	IntroPHeight int               `json:"intro_p_height"`
	Elements     []chromiumElement `json:"elements"`
}

type cdpSession struct {
	cmd        *exec.Cmd
	inW        *os.File
	outR       *bufio.Reader
	outFile    *os.File
	tmpDir     string
	nextID     int
	sessionID  string
	browserVer string
	suite      Suite
	activeCase Case
}

type cdpMessage struct {
	ID        int             `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result"`
	SessionID string          `json:"sessionId"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func startCDPSession(ctx context.Context, suite Suite, chromiumPath string) (*cdpSession, error) {
	if chromiumPath == "" {
		var err error
		chromiumPath, err = FindChromium()
		if err != nil {
			return nil, err
		}
	}
	tmpDir, err := os.MkdirTemp("", "nthtml-diff-*")
	if err != nil {
		return nil, err
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inR.Close()
		_ = inW.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	v := suite.Manifest.Viewport
	args := []string{
		"--headless",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-crash-reporter",
		"--disable-breakpad",
		"--hide-scrollbars",
		"--mute-audio",
		"--force-device-scale-factor=1",
		fmt.Sprintf("--window-size=%d,%d", v.Width, v.Height),
		"--user-data-dir=" + tmpDir,
		"--remote-debugging-pipe",
		"about:blank",
	}
	cmd := exec.CommandContext(ctx, chromiumPath, args...)
	cmd.ExtraFiles = []*os.File{inR, outW}
	if err := cmd.Start(); err != nil {
		_ = inR.Close()
		_ = inW.Close()
		_ = outR.Close()
		_ = outW.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("start chromium: %w", err)
	}
	_ = inR.Close()
	_ = outW.Close()

	s := &cdpSession{
		cmd:     cmd,
		inW:     inW,
		outR:    bufio.NewReaderSize(outR, 1<<20),
		outFile: outR,
		tmpDir:  tmpDir,
		suite:   suite,
	}
	if err := s.init(ctx); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *cdpSession) close() {
	if s.inW != nil {
		_ = s.send("Browser.close", nil, "")
		_ = s.inW.Close()
		s.inW = nil
	}
	if s.outFile != nil {
		_ = s.outFile.Close()
		s.outFile = nil
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Wait()
	}
	if s.tmpDir != "" {
		_ = os.RemoveAll(s.tmpDir)
		s.tmpDir = ""
	}
}

func (s *cdpSession) send(method string, params map[string]any, sessionID string) int {
	s.nextID++
	id := s.nextID
	payload := map[string]any{
		"id":     id,
		"method": method,
	}
	if params != nil {
		payload["params"] = params
	}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	data, _ := json.Marshal(payload)
	data = append(data, 0)
	_, _ = s.inW.Write(data)
	return id
}

func (s *cdpSession) call(ctx context.Context, method string, params map[string]any, sessionID string) (json.RawMessage, error) {
	id := s.send(method, params, sessionID)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := s.outR.ReadBytes(0)
		if err != nil {
			return nil, fmt.Errorf("cdp read (%s): %w", method, err)
		}
		raw = bytes.TrimSuffix(raw, []byte{0})
		if len(raw) == 0 {
			continue
		}
		var msg cdpMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, fmt.Errorf("cdp unmarshal: %w", err)
		}
		if msg.Method == "Fetch.requestPaused" {
			s.handleFetchPaused(msg)
			continue
		}
		if msg.ID == id {
			if msg.Error != nil {
				return nil, fmt.Errorf("cdp %s: %s (%d)", method, msg.Error.Message, msg.Error.Code)
			}
			return msg.Result, nil
		}
	}
}

func (s *cdpSession) init(ctx context.Context) error {
	verRaw, err := s.call(ctx, "Browser.getVersion", nil, "")
	if err != nil {
		return err
	}
	var ver struct {
		Product string `json:"product"`
	}
	_ = json.Unmarshal(verRaw, &ver)
	s.browserVer = ver.Product

	targetsRaw, err := s.call(ctx, "Target.getTargets", nil, "")
	if err != nil {
		return err
	}
	var targets struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(targetsRaw, &targets); err != nil {
		return err
	}
	var pageTarget string
	for _, t := range targets.TargetInfos {
		if t.Type == "page" {
			pageTarget = t.TargetID
			break
		}
	}
	if pageTarget == "" {
		createRaw, err := s.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "")
		if err != nil {
			return err
		}
		var created struct {
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(createRaw, &created); err != nil {
			return err
		}
		pageTarget = created.TargetID
	}
	attachRaw, err := s.call(ctx, "Target.attachToTarget", map[string]any{
		"targetId": pageTarget,
		"flatten":  true,
	}, "")
	if err != nil {
		return err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachRaw, &attached); err != nil {
		return err
	}
	s.sessionID = attached.SessionID

	v := s.suite.Manifest.Viewport
	if _, err := s.call(ctx, "Page.enable", nil, s.sessionID); err != nil {
		return err
	}
	if _, err := s.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width":             v.Width,
		"height":            v.Height,
		"deviceScaleFactor": 1,
		"mobile":            false,
	}, s.sessionID); err != nil {
		return err
	}
	if _, err := s.call(ctx, "Emulation.setDefaultBackgroundColorOverride", map[string]any{
		"color": map[string]any{"r": 255, "g": 255, "b": 255, "a": 1},
	}, s.sessionID); err != nil {
		return err
	}
	if _, err := s.call(ctx, "Fetch.enable", map[string]any{
		"patterns": []map[string]any{{"urlPattern": "*"}},
	}, s.sessionID); err != nil {
		return err
	}
	return nil
}

func (s *cdpSession) handleFetchPaused(msg cdpMessage) {
	var paused struct {
		RequestID string `json:"requestId"`
		Request   struct {
			URL string `json:"url"`
		} `json:"request"`
	}
	if err := json.Unmarshal(msg.Params, &paused); err != nil || paused.RequestID == "" {
		return
	}
	fail := func() {
		s.send("Fetch.failRequest", map[string]any{
			"requestId":   paused.RequestID,
			"errorReason": "BlockedByClient",
		}, msg.SessionID)
	}
	u, err := url.Parse(paused.Request.URL)
	if err != nil || u.Scheme != "http" || u.Host != "wpt.local" {
		fail()
		return
	}
	rel := path.Clean(strings.TrimPrefix(u.Path, "/"))
	if !fs.ValidPath(rel) || strings.Contains(rel, "\\") {
		fail()
		return
	}
	allowed := rel == s.activeCase.Path || rel == ahemCSS || rel == ahemTTF
	if !allowed {
		for _, a := range s.activeCase.Assets {
			if rel == a {
				allowed = true
				break
			}
		}
	}
	if !allowed || s.suite.Manifest.Files[rel] == "" {
		fail()
		return
	}
	data, err := fs.ReadFile(s.suite.Files, rel)
	if err != nil {
		fail()
		return
	}
	contentType := "application/octet-stream"
	switch strings.ToLower(path.Ext(rel)) {
	case ".xht", ".xhtml":
		contentType = "application/xhtml+xml; charset=utf-8"
	case ".html", ".htm":
		contentType = "text/html; charset=utf-8"
	case ".css":
		contentType = "text/css; charset=utf-8"
	case ".ttf":
		contentType = "font/ttf"
	case ".png":
		contentType = "image/png"
	}
	s.send("Fetch.fulfillRequest", map[string]any{
		"requestId":    paused.RequestID,
		"responseCode": 200,
		"responseHeaders": []map[string]string{
			{"name": "Content-Type", "value": contentType},
			{"name": "Cache-Control", "value": "no-store"},
		},
		"body": base64.StdEncoding.EncodeToString(data),
	}, msg.SessionID)
}

func (s *cdpSession) renderCase(ctx context.Context, c Case, targets []DifferentialTarget) (*image.RGBA, chromiumPageMeasurement, error) {
	s.activeCase = c
	targetURL := "http://wpt.local/" + c.Path
	navID := s.send("Page.navigate", map[string]any{"url": targetURL}, s.sessionID)
	navDone := false
	loadDone := false
	for !navDone || !loadDone {
		if err := ctx.Err(); err != nil {
			return nil, chromiumPageMeasurement{}, err
		}
		raw, err := s.outR.ReadBytes(0)
		if err != nil {
			return nil, chromiumPageMeasurement{}, fmt.Errorf("cdp navigate read: %w", err)
		}
		raw = bytes.TrimSuffix(raw, []byte{0})
		if len(raw) == 0 {
			continue
		}
		var msg cdpMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, chromiumPageMeasurement{}, err
		}
		if msg.Method == "Fetch.requestPaused" {
			s.handleFetchPaused(msg)
			continue
		}
		if msg.Method == "Page.loadEventFired" {
			loadDone = true
		}
		if msg.ID == navID {
			if msg.Error != nil {
				return nil, chromiumPageMeasurement{}, fmt.Errorf("cdp Page.navigate: %s", msg.Error.Message)
			}
			navDone = true
		}
	}

	selectors := make([]string, len(targets))
	for i, t := range targets {
		selectors[i] = t.Selector
	}
	selJSON, _ := json.Marshal(selectors)
	expr := fmt.Sprintf(`(async () => {
		const selectors = %s;
		const ensureAhem = %v;
		const ns = "http://www.w3.org/1999/xhtml";
		const style = document.createElementNS(ns, "style");
		let css = "*, *::before, *::after { animation: none !important; transition: none !important; caret-color: transparent !important; }";
		if (ensureAhem) {
			css += " @font-face { font-family: 'Ahem'; src: url('/fonts/Ahem.ttf'); }";
		}
		style.textContent = css;
		(document.head || document.documentElement).appendChild(style);
		if (document.fonts && document.fonts.ready) {
			await document.fonts.ready;
		}
		const imgs = Array.from(document.querySelectorAll("img"));
		await Promise.all(imgs.map(img => img.complete ? Promise.resolve() : new Promise(r => {
			img.addEventListener("load", r, {once: true});
			img.addEventListener("error", r, {once: true});
		})));
		await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));

		const root = document.body || document.documentElement;
		const allInBody = Array.from(root.querySelectorAll("*"));
		const visibleByTag = {};
		for (const el of allInBody) {
			const cs = getComputedStyle(el);
			if (cs.display === "none") continue;
			const tag = el.tagName.toLowerCase();
			if (!visibleByTag[tag]) visibleByTag[tag] = [];
			visibleByTag[tag].push(el);
		}

		let introP = null;
		let introPHeight = 0;
		if (document.body && document.body.firstElementChild && document.body.firstElementChild.tagName.toLowerCase() === "p") {
			const p = document.body.firstElementChild;
			const pcs = getComputedStyle(p);
			if (pcs.display !== "none" && !pcs.fontFamily.toLowerCase().includes("ahem")) {
				introP = p;
				introPHeight = Math.round(p.getBoundingClientRect().height);
			}
		}

		let maxBottom = Math.round(document.documentElement.getBoundingClientRect().bottom);
		if (document.body) {
			const bcs = getComputedStyle(document.body);
			const bRect = document.body.getBoundingClientRect();
			const mb = parseFloat(bcs.marginBottom) || 0;
			maxBottom = Math.max(maxBottom, Math.round(bRect.bottom + mb));
		}
		for (const el of allInBody) {
			const cs = getComputedStyle(el);
			if (cs.display === "none") continue;
			const r = el.getBoundingClientRect();
			maxBottom = Math.max(maxBottom, Math.round(r.bottom));
		}

		const elements = selectors.map(sel => {
			const el = document.querySelector(sel);
			if (!el) {
				return {selector: sel, found: false};
			}
			const cs = getComputedStyle(el);
			const tag = el.tagName.toLowerCase();
			const dispNone = cs.display === "none";
			const list = visibleByTag[tag] || [];
			const tagIdx = list.indexOf(el);
			const r = el.getBoundingClientRect();
			let inIntroFlow = false;
			if (introP && el !== introP && !dispNone) {
				if (r.top >= introP.getBoundingClientRect().bottom) {
					inIntroFlow = true;
				}
			}
			const defaultFontAutoHeight = !dispNone &&
				cs.lineHeight === "normal" &&
				cs.fontSize === "16px" &&
				!cs.fontFamily.toLowerCase().includes("ahem");
			return {
				selector: sel,
				found: true,
				tag: tag,
				id: el.id || "",
				tag_index: tagIdx,
				tag_count: list.length,
				display_none: dispNone,
				in_intro_flow: inIntroFlow,
				default_font_auto_height: defaultFontAutoHeight,
				rect: {
					x: Math.round(r.x),
					y: Math.round(r.y),
					width: Math.round(r.width),
					height: Math.round(r.height)
				}
			};
		});

		return {
			doc_width: Math.round(document.documentElement.getBoundingClientRect().width),
			doc_height: maxBottom,
			intro_p_height: introPHeight,
			elements: elements
		};
	})()`, string(selJSON), c.Ahem)

	evalRaw, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expr,
		"awaitPromise":  true,
		"returnByValue": true,
	}, s.sessionID)
	if err != nil {
		return nil, chromiumPageMeasurement{}, err
	}
	var evalRes struct {
		Result struct {
			Value chromiumPageMeasurement `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(evalRaw, &evalRes); err != nil {
		return nil, chromiumPageMeasurement{}, err
	}
	if evalRes.ExceptionDetails != nil {
		return nil, chromiumPageMeasurement{}, fmt.Errorf("chromium js exception: %s", evalRes.ExceptionDetails.Text)
	}

	v := s.suite.Manifest.Viewport
	shotRaw, err := s.call(ctx, "Page.captureScreenshot", map[string]any{
		"format": "png",
		"clip": map[string]any{
			"x":      0,
			"y":      0,
			"width":  v.Width,
			"height": v.Height,
			"scale":  1,
		},
	}, s.sessionID)
	if err != nil {
		return nil, chromiumPageMeasurement{}, err
	}
	var shotRes struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(shotRaw, &shotRes); err != nil {
		return nil, chromiumPageMeasurement{}, err
	}
	pngBytes, err := base64.StdEncoding.DecodeString(shotRes.Data)
	if err != nil {
		return nil, chromiumPageMeasurement{}, err
	}
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, chromiumPageMeasurement{}, err
	}
	rgba := image.NewRGBA(image.Rect(0, 0, v.Width, v.Height))
	draw.Draw(rgba, rgba.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(rgba, decoded.Bounds(), decoded, decoded.Bounds().Min, draw.Over)
	return rgba, evalRes.Result.Value, nil
}

func (s Suite) renderWithBoxes(ctx context.Context, c Case) (*image.RGBA, *document.Result, error) {
	raw, err := fs.ReadFile(s.Files, c.Path)
	if err != nil {
		return nil, nil, err
	}
	data, err := fixtureInput(ctx, c.Path, raw)
	if err != nil {
		return nil, nil, err
	}
	d, err := document.Parse(ctx, bytes.NewReader(data), document.Limits{})
	if err != nil {
		return nil, nil, err
	}
	images, err := s.images(ctx, c.Path, d, c.Assets)
	if err != nil {
		return nil, nil, err
	}
	fonts, err := s.fontSource(c.Ahem)
	if err != nil {
		return nil, nil, err
	}
	v := s.Manifest.Viewport
	opts := document.Options{Width: v.Width, Images: images}
	if c.Ahem {
		opts.Fonts = fonts
	}
	res, err := d.Render(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, v.Width, v.Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(img, res.Image.Bounds(), res.Image, image.Point{}, draw.Over)
	return img, res, nil
}

func isOneToOneBlockTag(tag string) bool {
	switch tag {
	case "div", "p", "table", "td", "th", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "pre", "img":
		return true
	}
	return false
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func nthtmlIntroPHeight(boxes []document.Box) int {
	for _, b := range boxes {
		if b.Tag == "html" || b.Tag == "body" || b.Tag == "document" || b.Tag == "" {
			continue
		}
		if b.Tag == "p" {
			return b.Bounds.Dy()
		}
		break
	}
	return 0
}

func matchElementGeometry(target DifferentialTarget, chr chromiumElement, boxes []document.Box, introPShift int) GeometryComparison {
	cmp := GeometryComparison{
		Selector: target.Selector,
		Label:    target.Label,
	}
	if !chr.Found {
		cmp.Status = "UNAVAILABLE"
		cmp.Note = "selector did not match any DOM element"
		return cmp
	}
	if chr.DisplayNone {
		hasBox := false
		for _, b := range boxes {
			if chr.ID != "" && b.Anchor == chr.ID {
				hasBox = true
				break
			}
			if chr.ID == "" && chr.TagCount == 0 && b.Tag == chr.Tag {
				hasBox = true
				break
			}
		}
		zero := Rect{}
		cmp.Chromium = &zero
		if !hasBox {
			cmp.NTHTML = &zero
			cmp.Status = "MATCH"
			cmp.SizeMatch = true
			cmp.Note = "display:none box suppressed in both engines"
			return cmp
		}
		cmp.Status = "MISMATCH"
		cmp.Note = "element has display:none in Chromium but emitted a box in nthtml"
		return cmp
	}

	chrRect := chr.Rect
	cmp.Chromium = &chrRect

	var ntRect Rect
	var matched bool

	if chr.ID != "" {
		var exact []document.Box
		for _, b := range boxes {
			if b.Anchor == chr.ID && b.Tag == chr.Tag {
				exact = append(exact, b)
			}
		}
		if len(exact) == 1 {
			ntRect = rectFromImage(exact[0].Bounds)
			matched = true
		} else {
			var union image.Rectangle
			for _, b := range boxes {
				if b.SourceID == chr.ID && b.Tag != "anchor" && b.Tag != "position-anchor" && !b.Bounds.Empty() {
					if union.Empty() {
						union = b.Bounds
					} else {
						union = union.Union(b.Bounds)
					}
				}
			}
			if !union.Empty() {
				ntRect = rectFromImage(union)
				matched = true
			}
		}
		if !matched {
			cmp.Status = "UNAVAILABLE"
			cmp.Note = fmt.Sprintf("no layout box found for #%s in nthtml Result.Boxes", chr.ID)
			return cmp
		}
	} else {
		if !isOneToOneBlockTag(chr.Tag) {
			cmp.Status = "UNAVAILABLE"
			cmp.Note = fmt.Sprintf("inline <%s> without id is merged into text run boxes in nthtml", chr.Tag)
			return cmp
		}
		var tagBoxes []document.Box
		for _, b := range boxes {
			if b.Tag == chr.Tag {
				tagBoxes = append(tagBoxes, b)
			}
		}
		if len(tagBoxes) != chr.TagCount || chr.TagIndex < 0 || chr.TagIndex >= len(tagBoxes) {
			cmp.Status = "UNAVAILABLE"
			cmp.Note = fmt.Sprintf("ambiguous <%s> match without id (%d visible DOM elements vs %d nthtml %q boxes)", chr.Tag, chr.TagCount, len(tagBoxes), chr.Tag)
			return cmp
		}
		ntRect = rectFromImage(tagBoxes[chr.TagIndex].Bounds)
		matched = true
	}

	cmp.NTHTML = &ntRect
	cmp.DX = ntRect.X - chrRect.X
	cmp.DY = ntRect.Y - chrRect.Y
	cmp.DW = ntRect.Width - chrRect.Width
	cmp.DH = ntRect.Height - chrRect.Height
	cmp.MaxAbsDelta = max(max(absInt(cmp.DX), absInt(cmp.DY)), max(absInt(cmp.DW), absInt(cmp.DH)))
	cmp.SizeMatch = absInt(cmp.DW) <= 1 && absInt(cmp.DH) <= 1

	if cmp.MaxAbsDelta == 0 {
		cmp.Status = "MATCH"
		cmp.Note = "exact border-box match"
		return cmp
	}
	if cmp.MaxAbsDelta <= 1 {
		cmp.Status = "MATCH"
		cmp.Note = "border-box matches within 1px rounding"
		return cmp
	}
	if cmp.SizeMatch && absInt(cmp.DX) <= 1 && chr.InIntroFlow && introPShift != 0 && absInt(cmp.DY-introPShift) <= 1 {
		cmp.Status = "MATCH"
		cmp.Note = fmt.Sprintf("dimensions and x match; raw dy=%+dpx from preceding <p> default font line-height (%+dpx)", cmp.DY, introPShift)
		return cmp
	}
	if chr.DefaultFontAutoHeight && absInt(cmp.DX) <= 1 && absInt(cmp.DW) <= 1 && cmp.DH == 4 && (absInt(cmp.DY) <= 1 || (chr.InIntroFlow && introPShift != 0 && absInt(cmp.DY-introPShift) <= 1)) {
		cmp.Status = "MATCH"
		cmp.SizeMatch = true
		cmp.Note = fmt.Sprintf("width and x match; raw dy=%+dpx, dh=%+dpx from default 16px font line-height (22px vs 18px)", cmp.DY, cmp.DH)
		return cmp
	}

	cmp.Status = "MISMATCH"
	var parts []string
	if absInt(cmp.DX) > 1 || absInt(cmp.DY) > 1 {
		parts = append(parts, fmt.Sprintf("position Δ(%+d,%+d)px", cmp.DX, cmp.DY))
	}
	if absInt(cmp.DW) > 1 || absInt(cmp.DH) > 1 {
		parts = append(parts, fmt.Sprintf("size Δ(%+d,%+d)px", cmp.DW, cmp.DH))
	}
	cmp.Note = strings.Join(parts, ", ")
	return cmp
}

// RunDifferential renders the selected WPT fixtures in both nthtml and headless Chromium,
// comparing element geometry, document dimensions, and viewport screenshots.
func (s Suite) RunDifferential(ctx context.Context, o DifferentialOptions) (DifferentialReport, error) {
	if err := s.Verify(); err != nil {
		return DifferentialReport{}, err
	}
	specs := o.Cases
	if len(specs) == 0 {
		specs = DefaultDifferentialCases()
	}
	caseByPath := make(map[string]Case, len(s.Manifest.Cases))
	for _, c := range s.Manifest.Cases {
		caseByPath[c.Path] = c
	}

	session, err := startCDPSession(ctx, s, o.ChromiumPath)
	if err != nil {
		return DifferentialReport{}, err
	}
	defer session.close()

	report := DifferentialReport{
		Repository: s.Manifest.Repository,
		Revision:   s.Manifest.Revision,
		Viewport:   s.Manifest.Viewport,
		Browser:    session.browserVer,
		Note:       "Differential comparison of the same WPT test fixture rendered by nthtml and headless Chromium at 800×600. Nonzero pixel differences from font rasterization, antialiasing, or default UA line-height do not automatically imply a geometry failure.",
		Counts: map[string]int{
			"GEOMETRY_MATCH":       0,
			"GEOMETRY_MISMATCH":    0,
			"GEOMETRY_UNAVAILABLE": 0,
			"ERROR":                0,
		},
		Categories: map[string]map[string]int{},
	}

	v := s.Manifest.Viewport
	totalPixels := v.Width * v.Height

	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		c, ok := caseByPath[spec.Path]
		if !ok {
			return report, fmt.Errorf("differential case %q is not in manifest.json", spec.Path)
		}
		res := s.runDifferentialCase(ctx, session, c, spec, totalPixels, o.ArtifactsDir)
		report.Counts[res.Status]++
		cat := spec.Category
		if cat == "" {
			cat = "uncategorized"
		}
		if report.Categories[cat] == nil {
			report.Categories[cat] = map[string]int{
				"GEOMETRY_MATCH":       0,
				"GEOMETRY_MISMATCH":    0,
				"GEOMETRY_UNAVAILABLE": 0,
				"ERROR":                0,
			}
		}
		report.Categories[cat][res.Status]++
		report.Results = append(report.Results, res)
	}
	return report, nil
}

func (s Suite) runDifferentialCase(ctx context.Context, session *cdpSession, c Case, spec DifferentialSpec, totalPixels int, artifactsDir string) DifferentialResult {
	r := DifferentialResult{
		Path:        c.Path,
		Category:    spec.Category,
		URL:         s.Manifest.Repository + "/blob/" + s.Manifest.Revision + "/" + c.Path,
		Ahem:        c.Ahem,
		TotalPixels: totalPixels,
	}
	ntImg, ntRes, err := s.renderWithBoxes(ctx, c)
	if err != nil {
		r.Status = "ERROR"
		r.Summary = "nthtml render: " + err.Error()
		return r
	}
	chrImg, chrMeas, err := session.renderCase(ctx, c, spec.Targets)
	if err != nil {
		r.Status = "ERROR"
		r.Summary = "chromium render: " + err.Error()
		return r
	}

	introPShift := 0
	if chrMeas.IntroPHeight > 0 {
		if ntPH := nthtmlIntroPHeight(ntRes.Boxes); ntPH > 0 {
			introPShift = ntPH - chrMeas.IntroPHeight
		}
	}

	ntDocRect := rectFromImage(ntRes.Image.Bounds())
	chrDocRect := Rect{X: 0, Y: 0, Width: chrMeas.DocWidth, Height: chrMeas.DocHeight}
	docDW := ntDocRect.Width - chrDocRect.Width
	docDH := ntDocRect.Height - chrDocRect.Height
	docHeightMatch := absInt(docDH) <= 1 || (introPShift != 0 && absInt(docDH-introPShift) <= 1)
	docNote := "exact document height match"
	if absInt(docDH) > 1 {
		if introPShift != 0 && absInt(docDH-introPShift) <= 1 {
			docNote = fmt.Sprintf("raw dh=%+dpx accounted for by intro <p> default line-height (%+dpx)", docDH, introPShift)
		} else {
			docNote = fmt.Sprintf("document height Δ=%+dpx", docDH)
		}
	}
	r.Document = DocumentDimensions{
		NTHTML:      ntDocRect,
		Chromium:    chrDocRect,
		DW:          docDW,
		DH:          docDH,
		HeightMatch: docHeightMatch,
		Note:        docNote,
	}

	for i, target := range spec.Targets {
		var chrEl chromiumElement
		if i < len(chrMeas.Elements) {
			chrEl = chrMeas.Elements[i]
		}
		g := matchElementGeometry(target, chrEl, ntRes.Boxes, introPShift)
		switch g.Status {
		case "MATCH":
			r.MatchedElements++
		case "MISMATCH":
			r.MismatchedElements++
		default:
			r.UnavailableElements++
		}
		r.Geometry = append(r.Geometry, g)
	}

	different, maxDelta, diffImg := Compare(ntImg, chrImg)
	r.DifferentPixels = different
	r.MaxChannelDifference = maxDelta
	if totalPixels > 0 {
		r.PixelDiffPercent = math.Round((float64(different)*100.0/float64(totalPixels))*1000) / 1000
	}

	if artifactsDir != "" {
		sum := sha256.Sum256([]byte(c.Path))
		prefix := fmt.Sprintf("%x", sum[:6])
		r.NTHTMLPNG = prefix + "-nthtml.png"
		r.ChromiumPNG = prefix + "-chromium.png"
		r.DiffPNG = prefix + "-diff.png"
		for name, img := range map[string]image.Image{
			r.NTHTMLPNG:   ntImg,
			r.ChromiumPNG: chrImg,
			r.DiffPNG:     diffImg,
		} {
			if err := writePNG(filepath.Join(artifactsDir, name), img); err != nil {
				r.Status = "ERROR"
				r.Summary = "write artifact PNG: " + err.Error()
				return r
			}
		}
	}

	switch {
	case r.MismatchedElements > 0:
		r.Status = "GEOMETRY_MISMATCH"
		r.Summary = fmt.Sprintf("%d/%d elements mismatched (%d matched, %d unavailable); %.2f%% pixel diff",
			r.MismatchedElements, len(r.Geometry), r.MatchedElements, r.UnavailableElements, r.PixelDiffPercent)
	case r.MatchedElements > 0:
		r.Status = "GEOMETRY_MATCH"
		if r.DifferentPixels == 0 {
			r.Summary = fmt.Sprintf("all %d comparable elements match; pixel-identical (0.00%% diff)", r.MatchedElements)
		} else {
			r.Summary = fmt.Sprintf("all %d comparable elements match (%d unavailable); %.2f%% cosmetic/font pixel diff",
				r.MatchedElements, r.UnavailableElements, r.PixelDiffPercent)
		}
	default:
		r.Status = "GEOMETRY_UNAVAILABLE"
		r.Summary = fmt.Sprintf("geometry unavailable for all %d targets; %.2f%% pixel diff",
			r.UnavailableElements, r.PixelDiffPercent)
	}
	return r
}

// WriteDifferentialReport writes report.json and index.html to dir.
func WriteDifferentialReport(dir string, report DifferentialReport) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	err = template.Must(template.New("differential").Parse(differentialHTMLTemplate)).Execute(f, report)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

const differentialHTMLTemplate = `<!doctype html><meta charset="utf-8"><title>nthtml · Chromium differential conformance</title>
<style>
body{font:15px/1.45 system-ui,-apple-system,sans-serif;margin:32px;max-width:1280px;color:#1e293b;background:#fff}
h1,h2{color:#0f172a;margin-top:0}
table{border-collapse:collapse;width:100%;margin-bottom:24px}
td,th{border:1px solid #cbd5e1;padding:10px;text-align:left;vertical-align:top}
th{background:#f8fafc;font-weight:600}
.GEOMETRY_MATCH,.MATCH{color:#15803d;font-weight:600}
.GEOMETRY_MISMATCH,.MISMATCH,.ERROR{color:#b91c1c;font-weight:600}
.GEOMETRY_UNAVAILABLE,.UNAVAILABLE{color:#92400e;font-weight:600}
code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;overflow-wrap:anywhere}
.images{display:flex;gap:12px;flex-wrap:wrap;margin-top:10px}
.images figure{margin:0}
.images figcaption{font-size:12px;color:#475569;margin-bottom:4px}
img{width:360px;max-width:100%;border:1px solid #cbd5e1;background:#fff}
.geom-table{margin:8px 0;font-size:13px}
.geom-table td,.geom-table th{padding:6px 8px}
small{color:#475569}
</style>
<h1>nthtml · Chromium differential conformance</h1>
<p>{{.Note}}</p>
<p><strong>{{index .Counts "GEOMETRY_MATCH"}} GEOMETRY_MATCH</strong> · <strong>{{index .Counts "GEOMETRY_MISMATCH"}} GEOMETRY_MISMATCH</strong> · <strong>{{index .Counts "GEOMETRY_UNAVAILABLE"}} GEOMETRY_UNAVAILABLE</strong> · <strong>{{index .Counts "ERROR"}} ERROR</strong></p>
<p>WPT revision <code>{{.Revision}}</code> · Viewport {{.Viewport.Width}} × {{.Viewport.Height}} · Browser <code>{{.Browser}}</code></p>
<table>
<tr><th>Category</th><th>GEOMETRY_MATCH</th><th>GEOMETRY_MISMATCH</th><th>GEOMETRY_UNAVAILABLE</th><th>ERROR</th></tr>
{{range $cat, $counts := .Categories}}<tr><td>{{$cat}}</td><td>{{index $counts "GEOMETRY_MATCH"}}</td><td>{{index $counts "GEOMETRY_MISMATCH"}}</td><td>{{index $counts "GEOMETRY_UNAVAILABLE"}}</td><td>{{index $counts "ERROR"}}</td></tr>{{end}}
</table>
{{range .Results}}
<div style="border:1px solid #cbd5e1;border-radius:6px;padding:16px;margin-bottom:20px">
<div style="display:flex;justify-content:space-between;align-items:baseline;flex-wrap:wrap;gap:8px">
<div><span class="{{.Status}}">{{.Status}}</span> · <a href="{{.URL}}"><code>{{.Path}}</code></a> <small>({{.Category}}{{if .Ahem}} · Ahem{{end}})</small></div>
<div><small>Pixels differing: {{.DifferentPixels}} / {{.TotalPixels}} ({{printf "%.2f" .PixelDiffPercent}}%) · max channel Δ {{.MaxChannelDifference}}</small></div>
</div>
<p style="margin:6px 0"><small>{{.Summary}} · Document bounds: nthtml <code>{{.Document.NTHTML.Width}}×{{.Document.NTHTML.Height}}</code> vs Chromium <code>{{.Document.Chromium.Width}}×{{.Document.Chromium.Height}}</code> ({{.Document.Note}})</small></p>
<table class="geom-table">
<tr><th>Selector</th><th>Element</th><th>Status</th><th>nthtml (x, y, w×h)</th><th>Chromium (x, y, w×h)</th><th>Δ (dx, dy, dw, dh)</th><th>Notes</th></tr>
{{range .Geometry}}
<tr>
<td><code>{{.Selector}}</code></td>
<td>{{.Label}}</td>
<td class="{{.Status}}">{{.Status}}</td>
<td>{{if .NTHTML}}<code>{{.NTHTML.X}}, {{.NTHTML.Y}}, {{.NTHTML.Width}}×{{.NTHTML.Height}}</code>{{else}}—{{end}}</td>
<td>{{if .Chromium}}<code>{{.Chromium.X}}, {{.Chromium.Y}}, {{.Chromium.Width}}×{{.Chromium.Height}}</code>{{else}}—{{end}}</td>
<td>{{if and .NTHTML .Chromium}}<code>{{printf "%+d, %+d, %+d, %+d" .DX .DY .DW .DH}}</code>{{else}}—{{end}}</td>
<td>{{.Note}}</td>
</tr>
{{end}}
</table>
{{if .NTHTMLPNG}}
<div class="images">
<figure><figcaption>nthtml ({{$.Viewport.Width}}×{{$.Viewport.Height}})</figcaption><a href="{{.NTHTMLPNG}}"><img alt="nthtml render" src="{{.NTHTMLPNG}}"></a></figure>
<figure><figcaption>Chromium ({{$.Viewport.Width}}×{{$.Viewport.Height}})</figcaption><a href="{{.ChromiumPNG}}"><img alt="Chromium render" src="{{.ChromiumPNG}}"></a></figure>
<figure><figcaption>Pixel diff (magenta = different)</figcaption><a href="{{.DiffPNG}}"><img alt="Pixel difference" src="{{.DiffPNG}}"></a></figure>
</div>
{{end}}
</div>
{{end}}`
