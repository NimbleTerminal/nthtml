package document

import (
	"cmp"
	"image"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"nimbleterminal.dev/ntfonts"
)

type faceRecord struct {
	key    faceKey
	parsed *opentype.Font
}
type coverageKey struct {
	parsed *opentype.Font
	r      rune
}
type splitKey struct {
	face        faceKey
	mono, small bool
	text        string
}

// Options are fixed for a layout, so generic-family mappings need not be
// repeated in this render-local cache key.
type textFallbackKey struct {
	face          faceKey
	primaryFamily string
	mono          bool
}

type glyphRun struct {
	key     faceKey
	text    string
	missing int
	basic   bool
}

func boxStyle(b Box) style {
	return style{size: b.FontSize, family: b.FontFamily, bold: b.Bold, italic: b.Italic, mono: b.Monospace, smallCaps: b.SmallCaps, wordSpacing: b.WordSpacing, letterSpacing: b.LetterSpacing}
}
func (l *layout) covers(f font.Face, r rune) bool {
	if zeroWidth(r) {
		return true
	}
	info, ok := l.faceInfo[f]
	if !ok {
		_, ok = f.GlyphAdvance(r)
		return ok
	}
	key := coverageKey{info.parsed, r}
	if found, ok := l.coverage[key]; ok {
		return found
	}
	index, err := info.parsed.GlyphIndex(nil, r)
	found := err == nil && index != 0
	if l.coverage == nil {
		l.coverage = map[coverageKey]bool{}
	}
	if len(l.coverage) < 262144 {
		l.coverage[key] = found
	} else {
		l.fontStopped = true
	}
	return found
}
func (l *layout) clusterFace(s style, text string) font.Face {
	primary := l.face(s)
	all := func(f font.Face) bool {
		for _, r := range text {
			if !l.covers(f, r) {
				return false
			}
		}
		return true
	}
	if all(primary) {
		return primary
	}
	base, _ := utf8.DecodeRuneInString(text)
	baseFace := primary
	baseFound := l.covers(primary, base)
	choose := func(k faceKey) font.Face {
		f := l.loadFace(k, true)
		if f == nil {
			return nil
		}
		if !baseFound && l.covers(f, base) {
			baseFace = f
			baseFound = true
		}
		if all(f) {
			return f
		}
		return nil
	}
	k := l.fontKey(s)
	for _, candidate := range l.cssFamilies(k) {
		if f := choose(candidate); f != nil {
			return f
		}
	}
	for i, host := range l.opts.FallbackFaces {
		if l.fontStopped {
			break
		}
		candidate := k
		candidate.family = host.Family
		candidate.source = i + 1
		if f := choose(candidate); f != nil {
			return f
		}
	}
	for _, family := range ntfonts.Fallbacks() {
		if l.fontStopped {
			break
		}
		candidate := k
		candidate.family = family
		candidate.source = -2
		if f := choose(candidate); f != nil {
			return f
		}
	}
	for _, candidate := range l.bundledTextFallbacks(s, primary) {
		if l.fontStopped {
			break
		}
		if f := choose(candidate); f != nil {
			return f
		}
	}
	// Keep bundled text coverage available even when a page source does not
	// supply these families. Loads share the same budgets as every fallback.
	for _, family := range l.bundledTextFamilies(s, l.faceInfo[primary].key.family) {
		if l.fontStopped {
			break
		}
		candidate := k
		candidate.family = family
		candidate.source = -2
		for _, known := range ntfonts.Families() {
			if strings.EqualFold(candidate.family, known) {
				candidate.family = known
				break
			}
		}
		if f := choose(candidate); f != nil {
			return f
		}
	}
	names := []string{"Go", "Go Mono"}
	if s.mono {
		names[0], names[1] = names[1], names[0]
	}
	for _, name := range names {
		candidate := k
		candidate.family = name
		candidate.source = -1
		if f := choose(candidate); f != nil {
			return f
		}
	}
	return baseFace
}

// Use the resolved primary family, rather than the whole CSS list, to
// choose compatible text fallbacks. Custom generic mappings participate too.
func (l *layout) bundledTextFallbacks(s style, primary font.Face) []faceKey {
	family := l.faceInfo[primary].key.family
	cacheKey := textFallbackKey{face: l.fontKey(s), primaryFamily: family, mono: s.mono}
	if candidates, ok := l.textFallbacks[cacheKey]; ok {
		return candidates
	}
	matches := func(config string) bool {
		if config == "" {
			return false
		}
		key := l.fontKey(s)
		key.family = config
		for _, candidate := range l.cssFamilies(key) {
			if strings.EqualFold(family, candidate.family) {
				return true
			}
		}
		return false
	}
	mono := s.mono || matches(l.opts.FontFamilies.Monospace)
	serif := matches(l.opts.FontFamilies.Serif)
	switch strings.ToLower(family) {
	case "go mono", "ibm plex mono", "iosevka", "jetbrains mono":
		mono = true
	case "literata", "newsreader", "source serif 4":
		serif = true
	}
	var names []string
	if mono {
		names = []string{cmp.Or(l.opts.FontFamilies.Monospace, "IBM Plex Mono"), "Iosevka"}
	} else {
		names = []string{cmp.Or(l.opts.FontFamilies.SansSerif, "IBM Plex Sans"), cmp.Or(l.opts.FontFamilies.Serif, "Literata")}
		if serif {
			names[0], names[1] = names[1], names[0]
		}
	}
	var out []faceKey
	for _, name := range names {
		key := l.fontKey(s)
		key.family = name
		for _, candidate := range l.cssFamilies(key) {
			// Bundled families remain available even with a custom page FaceSource.
			// A configured non-bundled family is supplied by that page source.
			for _, bundled := range ntfonts.Families() {
				if candidate.family == bundled {
					candidate.source = -2
					break
				}
			}
			duplicate := false
			for _, previous := range out {
				if candidate == previous {
					duplicate = true
					break
				}
			}
			if !duplicate {
				out = append(out, candidate)
			}
		}
	}
	// Bound retained lists independently of how many styles the input requests.
	// Cached slices are immutable; callers copy candidate keys before use.
	if len(l.textFallbacks) < 128 {
		if l.textFallbacks == nil {
			l.textFallbacks = map[textFallbackKey][]faceKey{}
		}
		l.textFallbacks[cacheKey] = out
	}
	return out
}

