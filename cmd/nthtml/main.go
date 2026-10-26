// nthtml renders an HTML file (or stdin) to PNG, TTY text, or an interactive
// terminal viewer comparing native raster, nttypeset-bridged raster, and TTY
// modes without network access.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nthtml/typeset"
	"nimbleterminal.dev/nttypeset"
)

const shutdownGrace = 150 * time.Millisecond

func main() {
	mode := flag.String("mode", "png", "output mode: png (native), typeset (bridged PNG), tty, or view (interactive)")
	viewFlag := flag.Bool("view", false, "shorthand for -mode view")
	bridgeFlag := flag.Bool("bridge", false, "start -mode view in bridged nttypeset mode (or render bridged PNG)")
	width := flag.Int("width", 800, "document width in pixels")
	height := flag.Int("height", 0, "viewport height in pixels for -mode typeset (0 = full laid extent)")
	columns := flag.Int("columns", 80, "TTY width in terminal cells")
	out := flag.String("o", "document.png", "output PNG file")
	flag.Parse()

	activeMode := *mode
	if *viewFlag {
		activeMode = "view"
	} else if *bridgeFlag && activeMode == "png" {
		activeMode = "typeset"
	}
	if *width < 32 || *columns < 2 {
		fail(fmt.Errorf("usage: nthtml [-mode png|typeset|tty|view] [-width pixels] [-columns cells] [-o output.png] [FILE.html]"))
	}

	var r io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fail(err)
		}
		defer f.Close()
		r = f
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	doc, err := document.Parse(ctx, r, document.Limits{})
	if err != nil {
		fail(err)
	}

	switch activeMode {
	case "tty":
		text, err := doc.TTY(ctx, *columns)
		if err != nil {
			fail(err)
		}
		if _, err := fmt.Fprintln(os.Stdout, text); err != nil {
			fail(err)
		}
	case "png":
		result, err := doc.Render(ctx, document.Options{Width: *width})
		if err != nil {
			fail(err)
		}
		for _, warning := range result.Warnings {
			fmt.Fprintln(os.Stderr, "warning:", warning)
		}
		if err := writePNG(*out, result.Image); err != nil {
			fail(err)
		}
	case "typeset":
		eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
		if err != nil {
			fail(err)
		}
		if *height > 0 {
			img, _, err := typeset.Raster(
				ctx,
				eng,
				doc,
				*width,
				nttypeset.View{Width: *width, Height: *height, Scale: 1},
				nttypeset.RasterOptions{},
				typeset.Options{Document: document.Options{Width: *width}},
			)
			if err != nil {
				fail(err)
			}
			if err := writePNG(*out, img); err != nil {
				fail(err)
			}
			return
		}
		res, _, _, err := typeset.RenderResult(ctx, eng, doc, *width, typeset.Options{Document: document.Options{Width: *width}})
		if err != nil {
			fail(err)
		}
		if err := writePNG(*out, res.Image); err != nil {
			fail(err)
		}
	case "view":
		if err := runInteractive(doc, *width, *columns, *bridgeFlag); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("unknown mode %q (choose png, typeset, tty, or view)", activeMode))
	}
}

func runInteractive(doc *document.Document, width, columns int, startBridged bool) error {
	ctx := context.Background()
	nativeRes, err := doc.Render(ctx, document.Options{Width: width})
	if err != nil {
		return err
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		return err
	}
	bridgeRes, _, sm, err := typeset.RenderResult(ctx, eng, doc, width, typeset.Options{Document: document.Options{Width: width}})
	if err != nil {
		return err
	}
	initial := nativeRes
	if startBridged {
		initial = bridgeRes
	}
	v := &interactiveViewer{
		doc:       doc,
		eng:       eng,
		nativeRes: nativeRes,
		bridgeRes: bridgeRes,
		sources:   sm,
		bridged:   startBridged,
		viewer:    nthtml.New(initial),
		width:     width,
		cols:      columns,
		notice:    strings.Join(nativeRes.Warnings, "; "),
	}
	defer func() { _ = v.viewer.Close() }()
	_, err = tea.NewProgram(v).Run()
	return err
}

type shutdownReadyMsg struct{}

