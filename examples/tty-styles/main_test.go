package main

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/nthtml/document"
)

func TestSharedSample(t *testing.T) {
	r, err := document.Render(context.Background(), strings.NewReader(sample), document.Options{Width: 550})
	if err != nil {
		t.Fatal(err)
	}
	tty, err := r.TTYLayout(context.Background(), 48)
	if err != nil {
		t.Fatal(err)
	}
	if len(tty.Styles) == 0 || len(tty.Spans) == 0 || len(r.Boxes) == 0 {
		t.Fatal("missing styled content")
	}
	if ansi.Strip(tty.ANSI()) != tty.Text {
		t.Fatal("plain text changed")
	}
	if len(tty.TextIndex().Search("Café")) == 0 {
		t.Fatal("missing searchable Unicode")
	}
}
