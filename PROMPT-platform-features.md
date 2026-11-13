# Task: support the declarative web-platform features agent-written pages now rely on

This is a handoff brief for an agent working in this repository (`nthtml`). It
was written from the `ntwebterm` session after a survey; nothing in this repo
has been changed. Delete this file when the work is done.

## Why

Google Chrome, Edge, Firefox and MDN now publish "Modern Web Guidance"
(<https://github.com/GoogleChrome/modern-web-guidance>): 209 guides that steer
coding agents towards built-in HTML/CSS features and away from JavaScript.
Pages written by agents will increasingly look like those guides. That suits
nthtml, because behaviour moves out of scripts into markup the renderer owns,
but a probe of current `main` (v0.6.0, `-mode tty`) found a few gaps that turn
into wrong or missing content.

The owner's goal: handle pages written to this guidance well, in both `nthtml`
and `ntbrowser`. This brief covers the `nthtml` half. `ntbrowser` consumes
nthtml only as a **published** module version (its AGENTS.md forbids sibling
replaces), so dependency adoption is blocked on a release of this work. Agree the
media and popover-state contracts with ntbrowser before implementing them;
interface design and integration-test preparation need not wait for release.

## What was measured

Probe pages were rendered with a binary built from `main` at `20a94ea`
(`go build ./cmd/nthtml`, then `-mode tty -columns 60`). Each row was a
one-rule stylesheet applying `text-transform: uppercase`, or a bare element.

Already correct, keep it that way: closed `<details>`, closed `<dialog>`,
`hidden`, `<template>`, `:has()`, CSS nesting, `@layer`, `var()`, `clamp()`,
`light-dark()`, range media queries, and unknown values or pseudo-classes
failing soft without poisoning neighbouring rules.

| Feature | Guides using it | Observed on `main` |
| --- | --- | --- |
| `@supports` | 62 | Every block dropped: `(display: grid)`, `not (anchor-name: --a)` and `selector(:has(a))` all had no effect |
| `prefers-reduced-motion` | 49 | Query never matches |
| `popover` attribute | 30 | `<div popover>` renders inline as if open |
| `@container` | 22 | Rules dropped (out of scope here, see below) |
| `<details name>` | 9 | Two `open` siblings with the same `name` both render open |
| `<progress>`, `<meter>` | few | Nothing in TTY output (raster has them in `document/controls.go`) |

The guide count is the number of guide files mentioning the feature, so treat
it as a ranking, not a precise figure.

## Work items

### 1. Evaluate `@supports`

Today `parseCSSBlock` (`document/css.go`) skips `@supports` bodies; the doc
comment on `parseCSS` says so. The guides wrap enhancements in
`@supports (...)` and put fallbacks in `@supports not (...)`, so both branches
are lost. Evaluate the condition and parse the body in place when it is true.

- Evaluate at parse time: a true condition parses its body as if it were
  written inline (same `media`, same `layer`, `depth+1`); a false or
  unparsable one is skipped as now. Handle it in both places block at-rules
  are handled: top level/inside `@media`/`@layer` in `parseCSSBlock`, and
  nested inside a style rule in `emitStyleRule` (next to the nested `@media`
  case).
- Grammar: `not X`, `X and X ...`, `X or X ...`, parenthesised nesting,
  `(property: value)`, `selector(...)`. Anything else (`font-tech()`,
  `font-format()`, `at-rule()`) is unsupported in this implementation. Keep
  condition syntax validity separate from its truth value: a valid unsupported
  feature can become true under `not`, but malformed syntax (including mixed
  `and`/`or` without parentheses) invalidates the condition rather than becoming
  true through negation. Follow CSS Conditional Rules grammar for unknown
  constructs/general-enclosed. Bound prelude length and nesting like media queries
  (`maxMediaPreludeBytes`, `maxMediaDepth`, `maxMediaParenDepth`).
- **Answer truthfully.** True must mean nthtml implements the declaration,
  rather than merely retaining its text or setting a placeholder flag. Start
  the audit by running `declarations("prop: value")`: for built-in properties,
  require a non-empty result where every resulting key is a
  property the resolver consumes. Note the trap: the `default` branch of
  `declarations` stores unknown properties verbatim, so "survived parsing" is
  not enough; you need an explicit set of consumed longhands (the `switch k`
  in `styled`, plus the margin/padding/border longhands, `content`,
  `color-scheme`). This set is only a first filter, not proof of support.
  Reuse or extract property/value validators shared with the resolver instead
  of maintaining a second, drifting interpretation of CSS. Syntactically valid
  custom-property declarations are supported; do not require their contents to
  be valid values for a built-in property.
- Values matter too. Several longhands are stored unvalidated and rejected
  later in `styled` (`display`, `position`, `top`, `z-index`, ...). At
  minimum, reject values using functions or units nthtml does not implement,
  and check `display`/`position` keywords against implemented layout behavior.
  Include CSS-wide keywords, shorthands, and valid `var()` values in tests;
  feature detection must not require resolving variables against an element.
  Audit partial implementations explicitly: setting a `masked` flag does not
  establish support for loading and applying a CSS mask. Record such exclusions
  alongside the capability checks.
- `selector(S)` is true when the selector parser accepts `S`
  (`parseSelectorWithPseudo`), which already rejects unknown pseudo-classes.

Conditions the guides actually use, as test cases (expected result for nthtml
in brackets):

```
not (position-area: block-end)                 [true]
(position-area: block-end)                     [false]
((animation-timeline: view()) and (animation-range: entry))   [false]
not ((container-type: scroll-state) or (animation-timeline: scroll()))  [true]
not selector(:has(*))                          [false]
not (width: calc(sign(1px) * 1px))             [true]
(color: light-dark(white, black))              [true]
(container-type: inline-size)                  [false until @container exists]
(font-size: 1cqi)                              [false]
(top: calc(sibling-index() * 1px))             [false]
(translate: 0px)                               [false]
(not (mask-image: url(x))) and (not (-webkit-mask-image: url(x)))  [true unless actual mask rendering is implemented]
at-rule(@starting-style)                       [false]
(display: grid)                                [true]
```

### 2. Hide `[popover]` by default

A popover is hidden until shown. Give `[popover]` (any value, including empty,
`auto`, `manual`, `hint`) `display: none` in the user-agent defaults, next to
the existing `hidden` and `dialog` handling in `styled`.

Agree the host-to-renderer open-state contract with ntbrowser before coding
it. In browsers this is internal state exposed through `:popover-open`, not
an `open` attribute. Prefer an explicit render-state input where practical;
if serialized markup needs a reserved marker, document its name, ownership,
and lifetime. ntbrowser must generate it from its own current DOM state,
exclude renderer-only markers from page-visible markup getters, and prevent
page-authored copies from impersonating host-generated state. The marker is
presentation state, never activation, navigation, or other host authority.
Standalone nthtml's treatment of the marker must also be explicit.

An open popover may initially render as a bordered box in normal flow. Document
that this does not implement top-layer placement, light dismissal, focus
management, or modal behavior. Cover author CSS overrides and interaction with
`dialog[open]` in the visibility tests instead of unconditionally hiding every
element carrying `popover`.

Accept `:popover-open` and `:open` in the selector parser, with distinct
semantics: `:popover-open` matches a shown popover; `:open` matches supported
HTML open states such as `details[open]` and `dialog[open]`. An arbitrary shown
popover does not thereby match `:open`. Do not advertise unsupported open
states for other controls. Reference: CSS Selectors Level 4, open-state and
popover-open sections (<https://www.w3.org/TR/selectors-4/#open-state>).

### 3. Define consistent media capabilities and preferences

`evalMediaFeature` currently does not implement `prefers-reduced-motion`.
Add it, but distinguish user/host preferences from renderer capabilities:
absence of a CSS animation engine alone does not establish a user's preference.
A standalone reduced-motion default is reasonable if documented. Support
`reduce`, `no-preference`, and the boolean form with consistent semantics.

Make the relevant state configurable by the embedding host, using a small
bounded options contract shared conceptually with ntbrowser's `jsvm.MediaState`.
CSS media evaluation and `window.matchMedia()` must agree for the same page.
At the time of this brief, ntbrowser's `jsvm/media.go` reports reduced motion
as `no-preference`, hover as available, and scripting as enabled. Do not
silently contradict these answers by hardcoding different renderer values.
Document standalone defaults and the required ntbrowser integration explicitly.

Keep the initial scope narrow. Add `hover`/`any-hover` or `scripting` only if
the same host contract supplies their values: a standalone document engine
can use `scripting: none`, but an embedded scripting browser cannot inherit
that answer unconditionally. Do not guess pointer capability. Validate feature
values and boolean forms rather than accepting arbitrary strings.

Separately, confirm `prefers-color-scheme: dark` follows `Options.ColorScheme`.
Test default and host-overridden preferences, both color schemes, and changes
that require media rules to be reevaluated. The ntbrowser follow-up must test
agreement between rendered CSS and `matchMedia()`, including child documents.

### 4. `<progress>` and `<meter>` in TTY output, and `<details name>`

- Raster mode draws `progress`/`meter` (`document/controls.go`); TTY output
  emits nothing for them. Render a text bar with the value, for example
  `[######----] 60%`, respecting `value`/`max` (and `min`/`low`/`high`/
  `optimum` for meter where it is cheap). An indeterminate `<progress>` with
  no `value` needs a distinct form. Fall back to the element's text content
  only if that is what the raster path does. Share value normalization with
  raster controls so defaults, malformed/non-finite numbers, out-of-range
  values, and clamping agree. Bound bar width to the available TTY width.
- `<details name="x">`: at most one element per name group (same tree, same
  `name`) is open. When markup has several `open`, the first in tree order
  wins and the rest render closed. Group only non-empty names, including
  non-sibling members of the same tree. Normalize state consistently for
  child visibility, disclosure markers, and selectors; do not merely hide
  children while `:open` still reports them open. Use a bounded tree pass or
  equivalent index rather than scanning the entire tree for every element.
  See `displayedChildren` in `document/element_defaults.go`.
- Static normalization is not the live DOM algorithm. The ntbrowser follow-up
  must update actual `open` attributes when another group member opens,
  preserving mutation/event semantics and insertion/name-change handling.
  Rendering must not silently substitute for that DOM behavior. Cover empty
  names, independent groups, non-siblings, and multiple initial `open` members.
  Reference: <https://html.spec.whatwg.org/multipage/interactive-elements.html#the-details-element>.

## Out of scope here

- `@container`, `@scope`, declarative shadow DOM, anchor positioning, view
  transitions, scroll-driven animation, scroll snap. Note in the README
  matrix that `@supports` reports them as unsupported, which is what lets the
  guides' fallbacks apply.
- Anything interactive: `popovertarget` / `commandfor` buttons, `showModal()`,
  `showPopover()`, `IntersectionObserver`. Those belong to `ntbrowser`, which
  owns the DOM and events. Its work depends on item 2's open-state convention
  and on a published release containing this work. Plan the declarative
  activation follow-up alongside default hiding: hidden popovers alone do not
  make popover-based navigation or controls usable in ntbrowser.

## Constraints and conventions

- This is a bounded, hostile-input renderer: every new parser needs length
  and depth limits, must not loop on malformed input, and must count rules
  against the existing budget. Add fuzz seeds if `task fuzz` covers CSS.
- Update the README support matrix: it currently says `@supports` bodies are
  skipped and that the supported properties are "exactly those named in the
  table". Keep the matrix true.
- Add a `CHANGELOG.md` entry under `## Unreleased`.
- Tests live beside the code (`document/*_test.go`; see
  `media_range_test.go` for the table style and `layers_test.go` for at-rule
  tests). Cover TTY and raster paths where behaviour differs, and add
  conformance cases if `internal/conformance/manifest.json` is the place for
  them.
- All builds and tests run with `GOWORK=off CGO_ENABLED=0`; `task check` and
  `task ci` are the gates.
- Follow this repo's own rules for commits. Tagging and publishing a release
  is the owner's decision; say when the work is ready for one, since
  `ntbrowser` cannot adopt it before then.

## Suggested order

Agree the media and popover contracts first. Item 1 is the largest and the
most valuable; item 4 can proceed independently. Items 2 and 3 are modest
renderer changes once their embedding contracts are settled. Use focused
commits, separating the two parts of item 4 where useful.

## How to check the result

Re-run the probes under the documented standalone defaults (the reduced-motion
probe below assumes the proposed `reduce` default). Each should behave as stated:

```sh
t() { printf '<style>%s</style>%s' "$1" "$2" > /tmp/one.html; bin/nthtml -mode tty -columns 60 /tmp/one.html; }
U='text-transform:uppercase'
t "@supports (display:grid) { p { $U } }" '<p>x</p>'                 # X
t "@supports (anchor-name:--a) { p { $U } }" '<p>x</p>'              # x
t "@supports not (anchor-name:--a) { p { $U } }" '<p>x</p>'          # X
t "@supports selector(:has(a)) { p { $U } }" '<p>x</p>'              # X
t "@media (prefers-reduced-motion: reduce) { p { $U } }" '<p>x</p>'  # X
t "" '<div popover>POP</div><p>after</p>'                            # after
t "" '<progress value=3 max=10></progress><meter value=.5></meter>'  # two bars
t "" '<details name=a open><summary>s1</summary>b1</details><details name=a open><summary>s2</summary>b2</details>'  # only s1 open
```

The uppercase probes test condition selection, not full feature behavior. Add
representative fixtures for a fallback/enhancement layout, responsive navigation
with a closed popover, grouped details, and progress/meter controls. Check TTY
text and raster layout, including narrow widths. In ntbrowser integration tests,
exercise opening/closing controls and compare CSS media results with JS results.
Do not claim end-to-end interactive compatibility from static rendering alone.

Add parser regressions for malformed conditions under `not`, nested boolean
conditions, valid unsupported features, invalid values of supported properties,
custom properties, `var()`, nested at-rules, and preservation of layer/source
order. Verify that oversized/deep input and large rule counts remain bounded.