type interactiveViewer struct {
	doc       *document.Document
	eng       *nttypeset.Engine
	nativeRes *document.Result
	bridgeRes *document.Result
	sources   *typeset.SourceMap
	bridged   bool
	viewer    *nthtml.Model
	width     int
	cols      int
	notice    string
	closing   bool
}

func (v *interactiveViewer) Init() tea.Cmd { return v.viewer.Init() }

func (v *interactiveViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v.closing {
		switch msg.(type) {
		case shutdownReadyMsg:
			return v, tea.Quit
		case tea.KeyPressMsg, tea.KeyReleaseMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg, tea.WindowSizeMsg, nthtml.LinkMsg:
			return v, nil
		default:
			return v, v.viewer.Update(msg)
		}
	}
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		v.cols = m.Width
		return v, tea.Batch(v.viewer.SetBounds(0, 0, m.Width, max(1, m.Height-2)), v.viewer.Update(msg))
	case nthtml.CopyMsg:
		if m.Owner != v.viewer {
			return v, nil
		}
		v.notice = fmt.Sprintf("Copied %d bytes", len(m.Text))
		return v, tea.SetClipboard(m.Text)
	case nthtml.LinkMsg:
		if m.Owner != v.viewer {
			return v, nil
		}
		href := document.ResolveURL("", m.Link.Href)
		if href == "" {
			v.notice = "External/relative link: " + m.Link.Href
		} else {
			v.notice = "External link (owned by host): " + href
		}
		return v, nil
	case tea.KeyPressMsg:
		if !v.viewer.SearchInputActive() {
			switch m.String() {
			case "q", "ctrl+c":
				v.closing = true
				return v, tea.Sequence(v.viewer.Close(), tea.Tick(shutdownGrace, func(time.Time) tea.Msg { return shutdownReadyMsg{} }))
			case "b":
				anchor, hasAnchor := v.viewer.ActiveSourceID()
				v.bridged = !v.bridged
				next := v.nativeRes
				if v.bridged {
					next = v.bridgeRes
				}
				if v.viewer.LayoutMode() == nthtml.LayoutTTY {
					_ = v.viewer.SetLayoutMode(nthtml.LayoutTypeset)
				}
				cmd := v.viewer.SetResult(next)
				if hasAnchor {
					if navCmd, ok := v.viewer.NavigateSource(anchor); ok {
						cmd = tea.Batch(cmd, navCmd)
					}
				}
				return v, cmd
			}
		}
	}
	return v, v.viewer.Update(msg)
}

func (v *interactiveViewer) View() tea.View {
	mode := "Native"
	if v.viewer.LayoutMode() == nthtml.LayoutTTY {
		mode = "TTY"
	} else if v.bridged {
		mode = "Typeset bridge"
	}
	status := v.notice
	if id, ok := v.viewer.ActiveSourceID(); ok {
		if parent := v.sources.ParentID(id); parent != "" {
			status = fmt.Sprintf("source: %s (in %s)", id, parent)
		} else {
			status = fmt.Sprintf("source: %s", id)
		}
	}
	if link, ok := v.viewer.FocusedLink(); ok {
		status = "Enter to activate: " + link.Href
	}
	if query, current, total := v.viewer.SearchStatus(); query != "" {
		status = fmt.Sprintf("Search %q · %d/%d · n/N next/previous", query, current, total)
	}
	if v.viewer.SearchInputActive() {
		status = "/" + v.viewer.SearchInput() + " · Enter search · Esc cancel"
	}
	if text := v.viewer.SelectedText(); text != "" && !v.viewer.SearchInputActive() {
		if id, ok := v.viewer.SelectedSourceID(); ok {
			status = fmt.Sprintf("%d bytes selected (%s) · y copy · Esc clear", len(text), id)
		} else {
			status = fmt.Sprintf("%d bytes selected · y copy · Esc clear", len(text))
		}
	}
	help := fmt.Sprintf("nthtml · %s · t: TTY/styled · b: native/bridge · /: search · n/N: matches · drag/y: copy · Tab/Enter: links · q: quit", mode)
	out := v.viewer.View() + "\n" + ansi.Truncate(status, max(1, v.cols), "…") + "\n" + ansi.Truncate(help, max(1, v.cols), "…")
	view := tea.NewView(out)
	view.AltScreen = true
	if !v.closing {
		view.MouseMode = tea.MouseModeCellMotion
	}
	return view
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
