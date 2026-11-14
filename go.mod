module nimbleterminal.dev/nthtml

go 1.26.11

require (
	charm.land/bubbletea/v2 v2.1.1
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/charmbracelet/colorprofile v0.4.3
	github.com/charmbracelet/x/ansi v0.11.9
	github.com/zkrebbekx/go-mermaid v0.1.6
	golang.org/x/image v0.48.0
	golang.org/x/net v0.62.0
	nimbleterminal.dev/ntfonts v0.3.1
	nimbleterminal.dev/ntpicture v0.2.0
	nimbleterminal.dev/ntsvg v0.6.0
	nimbleterminal.dev/nttypeset v0.3.0
)

require (
	charm.land/bubbles/v2 v2.2.1 // indirect
	charm.land/lipgloss/v2 v2.1.0 // indirect
	github.com/NimbleMarkets/oksvg v0.1.7 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20261023060817-a45b7c86333f // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/dlclark/regexp2/v2 v2.13.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-runewidth v0.0.31 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	github.com/xo/terminfo v1.3.1 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	nimbleterminal.dev/ntconfine v0.2.0 // indirect
	nimbleterminal.dev/ntconfine/pkg/confine v0.2.0 // indirect
	nimbleterminal.dev/ntmediabox v0.2.0 // indirect
	nimbleterminal.dev/ntmediabox/pkg/mediabox v0.2.0 // indirect
)

// Vector rendering fork used by nttypeset and ntsvg.
replace github.com/srwiley/rasterx => github.com/neomantra/rasterx v0.1.1

// WASM-capable Bubble Tea pin; all builds keep GOWORK=off.
replace charm.land/bubbletea/v2 => github.com/neomantra/bubbletea/v2 v2.0.0-20261020122444-536722b0bdb6
