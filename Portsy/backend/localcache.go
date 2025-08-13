package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// LocalCache lives at .portsy/cache.json inside a project.
type LocalCache struct {
	UpdatedAt time.Time         `json:"updatedAt"`
	Manifest  map[string]string `json:"manifest"` // path -> sha256
}

func cacheFile(projectPath string) string {
	return filepath.Join(projectPath, ".portsy", "cache.json")
}

func LoadLocalCache(projectPath string) (*LocalCache, error) {
	b, err := os.ReadFile(cacheFile(projectPath))
	if err != nil {
		// no cache yet
		return &LocalCache{Manifest: map[string]string{}}, nil
	}
	var lc LocalCache
	if err := json.Unmarshal(b, &lc); err != nil {
		// corrupt cache → start fresh
		return &LocalCache{Manifest: map[string]string{}}, nil
	}
	if lc.Manifest == nil {
		lc.Manifest = map[string]string{}
	}
	return &lc, nil
}

func SaveLocalCache(projectPath string, lc *LocalCache) error {
	p := cacheFile(projectPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	lc.UpdatedAt = time.Now()
	b, err := json.MarshalIndent(lc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// ManifestFromState converts a ProjectState to a simple path->hash map.
func ManifestFromState(ps ProjectState) map[string]string {
	m := make(map[string]string, len(ps.Files))
	for _, f := range ps.Files {
		// BuildManifest already excludes .portsy
		m[f.Path] = f.Hash
	}
	return m
}

type FileChange struct {
	Path string
	Type string // "added" | "modified" | "deleted"
}

func DiffManifests(current, cached map[string]string) (changes []FileChange) {
	seen := map[string]struct{}{}

	for p, h := range current {
		if ch, ok := cached[p]; !ok {
			changes = append(changes, FileChange{Path: p, Type: "added"})
		} else if ch != h {
			changes = append(changes, FileChange{Path: p, Type: "modified"})
		}
		seen[p] = struct{}{}
	}
	for p := range cached {
		if _, ok := seen[p]; !ok {
			changes = append(changes, FileChange{Path: p, Type: "deleted"})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return
}

// WriteCacheFromState writes the given state as the latest local cache.
func WriteCacheFromState(projectPath string, ps ProjectState) error {
	lc := &LocalCache{Manifest: ManifestFromState(ps)}
	return SaveLocalCache(projectPath, lc)
}
