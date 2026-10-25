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
| PASS | 32 | Display-none, typography, selectors, zero/min/max dimensions, named/RGB/RGBA/hex colors, and collapsed paragraph margins. |
| FAIL | 3 | One default-foreground mismatch; two positioning cases whose references expose auto-width table sizing differences. |
| UNSUPPORTED | 6 | Dependencies outside our reviewed subset; never counted as passes. |
| ERROR | 0 | Harness errors, missing fixtures, or rejected prerequisites. |

The manifest pins revision `ffe56d4420d270941329689aff42d925f615d0b4` and SHA-256 hashes for every vendored file. Original fixture bytes and upstream `LICENSE.md` are retained under `testdata/wpt`. Unsupported cases document gaps in Ahem fonts, external resources and table prerequisites, including prerequisites in their reference documents.

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

The current inventory is **41 selected HTML/CSS cases**, not the full HTML/CSS suite. Of 35 runnable reference cases, 32 pass and three fail; six others are explicitly unsupported. Some unit tests upstream use JavaScript `getComputedStyle` assertions rather than visual references. They remain unsupported by this runner even when the underlying unit conversion is implemented and covered locally. The four color-square cases now match after paragraph defaults and vertical margin collapse were corrected. Original test/reference files remain unchanged.

The four formerly CDATA-blocked color cases now run. Three pass; the fourth exposes the renderer's dark default foreground versus explicit CSS black. It remains a visible FAIL in the baseline. Fixture integrity checks continue to use original upstream bytes.

Constrained positioning enables three more cases. `border-collapse-offset-001` passes. The padding/margin cases render the intended positioned boxes, but their original table reference expands to the container width instead of its intrinsic width; both remain FAIL with image diffs. Remaining skips still require additional prerequisites such as fonts, per-side borders, external assets, or JavaScript.

The box-sizing float case and `max-height-025` now pass. The former checks side-by-side 50% border-box floats; its static `z-index` declarations are inapplicable. The latter uses `assets` to allowlist `css/CSS2/normal-flow/support/black96x96.png`, pinned at the same revision and SHA-256 verified along with the documents. Image paths cannot resolve to network URLs or escape the fixture tree. Missing, unpinned, or corrupt images fail the run instead of silently rendering alt text. Per-document decoding is limited to 16 images, 4MiB encoded bytes, and four million pixels. No network access occurs during testing.
