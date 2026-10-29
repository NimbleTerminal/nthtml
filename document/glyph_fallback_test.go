package document

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"nimbleterminal.dev/ntfonts"
)

// Reuse Go's licensed outlines, replacing only cmap with a tiny test coverage
// table. Every requested character maps to A; no extra font binary is vendored.
func coverageFont(t *testing.T, chars ...rune) []byte {
	t.Helper()
	data := append([]byte(nil), goregular.TTF...)
	parsed, err := opentype.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	glyph, err := parsed.GlyphIndex(nil, 'A')
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	n := len(chars) + 1
	table := make([]byte, 12+16+8*n)
	put := func(i, v int) { binary.BigEndian.PutUint16(table[i:], uint16(v)) }
	put(2, 1)
	put(4, 3)
	put(6, 1)
	binary.BigEndian.PutUint32(table[8:], 12)
	put(12, 4)
	put(14, len(table)-12)
	put(18, n*2)
	power, entry := 1, 0
	for power*2 <= n {
		power *= 2
		entry++
	}
	put(20, 2*power)
	put(22, entry)
	put(24, 2*n-2*power)
	for i, r := range append(chars, 0xffff) {
		put(26+2*i, int(r))
		put(28+2*n+2*i, int(r))
		delta := int(glyph) - int(r)
		if r == 0xffff {
			delta = 1
		}
		put(28+4*n+2*i, delta)
	}
	tables := int(binary.BigEndian.Uint16(data[4:]))
	for i := 0; i < tables; i++ {
		rec := data[12+16*i : 12+16*(i+1)]
		if string(rec[:4]) != "cmap" {
			continue
		}
		off := int(binary.BigEndian.Uint32(rec[8:]))
		old := int(binary.BigEndian.Uint32(rec[12:]))
		if len(table) > old {
			t.Fatal("test cmap too large")
		}
		copy(data[off:], table)
		binary.BigEndian.PutUint32(rec[12:], uint32(len(table)))
		return data
	}
	t.Fatal("no cmap")
	return nil
}
func fallbackRender(t *testing.T, html string, o Options) *Result {
	t.Helper()
	if o.Width == 0 {
		o.Width = 500
	}
	r, err := Render(context.Background(), strings.NewReader(html), o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestGlyphFallbackGoAndMissing(t *testing.T) {
	r := fallbackRender(t, "<p style='font-family:Literata'>a ◦ b</p>", Options{})
	missing := fallbackRender(t, "<p style='font-family:Literata'>a \uffff b</p>", Options{})
	if r.MissingGlyphs != 0 || missing.MissingGlyphs != 1 {
		t.Fatalf("counts %d %d", r.MissingGlyphs, missing.MissingGlyphs)
	}
	if string(r.Image.Pix) == string(missing.Image.Pix) {
		t.Fatal("fallback still .notdef")
	}
	// The primary remains Go; bundled sans and serif text faces are now
	// eligible after symbol fallback.
	text := "Dvořák Ελληνικά Русский Tiếng Việt → ✓"
	expected := 0
	var fonts []*opentype.Font
	for _, family := range append(ntfonts.Fallbacks(), "IBM Plex Sans", "Literata", "Go", "Go Mono") {
		source := FaceSource(ntfonts.Embedded)
		if strings.HasPrefix(family, "Go") {
			source = MinimalFonts
		}
		data, _ := source.Face(family, "Regular")
		f, err := opentype.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		fonts = append(fonts, f)
	}
	for _, ch := range text {
		covered := false
		for _, f := range fonts {
			id, _ := f.GlyphIndex(nil, ch)
			if id != 0 {
				covered = true
				break
			}
		}
		if !covered {
			expected++
		}
	}
	r = fallbackRender(t, "<p>"+text+"</p>", Options{})
	if r.MissingGlyphs != expected {
		t.Fatalf("got %d want %d", r.MissingGlyphs, expected)
	}
}
func TestHostGlyphFallbackAndRegularVariant(t *testing.T) {
	data := coverageFont(t, '漢')
	calls := 0
	host := fontSource(func(family, variant string) ([]byte, error) {
		calls++
		if family != "CJK" || variant != "Regular" {
			return nil, fmt.Errorf("missing")
		}
		return data, nil
	})
	without := fallbackRender(t, "<p>漢</p>", Options{})
	if without.MissingGlyphs != 1 {
		t.Fatal(without.MissingGlyphs)
	}
	r := fallbackRender(t, "<p><b>漢漢</b></p>", Options{FallbackFaces: []FallbackFace{{Family: "CJK", Source: host}}})
	if r.MissingGlyphs != 0 || calls != 2 {
		t.Fatalf("missing=%d calls=%d", r.MissingGlyphs, calls)
	}
	repainted, err := r.Paint(context.Background(), r.Bounds(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(repainted.Pix) != string(r.Image.Pix) {
		t.Fatal("repaint changed font split")
	}
}
func TestCSSGlyphFallbackAndCombining(t *testing.T) {
	primary := coverageFont(t, 'e')
	second := coverageFont(t, 'e', '\u0301', '漢')
	source := fontSource(func(family, variant string) ([]byte, error) {
		switch family {
		case "Iosevka":
			return primary, nil
		case "Literata":
			return second, nil
		}
		return nil, fmt.Errorf("missing")
	})
	for _, html := range []string{
		"<p style='font-family:Iosevka,Literata'>é漢</p>",
		"<pre style='font-family:Iosevka,Literata;width:1px'>é漢</pre>",
	} {
		r := fallbackRender(t, html, Options{Fonts: source})
		if r.MissingGlyphs != 0 {
			t.Fatal(r.MissingGlyphs)
		}
		found := false
		for _, op := range r.ops {
			for _, run := range op.runs {
				if strings.Contains(run.text, "é") {
					found = true
					if run.key.family != "Literata" {
						t.Fatal(run)
					}
				}
			}
		}
		if !found {
			t.Fatal("combining sequence split")
		}
	}
	r := fallbackRender(t, "<p style='font-family:Missing,Literata'>e</p>", Options{Fonts: source})
	for _, op := range r.ops {
		for _, run := range op.runs {
			if run.key.family != "Literata" {
				t.Fatal(run)
			}
		}
	}
}
func TestGlyphFallbackBudget(t *testing.T) {
	var hosts []FallbackFace
	calls := 0
	source := fontSource(func(string, string) ([]byte, error) { calls++; return nil, fmt.Errorf("absent") })
	for i := 0; i < 100; i++ {
		hosts = append(hosts, FallbackFace{Family: fmt.Sprint(i), Source: source})
	}
	r := fallbackRender(t, "<p>漢漢漢</p>", Options{FallbackFaces: hosts})
	if r.MissingGlyphs != 3 || calls > 64 {
		t.Fatalf("missing=%d calls=%d", r.MissingGlyphs, calls)
	}
}
func TestGlyphFallbackWrapping(t *testing.T) {
	r := fallbackRender(t, "<p style='font-family:Literata;width:70px'>ařa ařa ařa</p>", Options{})
	lines := map[int]bool{}
	for _, op := range r.ops {
		if op.box.Text == "" {
			continue
		}
		lines[op.box.Bounds.Min.Y] = true
		l := &layout{opts: r.opts, faces: map[faceKey]font.Face{}, fonts: map[string]*opentype.Font{}}
		width := l.runsAdvance(op.runs, boxStyle(op.box)).Ceil()
		for _, face := range l.faces {
			face.Close()
		}
		if width != op.box.Bounds.Dx() {
			t.Fatalf("paint width %d layout %d", width, op.box.Bounds.Dx())
		}
	}
	if len(lines) < 2 {
		t.Fatal("expected wrapping")
	}
}
func TestFallbackPaintCropAndNoWarnings(t *testing.T) {
	r := fallbackRender(t, "<p style='font-family:Literata'>ř \uffff</p>", Options{NoImage: true})
	if r.MissingGlyphs != 1 || len(r.Warnings) != 0 {
		t.Fatalf("missing=%d warnings=%v", r.MissingGlyphs, r.Warnings)
	}
	if _, err := r.Paint(context.Background(), image.Rect(0, 0, 200, r.Bounds().Dy()), 2); err != nil {
		t.Fatal(err)
	}
	if got := defaultTTY(t, "<p>漢é</p>"); utf8.RuneCountInString(got) != 3 {
		t.Fatal(got)
	}
}

func TestBundledSymbolFallback(t *testing.T) {
	// A page FaceSource must not intercept the bundled fallback lookup.
	source := fontSource(func(string, string) ([]byte, error) { return nil, fmt.Errorf("missing") })
	r := fallbackRender(t, "<p style='font-family:Missing'>▸▾▶▼◦▪■□→✓</p>", Options{Fonts: source})
	if r.MissingGlyphs != 0 {
		t.Fatalf("missing symbols: %d", r.MissingGlyphs)
	}
	found := false
	for _, op := range r.ops {
		for _, run := range op.runs {
			if run.key.source == -2 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("bundled symbol fallback was not used")
	}
}

func TestHostPrecedesBundledFallback(t *testing.T) {
	data := coverageFont(t, '▸')
	host := fontSource(func(string, string) ([]byte, error) { return data, nil })
	r := fallbackRender(t, "<p>▸</p>", Options{FallbackFaces: []FallbackFace{{Family: "HostSymbols", Source: host}}})
	if r.MissingGlyphs != 0 {
		t.Fatal(r.MissingGlyphs)
	}
	for _, op := range r.ops {
		for _, run := range op.runs {
			if run.key.source != 1 {
				t.Fatalf("host fallback lost precedence: %+v", run)
			}
		}
	}
}

func TestExpandedTextCoverage(t *testing.T) {
	r := fallbackRender(t, "<p>ř ł ş ő ơ ạ α ж</p>", Options{FontFamilies: FontFamilies{Default: "Literata"}})
	if r.MissingGlyphs != 0 {
		t.Fatalf("expanded text missing %d glyphs", r.MissingGlyphs)
	}
}

func TestBundledTextFallbackCoverage(t *testing.T) {
	for _, html := range []string{
		"<p>Tiếng Việt ₹</p>",
		"<p style='font-family:Missing'>Tiếng Việt ₹</p>",
		"<p style='font-family:monospace'>Tiếng ┌─┐</p>",
	} {
		r := fallbackRender(t, html, Options{})
		if r.MissingGlyphs != 0 {
			t.Errorf("%s: missing %d", html, r.MissingGlyphs)
		}
	}
	r := fallbackRender(t, "<p>ꯀ</p>", Options{})
	if r.MissingGlyphs != 1 {
		t.Fatal(r.MissingGlyphs)
	}
	r = fallbackRender(t, "<p style='font-family:Go'>a</p>", Options{})
	for _, op := range r.ops {
		for _, run := range op.runs {
			if run.key.family != "Go" || run.key.source != -1 {
				t.Fatalf("primary changed: %+v", run)
			}
		}
	}
}

func TestBundledTextFallbackOrder(t *testing.T) {
	primary := coverageFont(t, 'a')
	fallback := coverageFont(t, '₹')
	source := fontSource(func(family, variant string) ([]byte, error) {
		switch family {
		case "CustomSerif", "CustomSans", "CustomMono":
			return fallback, nil
		case "Newsreader", "Iosevka":
			return primary, nil
		}
		return nil, fmt.Errorf("missing")
	})
	families := FontFamilies{Serif: "CustomSerif", SansSerif: "CustomSans", Monospace: "CustomMono"}
	for _, tc := range []struct{ primary, want string }{
		{"Go", "CustomSans"}, {"Newsreader", "CustomSerif"}, {"Iosevka", "CustomMono"},
	} {
		r := fallbackRender(t, "<p style='font-family:"+tc.primary+";font-weight:bold;font-style:italic'>₹</p>", Options{Fonts: source, FontFamilies: families})
		if r.MissingGlyphs != 0 {
			t.Fatal(r.MissingGlyphs)
		}
		for _, op := range r.ops {
			for _, run := range op.runs {
				if run.key.family != tc.want || !run.key.bold || !run.key.italic {
					t.Fatalf("%s: %+v", tc.primary, run)
				}
			}
		}
	}
}

func TestBundledTextFallbackCacheAllocations(t *testing.T) {
	l := &layout{}
	s := style{size: 16, family: "Go"}
	primary := l.face(s)
	defer func() {
		for _, f := range l.faces {
			f.Close()
		}
	}()
	l.bundledTextFallbacks(s, primary)
	if allocations := testing.AllocsPerRun(100, func() { l.bundledTextFallbacks(s, primary) }); allocations != 0 {
		t.Fatalf("cached candidate lookup allocated %g times", allocations)
	}
	// Variant and monospace changes must not reuse incompatible candidates.
	s.bold = true
	if got := l.bundledTextFallbacks(s, primary); !got[0].bold {
		t.Fatal("cached regular variant reused")
	}
	s.mono = true
	if got := l.bundledTextFallbacks(s, primary); got[0].family != "IBM Plex Mono" {
		t.Fatal("cached proportional family reused")
	}
	serif := l.face(style{size: 16, family: "Literata"})
	s = style{size: 16, family: "Go"}
	if got := l.bundledTextFallbacks(s, serif); got[0].family != "Literata" {
		t.Fatal("cached sans order reused for serif primary")
	}
}

func TestBundledTextFallback(t *testing.T) {
	for _, tc := range []struct {
		name, html string
		missing    int
	}{
		{"default", "<p>Tiếng Việt ₹</p>", 0},
		{"unknown CSS", "<p style='font-family:Unavailable'>Tiếng Việt ₹</p>", 0},
		{"monospace", "<p style='font-family:monospace'>Tiếng ┌─┐</p>", 0},
		{"missing", "<p>ꯀ</p>", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fallbackRender(t, tc.html, Options{Width: 300})
			if r.MissingGlyphs != tc.missing {
				t.Fatalf("missing=%d, want %d", r.MissingGlyphs, tc.missing)
			}
		})
	}
	for _, html := range []string{"<p>a</p>", "<p style='font-family:Go'>a</p>"} {
		r := fallbackRender(t, html, Options{})
		found := false
		for _, op := range r.ops {
			for _, run := range op.runs {
				if run.text == "a" {
					found = true
					if run.key.family != "Go" || run.key.source != -1 {
						t.Fatalf("primary changed: %+v", run)
					}
				}
			}
		}
		if !found {
			t.Fatal("missing primary run")
		}
	}
}

func TestBundledTextFallbackOrderAndVariants(t *testing.T) {
	// The page's primary faces deliberately lack the probe. Bundled candidates
	// must bypass that source and preserve the requested weight and slant.
	primary := coverageFont(t, 'a')
	source := fontSource(func(string, string) ([]byte, error) { return primary, nil })
	for _, tc := range []struct {
		family      string
		mappings    FontFamilies
		probe, want string
	}{
		{"Literata", FontFamilies{}, "ế", "Literata"},
		{"IBM Plex Sans", FontFamilies{}, "ế", "IBM Plex Sans"},
		{"Newsreader", FontFamilies{Serif: "Source Serif 4"}, "ế", "Source Serif 4"},
		{"IBM Plex Sans", FontFamilies{SansSerif: "JetBrains Mono"}, "ế", "JetBrains Mono"},
		{"Go Mono", FontFamilies{}, "ế", "IBM Plex Mono"},
		{"Iosevka", FontFamilies{}, "ế", "IBM Plex Mono"},
		{"JetBrains Mono", FontFamilies{Monospace: "Iosevka"}, "ế", "Iosevka"},
		{"monospace", FontFamilies{Monospace: "Unavailable"}, "┌", "Iosevka"},
	} {
		for _, variant := range []struct {
			css          string
			bold, italic bool
		}{
			{"", false, false}, {"font-weight:bold", true, false}, {"font-style:italic", false, true}, {"font-weight:bold;font-style:italic", true, true},
		} {
			t.Run(tc.family+"/"+variant.css, func(t *testing.T) {
				html := fmt.Sprintf("<p style='font-family:%s;%s'>%s</p>", tc.family, variant.css, tc.probe)
				r := fallbackRender(t, html, Options{Fonts: source, FontFamilies: tc.mappings})
				if r.MissingGlyphs != 0 {
					t.Fatal("missing probe", r.MissingGlyphs)
				}
				found := false
				for _, op := range r.ops {
					for _, run := range op.runs {
						if run.text == tc.probe {
							found = true
							if run.key.family != tc.want || run.key.source != -2 || run.key.bold != variant.bold || run.key.italic != variant.italic {
								t.Fatalf("wrong fallback: %+v; want %s", run, tc.want)
							}
						}
					}
				}
				if !found {
					t.Fatal("probe not rendered")
				}
			})
		}
	}
}

func TestBundledTextFallbackUsesActualPrimary(t *testing.T) {
	primary := coverageFont(t, 'a')
	source := fontSource(func(family, variant string) ([]byte, error) {
		if family == "Newsreader" {
			return primary, nil
		}
		return nil, fmt.Errorf("absent")
	})
	r := fallbackRender(t, "<p style='font-family:Unavailable,Newsreader'>ế</p>", Options{Fonts: source})
	for _, op := range r.ops {
		for _, run := range op.runs {
			if run.text == "ế" && (run.key.family != "Literata" || run.key.source != -2) {
				t.Fatalf("not serif-first: %+v", run)
			}
		}
	}
	if r.MissingGlyphs != 0 {
		t.Fatal(r.MissingGlyphs)
	}
}
