package nthtml

import (
	"bytes"
	"context"
	"errors"
	"image"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

func paintedPage(t *testing.T) *document.Result {
	t.Helper()
	r, err := document.Render(context.Background(), strings.NewReader(`<body style="background:#def"><a href="/top">top</a>`+strings.Repeat(`<div style="height:1000px;background:#fda">band</div>`, 40)+`<p id="end"><a href="/end">end</a></p></body>`), document.Options{Width: 400, NoImage: true, Fonts: document.MinimalFonts})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func paintInitial(t *testing.T, r *document.Result, v PaintViewport, rev uint64) PaintedResult {
	t.Helper()
	plan, err := PlanPaint(r, v)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Paint(context.Background(), plan.Area, plan.Scale)
	if err != nil {
		t.Fatal(err)
	}
	return PaintedResult{Result: r, Revision: rev, Plan: plan, Image: img}
}
func requestPaint(t *testing.T, m *Model, delta int) PaintRequest {
	t.Helper()
	cmd := m.Pan(0, delta)
	if cmd == nil {
		t.Fatal("missing paint request")
	}
	req, ok := cmd().(PaintRequest)
	if !ok {
		t.Fatal("not a paint request")
	}
	return req
}
func readyPaint(t *testing.T, req PaintRequest) PaintReady {
	t.Helper()
	img, err := req.Result.Paint(req.Context, req.Plan.Area, req.Plan.Scale)
	if err != nil {
		t.Fatal(err)
	}
	return PaintReady{Request: req, Image: img}
}

func TestHostPaintedAtomicAndStaleRequests(t *testing.T) {
	r := paintedPage(t)
	m := NewWithOptions(nil, ModelOptions{HostPainted: true, Picture: ntpicture.Options{StableID: true}})
	m.SetSize(40, 12)
	frame := paintInitial(t, r, PaintViewport{Cols: 40, Rows: 12}, 1)
	cmd, err := m.SetPaintedResult(frame, false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	before, _ := m.DocumentViewport()
	if m.Result() != r || m.source.Bounds() != frame.Image.Bounds() || m.source.Bounds().Dy() >= r.Bounds().Dy() {
		t.Fatal("layout/raster were not bounded and atomic")
	}
	req1 := requestPaint(t, m, 8000)
	after, _ := m.DocumentViewport()
	if after != before {
		t.Fatal("pending scroll moved displayed viewport")
	}
	ready1 := readyPaint(t, req1)
	req2 := requestPaint(t, m, 8000)
	if !errors.Is(req1.Context.Err(), context.Canceled) {
		t.Fatal("superseded request not cancelled")
	}
	if m.Update(ready1) != nil {
		t.Fatal("accepted stale raster")
	}
	driveRaw(t, m, m.Update(readyPaint(t, req2)))
	after, _ = m.DocumentViewport()
	if after.Min.Y <= before.Min.Y {
		t.Fatal("completion did not move viewport")
	}
	req3 := requestPaint(t, m, 8000)
	ready3 := readyPaint(t, req3)
	frame.Revision = 2
	cmd, err = m.SetPaintedResult(frame, false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	if m.Update(ready3) != nil || m.PaintedRevision() != 2 {
		t.Fatal("old layout completion accepted")
	}
	req4 := requestPaint(t, m, 8000)
	driveRaw(t, m, m.Close())
	if !errors.Is(req4.Context.Err(), context.Canceled) || m.Update(PaintReady{Request: req4}) != nil {
		t.Fatal("close did not invalidate work")
	}
}

func TestHostPaintScaleExtremesAndErrorRecovery(t *testing.T) {
	r := paintedPage(t)
	for _, v := range []PaintViewport{{Cols: 80, Rows: 24}, {Cols: 80, Rows: 24, CellWidth: 10, CellHeight: 20, ZoomSteps: 24}} {
		frame := paintInitial(t, r, v, 1)
		if frame.Plan.Scale < 1.0/16 || frame.Plan.Scale > 16 {
			t.Fatal("invalid scale")
		}
	}
	wide, err := document.Render(context.Background(), strings.NewReader(`<p>wide</p><div style="height:8000px"></div>`), document.Options{Width: 4096, NoImage: true, Fonts: document.MinimalFonts})
	if err != nil {
		t.Fatal(err)
	}
	p, err := PlanPaint(wide, PaintViewport{Cols: 80, Rows: 24})
	if err != nil || p.Scale != 1.0/16 {
		t.Fatalf("glyph scale %g %v", p.Scale, err)
	}
	m := NewWithOptions(nil, ModelOptions{HostPainted: true})
	m.SetSize(40, 12)
	frame := paintInitial(t, r, PaintViewport{Cols: 40, Rows: 12}, 1)
	cmd, err := m.SetPaintedResult(frame, false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	before, _ := m.DocumentViewport()
	req := requestPaint(t, m, 8000)
	m.Update(PaintReady{Request: req, Err: context.DeadlineExceeded})
	after, _ := m.DocumentViewport()
	if after != before || !errors.Is(m.Err(), context.DeadlineExceeded) {
		t.Fatal("error replaced good band")
	}
	req = requestPaint(t, m, 1)
	driveRaw(t, m, m.Update(readyPaint(t, req)))
	if m.Err() != nil {
		t.Fatal(m.Err())
	}
	bad := frame
	bad.Revision = 2
	bad.Image = image.NewRGBA(image.Rect(0, 0, 1, 1))
	if _, err = m.SetPaintedResult(bad, false); err == nil || m.PaintedRevision() != 1 {
		t.Fatal("invalid atomic frame mutated viewer")
	}
}

func TestHostPaintKittyTransportAndCoordinates(t *testing.T) {
	r := paintedPage(t)
	ids := 0
	m := NewWithOptions(nil, ModelOptions{HostPainted: true, NextID: func() (int, error) { ids++; return ids, nil }, Picture: ntpicture.Options{StableID: true, RetireDelay: -1, Render: ntpicture.Config{KittyFormat: ntpicture.KittyFormatZlib}}})
	defer m.Close()
	driveRaw(t, m, m.Picture().SetTerminal(true, 10, 20), m.SetBounds(7, 4, 40, 12))
	frame := paintInitial(t, r, m.PaintViewport(), 1)
	cmd, err := m.SetPaintedResult(frame, false)
	if err != nil {
		t.Fatal(err)
	}
	raw := driveRaw(t, m, cmd)
	if !strings.Contains(raw, "o=z") || !m.InputReady() {
		t.Fatalf("missing ready compressed frame: %q", raw[:min(100, len(raw))])
	}
	baseIDs := ids
	if raw = driveRaw(t, m, m.Pan(0, 20)); raw != "" {
		t.Fatal("cached scroll retransmitted image")
	}
	req := requestPaint(t, m, 8000)
	driveRaw(t, m, m.Update(readyPaint(t, req)))
	if ids != baseIDs {
		t.Fatal("band replacement changed Kitty ID")
	}
	viewport, ok := m.DocumentViewport()
	if !ok || viewport != req.Plan.Viewport {
		t.Fatalf("viewport %v want %v", viewport, req.Plan.Viewport)
	}
	cell, ok := m.CellToDocument(image.Pt(7, 4))
	if !ok || cell.Min != viewport.Min || cell.Dx() != 10 || cell.Dy() != 20 {
		t.Fatalf("global cell %v viewport %v", cell, viewport)
	}
	// Search copies only the resident band, and keeps the source immutable.
	source := m.source
	before := append([]byte(nil), source.(*image.RGBA).Pix...)
	m.matches = m.textIndex().Search("band")
	m.match = 8
	driveRaw(t, m, m.presentPainted(false, true))
	if m.overlayImg == nil || m.overlayImg.Bounds() != source.Bounds() {
		t.Fatal("overlay is not band-local")
	}
	if !bytes.Equal(before, source.(*image.RGBA).Pix) {
		t.Fatal("overlay changed immutable source")
	}
	// A source focus at the bottom uses global coordinates and a small Kitty patch.
	cmd, ok = m.FocusSource("end")
	if !ok || cmd == nil {
		t.Fatal("missing distant source focus")
	}
	req, ok = cmd().(PaintRequest)
	if !ok {
		t.Fatal("focus did not request a band")
	}
	raw = driveRaw(t, m, m.Update(readyPaint(t, req)))
	if m.focusCellRect.Empty() || !strings.Contains(raw, "o=z") {
		t.Fatal("missing band-local focus patch")
	}
	if m.focusCellRect.Dy() >= m.hostPaint.plan.BandRows {
		t.Fatal("focus patch spans whole band")
	}
}

func TestHostPaintModeSwitchCancelsWork(t *testing.T) {
	r := paintedPage(t)
	m := NewWithOptions(nil, ModelOptions{HostPainted: true})
	frame := paintInitial(t, r, m.PaintViewport(), 1)
	cmd, err := m.SetPaintedResult(frame, false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	req := requestPaint(t, m, 8000)
	ready := readyPaint(t, req)
	driveRaw(t, m, m.SetLayoutMode(LayoutTTY))
	if req.Context.Err() != context.Canceled {
		t.Fatal("TTY did not cancel pending paint")
	}
	if m.Update(ready) != nil {
		t.Fatal("TTY accepted old paint")
	}
	if !strings.Contains(m.View(), "band") {
		t.Fatal("TTY lost whole-document text")
	}
}

func TestHostPaintFractionalGeometryAndBudgets(t *testing.T) {
	r, err := document.Render(context.Background(), strings.NewReader(strings.Repeat(`<p style="height:1000px">odd width</p>`, 30)), document.Options{Width: 801, NoImage: true, Fonts: document.MinimalFonts})
	if err != nil {
		t.Fatal(err)
	}
	for _, zoom := range []int{0, 3, 24} {
		v := PaintViewport{Cols: 79, Rows: 23, CellWidth: 9, CellHeight: 19, ZoomSteps: zoom, Origin: image.Pt(11, 17001), MaxRasterPixels: 79 * 9 * 19 * 60}
		frame := paintInitial(t, r, v, 1)
		p := frame.Plan
		if p.BandRows > 60 || frame.Image.Bounds().Dx()*frame.Image.Bounds().Dy() > r.PixelLimit() {
			t.Fatal("exceeded band budget")
		}
		m := NewWithOptions(nil, ModelOptions{HostPainted: true})
		driveRaw(t, m, m.Picture().SetTerminal(true, 9, 19), m.SetSize(79, 23))
		cmd, err := m.SetPaintedResult(frame, false)
		if err != nil {
			t.Fatal(err)
		}
		driveRaw(t, m, cmd)
		var cells image.Rectangle
		for y := 0; y < p.Rows; y++ {
			for x := 0; x < p.Cols; x++ {
				cell, ok := m.CellToDocument(image.Pt(x, y))
				if !ok {
					t.Fatalf("unmapped cell at zoom %d", zoom)
				}
				cells = cells.Union(cell)
			}
		}
		if cells != p.Viewport {
			t.Fatalf("zoom %d cells %v viewport %v", zoom, cells, p.Viewport)
		}
		m.Close()
	}
}

func TestHostPaintTTYReplacementPresentsNewBand(t *testing.T) {
	m := NewWithOptions(nil, ModelOptions{HostPainted: true, Picture: ntpicture.Options{StableID: true, RetireDelay: -1, Render: ntpicture.Config{KittyFormat: ntpicture.KittyFormatZlib}}})
	defer m.Close()
	driveRaw(t, m, m.Picture().SetTerminal(true, 10, 20), m.SetSize(40, 24))
	install := func(color string, revision uint64) string {
		r, err := document.Render(context.Background(), strings.NewReader(`<body style="background:`+color+`"><a href="/`+color+`">new link</a>`+strings.Repeat(`<p style="height:1000px">text</p>`, 20)), document.Options{Width: 400, NoImage: true, Fonts: document.MinimalFonts})
		if err != nil {
			t.Fatal(err)
		}
		cmd, err := m.SetPaintedResult(paintInitial(t, r, m.PaintViewport(), revision), false)
		if err != nil {
			t.Fatal(err)
		}
		return driveRaw(t, m, cmd)
	}
	first := install("red", 1)
	driveRaw(t, m, m.SetLayoutMode(LayoutTTY))
	install("blue", 2)
	if m.hostPaint.presentedRevision != 1 {
		t.Fatal("hidden band reported presented")
	}
	cmd := m.SetLayoutMode(LayoutTypeset)
	if m.InputReady() {
		t.Fatal("old picture accepted input for new layout")
	}
	second := driveRaw(t, m, cmd)
	if !strings.Contains(second, "o=z") || kittyBandPayload(second) == kittyBandPayload(first) {
		t.Fatal("new band not transmitted")
	}
	if m.hostPaint.presentedRevision != 2 || !m.InputReady() {
		t.Fatal("replacement not ready")
	}
	link, ok := m.LinkAtCell(image.Pt(0, 0))
	if !ok || link.Href != "/blue" {
		t.Fatalf("new layout hit: %+v %v", link, ok)
	}
}

func TestHostPaintInstallReconcilesGeometry(t *testing.T) {
	for _, kind := range []string{"rows", "cols", "cells"} {
		t.Run(kind, func(t *testing.T) {
			m := NewWithOptions(nil, ModelOptions{HostPainted: true})
			defer m.Close()
			driveRaw(t, m, m.Picture().SetTerminal(true, 10, 20), m.SetSize(40, 24))
			stale := m.PaintViewport()
			switch kind {
			case "rows":
				stale.Rows = 8
			case "cols":
				stale.Cols = 20
			case "cells":
				stale.CellHeight = 44
			}
			frame := paintInitial(t, paintedPage(t), stale, 1)
			cmd, err := m.SetPaintedResult(frame, false)
			if err != nil {
				t.Fatal(err)
			}
			driveRaw(t, m, cmd)
			if kind == "rows" {
				if m.hostPaint.pending != nil || m.hostPaint.plan.Rows != 24 {
					t.Fatal("row-only resize did not reuse band")
				}
			} else {
				if m.hostPaint.pending == nil {
					t.Fatal("stale geometry did not request paint")
				}
				req := *m.hostPaint.pending
				if req.Plan.Cols != 40 || req.Plan.Rows != 24 || req.Plan.CellHeight != 20 {
					t.Fatalf("stale request %+v", req.Plan)
				}
				driveRaw(t, m, m.Update(readyPaint(t, req)))
			}
			viewport, _ := m.Picture().Viewport()
			if viewport.Width != 40 || viewport.Height != 24 {
				t.Fatalf("stale picture viewport %+v", viewport)
			}
		})
	}
}

func TestHostPaintSubRowPansAccumulate(t *testing.T) {
	m := NewWithOptions(nil, ModelOptions{HostPainted: true})
	defer m.Close()
	driveRaw(t, m, m.Picture().SetTerminal(true, 20, 44), m.SetSize(20, 12))
	cmd, err := m.SetPaintedResult(paintInitial(t, paintedPage(t), m.PaintViewport(), 1), false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	for range 5 {
		driveRaw(t, m, m.Pan(0, 40))
	}
	if m.PaintViewport().Origin.Y != 200 {
		t.Fatalf("lost desired movement: %v", m.PaintViewport().Origin)
	}
	view, _ := m.DocumentViewport()
	if view.Min.Y != 176 {
		t.Fatalf("displayed row %v", view)
	}
	for range 5 {
		driveRaw(t, m, m.Pan(0, -40))
	}
	view, _ = m.DocumentViewport()
	if view.Min.Y != 0 || m.PaintViewport().Origin.Y != 0 {
		t.Fatal("reverse movement lost")
	}
}

func TestHostPaintFailedColumnResizeKeepsMapping(t *testing.T) {
	m := NewWithOptions(nil, ModelOptions{HostPainted: true})
	defer m.Close()
	driveRaw(t, m, m.Picture().SetTerminal(true, 10, 20), m.SetSize(40, 24))
	cmd, err := m.SetPaintedResult(paintInitial(t, paintedPage(t), m.PaintViewport(), 1), false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	before, _ := m.DocumentViewport()
	driveRaw(t, m, m.SetSize(20, 24))
	if m.hostPaint.pending == nil {
		t.Fatal("resize didn't request paint")
	}
	req := *m.hostPaint.pending
	driveRaw(t, m, m.Update(PaintReady{Request: req, Err: context.DeadlineExceeded}))
	if m.Picture().RasterGeometry().Cells.X != 20 {
		t.Fatal("failed resize kept old picture width")
	}
	after, _ := m.DocumentViewport()
	left, ok := m.CellToDocument(image.Pt(0, 0))
	right, rok := m.CellToDocument(image.Pt(19, 0))
	if before != after || !ok || !rok || left.Min.X != before.Min.X || right.Max.X != before.Max.X {
		t.Fatalf("retained band mapping: %v %v %v %v", before, after, left, right)
	}
	if !m.InputReady() {
		t.Fatal("retained band is not usable after resize failure")
	}
}

// Compare image payloads independently of visibility and placement commands.
func kittyBandPayload(raw string) string {
	var payload strings.Builder
	for _, frame := range strings.Split(raw, "\x1b_G")[1:] {
		frame, _, _ = strings.Cut(frame, "\x1b\\")
		_, data, ok := strings.Cut(frame, ";")
		if ok {
			payload.WriteString(data)
		}
	}
	return payload.String()
}
