package backend

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// ObjectGetter is the tiny bit of R2 we need (DownloadTo).
// Your R2 client already satisfies this method signature.
type ObjectGetter interface {
	DownloadTo(ctx context.Context, key string, w io.Writer) error
}

// DiffPath is a lightweight JSON view for the GUI; avoids colliding with core FileEntry.
type DiffPath struct {
	Path string `json:"path"`
}

type DiffJSON struct {
	Added   []DiffPath      `json:"added"`
	Changed []DiffPath      `json:"changed"`
	Removed []DiffPath      `json:"removed"`
	Logical *ALSLogicalDiff `json:"logical,omitempty"`
}

// BuildDiffJSON produces UI-ready diff output, including ALS logical info if possible.
// - projectName: used to build the R2 key for prev ALS
// - projectPath: local path to the project folder
// - current: current manifest (path -> sha) computed from disk
// - cached: last-synced manifest (path -> sha) from .portsy/cache.json
// - blobs: R2 client (may be nil; ALS enrichment will be skipped)
func BuildDiffJSON(
	ctx context.Context,
	projectName, projectPath string,
	current, cached map[string]string,
	blobs ObjectGetter,
) ([]byte, error) {

	changes := DiffManifests(current, cached)

	out := DiffJSON{}
	changedPaths := make([]string, 0, len(changes))

	for _, c := range changes {
		switch c.Type {
		case "added":
			out.Added = append(out.Added, DiffPath{Path: c.Path})
		case "modified":
			out.Changed = append(out.Changed, DiffPath{Path: c.Path})
			changedPaths = append(changedPaths, c.Path)
		case "deleted":
			out.Removed = append(out.Removed, DiffPath{Path: c.Path})
		}
	}

	// Try ALS logical enrichment (non-fatal).
	if logical, err := enrichALS(ctx, projectName, projectPath, current, cached, blobs, changedPaths); err == nil && logical != nil {
		out.Logical = logical
	}

	sort.Slice(out.Added, func(i, j int) bool { return out.Added[i].Path < out.Added[j].Path })
	sort.Slice(out.Changed, func(i, j int) bool { return out.Changed[i].Path < out.Changed[j].Path })
	sort.Slice(out.Removed, func(i, j int) bool { return out.Removed[i].Path < out.Removed[j].Path })

	return json.Marshal(out)
}

func enrichALS(
	ctx context.Context,
	projectName, projectPath string,
	current, cached map[string]string,
	blobs ObjectGetter,
	changedPaths []string,
) (*ALSLogicalDiff, error) {

	alsRel := topLevelALS(current)
	if alsRel == "" {
		return nil, nil
	}

	// Only if ALS changed or there's no previous ALS in cache.
	alsChanged := cached[alsRel] == ""
	if !alsChanged {
		for _, p := range changedPaths {
			if filepath.Clean(p) == filepath.Clean(alsRel) {
				alsChanged = true
				break
			}
		}
	}
	if !alsChanged {
		return nil, nil
	}

	// If no blob getter, skip enrichment quietly.
	if blobs == nil {
		return nil, nil
	}

	// prev ALS (ungzipped XML) from R2 using cached manifest hash (if any)
	var prevXML []byte
	if prevSHA := cached[alsRel]; prevSHA != "" {
		key := BuildR2Key(projectName, alsRel, prevSHA)
		var buf bytes.Buffer
		if err := blobs.DownloadTo(ctx, key, &buf); err == nil {
			// buf contains gzipped ALS
			if gr, err := gzip.NewReader(bytes.NewReader(buf.Bytes())); err == nil {
				defer gr.Close()
				prevXML, _ = io.ReadAll(gr) // ok if it fails; we just skip prevXML
			}
		}
	}

	// prev sample hash lookup from cached manifest
	prevHash := func(rel string) string {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if h, ok := cached[rel]; ok {
			return h
		}
		return ""
	}

	// current ALS path on disk (gz)
	currALSPath := filepath.Join(projectPath, alsRel)

	return ComputeALSLogicalDiff(prevXML, currALSPath, projectPath, prevHash)
}

// topLevelALS picks the main .als: a .als directly under the project root (not in subfolders or Backup/).
func topLevelALS(manifest map[string]string) string {
	candidate := ""
	for p := range manifest {
		if !strings.EqualFold(filepath.Ext(p), ".als") {
			continue
		}
		dir := filepath.Dir(p)
		if dir != "." && dir != "" {
			continue // ignore subfolders like Backup/
		}
		// prefer the lexicographically first if multiple
		if candidate == "" || p < candidate {
			candidate = p
		}
	}
	return filepath.ToSlash(candidate)
}
