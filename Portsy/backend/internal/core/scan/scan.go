package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type FileEntry struct {
	Rel  string
	Abs  string
	Size int64
	Mt   int64 // unix nano
}

func WalkProject(root string, ignores map[string]struct{}) ([]FileEntry, error) {
	var out []FileEntry
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if shouldIgnoreDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnoreFile(rel, ignores) {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		out = append(out, FileEntry{
			Rel: normalizeRel(rel), Abs: p, Size: info.Size(), Mt: info.ModTime().UnixNano(),
		})
		return nil
	})
	return out, err
}

func normalizeRel(rel string) string {
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "./")
	if runtime.GOOS == "windows" {
		rel = strings.ToLower(rel)
	}
	return rel
}

func shouldIgnoreDir(rel string) bool {
	rel = strings.ReplaceAll(rel, "\\", "/")
	parts := strings.Split(rel, "/")
	if len(parts) > 0 {
		switch parts[0] {
		case ".portsy", "Build", "Cache", ".git", ".idea", ".vs":
			return true
		}
	}
	return false
}

func shouldIgnoreFile(rel string, ignores map[string]struct{}) bool {
	rel = strings.ReplaceAll(rel, "\\", "/")
	if strings.HasSuffix(rel, ".DS_Store") {
		return true
	}
	if _, ok := ignores[rel]; ok {
		return true
	}
	return false
}
