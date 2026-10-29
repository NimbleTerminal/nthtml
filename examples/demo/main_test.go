package main

import (
	"context"
	"strings"
	"testing"

	"nimbleterminal.dev/nthtml/document"
)

func TestDemoDocuments(t *testing.T) {
	for _, name := range []string{"overview", "invoice", "field-notes", "status-report", "guide", "typography", "diagrams", "navigation", "reports", "positioning", "tty-styles"} {
		t.Run(name, func(t *testing.T) {
			source, err := demoSource(name)
			if err != nil {
				t.Fatal(err)
			}
			result, err := document.Render(context.Background(), strings.NewReader(source), document.Options{Width: 800})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Warnings) > 0 {
				t.Fatal(result.Warnings)
			}
			if result.Image.Bounds().Dy() < 500 {
				t.Fatal("unexpectedly short document")
			}
			if len(result.Boxes) < 20 {
				t.Fatal("missing document content")
			}
		})
	}
	if _, err := demoSource("../sample"); err == nil {
		t.Fatal("accepted unknown demo")
	}
}
