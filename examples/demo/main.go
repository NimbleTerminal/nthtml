package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"os"
	"strings"

	"nimbleterminal.dev/nthtml/document"
	"nimbleterminal.dev/nthtml/internal/demos"
)

//go:embed sample.html documents/*.html
var documents embed.FS

func main() {
	name := flag.String("demo", "overview", "document: overview, invoice, field-notes, status-report, guide, typography, diagrams, navigation, reports, positioning, tty-styles")
	flag.Parse()
	source, err := demoSource(*name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	result, err := document.Render(context.Background(), strings.NewReader(source), document.Options{Width: 800})
	if err != nil {
		panic(err)
	}
	run(result, source)
}

func demoSource(name string) (string, error) {
	path := "sample.html"
	switch name {
	case "tty-styles":
		return demos.TTYStyles, nil
	case "overview":
	case "invoice", "field-notes", "status-report", "guide", "typography", "diagrams", "navigation", "reports", "positioning":
		path = "documents/" + name + ".html"
	default:
		return "", fmt.Errorf("unknown demo %q; choose overview, invoice, field-notes, status-report, guide, typography, diagrams, navigation, reports, positioning, or tty-styles", name)
	}
	data, err := documents.ReadFile(path)
	return string(data), err
}
