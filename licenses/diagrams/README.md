# Diagram dependencies

nthtml uses these open-source libraries for its default diagram renderer:

- nimbleterminal.dev/ntsvg v0.5.3 — [license](ntsvg-LICENSE.txt)
- github.com/zkrebbekx/go-mermaid v0.1.6 — [license](go-mermaid-LICENSE)
- github.com/NimbleMarkets/oksvg v0.1.3 — [license](oksvg-LICENSE)
- github.com/neomantra/rasterx v0.1.0 (fork of github.com/srwiley/rasterx) — [license](rasterx-LICENSE)

The Mermaid raster subpackage is not imported. nthtml uses ntsvg for all SVG
rasterization. Embedded Go-font notices are included alongside the font licenses.
