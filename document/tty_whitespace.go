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
func wrapTTYRuns(runs []ttyRun, width int) string {
	type unit struct {
		text            string
		width           int
		breakable, wrap bool
	}
	var out strings.Builder
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
			word = word[:len(word)-1]
			wordWidth--
			trailing = true
		}
		flushStructural()
		space := 0
		if pendingSpace && lineWidth > 0 {
			space = 1
		}
		wrap := false
		for _, u := range word {
			wrap = wrap || u.wrap
		}
		if wrap && lineWidth > 0 && lineWidth+space+wordWidth > width {
			newline()
			space = 0
		}
		if space > 0 {
			out.WriteByte(' ')
			lineWidth++
		}
		for _, u := range word {
			if u.breakable && lineWidth > 0 && lineWidth+u.width > width {
				newline()
			}
			out.WriteString(u.text)
			lineWidth += u.width
		}
		pendingSpace = trailing
		word = word[:0]
		wordWidth = 0
	}
	for _, run := range runs {
		s := run.policy
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
						word = append(word, unit{" ", 1, false, false})
						wordWidth++
					}
				} else {
					flushWord()
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
				out.WriteString(seq)
				lineWidth += w
				continue
			}
			// normal overflow-wrap wraps words, but not their interior. Preserve that
			// distinction by flushing the word at whitespace and handling it below.
			canBreak := !noWrap && s.overflowWrap != "normal"
			word = append(word, unit{seq, w, canBreak, !noWrap})
			wordWidth += w
		}
	}
	flushWord()
	return out.String()
}
