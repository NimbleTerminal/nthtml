# CSS styling in terminal text

TTY layout uses terminal cells and the terminal's font. It carries CSS color and
emphasis without trying to reproduce pixel box layout.

`Document.TTY(ctx, columns)` and `TTYResult.Text` are plain, escape-free text.
`Document.TTYLayout` (also available on `Result`) additionally returns:

- `Styles []TTYStyleSpan`: sorted, non-overlapping style runs with UTF-8 byte
  offsets into `Text` and terminal-cell `Bounds`.
- `Spans []TextSpan` and `Links`: link offsets and hit regions.
- Source anchors and the same plain-text search/selection index used by the viewer.

A `TTYStyle` contains foreground/background RGBA colors and bold, italic,
underline, and strikethrough flags. Zero-alpha colors mean the terminal default.
Metadata is independent of escape sequences; never search or hit-test the ANSI
string. Treat generated metadata as read-only.

## Emitting ANSI

```go
tty, err := doc.TTYLayout(ctx, 80)
if err != nil {
    return err
}
plain := tty.Text
styled := tty.ANSI()
matches := tty.TextIndex().Search("Café")
highlighted := tty.ANSIHighlights(matches)
_, _, _ = plain, styled, highlighted
```

`ANSI()` emits truecolor SGR and safe OSC 8 hyperlinks. `ANSIHighlights` overlays
inverse-video matches/selection without losing CSS attributes or links.
Overlapping ranges are combined. Equal explicit foreground/background colors
are replaced with a contrasting black/white pair inside highlights.

Every styled segment restores its terminal state. Each physical line closes
SGR attributes and OSC 8 links; wrapped lines reopen the applicable state.
A viewport can crop whole lines without leaking styling. Horizontal cropping
must use an ANSI-aware cell operation, not a byte slice.

`nthtml.Model` uses these methods automatically in `LayoutTTY`, including its
search and selection highlights. No host-side HTML rewriting is needed.

## Color profiles

The library does not inspect the process environment when formatting a result.
Use the existing `github.com/charmbracelet/colorprofile` conventions explicitly:

```go
out := tty.ANSIWithProfile(colorprofile.ANSI256)
highlighted := tty.ANSIHighlightsWithProfile(matches, colorprofile.ASCII)
```

| Profile | Output |
| --- | --- |
| `TrueColor` | 24-bit RGB, emphasis, highlights, OSC 8 links |
| `ANSI256` | Colors reduced to the 256-color palette; other attributes retained |
| `ANSI` | Colors reduced to the 16-color palette; other attributes retained |
| `ASCII` | No color SGR; emphasis, inverse highlights, and OSC 8 links retained |
| `NoTTY`, `Unknown` | Exactly `Text`, with no escapes, links, or highlights |

For `NO_COLOR`, select `ASCII` if you still want typographic emphasis; use
`NoTTY` for redirection or completely plain output. Hosts may obtain a profile
through `colorprofile.Detect`, their terminal session, or Bubble Tea's profile
handling. The explicit methods are deterministic and do not change layout.

## Supported CSS and semantic defaults

- `color` and its existing cascade/inheritance, including author overrides.
- Solid `background-color` / `background` colors on text cells. Transparent
  descendants show their nearest ancestor's color.
- `font-weight` (normal/bold), `font-style` (italic/oblique), and
  `text-decoration-line` / `text-decoration` (underline/line-through).
- Existing HTML defaults: headings and strong/b are bold; em/i and other
  italic phrasing elements are italic; u/ins underline; del/s/strike strike;
  links have their default foreground and underline. Author CSS takes precedence.
- Inline code already uses the terminal's monospace font. Code and preformatted
  blocks retain CSS colors/emphasis and existing whitespace/wrapping rules;
  they do not receive an extra invented ANSI effect.
- Styles survive nested spans, wrapping, list markers, table cells (including
  wrapped and narrow stacked tables), and preformatted blocks.

The terminal controls which attributes it can display. Font families, font
sizes, pixel padding/margins, gradients, background images, rounded corners,
and graphical borders are not reproduced with SGR. Backgrounds color emitted
text cells, not full rectangular boxes or synthetic table padding/borders.
Nonzero alpha is treated as opaque RGB; terminal alpha blending is unavailable.
Unspecified colors use terminal defaults. Unicode width and plain text remain
the responsibility of the existing terminal-cell layout, not raster font fallback.

## Safety and bounds

Source text is sanitized before internal metadata is introduced. Source HTML
cannot inject SGR, OSC, clipboard commands, cursor motion, or internal style
markers. Only library-generated SGR and validated OSC 8 URLs reach ANSI output.

TTY output retains its 4 MiB budget. Style tables/spans are bounded, and metadata
counts toward intermediate output limits. If ANSI expansion exceeds 4 MiB,
formatting falls back to plain `Text`. Search offsets, source mappings, and
selection text always refer to the escape-free representation.

## Side-by-side demo

Run from the repository root:

```sh
go run ./examples/tty-styles
```

The left pane reflows Typeset to its available pixel width, including after
terminal cell-size discovery and window resizing. The right pane uses styled TTY from the same
[HTML/CSS](../examples/tty-styles/sample.html). Kitty-capable terminals use raster
graphics on the left; other terminals use the existing picture fallback.
Use a terminal at least 100 columns wide for a useful comparison.

Press `F6` to switch the active pane (TTY initially). The active pane is marked
in its header. Both Typeset and TTY support search and selection; each pane
keeps its own query and matches. Typeset highlights use the rendered HTML's
text index and pixel coordinates, without OCR.

Press `/` to open the active pane's search prompt, type a query, and press
`Enter` to search. The footer shows the current match count or “no matches.”
The active pane also supports `n` next match, `v` select match, `Esc`
clear, and arrow-key scrolling. Press `q` or `Ctrl+C` to exit. The demo never
launches external links or modifies another application.
