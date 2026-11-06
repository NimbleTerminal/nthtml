# HTML/CSS subset for shared typesetting

Status: proposed conformance target, with an audited implementation inventory.
This is the migration contract for consumers seeking one nttypeset rendering
path. It is **not a claim that the current adapter conforms to this subset**.
`typeset.Build`, `BuildWithSources`, and `BuildChunked` currently preserve a
reading document with some resolved CSS. `document.Render` remains the page
renderer while the gaps below are closed.

## What conformance will mean

For a supported document, HTML parsing, CSS cascade/inheritance, used values,
box layout, painting, and document coordinates must agree with the documented
subset. Preserving a property in an AST is necessary but does not demonstrate
its layout semantics. Non-browser paragraph spacing, table rules or heading
colors must not override supported author CSS. A uniform CSS-pixel scale must
be specified before comparing browser geometry.

Two separate result sets are required:

- Content/interaction: text and image preservation, source order, link targets,
  fragments, source identity, selection and copying, including chunk navigation.
- HTML/CSS rendering: geometry and pixels against independent browser results
  under fixed viewport, fonts, assets, zoom and device scale. Every case states
  its prerequisites and its geometry/pixel tolerance before baselining.

Native `document.Render` WPT results do not transfer to the typeset adapter.
The existing WPT reference harness renders both sides with the native renderer;
its Chromium differential harness provides independent comparisons. Extend
these with an explicitly named typeset backend and separate reviewed results.
Never overwrite native baselines or count an unsupported case as passing.

## Initial target and current gaps

The initial target is static, single-column documents. This table is a work
list, not an automatic capability detector. Existing README preservation claims
remain useful implementation details; they do not imply browser conformance.

| Area | Initial target | Current adapter evidence / gap |
| --- | --- | --- |
| Structure | HTML parsing; body, headings, paragraphs, nested div/section/article/main/aside/header/footer; source-order content | Blocks and nested sections exist. Browser-equivalent default spacing and used dimensions are not established. |
| Cascade | Documented native selectors, inheritance, specificity and supported declarations, including inline style | Reuses nthtml's resolved styles. Native selector passes still need adapter-specific rendering coverage. |
| Text | Uniform run font metrics, bold/italic/monospace, author colors, explicit font size and line height, normal/preformatted whitespace, explicit breaks | Font roles, default margins and line breaking differ; mixed-size inline runs and inline background boxes are gaps. |
| Boxes | Normal block flow, CSS width/min/max sizing and box sizing, solid backgrounds, per-side solid borders, padding, nonnegative margins | Section decoration is translated. Margins are additive, not CSS-collapsed; width/box sizing and inherited-versus-noninherited values need independent geometry coverage. |
| Images | Explicitly supplied local images, alt fallback, intrinsic/aspect-ratio sizing, explicit dimensions, image-only lines and linked images | Bare image-only runs between blocks now preserve the asset. Mixed inline images become alt text. The fix does not establish replaced-element baseline or used-size conformance; linked-image styles also need auditing. |
| Lists | Ordered/unordered lists, numbering and markers, nested lists | Reading-order list extraction exists; indentation, marker geometry and spacing require browser comparisons. |
| Tables | Simple rectangular tables without spans, cell content and author foreground/background/border styling | nttypeset uses content-weighted columns and default header fill/row rules. This is a layout gap, not conformant table CSS. |
| Identity | IDs, local fragments, external links, selection and logical copy text | Source maps and chunk ranges exist. Image-only paragraphs preserve their paragraph, link and image IDs, with regression coverage for reflow, chunk ranges, hidden/duplicate IDs and alt fallback. Test chunk eviction and composed-document fragment jumps; do not reuse chunk-local coordinates globally. |
| Hidden content | `display:none` and documented HTML hidden behavior | Hidden subtrees are omitted, including image-only paragraphs and their enclosing links. Complete/chunked regression tests and an authored Chromium image-pixel probe cover this shortcut. Preserve visible anchor placeholders only under the existing documented adapter contract. |

### Outside the initial target

