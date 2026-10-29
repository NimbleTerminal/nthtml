package document

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type ttyRun struct {
	text       string
	policy     style
	structural bool
}
type ttyBuilder struct {
	strings.Builder
	policy  style
	runs    []ttyRun
	special bool
}

func (b *ttyBuilder) WriteString(text string) (int, error) {
	if text != "" {
		b.runs = append(b.runs, ttyRun{text, b.policy, false})
		b.special = b.special || b.policy.whiteSpace != "" || b.policy.overflowWrap != ""
	}
	return b.Builder.WriteString(text)
}
func (b *ttyBuilder) WriteByte(c byte) error {
	b.runs = append(b.runs, ttyRun{string(c), b.policy, true})
	return b.Builder.WriteByte(c)
}
func (b *ttyBuilder) Reset() { b.Builder.Reset(); b.runs = nil; b.special = false }

// Styled TTY runs keep inline whitespace overrides scoped to their own text.
// ANSI link/anchor markers occupy zero cells and survive all line breaks.
func wrapTTYRuns(runs []ttyRun, width int, styles *ttyStyles) string {
	type unit struct {
		text            string
		width           int
		styleID         int
		breakable, wrap bool
	}
	var out strings.Builder
	activeStyle := 0
	emit := func(text string, id int) {
		if !strings.HasPrefix(text, "\x1b") && id != activeStyle {
			out.WriteString(ttyStyleMarker(id))
			activeStyle = id
		}
		out.WriteString(text)
	}
	pendingStyle := 0
	var word []unit
	wordWidth, lineWidth, structural := 0, 0, 0
	pendingSpace := false
	newline := func() { out.WriteByte('\n'); lineWidth = 0 }
	flushStructural := func() {
		if structural > 0 && out.Len() > 0 {
			for i := 0; i < min(2, structural); i++ {
				newline()
			}
		}
		structural = 0
	}
	flushWord := func() {
		if len(word) == 0 {
			return
		}
		trailing := false
		for len(word) > 0 && word[len(word)-1].text == " " {
			pendingStyle = word[len(word)-1].styleID
			word = word[:len(word)-1]
			wordWidth--
			trailing = true
		}
		flushStructural()
		space := 0
		if pendingSpace && lineWidth > 0 {
			space = 1
		}
		wrap := pendingSpace
		for _, u := range word {
			wrap = wrap || u.wrap
		}
		if wrap && lineWidth > 0 && lineWidth+space+wordWidth > width {
			newline()
			space = 0
		}
		if space > 0 {
			emit(" ", pendingStyle)
			lineWidth++
		}
		for _, u := range word {
			if u.breakable && lineWidth > 0 && lineWidth+u.width > width {
				newline()
			}
			emit(u.text, u.styleID)
			lineWidth += u.width
		}
		pendingSpace = trailing
		word = word[:0]
		wordWidth = 0
	}
	for _, run := range runs {
		s := run.policy
		styleID := styles.id(s)
		noWrap := s.whiteSpace == "nowrap" || s.whiteSpace == "pre"
		for text := run.text; len(text) > 0; {
			seq, w, n, _ := ansi.DecodeSequence(text, 0, nil)
			if n == 0 {
				break
			}
			text = text[n:]
			if seq == "\n" {
				flushWord()
				pendingSpace = false
				if run.structural {
					structural++
				} else {
					flushStructural()
					newline()
				}
				continue
			}
			space := len(seq) == 1 && cssSpace(rune(seq[0]))
			if space && !s.pre {
				if noWrap {
					// A collapsed space inside nowrap is part of its unbreakable run.
					if len(word) > 0 && word[len(word)-1].text != " " {
						word = append(word, unit{" ", 1, styleID, false, false})
						wordWidth++
					}
				} else {
					flushWord()
					pendingStyle = styleID
					pendingSpace = true
				}
				continue
			}
			if space && s.pre {
				flushWord()
				flushStructural()
				pendingSpace = false
				if !noWrap && lineWidth >= width {
					newline()
				}
				emit(seq, styleID)
				lineWidth += w
				continue
			}
			// normal overflow-wrap wraps words, but not their interior. Preserve that
			// distinction by flushing the word at whitespace and handling it below.
			canBreak := !noWrap && s.overflowWrap != "normal"
			word = append(word, unit{seq, w, styleID, canBreak, !noWrap})
			wordWidth += w
		}
	}
	flushWord()
	if activeStyle != 0 {
		out.WriteString(ttyStyleEnd)
	}
	return out.String()
}
