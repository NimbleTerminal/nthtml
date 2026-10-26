package document

import (
	"context"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"image"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TextRun maps UTF-8 byte offsets to a visible text rectangle.
// For Typeset runs, CharX holds the right-edge X coordinate of each rune in
// Text[Start:End].
type TextRun struct {
	Start, End int
	Bounds     image.Rectangle
	CharX      []int
	SourceID   string
}

// SourceScope describes a visible identified HTML element in document order,
// its nearest enclosing identified ancestor (ParentID), and its tag name.
type SourceScope struct {
	ID       string
	ParentID string
	Tag      string
}

// TextIndex is a layout-specific text snapshot. Typeset runs are words or
// styled fragments; TTY runs are lines. Offsets must not cross layout snapshots.
type TextIndex struct {
	Text  string
	Runs  []TextRun
	Cells bool
}
type Match struct{ Start, End int }

func anchorName(n *node) string {
	if id := n.attrs["id"]; id != "" {
		return id
	}
	if n.tag == "a" {
		return n.attrs["name"]
	}
	return ""
}

// Fragment decodes local-only fragment destinations, including "#" for top.
func Fragment(href string) (string, bool) {
	if !strings.HasPrefix(href, "#") {
		return "", false
	}
	name, err := url.PathUnescape(href[1:])
	return name, err == nil
}
func normalizeAnchorID(idOrHref string) (string, bool) {
	if strings.HasPrefix(idOrHref, "#") {
		return Fragment(idOrHref)
	}
	return idOrHref, idOrHref != ""
}

// Sources returns visible identified HTML elements (id or <a name>) in document
// order with their enclosing identified ParentID, honoring display:none.
func (d *Document) Sources(ctx context.Context) ([]SourceScope, error) {
	if d == nil || d.root == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []SourceScope
	var walk func(*node, style, string) error
	walk = func(n *node, parent style, parentID string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n.tag == "" {
			return nil
		}
		s := d.styled(n, parent)
		if s.display == "none" {
			return nil
		}
		curParent := parentID
		if id := anchorName(n); id != "" && !seen[id] {
			seen[id] = true
			out = append(out, SourceScope{ID: id, ParentID: parentID, Tag: n.tag})
			curParent = id
		}
		for _, c := range n.children {
			if err := walk(c, s, curParent); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(d.root, style{rootSize: 16, size: 16, line: 22}, ""); err != nil {
		return nil, err
	}
	return out, nil
}

// ParentID returns the nearest visible enclosing identified ancestor's ID for
// id (or #fragment), or "" if id is top-level, hidden, or absent.
func (d *Document) ParentID(id string) string {
	name, ok := normalizeAnchorID(id)
	if !ok || name == "" {
		return ""
	}
	scopes, err := d.Sources(context.Background())
	if err != nil {
		return ""
	}
	for _, sc := range scopes {
		if sc.ID == name {
			return sc.ParentID
		}
	}
	return ""
}

// RootID returns the outermost visible enclosing identified ancestor's ID for
// id (or id itself if top-level), or "" if id is hidden or absent.
func (d *Document) RootID(id string) string {
	name, ok := normalizeAnchorID(id)
	if !ok || name == "" {
		return ""
	}
	scopes, err := d.Sources(context.Background())
	if err != nil {
		return ""
	}
	parents := make(map[string]string, len(scopes))
	exists := false
	for _, sc := range scopes {
		parents[sc.ID] = sc.ParentID
		if sc.ID == name {
			exists = true
		}
	}
	if !exists {
		return ""
	}
	cur := name
	for range len(scopes) + 1 {
		p := parents[cur]
		if p == "" {
			return cur
		}
		cur = p
	}
	return cur
}

func (r *Result) Fragment(href string) (image.Point, bool) {
	if r == nil {
		return image.Point{}, false
	}
	name, ok := Fragment(href)
	if !ok {
		return image.Point{}, false
	}
	if name == "" {
		return image.Point{}, true
	}
	p, ok := r.Anchors[name]
	return p, ok
}

// AnchorBounds returns the laid-out pixel bounding box for id (or #fragment).
func (r *Result) AnchorBounds(idOrHref string) (image.Rectangle, bool) {
	if r == nil {
		return image.Rectangle{}, false
	}
	name, ok := normalizeAnchorID(idOrHref)
	if !ok {
		return image.Rectangle{}, false
	}
	if name == "" {
		return r.Bounds(), true
	}
	var out image.Rectangle
	for _, b := range r.Boxes {
		if b.Anchor == name || b.SourceID == name {
			rect := b.Bounds
			if rect.Empty() {
				rect = b.InkBounds
			}
			if !rect.Empty() {
				out = out.Union(rect)
			}
		}
	}
	if !out.Empty() {
		return out, true
	}
	if p, exists := r.Anchors[name]; exists {
		return image.Rect(p.X, p.Y, p.X+1, p.Y+1), true
	}
	return image.Rectangle{}, false
}

// SourceAt returns the innermost HTML element ID at pixel point p.
func (r *Result) SourceAt(p image.Point) (string, bool) {
	if r == nil {
		return "", false
	}
	for i := len(r.Boxes) - 1; i >= 0; i-- {
		b := r.Boxes[i]
		if b.SourceID != "" && p.In(b.Bounds) {
			return b.SourceID, true
		}
	}
	return r.TextIndex().HitSource(image.Rect(p.X, p.Y, p.X+1, p.Y+1))
}

func (r *TTYResult) Fragment(href string) (image.Point, bool) {
	if r == nil {
		return image.Point{}, false
	}
	name, ok := Fragment(href)
	if !ok {
		return image.Point{}, false
	}
	if name == "" {
		return image.Point{}, true
	}
	p, ok := r.Anchors[name]
	return p, ok
}

// AnchorBounds returns the terminal-cell bounding box for id (or #fragment).
func (r *TTYResult) AnchorBounds(idOrHref string) (image.Rectangle, bool) {
	if r == nil {
		return image.Rectangle{}, false
	}
	name, ok := normalizeAnchorID(idOrHref)
	if !ok {
		return image.Rectangle{}, false
	}
	if name == "" {
		lines := strings.Count(r.Text, "\n") + 1
		w := max(1, r.width)
		return image.Rect(0, 0, w, lines), true
	}
	if rect, exists := r.AnchorRects[name]; exists && !rect.Empty() {
		return rect, true
	}
	if p, exists := r.Anchors[name]; exists {
		w := max(1, r.width)
		return image.Rect(p.X, p.Y, w, p.Y+1), true
	}
	return image.Rectangle{}, false
}

// SourceAt returns the innermost HTML element ID at terminal cell p (col, line).
func (r *TTYResult) SourceAt(p image.Point) (string, bool) {
	if r == nil || p.Y < 0 {
		return "", false
	}
	if p.Y < len(r.lineSources) && r.lineSources[p.Y] != "" {
		return r.lineSources[p.Y], true
	}
	return r.TextIndex().HitSource(image.Rect(p.X, p.Y, p.X+1, p.Y+1))
}
func (r *Result) TextIndex() *TextIndex {
	out := &TextIndex{}
	if r == nil {
		return out
	}
	var l *layout
	if r.document != nil {
		l = &layout{
			ctx:   context.Background(),
			d:     r.document,
			opts:  r.opts,
			lim:   r.limits,
			faces: map[faceKey]font.Face{},
			fonts: map[string]*opentype.Font{},
		}
		defer func() {
			for _, f := range l.faces {
				f.Close()
			}
		}()
	}
	var text strings.Builder
	for _, b := range r.Boxes {
		if b.Tag != "text" || b.Text == "" {
			continue
		}
		if text.Len() > 0 {
			text.WriteString(b.TextPrefix)
		}
		start := text.Len()
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, b.Text)
		text.WriteString(clean)
		bounds := b.Bounds
		if bounds.Empty() {
			bounds = b.InkBounds
		}
		out.Runs = append(out.Runs, TextRun{
			Start:    start,
			End:      text.Len(),
			Bounds:   bounds,
			CharX:    measureRunCharX(l, b, clean, bounds),
			SourceID: b.SourceID,
		})
	}
	out.Text = text.String()
	return out
}

func measureRunCharX(l *layout, b Box, clean string, bounds image.Rectangle) []int {
	n := utf8.RuneCountInString(clean)
	if n == 0 {
		return nil
	}
	charX := make([]int, 0, n)
	if l != nil && b.FontSize > 0 {
		st := style{
			size:          b.FontSize,
			bold:          b.Bold,
			mono:          b.Monospace,
			family:        b.FontFamily,
			italic:        b.Italic,
			smallCaps:     b.SmallCaps,
			wordSpacing:   b.WordSpacing,
			letterSpacing: b.LetterSpacing,
		}
		f := l.face(st)
		x := bounds.Min.X
		for _, ch := range clean {
			x += l.glyphAdvanceStyled(st, f, ch)
			charX = append(charX, min(bounds.Max.X, max(bounds.Min.X, x)))
		}
		if len(charX) > 0 {
			charX[len(charX)-1] = bounds.Max.X
		}
		return charX
	}
	for i := 0; i < n; i++ {
		charX = append(charX, bounds.Min.X+(i+1)*bounds.Dx()/n)
	}
	return charX
}

func (r *TTYResult) TextIndex() *TextIndex {
	out := &TextIndex{Cells: true}
	if r == nil {
		return out
	}
	out.Text = r.Text
	offset := 0
	for y, line := range strings.Split(r.Text, "\n") {
		if line != "" {
			srcID := ""
			if y < len(r.lineSources) {
				srcID = r.lineSources[y]
			}
			out.Runs = append(out.Runs, TextRun{
				Start:    offset,
				End:      offset + len(line),
				Bounds:   image.Rect(0, y, ansi.StringWidth(line), y+1),
				SourceID: srcID,
			})
		}
		offset += len(line) + 1
	}
	return out
}

// Search is literal, Unicode simple-case-insensitive, and collapses whitespace.
// It scans in linear time, caps queries at 256 runes, and returns at most 1000 matches.
func (t *TextIndex) Search(query string) []Match {
	if t == nil || len(query) > 1024 || utf8.RuneCountInString(query) > 256 {
		return nil
	}
	q := []rune(strings.ToLower(strings.Join(strings.Fields(query), " ")))
	if len(q) == 0 || len(q) > 256 {
		return nil
	}
	failure := make([]int, len(q))
	for i, j := 1, 0; i < len(q); i++ {
		for j > 0 && q[i] != q[j] {
			j = failure[j-1]
		}
		if q[i] == q[j] {
			j++
		}
		failure[i] = j
	}
	starts := make([]int, len(q))
	matched, count := 0, 0
	space := false
	var out []Match
	for offset, r := range t.Text {
		end := offset + utf8.RuneLen(r)
		if unicode.IsSpace(r) {
			if space {
				continue
			}
			r = ' '
			space = true
		} else {
			space = false
			r = unicode.ToLower(r)
		}
		starts[count%len(q)] = offset
		count++
		for matched > 0 && r != q[matched] {
			matched = failure[matched-1]
		}
		if r == q[matched] {
			matched++
		}
		if matched == len(q) {
			out = append(out, Match{starts[(count-len(q))%len(q)], end})
			if len(out) == 1000 {
				return out
			}
			matched = failure[matched-1]
		}
	}
	return out
}
func (t *TextIndex) Range(start, end int) (int, int) {
	if start > end {
		start, end = end, start
	}
	start = max(0, min(len(t.Text), start))
	end = max(0, min(len(t.Text), end))
	for start > 0 && start < len(t.Text) && !utf8.RuneStart(t.Text[start]) {
		start--
	}
	for end < len(t.Text) && !utf8.RuneStart(t.Text[end]) {
		end++
	}
	return start, end
}
func (t *TextIndex) Selected(start, end int) string {
	if t == nil {
		return ""
	}
	start, end = t.Range(start, end)
	return t.Text[start:end]
}
func (t *TextIndex) Regions(start, end int) []image.Rectangle {
	if t == nil {
		return nil
	}
	start, end = t.Range(start, end)
	var regions []image.Rectangle
	for _, run := range t.Runs {
		if run.End <= start || run.Start >= end {
			continue
		}
		rect := run.Bounds
		a, b := max(start, run.Start), min(end, run.End)
		if t.Cells {
			rect.Min.X += ansi.StringWidth(t.Text[run.Start:a])
			rect.Max.X = run.Bounds.Min.X + ansi.StringWidth(t.Text[run.Start:b])
		} else if a > run.Start || b < run.End {
			sub := t.Text[run.Start:run.End]
			nRunes := max(1, utf8.RuneCountInString(sub))
			x := run.Bounds.Min.X
			minX, maxX := run.Bounds.Min.X, run.Bounds.Max.X
			i := 0
			for offset, r := range sub {
				pos := run.Start + offset
				var nextX int
				if i < len(run.CharX) {
					nextX = run.CharX[i]
				} else {
					nextX = run.Bounds.Min.X + (i+1)*run.Bounds.Dx()/nRunes
				}
				if pos == a {
					minX = x
				}
				if pos+utf8.RuneLen(r) == b {
					maxX = nextX
				}
				x = nextX
				i++
			}
			rect.Min.X = minX
			rect.Max.X = max(minX+1, maxX)
		}
		if !rect.Empty() {
			regions = append(regions, rect)
		}
	}
	return regions
}

// Hit returns the visible character range nearest an area in both Typeset and
// TTY modes.
func (t *TextIndex) Hit(area image.Rectangle) (Match, bool) {
	if t == nil || area.Empty() {
		return Match{}, false
	}
	best := -1
	distance := int(^uint(0) >> 1)
	for i, run := range t.Runs {
		dx := max(0, run.Bounds.Min.X-area.Max.X+1, area.Min.X-run.Bounds.Max.X+1)
		dy := max(0, run.Bounds.Min.Y-area.Max.Y+1, area.Min.Y-run.Bounds.Max.Y+1)
		d := dy*100000 + dx
		if d < distance {
			distance = d
			best = i
		}
	}
	if best < 0 {
		return Match{}, false
	}
	run := t.Runs[best]
	sub := t.Text[run.Start:run.End]
	nRunes := max(1, utf8.RuneCountInString(sub))
	x := run.Bounds.Min.X
	i := 0
	for offset, r := range sub {
		var nextX int
		if t.Cells {
			nextX = x + ansi.StringWidth(string(r))
		} else if i < len(run.CharX) {
			nextX = run.CharX[i]
		} else {
			nextX = run.Bounds.Min.X + (i+1)*run.Bounds.Dx()/nRunes
		}
		if max(x+1, nextX) > area.Min.X {
			return Match{run.Start + offset, run.Start + offset + utf8.RuneLen(r)}, true
		}
		x = nextX
		i++
	}
	return Match{run.End, run.End}, true
}

// SourceAt returns the HTML element SourceID owning the text run at or nearest
// UTF-8 byte offset.
func (t *TextIndex) SourceAt(offset int) (string, bool) {
	if t == nil || len(t.Runs) == 0 {
		return "", false
	}
	for _, run := range t.Runs {
		if offset >= run.Start && offset < run.End && run.SourceID != "" {
			return run.SourceID, true
		}
	}
	best := ""
	bestDist := int(^uint(0) >> 1)
	for _, run := range t.Runs {
		if run.SourceID == "" {
			continue
		}
		d := 0
		if offset < run.Start {
			d = run.Start - offset
		} else if offset >= run.End {
			d = offset - run.End + 1
		}
		if d < bestDist {
			bestDist = d
			best = run.SourceID
		}
	}
	return best, best != ""
}

// HitSource returns the HTML element SourceID nearest area in either Typeset
// or TTY coordinates.
func (t *TextIndex) HitSource(area image.Rectangle) (string, bool) {
	hit, ok := t.Hit(area)
	if !ok {
		return "", false
	}
	return t.SourceAt(hit.Start)
}
