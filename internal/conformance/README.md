# WPT subset

This harness runs audited visual reference tests from [Web Platform Tests](https://github.com/web-platform-tests/wpt), the shared upstream suite used for web standards testing. It is not the browser WPT runner, full HTML/CSS conformance, or certification.

```sh
task conformance                 # offline baseline check and report
task conformance -- -strict       # fail on any rendering failure
task differential                # Chromium differential geometry/screenshot report
go test ./internal/conformance   # regression check, also in normal tests
task test-wasm                   # runs the same subset under Node/WASM
```

Open `artifacts/conformance/index.html` for results and failure images. `report.json` contains machine-readable results. Generated artifacts are ignored by Git and uploaded by CI.

## Current baseline

| Result | Cases | Meaning |
|---|---:|---|
| PASS | 187 | Audited selectors/cascade (including attribute, sibling, structural pseudo-class, and pseudo-element rules), colors, dimensions, per-side 96px borders, z-index stacking, zero font sizes, small-caps, Ahem typography, inline-block/inline-image whitespace & vertical alignment, word-spacing, auto-width & collapsed-border tables, and core document cases. |
| FAIL | 0 | Known rendering discrepancies in runnable cases. |
| UNSUPPORTED | 13 | Required features outside the subset, including dependencies in references; never counted as passes. |
| ERROR | 0 | Harness errors, missing fixtures, or rejected prerequisites. |

| Feature area | PASS | FAIL | UNSUPPORTED |
|---|---:|---:|---:|
| Boxes and positioning | 50 | 0 | 0 |
| Colors | 41 | 0 | 1 |
| Selectors and cascade | 43 | 0 | 1 |
| Tables | 2 | 0 | 1 |
| Typography and whitespace | 51 | 0 | 8 |
| Units (JavaScript testharness cases) | 0 | 0 | 2 |

The corpus contains **200 selected HTML/CSS cases**, expanded from 41. It samples consecutive numeric ranges for class/ID/descendant selectors, cascade, font/spacing syntax, min/max dimensions, and RGB/HSL/hex colors, plus explicit gap cases. Only upstream tests with reference documents were added; cases with unsupported prerequisites were retained. Many cases exercise syntax variants of the same feature, so case counts do not measure feature completeness. Selection and prerequisites were reviewed before pixel baselines were recorded.

**187/187 runnable cases pass (100%)**, or **187/200 selected cases (93.5%)** including unsupported cases. Neither percentage measures the full HTML/CSS suite. Native and WASM run the same corpus. HTML, JSON, and CLI reports include counts by feature area.

The manifest pins revision `ffe56d4420d270941329689aff42d925f615d0b4` and SHA-256 hashes for every vendored file. Original fixture bytes and upstream `LICENSE.md` are retained under `testdata/wpt`. Unsupported cases document gaps such as inline box geometry, attribute/pseudo selectors, generated content, per-side borders, synthetic Ahem variants, and JavaScript testharness execution.

Default exit status checks the reviewed baseline: an unexpected failure **or improvement** fails until reviewed. Known failures remain labelled FAIL. `-strict` also fails on known failures. Unsupported cases are deliberately skipped, so implementing a feature requires reviewing and enabling those cases explicitly.

## Comparison boundaries

Following [WPT reference-test semantics](https://web-platform-tests.org/writing-tests/reftests.html), at least one match reference must match when present, and every mismatch reference must differ. Comparisons use exact RGBA pixels in an opaque white 800×600 viewport. Documents taller than the viewport are rejected rather than silently cropped. Failure artifacts include test, reference, and magenta difference PNGs.

Both sides are rendered by nthtml with its own defaults. Shared rendering defects can therefore pass a reference comparison; these results do not establish pixel equivalence with a browser. No browser user-agent stylesheet is injected. The harness exercises image layout, not TTY tables, mouse navigation, search, or clipboard behavior; those retain their own tests.

Fixtures are audited manually for supported prerequisites in **both** test and reference. Preflight additionally rejects scripts, external stylesheets, unallowlisted images, fuzzy metadata, custom viewport metadata, asynchronous/print test modes, and unreviewed prerequisites. `.xht` / `.xhtml` fixtures first pass through a bounded XML-to-HTML adapter. It decodes CSS CDATA and character/entity data, emits explicit closing tags for non-void elements, and ignores external DOCTYPE identifiers without fetching them. Custom DTD entities, foreign namespaces, and unsupported case-sensitive element names are rejected. The public renderer remains an HTML parser. There is no general CSS support detector: ignored CSS must never be justified by a matching screenshot alone.

## Adding or updating cases

1. Select a fixed upstream commit and inspect each test and all reference dependencies. Prefer small cases that isolate a supported feature.
2. Vendor original files unchanged, retaining the upstream license; record their SHA-256 hashes in `manifest.json`. Never edit a fixture to make it pass.
3. Record the expected result and a concrete reason. Cases requiring unavailable features/assets stay UNSUPPORTED; runnable rendering discrepancies stay FAIL.
4. Run the native and WASM tests and inspect failing image triples. Review every baseline change and update this table.

Fetching/updating upstream fixtures is a separate, intentional maintenance operation. Ordinary tests and report generation never access the network.

Selector/cascade combinations, whitespace modes, and merged-table geometry/TTY output also have native and WASM regression tests in `document`. These local tests are not counted as upstream WPT passes.

## Deterministic font and image prerequisites

Cases opt into `ahem: true` to use the original pinned `fonts/Ahem.ttf` through `document.Options.Fonts`. Only the accompanying `/fonts/ahem.css` stylesheet is accepted, and only when explicitly enabled. No stylesheet fetching or general `@font-face` implementation is involved. Other fonts use the renderer's usual embedded source. Synthetic bold/italic Ahem variants are rejected. See [Ahem provenance and license](AHEM-LICENSE.md). Local tests independently assert exact Ahem square pixels and advances, half-leading, zero/short line heights, and positive/negative word spacing; those tests do not count as upstream passes.

Images require explicit per-case `assets` paths, pinned hashes, and bounded decoding. Paths cannot resolve to network URLs or escape the fixture tree. Missing, unpinned, or corrupt images fail instead of silently rendering alt text. Per-document decoding is limited to 16 images, 4MiB encoded bytes, and four million pixels. Fonts are limited to 1MiB in this harness. Ordinary tests and reports perform no network access. Original documents are never rewritten to hide rendering differences.


## Chromium differential harness (`task differential`)

Because standard WPT reference tests render both the test document and its reference with `nthtml`, shared layout defects or invisible white-background box sizing errors can pass self-referential pixel comparisons. The differential harness complements the reftest suite by rendering the **same** WPT test document in both `nthtml` and headless Chromium at `800×600` (`deviceScaleFactor: 1`).

```sh
task differential                                       # auto-detect local Chromium/Chrome and write artifacts/differential/
NTHTML_CHROMIUM=/path/to/chromium task differential     # use an explicit browser binary
go run ./cmd/nthtml-differential -strict                # fail if Chromium is unavailable or any case has GEOMETRY_MISMATCH
```

### How it works

- **Offline CDP execution**: `internal/conformance` drives a local headless Chromium or Chrome binary directly over `--remote-debugging-pipe` using standard-library Go (`os/exec` + JSON-RPC). No Playwright, Node, or third-party Go dependencies are added.
- **Zero network access**: All requests are intercepted via CDP `Fetch.enable`. Only the active WPT fixture, its allowlisted `assets`, and the pinned `/fonts/ahem.css` + `/fonts/Ahem.ttf` files are served from the embedded `testdata/wpt` filesystem; any other URL fails with `BlockedByClient`.
- **Deterministic rendering**: Animations and transitions are disabled, `Ahem.ttf` is loaded via `@font-face` when `ahem: true`, and `document.fonts.ready` + image decode promises complete before layout extraction and screenshot capture.

### Interpreting `artifacts/differential/index.html` and `report.json`

For each of the 20 representative fixtures (covering block layout/margins, width/height, padding/borders, selectors/cascade, inline text layout, and tables), the report records:

1. **Element geometry (`MATCH`, `MISMATCH`, `UNAVAILABLE`)**:
   - Compares `nthtml`'s `Result.Boxes` against Chromium's `getBoundingClientRect()`.
   - Elements are matched only when unambiguous: by DOM `#id` (`Box.Anchor` or union of inline `Box.SourceID` runs) or by 1-to-1 pre-order tag count for block/table/replaced elements.
   - Where `nthtml` does not emit a standalone element box (such as an un-ID'd inline `<span>` merged into text runs), geometry is reported as `UNAVAILABLE` rather than guessed.
   - Preceding unstyled `<p>` instruction lines use `nthtml`'s default 16px line-height (`22px`) vs Chromium's default serif line-height (`18px`); the resulting `+4px` normal-flow vertical shift is explicitly accounted for and noted alongside the raw coordinates.
2. **Overall document dimensions**:
   - Compares `nthtml`'s rendered canvas size (`Result.Image.Bounds()`) against Chromium's rendered document bounds.
3. **Screenshots and pixel difference percentage**:
   - Side-by-side `nthtml`, Chromium, and magenta diff PNGs at `800×600`.
   - **Cosmetic pixel differences vs. geometry mismatches**: A nonzero pixel difference percentage (typically 0.3%–3% from glyph rasterization, antialiasing, or the `+4px` intro `<p>` line-height shift) does **not** imply a geometry failure. Fixtures where all comparable boxes match are classified as `GEOMETRY_MATCH`; fixtures with actual box size or position discrepancies are classified as `GEOMETRY_MISMATCH`.

## Typeset adapter backend

```sh
task differential:typeset
# Equivalent:
go run ./cmd/nthtml-differential -backend typeset
# Require measured conformance, rather than merely producing diagnostics:
task differential:typeset -- -strict
```

The default `native` backend and its WPT baseline are unchanged. The `typeset`
backend uses the same 20 pinned differential probes, offline fixture/asset
allowlists and Chromium capture. It calls `typeset.RenderResult` with the
embedded nttypeset fonts, at width 800 and scale 1, and captures the first 600
pixels alongside the full rendered document dimensions. It refuses a clipped
full-document measurement. No CSS reset, HTML rewriting, margin subtraction or
native font-height correction makes the adapter appear to match. These probes
include features outside the proposed initial profile; this is a diagnostic
survey, not a count of supported profile features.

Reports identify `backend` at the report and case level. The legacy JSON
`nthtml`, `nthtml_png` fields describe the selected backend. Default artifacts
live in `artifacts/differential-typeset`, with renderer-qualified PNG filenames.
A directory containing the other backend's report is rejected before rendering
or writing. Use separate directories for native and typeset results.

### What is measured

- Geometry uses existing source IDs only, never guessed DOM tag order. Adapter
  source envelopes are compared to Chromium border boxes with **1 CSS pixel**
  rounding tolerance. They are not guaranteed to be equivalent box concepts.
  Missing mappings remain `GEOMETRY_UNAVAILABLE`; partial comparisons retain
  their unavailable-element count. The adapter must improve its geometry API
  before these can become complete conformance assertions.
- Pixels are compared as exact RGBA with **zero tolerance**, reported separately
  as `EXACT`, `DIFFERENT`, or `UNAVAILABLE`. Default browser and adapter fonts
  can differ; differences are measured without attributing all of them to fonts
  or normalizing them away. `GEOMETRY_MATCH` is not a pixel-conformance pass.
- Ahem-dependent cases are `UNSUPPORTED` because the adapter currently cannot
  map arbitrary CSS font families into nttypeset's configured faces. They are
  not drawn with a substitute font and are never counted as passes. Existing
  manifest-level unsupported prerequisites also stay unsupported.
- Render, asset, browser and report errors remain `ERROR`. Missing Chromium
  produces an explicit skip in diagnostic CLI mode and fails `-strict`.

Diagnostic mode exits successfully when a report is produced without errors,
including reports of known mismatches. Typeset `-strict` requires every case to
have complete matching geometry, exact full-document dimensions, exact pixels,
and supported prerequisites. An empty report, missing geometry, unsupported
font or unmeasured pixels cannot pass. Native strict-mode semantics remain as
before. Strict mode is expected to fail for the initial typeset implementation.

### Initial observations and regression gate

`typeset-baseline.json` records the independently measured **case statuses**, not
renderer-generated golden pixels. The first run used HeadlessChrome 143.0.7499.4:

| Result | Cases |
| --- | ---: |
| GEOMETRY_MATCH | 0 |
| GEOMETRY_MISMATCH | 7 |
| GEOMETRY_UNAVAILABLE | 7 |
| UNSUPPORTED (Ahem prerequisites) | 6 |
| ERROR | 0 |

All 14 rendered cases had pixel differences. For example, `width-006.xht` loses
the two 96px black boxes and retains only the text and a thin rule: this is an
actual layout/content discrepancy, not cosmetic antialiasing.

`TestTypesetDifferentialSuite` compares measured statuses with this reviewed
baseline, failing on changes including improvements. It runs in normal tests
when Chromium is available; otherwise it explicitly skips. The JSON, HTML and
PNG artifacts are regenerated by the CLI, not committed. Review changed fixture
requirements and comparison images before accepting a baseline change. Never
regenerate expectations merely to make CI green, or transfer native WPT pass
counts to this backend. Unit tests additionally enforce strict-result handling,
source-mapping ambiguity and protection of native report directories.
