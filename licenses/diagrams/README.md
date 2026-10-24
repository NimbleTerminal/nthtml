# Diagram dependencies

nthtml uses these MIT-licensed libraries for its default diagram renderer:

- nimbleterminal.dev/ntsvg v0.5.2 — [license](ntsvg-LICENSE.txt)
- github.com/zkrebbekx/go-mermaid v0.1.6 — [license](go-mermaid-LICENSE)
- github.com/NimbleMarkets/oksvg v0.1.2 — [license](oksvg-LICENSE)
- github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef — [license](rasterx-LICENSE)

The Mermaid raster subpackage is not imported. nthtml uses ntsvg for all SVG
rasterization. Embedded Go-font notices are included alongside the font licenses.
