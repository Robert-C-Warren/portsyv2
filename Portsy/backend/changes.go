package backend

import "path/filepath"

type ProjectChange struct {
	Name     string
	Path     string
	Added    int
	Modified int
	Deleted  int
	Total    int
}

// ChangedProjectsSinceCache scans the root, builds current manifest, diffs against .portsy/cache.json
func ChangedProjectsSinceCache(root string) ([]ProjectChange, error) {
	projs, err := ScanProjects(root)
	if err != nil {
		return nil, err
	}
	var out []ProjectChange
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
		pc := ProjectChange{Name: p.Name, Path: pp, Total: len(changes)}
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
		out = append(out, pc)
	}
	return out, nil
}
