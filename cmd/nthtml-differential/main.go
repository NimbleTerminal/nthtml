// nthtml-differential renders a representative subset of pinned WPT fixtures in
// both nthtml and headless Chromium, writing side-by-side PNGs, geometry
// comparisons, and HTML/JSON reports under artifacts/differential.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"nimbleterminal.dev/nthtml/internal/conformance"
)

func main() {
	out := flag.String("out", "artifacts/differential", "report and PNG output directory")
	chromium := flag.String("chromium", "", "path to headless Chromium/Chrome binary (default: auto-detect or $NTHTML_CHROMIUM)")
	strict := flag.Bool("strict", false, "exit non-zero if Chromium is missing or any fixture has GEOMETRY_MISMATCH")
	flag.Parse()

	suite, err := conformance.Load()
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	report, err := suite.RunDifferential(ctx, conformance.DifferentialOptions{
		ArtifactsDir: *out,
		ChromiumPath: *chromium,
	})
	if err != nil {
		if errors.Is(err, conformance.ErrChromiumUnavailable) {
			fmt.Fprintf(os.Stderr, "SKIP: %v\n", err)
			if *strict {
				os.Exit(1)
			}
			return
		}
		fatal(err)
	}
	if err := conformance.WriteDifferentialReport(*out, report); err != nil {
		fatal(err)
	}

	for _, r := range report.Results {
		fmt.Printf("%-20s %-52s (%5.2f%% px diff)\n", r.Status, r.Path, r.PixelDiffPercent)
		for _, g := range r.Geometry {
			if g.Status != "MATCH" || len(r.Geometry) <= 4 {
				if g.NTHTML != nil && g.Chromium != nil {
					fmt.Printf("  %-12s %-16s nt=(%d,%d %dx%d) chr=(%d,%d %dx%d) d=(%+d,%+d,%+d,%+d) %s\n",
						g.Status, g.Selector,
						g.NTHTML.X, g.NTHTML.Y, g.NTHTML.Width, g.NTHTML.Height,
						g.Chromium.X, g.Chromium.Y, g.Chromium.Width, g.Chromium.Height,
						g.DX, g.DY, g.DW, g.DH, g.Note)
				} else {
					fmt.Printf("  %-12s %-16s %s\n", g.Status, g.Selector, g.Note)
				}
			}
		}
	}

	fmt.Printf("\n%d GEOMETRY_MATCH · %d GEOMETRY_MISMATCH · %d GEOMETRY_UNAVAILABLE · %d ERROR\n",
		report.Counts["GEOMETRY_MATCH"],
		report.Counts["GEOMETRY_MISMATCH"],
		report.Counts["GEOMETRY_UNAVAILABLE"],
		report.Counts["ERROR"],
	)
	cats := make([]string, 0, len(report.Categories))
	for cat := range report.Categories {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	for _, cat := range cats {
		c := report.Categories[cat]
		fmt.Printf("  %-28s %2d MATCH · %2d MISMATCH · %2d UNAVAILABLE · %d ERROR\n",
			cat, c["GEOMETRY_MATCH"], c["GEOMETRY_MISMATCH"], c["GEOMETRY_UNAVAILABLE"], c["ERROR"])
	}
	fmt.Printf("Browser: %s\nPinned WPT: %s\nReport: %s\n",
		report.Browser, report.Revision, filepath.Join(*out, "index.html"))

	if report.Counts["ERROR"] > 0 || (*strict && report.Counts["GEOMETRY_MISMATCH"] > 0) {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
