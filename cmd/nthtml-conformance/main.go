// nthtml-conformance runs the pinned, offline WPT subset and writes a report.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"nimbleterminal.dev/nthtml/internal/conformance"
	"os"
	"path/filepath"
	"time"
)

func main() {
	out := flag.String("out", "artifacts/conformance", "report and PNG output directory")
	strict := flag.Bool("strict", false, "exit unsuccessfully for any FAIL, including known failures")
	flag.Parse()
	suite, e := conformance.Load()
	if e != nil {
		fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, e := suite.Run(ctx, conformance.Options{ArtifactsDir: *out})
	if e != nil {
		fatal(e)
	}
	if e = os.MkdirAll(*out, 0755); e != nil {
		fatal(e)
	}
	data, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		fatal(e)
	}
	if e = os.WriteFile(filepath.Join(*out, "report.json"), append(data, '\n'), 0644); e != nil {
		fatal(e)
	}
	page, e := os.Create(filepath.Join(*out, "index.html"))
	if e != nil {
		fatal(e)
	}
	e = template.Must(template.New("report").Parse(reportHTML)).Execute(page, report)
	closeErr := page.Close()
	if e != nil {
		fatal(e)
	}
	if closeErr != nil {
		fatal(closeErr)
	}
	for _, r := range report.Results {
		fmt.Printf("%-11s %s\n", r.Status, r.Path)
		if r.Status != "PASS" {
			fmt.Printf("            %s\n", r.Reason)
		}
	}
	fmt.Printf("\n%d PASS · %d FAIL · %d UNSUPPORTED · %d ERROR · %d unexpected\n", report.Counts["PASS"], report.Counts["FAIL"], report.Counts["UNSUPPORTED"], report.Counts["ERROR"], report.Unexpected)
	fmt.Printf("Pinned WPT %s\nReport: %s\n", report.Revision, filepath.Join(*out, "index.html"))
	if report.Unexpected > 0 || *strict && report.Counts["FAIL"] > 0 {
		os.Exit(1)
	}
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }

const reportHTML = `<!doctype html><meta charset="utf-8"><title>nthtml · WPT subset</title>
<style>body{font:16px system-ui;margin:32px;max-width:1200px;color:#203047}table{border-collapse:collapse;width:100%}td,th{border:1px solid #ccd5e0;padding:10px;text-align:left;vertical-align:top}.PASS{color:#187134}.FAIL,.ERROR{color:#a32727}.UNSUPPORTED{color:#665300}code{overflow-wrap:anywhere}.images{display:flex;gap:10px;flex-wrap:wrap}img{width:360px;max-width:100%;border:1px solid #ccd5e0}small{color:#536277}</style>
<h1>nthtml · WPT reference subset</h1><p>{{.Note}}</p>
<p>{{index .Counts "PASS"}} PASS · {{index .Counts "FAIL"}} FAIL · {{index .Counts "UNSUPPORTED"}} UNSUPPORTED · {{index .Counts "ERROR"}} ERROR · {{.Unexpected}} unexpected</p>
<p>Revision <code>{{.Revision}}</code> · {{.Viewport.Width}} × {{.Viewport.Height}} · exact pixels</p>
<table><tr><th>Observed</th><th>Upstream case</th><th>Scope / known gap</th></tr>
{{range .Results}}<tr><td class="{{.Status}}"><strong>{{.Status}}</strong><br><small>Expected {{.Expected}}{{if .Unexpected}} · CHANGED{{end}}</small></td><td><a href="{{.URL}}"><code>{{.Path}}</code></a></td><td>{{.Reason}}
{{range .Comparisons}}<p><small>{{.Relation}} {{.Reference}} · {{.DifferentPixels}} different pixels · max delta {{.MaxChannelDifference}}</small></p>
{{if .TestPNG}}<div class="images"><a href="{{.TestPNG}}"><img alt="nthtml test render" src="{{.TestPNG}}"></a><a href="{{.ReferencePNG}}"><img alt="nthtml reference render" src="{{.ReferencePNG}}"></a><a href="{{.DiffPNG}}"><img alt="Magenta pixel differences" src="{{.DiffPNG}}"></a></div>{{end}}{{end}}</td></tr>{{end}}</table>
<p>Unsupported cases are not rendered or counted as passes. Expected failures remain FAIL; CI detects changed outcomes. This is not a percentage of the full WPT suite.</p>`
