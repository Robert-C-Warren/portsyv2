package als

import (
	"compress/gzip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func IsALS(p string) bool {
	return strings.EqualFold(filepath.Ext(p), ".als")
}

type Meta struct {
	DetectedSamples []string // project-relative if we can resolve them later
	RawXML          []byte   // optional, for debug or future diffs
}

// Read parses a gzipped .als and extracts sample references.
func Read(path string) (*Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f) // Ableton uses gzip, not zlib
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	xmlBytes, err := io.ReadAll(gr)
	if err != nil {
		return nil, err
	}

	refs := extractSampleRefs(xmlBytes)
	return &Meta{DetectedSamples: refs, RawXML: xmlBytes}, nil
}

// Ableton XML is huge; we only stream for tags that matter.
// We’re defensive: different Live versions vary in tag names.
type fileRef struct {
	RelativePath string `xml:"RelativePath,attr"`
	Name         string `xml:"Name,attr"`
}

// Minimal streaming extractor: looks for common patterns:
//
//	FileRef -> RelativePath (attr) or nested elements containing pathy text.
func extractSampleRefs(b []byte) []string {
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	paths := make(map[string]struct{})
	var stack []string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return keys(paths)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)

			// Common: <FileRef> … attributes or nested with path info
			if t.Name.Local == "FileRef" || t.Name.Local == "SampleRef" {
				var fr fileRef
				// Try attributes first
				for _, a := range t.Attr {
					if a.Name.Local == "RelativePath" && a.Value != "" {
						paths[normalizeRel(a.Value)] = struct{}{}
					}
				}
				// Also try to decode nested (non-fatal)
				_ = dec.DecodeElement(&fr, &t)
				if fr.RelativePath != "" {
					paths[normalizeRel(fr.RelativePath)] = struct{}{}
				} else if fr.Name != "" && looksPathy(fr.Name) {
					paths[normalizeRel(fr.Name)] = struct{}{}
				}
				// we consumed end element via DecodeElement
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return keys(paths)
}

func normalizeRel(p string) string {
	// Ableton may embed backslashes; normalize to forward slashes and trim junk.
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	return p
}

func looksPathy(s string) bool {
	return strings.Contains(s, "/") || strings.Contains(s, "\\") || strings.Contains(strings.ToLower(s), ".wav")
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
