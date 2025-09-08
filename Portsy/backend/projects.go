package backend

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type AbletonProject struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	AlsFile    string      `json:"alsFile"`
	HasPortsy  bool        `json:"hasPortsy"`
	LastCommit *CommitMeta `json:"lastCommit,omitempty"`
}

// ScanProjects is a convenience wrapper that scans without cancellation.
func ScanProjects(rootPath string) ([]AbletonProject, error) {
	return ScanProjectsCtx(context.Background(), rootPath)
}

// ScanProjectsCtx scans rootPath for immediate subfolders containing .als files.
// It prefers <FolderName>.als (case-insensitive). If absent, it picks the
// lexicographically smallest .als (case-insensitive) for determinism.
func ScanProjectsCtx(ctx context.Context, rootPath string) ([]AbletonProject, error) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	// Sort project directories by name (case-insensitive) for stable traversal.
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	var projects []AbletonProject
	for _, entry := range entries {
		if ctx.Err() != nil {
			// Respect cancellation
			return projects, ctx.Err()
		}
		if !entry.IsDir() {
			continue
		}

		projectName := entry.Name()
		projectPath := filepath.Join(rootPath, projectName)

		files, err := os.ReadDir(projectPath)
		if err != nil {
			// unreadable folder — skip but keep scanning others
			continue
		}

		// Deterministic order for ALS selection
		sort.Slice(files, func(i, j int) bool {
			return strings.ToLower(files[i].Name()) < strings.ToLower(files[j].Name())
		})

		var alsPath string
		var candidates []string
		preferred := projectName + ".als"

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			// Using case-insensitive match on extension
			if !strings.EqualFold(filepath.Ext(f.Name()), ".als") {
				continue
			}
			fp := filepath.Join(projectPath, f.Name())
			candidates = append(candidates, fp)

			// Prefer <FolderName>.als (case-insensitive)
			if strings.EqualFold(f.Name(), preferred) {
				alsPath = fp
				break
			}
		}

		if alsPath == "" && len(candidates) > 0 {
			// Pick lexicographically smallest candidate (case-insensitive) for determinism
			alsPath = candidates[0]
		}
		if alsPath == "" {
			// No .als directly inside folder
			continue
		}

		// .portsy presence
		hasPortsy := false
		if fi, err := os.Stat(filepath.Join(projectPath, ".portsy")); err == nil && fi.IsDir() {
			hasPortsy = true
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			// Unknown FS error — do not fail the scan; continue gracefully
		}

		// Normalize paths to forward slashes; lowercase on Windows per policy
		norm := func(p string) string {
			p = filepath.ToSlash(p)
			if runtime.GOOS == "windows" {
				p = strings.ToLower(p)
			}
			return p
		}

		projects = append(projects, AbletonProject{
			Name:      projectName,
			Path:      norm(projectPath),
			AlsFile:   norm(alsPath),
			HasPortsy: hasPortsy,
		})
	}

	// Stable ordering in the final result (case-insensitive by name)
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	return projects, nil
}
