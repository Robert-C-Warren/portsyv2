package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// LocalCache lives at .portsy/cache.json inside a project.
type LocalCache struct {
	Version   int               `json:"version"`   // schema version for migrations
	Algo      string            `json:"algo"`      // e.g. "sha256" | "blake3"
	UpdatedAt time.Time         `json:"updatedAt"` // RFC3339 via time.Time marshal
	Manifest  map[string]string `json:"manifest"`  // path -> content hash (per Algo)
}

// Current schema version for LocalCache.
const localCacheVersion = 1

func cacheFile(projectPath string) string {
	return filepath.Join(projectPath, ".portsy", "cache.json")
}

func cacheTmpFile(projectPath string) string {
	return filepath.Join(projectPath, ".portsy", "cache.json.tmp")
}

// LoadLocalCache reads cache.json, returning an empty cache if it does not exist.
// Distinguishes ENOENT fom other IO errors and preserves corrupt files for debugging.
func LoadLocalCache(projectPath string) (*LocalCache, error) {
	p := cacheFile(projectPath)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &LocalCache{
				Version:  localCacheVersion,
				Algo:     "sha256", // default; caller may override before Save
				Manifest: map[string]string{},
			}, nil
		}
		// Real IO error (permission, transient FS issue) -> surface it.
		return nil, fmt.Errorf("read local cache: %w", err)
	}

	var lc LocalCache
	if err := json.Unmarshal(b, &lc); err != nil {
		// Preserve the corrupt file for post-mortem
		_ = preserveCorruptCache(p, b)
		return &LocalCache{
			Version:  localCacheVersion,
			Algo:     "sha256",
			Manifest: map[string]string{},
		}, nil
	}

	// Fill defaults / migrations
	if lc.Manifest == nil {
		lc.Manifest = map[string]string{}
	}
	if lc.Version == 0 {
		lc.Version = localCacheVersion
	}
	if lc.Algo == "" {
		lc.Algo = "sha256"
	}

	// Normalize keys on load
	lc.Manifest = normalizeManifestKeys(lc.Manifest)

	return &lc, nil
}

// SaveLocalCache writes the cache atomically: write -> fsync -> rename.
// This prevents truncated JSON if the process crashes mid-write.
func SaveLocalCache(projectPath string, lc *LocalCache) error {
	p := cacheFile(projectPath)
	tmp := cacheTmpFile(projectPath)

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("ensure .portsy dir: %w", err)
	}

	lc.Version = localCacheVersion
	// ensure UTC for consistency
	lc.UpdatedAt = time.Now().UTC()

	b, err := json.MarshalIndent(lc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal local cache: %w", err)
	}

	// atomic write
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open tmp cache for write: %w", err)
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil {
		return fmt.Errorf("write tmp cache: %w", werr)
	}
	if cerr != nil {
		return fmt.Errorf("close tmp cache: %w", cerr)
	}

	// On POSIX, fsync directory after rename is the belt; file is enough for most cases.
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("atomic rename cache: %w", err)
	}

	// Best effort: fsync directory to persist the rename across caches.
	dir, derr := os.Open(filepath.Dir(p))
	if derr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

// ManifestFromState converts a ProjectState to a simple path->hash map.
func ManifestFromState(ps ProjectState) map[string]string {
	m := make(map[string]string, len(ps.Files))
	for _, f := range ps.Files {
		// BuildManifest already excludes .portsy
		m[normalizeKey(f.Path)] = f.Hash
	}
	return m
}

type FileChange struct {
	Path string
	Type string // "added" | "modified" | "deleted"
}

func DiffManifests(current, cached map[string]string) (changes []FileChange) {
	seen := make(map[string]struct{}, len(current))

	for p, h := range current {
		cp := normalizeKey(p)
		if ch, ok := cached[cp]; !ok {
			changes = append(changes, FileChange{Path: cp, Type: "added"})
		} else if ch != h {
			changes = append(changes, FileChange{Path: cp, Type: "modified"})
		}
		seen[cp] = struct{}{}
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
// The caller should set lc.Algo to the active hashers name if not sha256
func WriteCacheFromState(projectPath string, ps ProjectState, algo string) error {
	if algo == "" {
		algo = "sha256"
	}
	lc := &LocalCache{
		Version:  localCacheVersion,
		Algo:     algo,
		Manifest: ManifestFromState(ps),
	}
	return SaveLocalCache(projectPath, lc)
}

// ---------- HELPERS -----------
func preserveCorruptCache(path string, data []byte) error {
	bad := filepath.Join(filepath.Dir(path), fmt.Sprintf("cache.bad-%s.json",
		time.Now().UTC().Format("20060102T150405Z")))
	// best effort only
	if err := os.WriteFile(bad, data, 0o644); err != nil {
		return err
	}
	return nil
}

func normalizeManifestKeys(in map[string]string) map[string]string {
	if len(in) == 0 {
		return in
	}
	if runtime.GOOS != "windows" {
		// On non-windows, callers already normalized to forward slashes
		return in
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[normalizeKey(k)] = v
	}
	return out
}

func normalizeKey(p string) string {
	// Ensure forward slashes, and lowercase on Windows to match scanner policy
	np := filepath.ToSlash(p)
	if runtime.GOOS == "windows" {
		np = lowerASCII(np)
	}
	return np
}

func lowerASCII(s string) string {
	// fast-path: ASCII lowercasing; avoids locale weirdness
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
