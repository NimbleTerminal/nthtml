// Copyright (c) 2026 Neomantra Corp. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0-only WITH LicenseRef-NimbleTerminal-Linking-Exception OR LicenseRef-Neomantra-Commercial

package nthtml

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/ntpicture"
)

// A page shorter than the viewer still yields a view of the viewer's full
// height: the rows below the page are blank cells in the canvas colour, so a
// host's status line under the viewer stays at the bottom of the terminal.
func TestHostPaintedViewFillsBoundsWithCanvas(t *testing.T) {
	r, err := document.Render(context.Background(), strings.NewReader(`<body style="background:#123456;margin:0"><p>short</p></body>`), document.Options{Width: 400, NoImage: true, Fonts: document.MinimalFonts})
	if err != nil {
		t.Fatal(err)
	}
	m := NewWithOptions(nil, ModelOptions{HostPainted: true})
	defer m.Close()
	driveRaw(t, m, m.SetBounds(0, 0, 40, 12))
	if m.View() != "" {
		t.Fatalf("nothing presented yet, view must stay empty: %q", m.View())
	}
	frame := paintInitial(t, r, m.PaintViewport(), 1)
	if frame.Plan.Rows >= 12 {
		t.Fatalf("test page is not short: %+v", frame.Plan)
	}
	cmd, err := m.SetPaintedResult(frame, false)
	if err != nil {
		t.Fatal(err)
	}
	driveRaw(t, m, cmd)
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 12 {
		t.Fatalf("view has %d rows, want the 12 of the bounds", len(lines))
	}
	last := lines[11]
	if !strings.Contains(last, "\x1b[48;2;18;52;86m") || ansi.StringWidth(last) != 40 || strings.TrimSpace(ansi.Strip(last)) != "" {
		t.Fatalf("padding row is not a blank canvas-coloured row of 40 cells: %q", last)
	}
	if lines[0] == last || strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		t.Fatalf("page rows must not be replaced by padding: %q", lines[0])
	}
}

// A terminal wider than the Kitty placeholder grid gets a band presented over
// the columns the protocol can address, not a refused frame and a blank page.
func TestHostPaintedKittyColumnsStayWithinPlaceholderGrid(t *testing.T) {
	limit := MaxKittyPlaceholderCells()
	if limit < 200 || limit > 1000 || kitty.Diacritic(limit) != kitty.Diacritic(0) || kitty.Diacritic(limit-1) == kitty.Diacritic(0) {
		t.Fatalf("placeholder limit %d does not match the diacritic table", limit)
	}
	r := paintedPage(t)
	m := NewWithOptions(nil, ModelOptions{HostPainted: true, Picture: ntpicture.Options{StableID: true, RetireDelay: -1}})
	defer m.Close()
	driveRaw(t, m, m.Picture().SetTerminal(true, 10, 20), m.SetBounds(0, 0, limit+17, 12))
	v := m.PaintViewport()
	v.Cols = limit // what a host that reads MaxKittyPlaceholderCells plans
	frame := paintInitial(t, r, v, 1)
	cmd, err := m.SetPaintedResult(frame, false)
	if err != nil {
		t.Fatal(err)
	}
	raw := driveRaw(t, m, cmd)
	if m.Err() != nil {
		t.Fatal(m.Err())
	}
	if m.bandCols != limit || !strings.Contains(raw, "a=T") {
		t.Fatalf("band presented over %d columns (transmitted %v), want %d", m.bandCols, strings.Contains(raw, "a=T"), limit)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) != 12 || ansi.StringWidth(lines[0]) < limit {
		t.Fatalf("view %d rows, first row %d cells", len(lines), ansi.StringWidth(lines[0]))
	}
}
