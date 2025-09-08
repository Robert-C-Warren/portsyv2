package backend

import (
	"path/filepath"
	"sort"
)

type ProjectChange struct {
	Name     string
	Path     string
	Added    int
	Modified int
	Deleted  int
	Total    int
}

// ChangedProjectsSinceCache scans the root, builds current manifest,
// diffs against .portsy/cache.json, and returns a stable, sorted list
// of projects that have at least one change.
func ChangedProjectsSinceCache(root string) ([]ProjectChange, error) {
	projs, err := ScanProjects(root)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectChange, 0, len(projs))

	for _, p := range projs {
		pp := filepath.Join(root, p.Name)

		ps, err := BuildManifest(pp)
		if err != nil {
			continue
		}

		cur := ManifestFromState(ps)

		lc, _ := LoadLocalCache(pp)
		changes := DiffManifests(cur, lc.Manifest)
		if len(changes) == 0 {
			continue
		}

		pc := ProjectChange{Name: p.Name, Path: pp}
		for _, c := range changes {
			switch c.Type {
			case "added":
				pc.Added++
			case "modified":
				pc.Modified++
			case "deleted":
				pc.Deleted++
			}
		}
		pc.Total = pc.Added + pc.Modified + pc.Deleted
		out = append(out, pc)
	}

	// Deterministic ordering helps the UI and tests (prevents list jitter)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})

	return out, nil
}