// bundledTextFamilies follows the actual primary face, not the first CSS
// family (which may have failed to load). Unknown faces use sans-first order.
func (l *layout) bundledTextFamilies(s style, primary string) []string {
	serif := cmp.Or(l.opts.FontFamilies.Serif, "Literata")
	sans := cmp.Or(l.opts.FontFamilies.SansSerif, "IBM Plex Sans")
	mono := cmp.Or(l.opts.FontFamilies.Monospace, "IBM Plex Mono")
	switch strings.ToLower(primary) {
	case "go mono", "ibm plex mono", "iosevka", "jetbrains mono":
		return []string{mono, "Iosevka"}
	}
	if s.mono || strings.EqualFold(primary, mono) {
		return []string{mono, "Iosevka"}
	}
	switch strings.ToLower(primary) {
	case "literata", "newsreader", "source serif 4":
		return []string{serif, sans}
	}
	if strings.EqualFold(primary, serif) {
		return []string{serif, sans}
	}
	return []string{sans, serif}
}

// Combining marks form an indivisible unit with their preceding base.
func textClusters(text string) []string {
	var clusters []string
	start := 0
	for i, r := range text {
		if i > 0 && !unicode.Is(unicode.M, r) {
			clusters = append(clusters, text[start:i])
			start = i
		}
	}
	if start < len(text) {
		clusters = append(clusters, text[start:])
	}
	return clusters
}
func (l *layout) splitText(s style, text string) []glyphRun {
	key := splitKey{face: l.fontKey(s), mono: s.mono, small: s.smallCaps, text: text}
	if runs, ok := l.splits[key]; ok {
		return runs
	}
	var runs []glyphRun
	var pending glyphRun
	var content strings.Builder
	flush := func() {
		if content.Len() > 0 {
			pending.text = content.String()
			runs = append(runs, pending)
			content = strings.Builder{}
		}
	}
	for _, cluster := range textClusters(stripZeroWidth(text)) {
		st := s
		first, _ := utf8.DecodeRuneInString(cluster)
		if st.smallCaps && unicode.IsLower(first) {
			st.size = smallCapSize(st.size)
			cluster = string(unicode.ToUpper(first)) + cluster[utf8.RuneLen(first):]
		}
		f := l.clusterFace(st, cluster)
		info, ok := l.faceInfo[f]
		run := glyphRun{key: info.key, text: cluster, basic: !ok}
		for _, r := range cluster {
			if !l.covers(f, r) {
				run.missing++
			}
		}
		if content.Len() > 0 && (pending.key != run.key || pending.basic != run.basic) {
			flush()
		}
		if content.Len() == 0 {
			pending = run
			pending.missing = 0
		}
		content.WriteString(cluster)
		pending.missing += run.missing
	}
	flush()
	cost := len(text)
	for _, r := range runs {
		cost += len(r.text) + 64
	}
	if len(l.splits) < 4096 && l.splitBytes+cost <= 1<<20 {
		if l.splits == nil {
			l.splits = map[splitKey][]glyphRun{}
		}
		l.splits[key] = runs
		l.splitBytes += cost
	}
	return runs
}
func (l *layout) runFace(run glyphRun) font.Face {
	if !run.basic {
		if f := l.loadFace(run.key, true); f != nil {
			return f
		}
	}
	return basicfont.Face7x13
}
func (l *layout) runsAdvance(runs []glyphRun, s style) fixed.Int26_6 {
	var width fixed.Int26_6
	for _, run := range runs {
		f := l.runFace(run)
		if s.wordSpacing == 0 && s.letterSpacing == 0 && !s.smallCaps {
			width += font.MeasureString(f, run.text)
		} else {
			for _, r := range run.text {
				width += fixed.I(glyphAdvanceWithLetter(f, r, s.wordSpacing, s.letterSpacing))
			}
		}
	}
	return width
}
func (l *layout) runsInk(runs []glyphRun, b Box) image.Rectangle {
	var ink image.Rectangle
	var offset fixed.Int26_6
	for _, run := range runs {
		part := b
		part.Text = run.text
		part.SmallCaps = false
		part.Bounds = part.Bounds.Add(image.Pt(offset.Floor(), 0))
		ink = ink.Union(textInk(l.runFace(run), part))
		offset += l.runsAdvance([]glyphRun{run}, boxStyle(b))
	}
	return ink
}
func (l *layout) drawRuns(dr *font.Drawer, runs []glyphRun, spacing, letterSpacing int, smallCaps bool) {
	for _, run := range runs {
		dr.Face = l.runFace(run)
		// Small caps retain the existing unkerned per-character advances.
		if smallCaps && spacing == 0 && letterSpacing == 0 {
			for _, r := range run.text {
				origin := dr.Dot.X
				dr.DrawString(string(r))
				dr.Dot.X = origin + fixed.I(glyphAdvanceWithLetter(dr.Face, r, 0, 0))
			}
		} else {
			drawTextWithLetter(dr, nil, run.text, spacing, letterSpacing)
		}
	}
}

func clusterSpace(text string) bool { r, _ := utf8.DecodeRuneInString(text); return cssSpace(r) }
