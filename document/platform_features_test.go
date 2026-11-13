package document

import (
	"context"
	"strings"
	"testing"
)

func TestSupportsCapabilities(t *testing.T) {
	cases := map[string]bool{
		"not (position-area: block-end)":                                         true,
		"(position-area: block-end)":                                             false,
		"((animation-timeline: view()) and (animation-range: entry))":            false,
		"not ((container-type: scroll-state) or (animation-timeline: scroll()))": true,
		"not selector(:has(*))":                                                  false,
		"not (width: calc(sign(1px) * 1px))":                                     true,
		"(color: light-dark(white, black))":                                      true,
		"(container-type: inline-size)":                                          false,
		"(font-size: 1cqi)":                                                      false,
		"(top: calc(sibling-index() * 1px))":                                     false,
		"(translate: 0px)":                                                       false,
		"(not (mask-image: url(x))) and (not (-webkit-mask-image: url(x)))":      true,
		"at-rule(@starting-style)":                                               false,
		"not at-rule(@starting-style)":                                           true,
		"(display: grid)":                                                        true,
		"(display: nonsense)":                                                    false,
		"(display: block inline)":                                                false,
		"(position: nonsense)":                                                   false,
		"(width: inherit)":                                                       true,
		"(margin: 1px 2px)":                                                      true,
		"(--custom: unknown(1cqi))":                                              true,
		"(color: var(--x))":                                                      true,
		"(color: VAR(x))":                                                        false,
		"(color: var(x))":                                                        false,
		"(unknown: var(--x))":                                                    false,
		"selector(:unknown)":                                                     false,
		"not ((display: grid) and (width: 1px) or (color: red))":                 false,
		"not (display: grid) garbage":                                            false,
		"not ((display: grid)":                                                   false,
	}
	for q, want := range cases {
		if got := Supports(q); got != want {
			t.Errorf("%s = %v, want %v", q, got, want)
		}
	}
	if Supports(strings.Repeat(" ", maxMediaPreludeBytes) + "(display:grid)") {
		t.Fatal("unbounded prelude")
	}
	if Supports(strings.Repeat("(", 100) + "display:grid" + strings.Repeat(")", 100)) {
		t.Fatal("unbounded nesting")
	}
}

func TestSupportsCascadeAndBudget(t *testing.T) {
	src := `<style>@layer base, enhanced; @layer base {p{color:red}} @layer enhanced {
 @media (min-width:10px) { @supports (display:grid) { p {color:blue; @supports not (anchor-name:--x) {text-transform:uppercase} } } }
 } @supports (unknown:yes) { p {display:none} }</style><p>hello</p>`
	d, err := Parse(context.Background(), strings.NewReader(src), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := d.TTY(context.Background(), 60)
	if err != nil || !strings.Contains(text, "HELLO") {
		t.Fatalf("%q %v", text, err)
	}
	r, err := d.Render(context.Background(), Options{Width: 200, NoImage: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range r.Boxes {
		if b.Text == "HELLO" {
			found = true
			if b.Color.B != 255 || b.Color.R != 0 {
				t.Fatalf("layer lost: %+v", b)
			}
		}
	}
	if !found {
		t.Fatal("missing raster content")
	}
	if _, err := parseCSS(`@supports (display:grid) { p{} p{} }`, 1); err == nil {
		t.Fatal("supports bypassed rule budget")
	}
}

func TestPlatformPresentationState(t *testing.T) {
	ctx := context.Background()
	src := `<style>:popover-open {text-transform:uppercase} div:open {display:none}
 @media (prefers-reduced-motion:reduce) { .motion{ text-transform:uppercase } }
 @media (hover:hover) and (scripting:enabled) { .host { text-transform:uppercase } }
 </style><div popover id=p>popup</div><dialog popover open>dialog</dialog><p class=motion>motion</p><p class=host>host</p>`
	d, err := Parse(ctx, strings.NewReader(src), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := d.TTY(ctx, 60)
	if strings.Contains(text, "popup") || !strings.Contains(text, "MOTION") || !strings.Contains(text, "dialog") {
		t.Fatalf("defaults: %q", text)
	}
	state := RenderState{Media: MediaPreferences{ReducedMotion: "no-preference", Hover: "hover", Scripting: "enabled"}, OpenPopovers: []string{"p"}}
	shown, err := d.WithRenderState(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	state.OpenPopovers[0] = "other"
	text, _ = shown.TTY(ctx, 60)
	if !strings.Contains(text, "POPUP") || !strings.Contains(text, "motion") || !strings.Contains(text, "HOST") {
		t.Fatalf("host state: %q", text)
	}
	unchanged, _ := d.TTY(ctx, 60)
	if strings.Contains(unchanged, "POPUP") {
		t.Fatal("mutated original")
	}
	hidden, err := shown.WithRenderState(ctx, RenderState{})
	if err != nil {
		t.Fatal(err)
	}
	text, _ = hidden.TTY(ctx, 60)
	if strings.Contains(text, "POPUP") {
		t.Fatal("open state survived close")
	}
	for _, q := range []string{"(prefers-reduced-motion:wat)", "not (prefers-reduced-motion:wat)"} {
		if evalMediaQueryList(q, 200, "light") {
			t.Fatal(q)
		}
	}
	if !evalMediaQueryList("(prefers-reduced-motion)", 200, "light") || evalMediaQueryList("(hover)", 200, "light") {
		t.Fatal("boolean preferences")
	}
}

func TestDetailsGroupsAndControlBars(t *testing.T) {
	d, err := Parse(context.Background(), strings.NewReader(`<style>details:open summary{text-transform:uppercase}</style>
 <details name=a open><summary>first</summary>body1</details><section><details name=a open><summary>second</summary>body2</details></section>
 <details name="" open><summary>third</summary>body3</details><details name="" open><summary>fourth</summary>body4</details>
 <progress value=3 max=10></progress><meter value=.5></meter><progress></progress>`), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := d.TTY(context.Background(), 60)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"FIRST", "second", "body1", "body3", "body4", "[###-------] 30%", "[#####-----] 50%", "[??????????]"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in %q", s, out)
		}
	}
	if strings.Contains(out, "body2") || strings.Contains(out, "SECOND") {
		t.Fatal(out)
	}
	for _, width := range []int{2, 3, 5, 10, 20} {
		for _, n := range []*node{{tag: "progress", attrs: map[string]string{"value": "NaN"}}, {tag: "meter", attrs: map[string]string{"value": "Infinity"}}, {tag: "progress"}} {
			if len(controlBar(n, width)) > width {
				t.Fatal("bar exceeds width")
			}
		}
	}
}

func FuzzSupports(f *testing.F) {
	for _, s := range []string{"(display:grid)", "not ((width:1px) and (color:red) or (display:flex))", "selector(:has(a))", "(color:var(--x))"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { Supports(s) })
}

func TestRenderResultPreservesPlatformSnapshot(t *testing.T) {
	ctx := context.Background()
	state := RenderState{OpenPopovers: []string{"p"}, Media: MediaPreferences{ReducedMotion: "no-preference"}}
	r, err := Render(ctx, strings.NewReader(`<div id=p popover>visible</div>`), Options{Width: 200, NoImage: true, State: state})
	if err != nil {
		t.Fatal(err)
	}
	state.OpenPopovers[0] = "other"
	out, err := r.TTY(ctx, 60)
	if err != nil || !strings.Contains(out, "visible") {
		t.Fatalf("%q %v", out, err)
	}
	found := false
	for _, b := range r.Boxes {
		if b.Text == "visible" {
			found = true
		}
	}
	if !found {
		t.Fatal("raster omitted shown popover")
	}
}