Scripts/DOM execution, forms interaction, remote fetching, flex/grid,
floats/clear, positioned/overlapping content and stacking, transforms,
pseudo-element generated content, background images, mixed inline replaced
objects, overflow clipping/ellipsis, negative margins, complex table spans and
collapsed-border conflict resolution are outside the initial target. Some are
supported by the native renderer or partially translated today; that does not
make them supported by this conformance profile. Add them only with explicit
semantics and independent tests.

### Unsupported features and failures

The legacy adapter is permissive: it may flatten positioned layouts, omit
background images, simplify tables, substitute image alt text, or approximate
borders and spacing. It does not currently emit a complete unsupported-feature
report. **A consumer cannot yet use this profile to promise faithful fallback.**

Before migration, add an opt-in audited capability report identifying unsupported
features with source IDs where possible. It must cover resolved requirements,
not merely scan raw CSS property names. The host then either retains its CSS
renderer or reports the unsupported document. Existing permissive APIs remain
compatible. Do not silently label fallback output conformant, and do not retry
untrusted content with relaxed resource limits.

### Text styling coverage and remaining limits

Headings retain resolved font weight and foreground colors. Without author
colors they use the document/host text palette, not an automatic heading accent;
link styling remains independent. Explicit and inherited positive line heights
are passed to heading, paragraph and preformatted layouts. Unitless inheritance
scales with the child's font size; inherited lengths and percentages retain their
computed length. Chromium computed-style probes check these values, and raster
regressions measure line spacing in complete and chunked documents.

This is not full CSS line-box conformance: nttypeset still clamps painted line
height to at least the shaped font's ascent plus descent. Zero/tight line heights,
mixed inline font sizes and browser-equivalent baseline placement require shared
layout changes. The supported spacing tests use heights above that minimum.

### Composed box styling probe

The authored `internal/conformance/testdata/authored/callout-boxes.html` fixture
combines nested heading, paragraph and preformatted blocks, asymmetric padding,
per-side solid borders and distinct backgrounds. Bordered leaf blocks now leave
box spacing/decoration to their enclosing section and suppress reading defaults
inside it. Complete/chunked raster tests verify border/background paint, padding
ownership and absence of background/padding inheritance.

An independent Chromium check confirms that adding the default `padding:0` to
its bordered heading/paragraph changes no pixels; the adapter must satisfy the
same equivalence. This is a focused property test, **not** a claim that the whole
callout matches Chromium's geometry or fonts. Default margins, collapsed margins,
width constraints, empty boxes and browser-equivalent chunk fragmentation remain
separate work. The next useful steps are shared width/image sizing semantics and simple table
styling; the explicit spacing audit below covers a further adapter gap.

### Explicit vertical spacing

Explicit top/bottom margins now survive extraction for preformatted blocks,
list containers without IDs/decoration, and simple list items. Zero and small
list-item bottom margins are no longer replaced with a four-pixel minimum.
Complete/chunked conversion tests cover longhands and zero-margin shorthand.
Chromium pixel probes independently check that adding 20px of top/bottom margin
moves the appropriate text by 20px when collapse is prevented by parent padding
and adjacent margins are zero.

This verifies explicit spacing, not general CSS margin layout. Margins still
add rather than collapse; horizontal/auto and negative margins, complex list
items containing blocks, and default list indentation/markers remain outside
this change. The image-cap step below covers the first part of width/image sizing.

### Image maximum dimensions

Image blocks now pass positive absolute `max-width` and `max-height` values to
nttypeset's shrink-only bounds, including CSS lengths such as `em` resolved by
the cascade. Complete/chunked raster tests and Chromium image-pixel measurements
cover individual/combined caps, smaller explicit width, stylesheet declarations,
non-inheritance and maxima larger than the intrinsic image. These measurements
compare image sizes, not full-page placement.

The supported target is aspect-ratio-preserving downscaling. Percentage caps need
the final containing-block dimensions; zero-size images, enlargement/minimum
sizes, independently forced width and height, border-box image sizing and general
container width/centering require further shared layout semantics. Those features
are not made conformant by this change; existing permissive behavior remains.

### Table text styles and the next model boundary

Simple table extraction now preserves resolved cell font weight: default `th`
remains bold, CSS normal/400 overrides survive, and `td` inside `thead` is not
made bold merely because of its group. Complete/chunked tests and an independent
Chromium computed-style probe cover these cases and inline overrides. Header
identity remains separate from font weight.

