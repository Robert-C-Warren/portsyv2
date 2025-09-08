package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type FileEntry struct {
	Rel  string
	Abs  string
	Size int64
	Mt   int64 // unix nano
}

// WalkProject walks root and returns a stable, normalized list of files.
// - Skips .portsy, Build, Cache, VCS/IDE dirs by default.
// - Skips common junk (.DS_Store).
// - Skips symlinked dirs (prevents loops) and symlinked files by default.
// - Normalizes rel paths to forward slashes; lowercases on Windows (NTFS semantics).
// - Returns results sorted by Rel for deterministic behavior.
func WalkProject(root string, ignores map[string]struct{}) ([]FileEntry, error) {
	var out []FileEntry

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Surface which path caused trouble; helpful in UI toasts.
			return fmt.Errorf("scan: %s: %w", p, walkErr)
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("scan: rel of %q: %w", p, err)
		}

		// Normalize early so ignore checks are consistent.
		rel = normalizeRel(rel)

		// Skip root itself (rel == ".") bookkeeping.
		if rel == "." {
			if d.IsDir() {
				return nil
			}
		}

		// Skip symlinked directories to avoid cycles.
		if d.IsDir() {
			// Quick dir ignore (first path segment).
			if shouldIgnoreDir(rel) {
				return filepath.SkipDir
			}
			// If this entry is a symlink to a dir, skip the subtree.
			if isSymlink(d) {
				return filepath.SkipDir
			}
			return nil
		}

		// Ignore files: junk, explicit ignores, and symlinked files.
		if shouldIgnoreFile(rel, ignores) || isSymlink(d) {
			return nil
		}

		info, e := d.Info()
		if e != nil {
			return fmt.Errorf("scan: info %q: %w", p, e)
		}

		out = append(out, FileEntry{
			Rel:  rel,
			Abs:  p,
			Size: info.Size(),
			Mt:   info.ModTime().UnixNano(),
		})
		return nil
	})

	// Ensure deterministic ordering for downstream diffs/UI.
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })

	return out, err
}

func isSymlink(d os.DirEntry) bool {
	return d.Type()&os.ModeSymlink != 0
}

func normalizeRel(rel string) string {
	// Normalize separators first.
	rel = strings.ReplaceAll(rel, "\\", "/")
	// filepath.Rel may return "." for root.
	if rel == "." {
		return rel
	}
	// Trim accidental leading "./"
	rel = strings.TrimPrefix(rel, "./")
	// NTFS is case-insensitive; lower-casing avoids duplicate logical paths.
	if runtime.GOOS == "windows" {
		rel = strings.ToLower(rel)
	}
	return rel
}

func shouldIgnoreDir(rel string) bool {
	// We only need the first path segment to decide “project-level” ignores.
	rel = strings.ReplaceAll(rel, "\\", "/")
	first := rel
	if i := strings.IndexByte(rel, '/'); i >= 0 {
		first = rel[:i]
	}
	switch first {
	case ".portsy", "Build", "Cache", ".git", ".idea", ".vs", ".svn", ".hg":
		return true
	}
	// Ableton-specific caches (add more as needed)
	if first == "Ableton Project Info" {
		return true
	}
	return false
}

func shouldIgnoreFile(rel string, ignores map[string]struct{}) bool {
	rel = strings.ReplaceAll(rel, "\\", "/")

	// Junk / platform artifacts
	if strings.HasSuffix(rel, ".DS_Store") || strings.HasSuffix(rel, "Thumbs.db") || strings.HasSuffix(rel, "desktop.ini") {
		return true
	}

	// Exact-path ignores provided by caller (already normalized).
	if ignores != nil {
		if _, ok := ignores[rel]; ok {
			return true
		}
	}

	return false
}
