package document

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestNestedListMarkers(t *testing.T) {
	const four = `<ul><li>a<ul><li>b<ul><li>c<ul><li>d</li></ul></li></ul></li></ul></li></ul>`
	for _, tc := range []struct {
		name, html string
		want       []string
	}{
		{"four levels", four, []string{"•", "◦", "▪", "▪"}},
		{"ordered ancestor", `<ol><li>a<ul><li>x</li></ul></li></ol>`, []string{"1.", "◦"}},
		{"ordered child", `<ul><li>a<ol><li>x</li></ol></li></ul>`, []string{"•", "1."}},
		{"intervening div", `<ul><li>a<div><ul><li>x</li></ul></div></li></ul>`, []string{"•", "◦"}},
		{"author rule", `<style>ul{list-style-type:disc}</style>` + four, []string{"•", "•", "•", "•"}},
		{"inherited author", `<ul style="list-style-type:square"><li>a<ul><li>b</li></ul></li></ul>`, []string{"▪", "▪"}},
		{"type attribute", `<ul><li>a<ul><li>b<ul type="disc"><li>c</li></ul></li></ul></li></ul>`, []string{"•", "◦", "•"}},
		{"item override", `<ul><li>a<ul><li style="list-style-type:disc">b</li></ul></li></ul>`, []string{"•", "•"}},
		{"menu ancestor", `<menu><ul><li>x</li></ul></menu>`, []string{"◦"}},
		{"dir and menu ancestors", `<dir><menu><ul><li>x</li></ul></menu></dir>`, []string{"▪"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := parseFeature(t, tc.html)
			ctx := context.Background()
			tty, err := d.TTY(ctx, 100)
			if err != nil {
				t.Fatal(err)
			}
			check := func(path, text string) {
				t.Helper()
				var got []string
				for _, word := range strings.Fields(text) {
					switch word {
					case "•", "◦", "▪", "1.":
						got = append(got, word)
					}
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("%s markers = %v, want %v (text %q)", path, got, tc.want, text)
				}
			}
			check("TTY", tty)
			blocks, err := d.Blocks(ctx, Options{Width: 400})
			if err != nil {
				t.Fatal(err)
			}
			var text strings.Builder
			for _, b := range blocks {
				for _, s := range b.Spans {
					text.WriteString(s.Text)
				}
				text.WriteByte('\n')
			}
			check("Blocks", text.String())
			td, err := d.ExtractTypeset(ctx, Options{Width: 400})
			if err != nil {
				t.Fatal(err)
			}
			text.Reset()
			for _, b := range td.Blocks {
				for _, s := range b.Spans {
					text.WriteString(s.Text)
				}
				text.WriteByte('\n')
			}
			check("ExtractTypeset", text.String())
			r, err := d.Render(ctx, Options{Width: 400})
			if err != nil {
				t.Fatal(err)
			}
			var markers []Box
			for _, b := range r.Boxes {
				if b.Tag == "marker" {
					markers = append(markers, b)
				}
			}
			sort.SliceStable(markers, func(i, j int) bool { return markers[i].Bounds.Min.Y < markers[j].Bounds.Min.Y })
			text.Reset()
			for _, b := range markers {
				text.WriteString(b.Text + " ")
			}
			check("raster", text.String())
			if tc.name == "four levels" && len(markers) == 4 {
				a, b := markers[0].Bounds, markers[1].Bounds
				differs := false
				for y := 0; y < min(a.Dy(), b.Dy()); y++ {
					for x := 0; x < min(a.Dx(), b.Dx()); x++ {
						if r.Image.RGBAAt(a.Min.X+x, a.Min.Y+y) != r.Image.RGBAAt(b.Min.X+x, b.Min.Y+y) {
							differs = true
						}
					}
				}
				if !differs {
					t.Error("disc and circle marker pixels are identical")
				}
			}
		})
	}
}
