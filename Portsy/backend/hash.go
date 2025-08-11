package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"
)

func HashFileSHA256(path string) (string, int64, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, 0, err
	}
	fi, _ := f.Stat()
	sum := hex.EncodeToString(h.Sum(nil))
	mod := fi.ModTime().Unix()
	return sum, n, mod, nil
}

// Build a manifest of all files to track for a project (no zips; excludes .portsy)
func BuildManifest(projectPath string) (ProjectState, error) {
	var files []FileEntry
	err := filepath.Walk(projectPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			// skip unreadable entries
			return nil
		}
		if info.IsDir() {
			// ignore .portsy internals
			if info.Name() == ".portsy" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(projectPath, p)
		hash, size, mod, err := HashFileSHA256(p)
		if err != nil {
			return nil
		}
		files = append(files, FileEntry{
			Path:     filepath.ToSlash(rel),
			Hash:     hash,
			Size:     size,
			Modified: mod,
		})
		return nil
	})
	if err != nil {
		return ProjectState{}, err
	}
	return ProjectState{
		Files:     files,
		CreatedAt: time.Now().Unix(),
	}, nil
}
