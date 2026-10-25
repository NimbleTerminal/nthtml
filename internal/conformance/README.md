# WPT subset

This harness runs audited visual reference tests from [Web Platform Tests](https://github.com/web-platform-tests/wpt), the shared upstream suite used for web standards testing. It is not the browser WPT runner, full HTML/CSS conformance, or certification.

```sh
task conformance                 # offline baseline check and report
task conformance -- -strict       # fail on any rendering failure
go test ./internal/conformance   # regression check, also in normal tests
task test-wasm                   # runs the same subset under Node/WASM
```

Open `artifacts/conformance/index.html` for results and failure images. `report.json` contains machine-readable results. Generated artifacts are ignored by Git and uploaded by CI.

## Current baseline

| Result | Cases | Meaning |
|---|---:|---|
| PASS | 125 | Audited selectors/cascade, colors, dimensions, zero font sizes, Ahem typography, auto-width tables, and core document cases. |
| FAIL | 0 | Known rendering discrepancies in runnable cases. |
| UNSUPPORTED | 75 | Required features outside the subset, including dependencies in references; never counted as passes. |
| ERROR | 0 | Harness errors, missing fixtures, or rejected prerequisites. |

| Feature area | PASS | FAIL | UNSUPPORTED |
|---|---:|---:|---:|
| Boxes and positioning | 35 | 0 | 15 |
| Colors | 41 | 0 | 1 |
| Selectors and cascade | 32 | 0 | 12 |
| Tables | 1 | 0 | 2 |
| Typography and whitespace | 16 | 0 | 43 |
| Units (JavaScript testharness cases) | 0 | 0 | 2 |

The corpus contains **200 selected HTML/CSS cases**, expanded from 41. It samples consecutive numeric ranges for class/ID/descendant selectors, cascade, font/spacing syntax, min/max dimensions, and RGB/HSL/hex colors, plus explicit gap cases. Only upstream tests with reference documents were added; cases with unsupported prerequisites were retained. Many cases exercise syntax variants of the same feature, so case counts do not measure feature completeness. Selection and prerequisites were reviewed before pixel baselines were recorded.

**125/125 runnable cases pass (100%)**, or **125/200 selected cases (62.5%)** including unsupported cases. Neither percentage measures the full HTML/CSS suite. Native and WASM run the same corpus. HTML, JSON, and CLI reports include counts by feature area.

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