Cell padding, alignment, backgrounds and per-side borders cannot yet be passed
through the shared table model: `nttypeset.Block.Rows` contains only spans.
Its layout supplies fixed 12px horizontal/10px vertical insets, a first-row fill
and row rules. The next substantive table step needs bounded opt-in per-cell
style metadata in nttypeset, preserving its existing defaults for other callers,
then nthtml translation and independent geometry/pixel tests. Do not simulate
those styles by flattening simple tables or painting over the shared renderer.

## Recurring fix classes

Use these groups to choose small, independently testable changes:

- **Traversal shortcuts:** specialized extraction must preserve the same resolved
  visibility, inherited styles, IDs and ancestry as ordinary traversal. The
  image-only path exposed both hidden-content and missing-fragment bugs. Audit
  preformatted-text, list and table shortcuts against the same invariants;
  candidates are not claims of confirmed failures.
- **Reading defaults overriding CSS:** preserve resolved values before applying
  adapter presentation defaults. Heading font weight now comes from the cascade,
  including normal-weight descendants; complete/chunked tests and an authored
  Chromium computed-style comparison cover it. Heading text now retains its resolved color instead of gaining an accent role;
  table header fills/rules remain audit targets. Computed-style agreement does not
  establish font-metric or pixel conformance.
- **Missing layout-model semantics:** width/min/max sizing, empty boxes, box
  sizing, margin collapsing and fragmentation need explicit shared layout
  semantics and independent geometry tests. Avoid approximating these through
  host HTML rewriting or unrelated spacing adjustments.

Keep measurement gaps separate from rendering bugs: missing source geometry,
unsupported font mapping and incomplete pixel results must stay explicit in the
harness. Improvements to instrumentation do not count as rendering fixes.

## Chunking contract

Parsing and source indexing remain whole-document and bounded. Per-chunk ASTs,
assets, layout, glyphs, rasters and caches retain their existing limits. Oversized
indivisible units fail without truncation. A chunk index accompanies every
selection or anchor; hosts own cross-chunk copying and navigation.

Repeated ancestor decoration at a chunk boundary is the current reading-mode
behavior. It is not CSS box-fragmentation conformance. Until a fragmentation
contract is tested, validate rendering conformance on complete documents and
report chunk boundary behavior separately. Do not claim that a document becomes
CSS-conformant merely by dividing it into bounded chunks.

## Implementation sequence and migration gate

1. Preserve essential content and identity. `TestSubsetStandaloneImageBetweenBlocks`
   covers the first fix through complete and chunked adapters, including actual
   asset pixels, order and fragment lookup. Boundary tests keep mixed-inline
   fallback and hidden-content behavior explicit. These are local preservation
   tests, not WPT or browser conformance passes.
2. The initial `task differential:typeset` backend now exercises the 20 existing
   pinned WPT differential probes, with separate JSON/HTML/PNG artifacts and
   reviewed status observations. It does not yet establish profile conformance:
   some probes are outside the initial target, source envelopes are incomplete,
   and Ahem font-family mapping is unsupported. Extend this infrastructure,
   starting with small authored cases for boxes, text, images, lists and tables.
   Keep genuine discrepancies FAIL; prerequisites outside the target are
   UNSUPPORTED; unavailable browser/font infrastructure is ERROR or explicitly
   skipped, never PASS. Gate CI against a reviewed baseline, including improvements.
3. Close translation gaps in nthtml and layout-model gaps in nttypeset. Required
   geometry fixes must not be implemented as host-specific HTML rewriting.
4. Add capability reporting and test fallback decisions. Validate composed
   documents as well as isolated properties. Include the Gloss field-notes and
   nested-section shapes, linked figures, tables, and conversation histories.
5. Migrate a host only when its required cases pass and behavior changes are
   documented. Include viewing, exports, search, selection, local fragments,
   resource ownership, cancellation and stale-frame rejection. Measure removed
   host code and retained dependency cost, not just the number of renderer APIs.

Gloss must keep its current renderer until that gate passes. No dependency pin,
renderer default, export contract, or ntbrowser rollout is changed by this work.
