package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	corehash "Portsy/backend/internal/core/hash"
)

// HashFileSHA256 returns (hashHex, sizeBytes, mtimeUnixSec) using SHA-256 ONLY.
func HashFileSHA256(path string) (string, int64, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, 0, err
	}
	if info.IsDir() || (info.Mode()&os.ModeSymlink != 0) {
		return "", 0, 0, os.ErrInvalid
	}

	// force SHA256 (don’t use FileHash, which follows DefaultAlg)
	sum, err := corehash.New(corehash.SHA256).File(path)
	if err != nil {
		return "", 0, 0, err
	}
	return sum, info.Size(), info.ModTime().Unix(), nil
}

// BuildManifest walks projectPath and returns a ProjectState of all tracked files.
// - Skips .portsy internals, common build/cache & VCS/IDE dirs.
// - Skips platform junk files (.DS_Store, Thumbs.db, desktop.ini).
// - Normalizes paths to forward slashes; lowercases on Windows (NTFS semantics).
// - Sorts entries by Path for deterministic output.
func BuildManifest(projectPath string) (ProjectState, error) {
	projectPath = filepath.Clean(projectPath)

	var files []FileEntry

	err := filepath.WalkDir(projectPath, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Silently skip unreadable entries to match previous behavior.
			return nil
		}

		name := d.Name()
		if d.IsDir() {
			// Skip known internal & noisy dirs at the top level of each subtree.
			switch name {
			case ".portsy", "Build", "Cache", ".git", ".idea", ".vs", ".svn", ".hg", "Ableton Project Info":
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinked files (avoid cross-tree surprises)
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		// Skip platform junk
		if name == ".DS_Store" || name == "Thumbs.db" || name == "desktop.ini" {
			return nil
		}

		rel, err := filepath.Rel(projectPath, p)
		if err != nil {
			return nil
		}

		// Normalize relative path
		rel = filepath.ToSlash(rel)
		if runtime.GOOS == "windows" {
			rel = strings.ToLower(rel)
		}

		hash, size, mod, err := HashFileSHA256(p)
		if err != nil {
			// Skip files we couldn't hash (permissions, transient IO, etc.)
			return nil
		}

		files = append(files, FileEntry{
			Path:     rel,
			Hash:     hash,
			Size:     size,
			Modified: mod,
		})
		return nil
	})
	if err != nil {
		return ProjectState{}, err
	}

	// Deterministic ordering helps diffs & tests.
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	return ProjectState{
		Files:     files,
		CreatedAt: time.Now().Unix(),
	}, nil
}
