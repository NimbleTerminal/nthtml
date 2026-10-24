package conformance

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	stdhtml "html"
	"io"
	"path"
	"regexp"
	"strings"

	"nimbleterminal.dev/nthtml/document"
)

var entityName = regexp.MustCompile(`&([A-Za-z][A-Za-z0-9]*);`)

// fixtureInput adapts XML-parsed XHTML into equivalent input for the HTML
// renderer. Original fixture bytes and hashes stay untouched. DTDs are never
// fetched and custom entity declarations are rejected.
func fixtureInput(ctx context.Context, name string, data []byte) ([]byte, error) {
	if path.Ext(name) != ".xht" && path.Ext(name) != ".xhtml" {
		return data, nil
	}
	if len(data) > document.DefaultLimits.Bytes {
		return nil, document.ErrLimit
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Entity = map[string]string{}
	for _, match := range entityName.FindAllSubmatch(data, -1) {
		key := string(match[1])
		value := stdhtml.UnescapeString(string(match[0]))
		if value != string(match[0]) {
			decoder.Entity[key] = value
		}
		if len(decoder.Entity) > 512 {
			return nil, document.ErrLimit
		}
	}
	var out bytes.Buffer
	var stack []string
	roots, tokens := 0, 0
	void := func(name string) bool {
		return strings.Contains(" area base br col embed hr img input link meta param source track wbr ", " "+name+" ")
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XHTML decode: %w", err)
		}
		tokens++
		if tokens > 40000 {
			return nil, document.ErrLimit
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) >= document.DefaultLimits.Depth-1 {
				return nil, document.ErrLimit
			}
			if len(stack) == 0 {
				roots++
				if roots > 1 {
					return nil, fmt.Errorf("XHTML must have one root")
				}
			}
			if t.Name.Space != "" && t.Name.Space != "http://www.w3.org/1999/xhtml" {
				return nil, fmt.Errorf("unreviewed XHTML namespace %q", t.Name.Space)
			}
			if t.Name.Local != strings.ToLower(t.Name.Local) {
				return nil, fmt.Errorf("case-sensitive XHTML element outside subset")
			}
			if len(stack) > 0 && (stack[len(stack)-1] == "style" || stack[len(stack)-1] == "script") {
				return nil, fmt.Errorf("markup inside XHTML raw-text element")
			}
			out.WriteString("<" + t.Name.Local)
			for _, attr := range t.Attr {
				if attr.Name.Local == "xmlns" || attr.Name.Space == "xmlns" {
					continue
				}
				key := attr.Name.Local
				if key != strings.ToLower(key) {
					return nil, fmt.Errorf("case-sensitive XHTML attribute outside subset")
				}
				if attr.Name.Space == "http://www.w3.org/XML/1998/namespace" {
					key = "xml:" + key
				} else if attr.Name.Space != "" {
					return nil, fmt.Errorf("unreviewed XHTML attribute namespace")
				}
				out.WriteString(" " + key + `="` + stdhtml.EscapeString(attr.Value) + `"`)
			}
			out.WriteByte('>')
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			name := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !void(name) {
				out.WriteString("</" + name + ">")
			}
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(t)) != "" {
					return nil, fmt.Errorf("XHTML text outside root")
				}
				continue
			}
			parent := stack[len(stack)-1]
			if parent == "style" || parent == "script" {
				if strings.Contains(strings.ToLower(string(t)), "</"+parent) {
					return nil, fmt.Errorf("XHTML raw-text closing sequence outside subset")
				}
				out.Write(t)
			} else {
				out.WriteString(stdhtml.EscapeString(string(t)))
			}
		case xml.ProcInst:
			if t.Target != "xml" {
				return nil, fmt.Errorf("unreviewed XHTML processing instruction")
			}
		case xml.Directive:
			if !strings.HasPrefix(strings.TrimSpace(string(t)), "DOCTYPE") || bytes.Contains(t, []byte("[")) {
				return nil, fmt.Errorf("XHTML custom DTD declarations are unsupported")
			}
		}
		if out.Len() > document.DefaultLimits.Bytes {
			return nil, document.ErrLimit
		}
	}
	if roots != 1 {
		return nil, fmt.Errorf("XHTML root is missing")
	}
	return out.Bytes(), nil
}
